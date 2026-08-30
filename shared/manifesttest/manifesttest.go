// Package manifesttest checks a plugin's herdr-plugin.toml against the plugin it belongs to; a broken manifest
// otherwise fails at install time on a user's machine.
package manifesttest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// FileName is the manifest Herdr reads.
const FileName = "herdr-plugin.toml"

// Platforms is what every plugin here declares. Omitting the key links with a warning.
var Platforms = []string{"linux", "macos"}

// Manifest is the part of herdr-plugin.toml these checks read.
type Manifest struct {
	ID              string   `toml:"id"`
	Name            string   `toml:"name"`
	Version         string   `toml:"version"`
	MinHerdrVersion string   `toml:"min_herdr_version"`
	Description     string   `toml:"description"`
	Platforms       []string `toml:"platforms"`
	Build           []struct {
		Command []string `toml:"command"`
	} `toml:"build"`
	Actions []Entrypoint `toml:"actions"`
	Panes   []Entrypoint `toml:"panes"`
}

// Entrypoint is an [[actions]] or [[panes]] entry.
type Entrypoint struct {
	ID      string   `toml:"id"`
	Title   string   `toml:"title"`
	Command []string `toml:"command"`
}

// Dir is the directory of the calling test's source file.
func Dir(t *testing.T) string {
	t.Helper()

	_, caller, _, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("cannot locate the calling test's source file")
	}

	return filepath.Dir(caller)
}

// Check runs every manifest check against the plugin in dir. version is the plugin's own version constant.
func Check(t *testing.T, dir, version string) {
	t.Helper()

	parsed, meta := load(t, dir)

	t.Run("required fields", func(t *testing.T) { requiredFields(t, parsed, version) })
	t.Run("binary path", func(t *testing.T) { binaryPath(t, parsed) })
	t.Run("subcommands", func(t *testing.T) { subcommands(t, dir, parsed) })
	t.Run("ids", func(t *testing.T) { ids(t, parsed) })
	t.Run("no hooks", func(t *testing.T) { noHooks(t, meta) })
	t.Run("fallback build agrees", func(t *testing.T) { fallbackBuild(t, dir, parsed) })
}

func load(t *testing.T, dir string) (Manifest, toml.MetaData) {
	t.Helper()

	var parsed Manifest
	meta, err := toml.DecodeFile(filepath.Join(dir, FileName), &parsed)
	if err != nil {
		t.Fatalf("decoding %s: %v", FileName, err)
	}

	return parsed, meta
}

func requiredFields(t *testing.T, parsed Manifest, version string) {
	for _, field := range []struct{ name, value string }{
		{"id", parsed.ID},
		{"name", parsed.Name},
		{"version", parsed.Version},
		{"min_herdr_version", parsed.MinHerdrVersion},
		{"description", parsed.Description},
	} {
		if field.value == "" {
			t.Errorf("%s is empty", field.name)
		}
	}

	if !slices.Equal(parsed.Platforms, Platforms) {
		t.Errorf("platforms = %v, want %v", parsed.Platforms, Platforms)
	}

	if parsed.Version != version {
		t.Errorf("%s version = %q, main.go version = %q", FileName, parsed.Version, version)
	}
}

// binaryPath pins that every [[actions]] and [[panes]] command runs the binary [[build]] writes.
func binaryPath(t *testing.T, parsed Manifest) {
	if len(parsed.Build) != 1 {
		t.Fatalf("want exactly one [[build]], got %d", len(parsed.Build))
	}

	built := outputPath(t, parsed.Build[0].Command)
	for _, entry := range entrypoints(parsed) {
		if got := filepath.Clean(entry.Command[0]); got != built {
			t.Errorf("%q runs %q, but [[build]] writes %q", entry.ID, got, built)
		}
	}
}

// subcommands checks every entrypoint against the subcommands main() dispatches, read out of main.go.
func subcommands(t *testing.T, dir string, parsed Manifest) {
	dispatched := dispatchedCommands(t, dir)

	for _, entry := range entrypoints(parsed) {
		if len(entry.Command) < 2 {
			t.Errorf("%q runs the binary with no subcommand", entry.ID)

			continue
		}

		if !slices.Contains(dispatched, entry.Command[1]) {
			t.Errorf("%q runs subcommand %q, which main() does not dispatch (it handles %v)",
				entry.ID, entry.Command[1], dispatched)
		}
	}
}

