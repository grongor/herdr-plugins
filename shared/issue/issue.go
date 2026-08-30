// Package issue turns an issue tracker reference (key or Jira browse URL) into a branch name via user-replaceable
// fetch and propose commands.
package issue

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
)

// Both anchored: a loose match would fire on a resolved branch name pasted back in, since it contains the key. Either
// case is a reference: the key is respelt in KeyCase before anything sees it.
var (
	keyPattern = regexp.MustCompile(`(?i)^[A-Z][A-Z0-9]*-[0-9]+$`)
	urlPattern = regexp.MustCompile(`(?i)^https?://[^/]+/browse/([A-Z][A-Z0-9]*-[0-9]+)/?(\?.*)?$`)
)

// KeyCase is how the issue key is spelt in the commands and the branch name, however it was typed.
type KeyCase string

const (
	KeyLower KeyCase = "lower"
	KeyUpper KeyCase = "upper"
)

var KeyCases = []KeyCase{KeyLower, KeyUpper}

func (c KeyCase) Valid() bool { return slices.Contains(KeyCases, c) }

// Apply treats anything but KeyUpper as KeyLower, the default.
func (c KeyCase) Apply(key string) string {
	if c == KeyUpper {
		return strings.ToUpper(key)
	}

	return strings.ToLower(key)
}

