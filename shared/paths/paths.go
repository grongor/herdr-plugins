// Package paths handles paths read from hand-written configuration files.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ExpandHome resolves a leading ~ against the user's home directory. On error the path is returned unexpanded so a
// caller that can tolerate a literal ~ may ignore the error.
func ExpandHome(path string) (string, error) {
	if !HasHomePrefix(path) {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return path, err
	}

	return filepath.Join(home, strings.TrimPrefix(path[1:], string(os.PathSeparator))), nil
}

// HasHomePrefix reports whether the path begins with a ~ component, as opposed to a name starting with ~.
func HasHomePrefix(path string) bool {
	return path == "~" ||
		strings.HasPrefix(path, "~"+string(os.PathSeparator)) ||
		(runtime.GOOS == "windows" && strings.HasPrefix(path, `~\`))
}
