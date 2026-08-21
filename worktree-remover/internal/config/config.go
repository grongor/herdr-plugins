package config

import (
	pluginconfig "github.com/grongor/herdr-plugins/shared/config"
)

// Config is the user configuration; every field is optional, defaults are documented in config.example.toml.
type Config struct {
	// DeleteBranch is the checkbox's initial state. A pointer because the default is true, so false and unset differ.
	DeleteBranch *bool `toml:"delete_branch"`
}

// Deleting the branch too is the reason this plugin exists rather than Herdr's own dialog.
const defaultDeleteBranch = true

// Load reads the plugin's config.toml; a missing file is not an error.
func Load() (*Config, error) {
	cfg := &Config{}
	if _, err := pluginconfig.Load(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) DeleteBranchDefault() bool {
	if c.DeleteBranch == nil {
		return defaultDeleteBranch
	}

	return *c.DeleteBranch
}
