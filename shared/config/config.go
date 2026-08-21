// Package config loads a plugin's own user configuration. Herdr creates the directory but never writes to it.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/grongor/herdr-plugins/shared/plugin"
)

// FileName is the config file name inside the plugin's config directory; Herdr does not prescribe one.
const FileName = "config.toml"

// Load decodes the plugin's config file into dst and reports the path it read. A missing file or a missing config
// directory (running outside Herdr) yields an empty path and no error.
func Load(dst any) (string, error) {
	dir := plugin.ConfigDir()
	if dir == "" {
		return "", nil
	}

	path := filepath.Join(dir, FileName)
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", nil
	case err != nil:
		return "", err
	}

	if err := toml.Unmarshal(data, dst); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}

	return path, nil
}

// Hint names the file an error message should point the user at; loaded is the path Load reported, possibly empty.
func Hint(loaded string) string {
	if loaded != "" {
		return loaded
	}
	if dir := plugin.ConfigDir(); dir != "" {
		return filepath.Join(dir, FileName)
	}

	return FileName
}
