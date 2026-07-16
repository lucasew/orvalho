// Package manager holds manager-side product logic.
//
// Skeleton only. Live code must not import orvalho/attic.
package manager

import "orvalho/pkg/version"

// Version is the product version (from pkg/version / goreleaser ldflags).
func Version() string { return version.Version() }

// Role is the SPEC role name for this package.
const Role = "manager"