// branchPattern only checks git-acceptable shape; the naming scheme (type, slug length) is the prompt's business, and
// the issue key is checked separately in accept.
var branchPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*(/[A-Za-z0-9][A-Za-z0-9._-]*)*$`)

// Placeholders replaced in the prompt template and command lines. Each is also passed as an environment variable,
// which a `sh -c` command line can use without quoting trouble.
const (
	PlaceholderKey          = "{key}"
	PlaceholderTitle        = "{title}"
	PlaceholderPrompt       = "{prompt}"
	PlaceholderSystemPrompt = "{system_prompt}"
)

const (
	EnvKey          = "ISSUE_KEY"
	EnvTitle        = "ISSUE_TITLE"
	EnvPrompt       = "BRANCH_PROMPT"
	EnvSystemPrompt = "BRANCH_SYSTEM_PROMPT"
)

var (
	// DefaultFetch prints "KEY: summary"; needs `jira` (github.com/ankitpokhrel/jira-cli) and `jq`. Only the title is
	// fetched: descriptions are screenfuls of reproduction logs the model does not need.
	DefaultFetch = []string{
		"sh", "-c", `jira issue view "$ISSUE_KEY" --raw | jq -r '"\(.key): \(.fields.summary)"'`,
	}

	// DefaultPropose uses `env` so MAX_THINKING_TOKENS stays inside an argv the config file can spell out.
	DefaultPropose = []string{
		"env", "MAX_THINKING_TOKENS=0",
		"claude", "--model", "haiku",
		"--safe-mode", "--tools", "", "--no-session-persistence",
		"--system-prompt", PlaceholderSystemPrompt,
		"-p", PlaceholderPrompt,
	}

	DefaultSystemPrompt = "You convert a Jira ticket title into a git branch name. " +
		"Reply with the branch name and nothing else."

	DefaultPrompt = "Ticket: " + PlaceholderTitle + "\n" +
		"Output a git branch name, format: <type>/<KEY>-<slug>. " +
		"type = one of feat, fix, chore, refactor, docs, test, perf, ci - inferred from the title. " +
		"KEY = " + PlaceholderKey + ", exactly as written here, in this case. " +
		"slug = kebab-case, 3-6 words from the title; do not repeat the type word in the slug. " +
		"Output the branch name only: no backticks, no quotes, no explanation."
)

// Resolver runs Fetch then Propose. Every empty field takes its Default*.
type Resolver struct {
	// Fetch prints the issue title on stdout. Runs with ISSUE_KEY set and "{key}" replaced in every argument.
	Fetch []string
	// Propose prints the branch name. Runs with ISSUE_KEY, ISSUE_TITLE, BRANCH_PROMPT and BRANCH_SYSTEM_PROMPT set,
	// and "{key}", "{title}", "{prompt}" and "{system_prompt}" replaced in every argument. Stdin is empty.
	Propose      []string
	SystemPrompt string
	// Prompt is the template handed to Propose, with "{title}" and "{key}" replaced.
	Prompt string
	// KeyCase spells the key in every command and in the proposal, which is respelt if the model ignored it.
	KeyCase KeyCase
}

// Reference reports the issue key text names, if any. Shape only: existence would cost a network round trip.
func Reference(text string) (string, bool) {
	text = strings.TrimSpace(text)

	if keyPattern.MatchString(text) {
		return text, true
	}
	if match := urlPattern.FindStringSubmatch(text); match != nil {
		return match[1], true
	}

	return "", false
}

// Resolve fetches the issue and returns the proposed branch name. Takes seconds; run it off the input loop.
func (r Resolver) Resolve(key string) (string, error) {
	key = r.KeyCase.Apply(key)

	title, err := r.fetch(key)
	if err != nil {
		return "", err
	}
	if title == "" {
		return "", fmt.Errorf("%s came back empty: is the key right?", key)
	}

	return r.propose(key, title)
}

func (r Resolver) fetch(key string) (string, error) {
	vars := map[string]string{PlaceholderKey: key}
	argv := expand(or(r.Fetch, DefaultFetch), vars)

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), EnvKey+"="+key)

	out, err := cmd.Output()
	if err != nil {
		return "", commandError(argv[0], err)
	}

	return strings.TrimSpace(string(out)), nil
}

func (r Resolver) propose(key, title string) (string, error) {
	system := orString(r.SystemPrompt, DefaultSystemPrompt)
	prompt := render(orString(r.Prompt, DefaultPrompt), key, title)

	argv := expand(or(r.Propose, DefaultPropose), map[string]string{
		PlaceholderKey:          key,
		PlaceholderTitle:        title,
		PlaceholderPrompt:       prompt,
		PlaceholderSystemPrompt: system,
	})

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(),
		EnvKey+"="+key,
		EnvTitle+"="+title,
		EnvPrompt+"="+prompt,
		EnvSystemPrompt+"="+system,
	)

	out, err := cmd.Output()
	if err != nil {
		return "", commandError(argv[0], err)
	}

	// The title carries the tracker's own spelling of the key, which a model tends to copy over the prompt's.
	spelling := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(key))

	// Last acceptable line: a model that ignores "nothing else" tends to explain first and answer last.
	for _, line := range slices.Backward(strings.Split(strings.TrimSpace(string(out)), "\n")) {
		if candidate := spelling.ReplaceAllLiteralString(strings.TrimSpace(line), key); accept(candidate, key) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("%s proposed no usable branch name", argv[0])
}

func render(template, key, title string) string {
	return strings.NewReplacer(PlaceholderKey, key, PlaceholderTitle, title).Replace(template)
}

// accept requires the key so that lines like "Done." are rejected and the branch stays traceable to its issue; a
// prompt that drops the key from the name must change this too.
func accept(candidate, key string) bool {
	return branchPattern.MatchString(candidate) &&
		!strings.Contains(candidate, "..") &&
		!strings.HasSuffix(candidate, ".lock") &&
		strings.Contains(candidate, key)
}

// commandError prefers the first stderr line: exec itself only reports a non-zero exit status.
func commandError(name string, err error) error {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if text := strings.TrimSpace(string(exit.Stderr)); text != "" {
			return fmt.Errorf("%s: %s", name, firstLine(text))
		}
	}

	return fmt.Errorf("%s: %w", name, err)
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")

	return line
}

// expand substitutes inside each argument, so a placeholder can sit in a `sh -c` script or beside a flag. Nothing is
// re-parsed by a shell unless the user's own command is one; the environment variables are the quoting-proof route.
func expand(argv []string, vars map[string]string) []string {
	pairs := make([]string, 0, 2*len(vars))
	for placeholder, value := range vars {
		pairs = append(pairs, placeholder, value)
	}
	replacer := strings.NewReplacer(pairs...)

	expanded := make([]string, len(argv))
	for i, arg := range argv {
		expanded[i] = replacer.Replace(arg)
	}

	return expanded
}

func or(configured, fallback []string) []string {
	if len(configured) > 0 {
		return configured
	}

	return fallback
}

func orString(configured, fallback string) string {
	if configured != "" {
		return configured
	}

	return fallback
}
