// Package version holds the product version string injected at link time.
package version

import (
	"runtime/debug"
	"strings"
)

// Set via goreleaser ldflags: -X orvalho/pkg/version.version={{ .Version }}
var version = "dev"

// Version returns the orvalho version (e.g. v0.1.0), or "dev" when unset.
func Version() string {
	v := strings.TrimSpace(version)
	if v == "" {
		return "dev"
	}
	return v
}

// BuildID returns a short VCS revision when available.
func BuildID() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			if len(setting.Value) > 8 {
				return setting.Value[:8]
			}
			return setting.Value
		}
	}
	return "dev"
}
