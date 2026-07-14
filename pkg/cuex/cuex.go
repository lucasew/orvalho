// Package cuex loads Orvalho CUE configuration (contapila/workspaced style).
//
// Embedded preludes are unified with user instances. There is no cue.mod /
// module system — only Compile + Unify + Validate. The live model is cue.Value;
// optional Decode into a struct is allowed only after validation.
package cuex

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueerrors "cuelang.org/go/cue/errors"
)

// InstanceFilename is the fixed name for host and package CUE instances.
const InstanceFilename = "orvalho.cue"

//go:embed prelude_common.cue prelude_host.cue prelude_package.cue
var preludeFS embed.FS

// Config holds a validated unified CUE value.
type Config struct {
	Value cue.Value
}

// LoadPackage unifies package preludes with instance CUE bytes (zip root orvalho.cue).
func LoadPackage(instance []byte, filename string) (*Config, error) {
	if filename == "" {
		filename = InstanceFilename
	}
	return load(instance, filename, "prelude_package.cue")
}

// LoadHost unifies host preludes with instance CUE bytes.
// Empty instance is valid (prelude defaults only).
func LoadHost(instance []byte, filename string) (*Config, error) {
	if filename == "" {
		filename = InstanceFilename
	}
	if len(instance) == 0 {
		instance = []byte("{}\n")
	}
	return load(instance, filename, "prelude_host.cue")
}

// LoadHostFile reads path (or empty file semantics) and LoadHost.
func LoadHostFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return LoadHost(nil, path)
		}
		return nil, err
	}
	return LoadHost(data, path)
}

// LoadHostDataDir loads orvalho.cue from dataDir (must be provided by caller).
func LoadHostDataDir(dataDir string) (*Config, error) {
	if dataDir == "" {
		return nil, fmt.Errorf("cuex: data dir is required")
	}
	return LoadHostFile(filepath.Join(dataDir, InstanceFilename))
}

// UnifyOverlay compiles overlay CUE and unifies it onto cfg (e.g. flag overrides).
func (c *Config) UnifyOverlay(overlay string, filename string) (*Config, error) {
	if c == nil {
		return nil, fmt.Errorf("cuex: nil config")
	}
	if overlay == "" {
		return c, nil
	}
	if filename == "" {
		filename = "overlay.cue"
	}
	// Compile overlay in the same context as Value.
	layer := c.Value.Context().CompileString(overlay, cue.Filename(filename))
	if err := layer.Err(); err != nil {
		return nil, fmt.Errorf("cuex: compile overlay: %w", formatErr(err))
	}
	unified := c.Value.Unify(layer)
	if err := unified.Validate(cue.Concrete(true)); err != nil {
		// Concrete may be too strict for open optionals; use final.
		if err2 := unified.Validate(); err2 != nil {
			return nil, fmt.Errorf("cuex: unify overlay: %w", formatErr(err2))
		}
	}
	return &Config{Value: unified}, nil
}

func load(instance []byte, instanceName, rolePrelude string) (*Config, error) {
	ctx := cuecontext.New()

	commonBytes, err := preludeFS.ReadFile("prelude_common.cue")
	if err != nil {
		return nil, fmt.Errorf("cuex: read common prelude: %w", err)
	}
	roleBytes, err := preludeFS.ReadFile(rolePrelude)
	if err != nil {
		return nil, fmt.Errorf("cuex: read %s: %w", rolePrelude, err)
	}

	// Contapila-style: compile fragments and unify.
	// Definitions in common must be visible to role+user. We concatenate common
	// definitions into one compilation unit with role schema and user data.
	//
	// Build: common + role as schema layer, then unify user instance.
	schemaSrc := string(commonBytes) + "\n" + string(roleBytes)
	schema := ctx.CompileString(schemaSrc, cue.Filename(rolePrelude))
	if err := schema.Err(); err != nil {
		return nil, fmt.Errorf("cuex: compile prelude: %w", formatErr(err))
	}

	user := ctx.CompileBytes(instance, cue.Filename(instanceName))
	if err := user.Err(); err != nil {
		return nil, fmt.Errorf("cuex: compile %s: %w", instanceName, formatErr(err))
	}

	unified := schema.Unify(user)
	// Concrete(true) fails incomplete required fields (e.g. missing package entry).
	if err := unified.Validate(cue.Concrete(true)); err != nil {
		return nil, fmt.Errorf("cuex: validate: %w", formatErr(err))
	}
	return &Config{Value: unified}, nil
}

func formatErr(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s", cueerrors.Details(err, nil))
}

// LookupString returns a string field at path, or ("", false) if absent.
func LookupString(v cue.Value, path string) (string, bool, error) {
	fv := v.LookupPath(cue.ParsePath(path))
	if !fv.Exists() {
		return "", false, nil
	}
	s, err := fv.String()
	if err != nil {
		return "", false, err
	}
	return s, true, nil
}
