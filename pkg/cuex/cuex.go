// Package cuex loads Orvalho CUE configuration (contapila/workspaced style).
//
// Embedded preludes are unified with user instances. There is no cue.mod /
// module system — only Compile + Unify + Validate. The live model is cue.Value;
// optional Decode into a struct is allowed only after validation.
package cuex

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueerrors "cuelang.org/go/cue/errors"
	"errors"
	"io/fs"
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
// Packages that project agent.env from runtime.env must be loaded with
// [LoadPackageWithEnv] so outside values can complete the instance.
func LoadPackage(instance []byte, filename string) (*Config, error) {
	return LoadPackageWithEnv(instance, filename, nil)
}

// LoadPackageWithEnv unifies package preludes, optional runtime.env from env,
// then validates. env is the outside-world map (serve/manager); nil or empty
// means no overlay. On validate failure the actor must not be allocated
// (SPEC never-allocate).
func LoadPackageWithEnv(instance []byte, filename string, env map[string]string) (*Config, error) {
	if filename == "" {
		filename = InstanceFilename
	}
	cfg, err := loadRaw(instance, filename, "prelude_package.cue")
	if err != nil {
		return nil, err
	}
	if len(env) > 0 {
		overlay, err := encodeRuntimeEnvOverlay(env)
		if err != nil {
			return nil, err
		}
		cfg, err = cfg.unifyOverlayRaw(overlay, "runtime_env.cue")
		if err != nil {
			return nil, err
		}
	}
	if err := cfg.Value.Validate(cue.Concrete(true)); err != nil {
		return nil, fmt.Errorf("cuex: validate: %w", formatErr(err))
	}
	if err := requireAtLeastOneAgent(cfg.Value); err != nil {
		return nil, err
	}
	return cfg, nil
}

func requireAtLeastOneAgent(v cue.Value) error {
	agents := v.LookupPath(cue.ParsePath("agents"))
	if !agents.Exists() {
		return fmt.Errorf("cuex: validate: missing agents")
	}
	iter, err := agents.Fields()
	if err != nil {
		return fmt.Errorf("cuex: validate: agents: %w", err)
	}
	n := 0
	for iter.Next() {
		n++
	}
	if n == 0 {
		return fmt.Errorf("cuex: validate: agents must declare at least one agent")
	}
	return nil
}

// encodeRuntimeEnvOverlay builds `runtime: { env: { ... } }` CUE from a string map.
func encodeRuntimeEnvOverlay(env map[string]string) (string, error) {
	// Use JSON object encoding for safe string escaping, then wrap as CUE.
	// CUE accepts JSON as a subset for this shape.
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	m := make(map[string]string, len(env))
	for _, k := range keys {
		m[k] = env[k]
	}
	var buf bytes.Buffer
	buf.WriteString("runtime: { env: ")
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return "", fmt.Errorf("cuex: encode runtime.env: %w", err)
	}
	// Encode adds trailing newline; trim and close braces.
	b := bytes.TrimSpace(buf.Bytes())
	return string(b) + " }\n", nil
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
		if errors.Is(err, fs.ErrNotExist) {
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
	cfg, err := c.unifyOverlayRaw(overlay, filename)
	if err != nil {
		return nil, err
	}
	if err := cfg.Value.Validate(cue.Concrete(true)); err != nil {
		// Concrete may be too strict for open optionals; use final.
		if err2 := cfg.Value.Validate(); err2 != nil {
			return nil, fmt.Errorf("cuex: unify overlay: %w", formatErr(err2))
		}
	}
	return cfg, nil
}

func (c *Config) unifyOverlayRaw(overlay string, filename string) (*Config, error) {
	if c == nil {
		return nil, fmt.Errorf("cuex: nil config")
	}
	if overlay == "" {
		return c, nil
	}
	if filename == "" {
		filename = "overlay.cue"
	}
	layer := c.Value.Context().CompileString(overlay, cue.Filename(filename))
	if err := layer.Err(); err != nil {
		return nil, fmt.Errorf("cuex: compile overlay: %w", formatErr(err))
	}
	unified := c.Value.Unify(layer)
	if err := unified.Err(); err != nil {
		return nil, fmt.Errorf("cuex: unify overlay: %w", formatErr(err))
	}
	return &Config{Value: unified}, nil
}

func load(instance []byte, instanceName, rolePrelude string) (*Config, error) {
	cfg, err := loadRaw(instance, instanceName, rolePrelude)
	if err != nil {
		return nil, err
	}
	// Concrete(true) fails incomplete required fields (e.g. missing package fields).
	if err := cfg.Value.Validate(cue.Concrete(true)); err != nil {
		return nil, fmt.Errorf("cuex: validate: %w", formatErr(err))
	}
	return cfg, nil
}

func loadRaw(instance []byte, instanceName, rolePrelude string) (*Config, error) {
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
	if err := unified.Err(); err != nil {
		return nil, fmt.Errorf("cuex: unify: %w", formatErr(err))
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

// LookupStringMap returns a map[string]string at path, or (nil, false) if absent.
func LookupStringMap(v cue.Value, path string) (map[string]string, bool, error) {
	fv := v.LookupPath(cue.ParsePath(path))
	if !fv.Exists() {
		return nil, false, nil
	}
	iter, err := fv.Fields()
	if err != nil {
		return nil, false, err
	}
	out := make(map[string]string)
	for iter.Next() {
		name := iter.Selector().Unquoted()
		s, err := iter.Value().String()
		if err != nil {
			return nil, false, fmt.Errorf("cuex: %s.%s: %w", path, name, err)
		}
		out[name] = s
	}
	return out, true, nil
}
