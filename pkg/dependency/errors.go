package dependency

import "errors"

var (
	// ErrNotFound means a Dependency name is not in the graph.
	ErrNotFound = errors.New("dependency: not found")
	// ErrLockfile means the project Lockfile is missing, invalid, or unrecognized.
	ErrLockfile = errors.New("dependency: lockfile")
	// ErrIntegrity means the tarball hash does not match.
	ErrIntegrity = errors.New("dependency: integrity")
	// ErrRegistry means the registry request failed.
	ErrRegistry = errors.New("dependency: registry")
	// ErrManifest means package.json is missing or invalid.
	ErrManifest = errors.New("dependency: manifest")
	// ErrSpecifier means the Dependency name is empty or invalid.
	ErrSpecifier = errors.New("dependency: specifier")
	// ErrVersion means a version string is not a dotted triple.
	ErrVersion = errors.New("dependency: version")
	// ErrTarPath means a tarball member escapes the extract root.
	ErrTarPath = errors.New("dependency: unsafe tar path")
)
