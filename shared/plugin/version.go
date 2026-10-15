package plugin

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

// Version reads the version out of the plugin's own herdr-plugin.toml, embedded into the binary, so the manifest is the
// only place it is written down. A manifest without one is a build mistake, not a runtime condition: it panics.
func Version(manifest []byte) string {
	var parsed struct {
		Version string `toml:"version"`
	}
	if err := toml.Unmarshal(manifest, &parsed); err != nil {
		panic(fmt.Sprintf("decoding the embedded herdr-plugin.toml: %v", err))
	}
	if parsed.Version == "" {
		panic("the embedded herdr-plugin.toml has no version")
	}

	return parsed.Version
}