// ids pins Herdr's rule that entrypoint ids carry no dots: actions are qualified as <plugin id>.<action id>.
func ids(t *testing.T, parsed Manifest) {
	for _, entry := range entrypoints(parsed) {
		if entry.ID == "" {
			t.Error("an entrypoint has no id")
		}
		if entry.Title == "" {
			t.Errorf("%q has no title", entry.ID)
		}
		if strings.Contains(entry.ID, ".") {
			t.Errorf("entrypoint id %q contains a dot", entry.ID)
		}
	}

	if !strings.Contains(parsed.ID, ".") {
		t.Errorf("plugin id %q is not of the form owner.plugin", parsed.ID)
	}
}

// noHooks pins that plugins here run only on user request: adding a hook must be a decision, not an unnoticed diff.
func noHooks(t *testing.T, meta toml.MetaData) {
	for _, section := range []string{"startup", "events", "link_handlers"} {
		if meta.IsDefined(section) {
			t.Errorf("[[%s]] was added — if that is intended, say so here and in the README", section)
		}
	}
}

// fallbackBuild pins that the no-Go fallback script writes the same binary as `go build -o`; a mismatch only shows up
// for users without Go. The script builds bin/<plugin directory> unless given a path argument.
func fallbackBuild(t *testing.T, dir string, parsed Manifest) {
	built := outputPath(t, parsed.Build[0].Command)

	joined := strings.Join(parsed.Build[0].Command, " ")
	match := regexp.MustCompile(`(\S*scripts/[\w.-]+\.sh)(?:[ \t]+([^\s|&;]+))?`).FindStringSubmatch(joined)
	if match == nil {
		t.Fatalf("the [[build]] command %q names no fallback script", joined)
	}
	script, argument := match[1], match[2]
	path := filepath.Join(dir, script)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the [[build]] command runs %s, which does not exist: %v", script, err)
	}
	// Herdr runs it as `sh <script>`; the mode matters for `make build-without-go` and direct invocation.
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("%s is not executable (mode %v)", script, info.Mode().Perm())
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	fallback := filepath.Join("bin", filepath.Base(abs))
	if argument != "" {
		fallback = filepath.Clean(argument)
	}
	if fallback != built {
		t.Errorf("the fallback script builds %q, but [[build]] writes %q", fallback, built)
	}
}

func entrypoints(parsed Manifest) []Entrypoint {
	return slices.Concat(parsed.Actions, parsed.Panes)
}

// outputPath is the `-o` of the [[build]] command. The command is a shell one-liner, so words are searched rather
// than argv elements.
func outputPath(t *testing.T, command []string) string {
	t.Helper()

	var words []string
	for _, arg := range command {
		words = append(words, strings.Fields(arg)...)
	}

	for i, word := range words {
		if word == "-o" && i+1 < len(words) {
			return filepath.Clean(words[i+1])
		}
	}

	t.Fatalf("the [[build]] command %v has no -o", command)

	return ""
}

// dispatchedCommands reads the string cases of main()'s `switch command`.
func dispatchedCommands(t *testing.T, dir string) []string {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, "main.go"), nil, 0)
	if err != nil {
		t.Fatalf("parsing main.go: %v", err)
	}

	var commands []string
	ast.Inspect(file, func(node ast.Node) bool {
		switchStmt, isSwitch := node.(*ast.SwitchStmt)
		if !isSwitch {
			return true
		}
		if tag, isIdent := switchStmt.Tag.(*ast.Ident); !isIdent || tag.Name != "command" {
			return true
		}

		for _, statement := range switchStmt.Body.List {
			for _, expression := range statement.(*ast.CaseClause).List {
				literal, isLiteral := expression.(*ast.BasicLit)
				if !isLiteral || literal.Kind != token.STRING {
					continue
				}
				if value, err := strconv.Unquote(literal.Value); err == nil {
					commands = append(commands, value)
				}
			}
		}

		return false
	})

	if len(commands) == 0 {
		t.Fatal("found no dispatch switch on `command` in main.go")
	}

	return commands
}
