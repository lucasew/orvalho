// Package ovpkg reads and writes Orvalho zip packages.
//
// An Orvalho package is a zip archive whose deploy unit (see SPEC.md) contains:
//
//   - orvalho.cue at the archive root (CUE package instance)
//   - payload files: JS worker graph, static assets, and other files the
//     manifest references
//
// Manifest validation uses pkg/cuex (embedded package prelude). This package
// does not sign or verify packages.
package ovpkg

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"cuelang.org/go/cue"

	"orvalho/pkg/cuex"
)

// ManifestName is the fixed path of the package CUE instance inside the archive.
const ManifestName = cuex.InstanceFilename // orvalho.cue

// Common errors returned by this package.
var (
	// ErrMissingManifest means the archive has no orvalho.cue at the root.
	ErrMissingManifest = errors.New("ovpkg: missing orvalho.cue")
	// ErrInvalidPath means a file path is not a safe archive-relative path.
	ErrInvalidPath = errors.New("ovpkg: invalid path")
	// ErrNotFound means a requested payload path is not in the package.
	ErrNotFound = errors.New("ovpkg: file not found")
	// ErrDuplicatePath means the same path appears more than once when writing.
	ErrDuplicatePath = errors.New("ovpkg: duplicate path")
)

// Package is an in-memory Orvalho zip package: CUE manifest plus payload files.
//
// Files maps archive paths (slash-separated, no leading slash) to contents.
// The map does not include ManifestName; use Manifest for raw CUE bytes.
// Config is the validated unified CUE value (package prelude ⊔ instance).
type Package struct {
	// Manifest is the raw contents of orvalho.cue.
	Manifest []byte
	// Files is the payload tree (everything except orvalho.cue).
	Files map[string][]byte
	// Config is set after successful CUE validation (Open/PackageFrom*).
	Config *cuex.Config
}

// Value returns the validated package CUE value, or an empty value if unset.
func (p *Package) Value() cue.Value {
	if p == nil || p.Config == nil {
		return cue.Value{}
	}
	return p.Config.Value
}

// Get returns the contents of path from the payload, or ErrNotFound.
func (p *Package) Get(name string) ([]byte, error) {
	if p == nil {
		return nil, ErrNotFound
	}
	clean, err := cleanArchivePath(name)
	if err != nil {
		return nil, err
	}
	data, ok := p.Files[clean]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, clean)
	}
	out := make([]byte, len(data))
	copy(out, data)
	return out, nil
}

// List returns sorted archive paths in the package, including ManifestName
// when a manifest is present.
func (p *Package) List() []string {
	if p == nil {
		return nil
	}
	n := len(p.Files)
	if len(p.Manifest) > 0 {
		n++
	}
	names := make([]string, 0, n)
	if len(p.Manifest) > 0 {
		names = append(names, ManifestName)
	}
	for name := range p.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Open reads an Orvalho package from a zip in r with the given size.
func Open(r io.ReaderAt, size int64) (*Package, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("ovpkg: open zip: %w", err)
	}
	return openZipReader(zr)
}

// OpenBytes opens a package from an in-memory zip archive.
func OpenBytes(data []byte) (*Package, error) {
	return Open(bytes.NewReader(data), int64(len(data)))
}

// OpenFile opens a package from a zip file on disk.
func OpenFile(path string) (*Package, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return Open(f, st.Size())
}

func openZipReader(zr *zip.Reader) (*Package, error) {
	pkg := &Package{Files: make(map[string][]byte)}
	for _, zf := range zr.File {
		if strings.HasSuffix(zf.Name, "/") || zf.FileInfo().IsDir() {
			continue
		}
		name, err := cleanArchivePath(zf.Name)
		if err != nil {
			return nil, fmt.Errorf("%w: %q", err, zf.Name)
		}
		rc, err := zf.Open()
		if err != nil {
			return nil, fmt.Errorf("ovpkg: open %s: %w", name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("ovpkg: read %s: %w", name, err)
		}
		if name == ManifestName {
			if pkg.Manifest != nil {
				return nil, fmt.Errorf("%w: %s", ErrDuplicatePath, ManifestName)
			}
			pkg.Manifest = data
			continue
		}
		if _, exists := pkg.Files[name]; exists {
			return nil, fmt.Errorf("%w: %s", ErrDuplicatePath, name)
		}
		pkg.Files[name] = data
	}
	if len(pkg.Manifest) == 0 {
		return nil, ErrMissingManifest
	}
	cfg, err := cuex.LoadPackage(pkg.Manifest, ManifestName)
	if err != nil {
		return nil, fmt.Errorf("ovpkg: validate manifest: %w", err)
	}
	pkg.Config = cfg
	return pkg, nil
}

// WriteOptions controls how a package is written to a zip.
type WriteOptions struct {
	Store bool
}

// Write writes an Orvalho package to w as a zip archive.
// manifest must be orvalho.cue bytes that validate against the package prelude.
func Write(w io.Writer, manifest []byte, files map[string][]byte) error {
	return WriteWithOptions(w, manifest, files, WriteOptions{})
}

// WriteWithOptions is Write with optional compression control.
func WriteWithOptions(w io.Writer, manifest []byte, files map[string][]byte, opts WriteOptions) error {
	if len(manifest) == 0 {
		return ErrMissingManifest
	}
	if _, err := cuex.LoadPackage(manifest, ManifestName); err != nil {
		return fmt.Errorf("ovpkg: validate manifest: %w", err)
	}

	zw := zip.NewWriter(w)
	method := zip.Deflate
	if opts.Store {
		method = zip.Store
	}
	if err := writeZipFile(zw, ManifestName, manifest, method); err != nil {
		_ = zw.Close()
		return err
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	seen := map[string]struct{}{ManifestName: {}}
	for _, name := range names {
		clean, err := cleanArchivePath(name)
		if err != nil {
			_ = zw.Close()
			return fmt.Errorf("%w: %q", err, name)
		}
		if clean == ManifestName {
			_ = zw.Close()
			return fmt.Errorf("%w: %s must be passed as manifest, not files", ErrDuplicatePath, ManifestName)
		}
		if _, ok := seen[clean]; ok {
			_ = zw.Close()
			return fmt.Errorf("%w: %s", ErrDuplicatePath, clean)
		}
		seen[clean] = struct{}{}
		if err := writeZipFile(zw, clean, files[name], method); err != nil {
			_ = zw.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("ovpkg: close zip: %w", err)
	}
	return nil
}

// WritePackage writes p to w.
func WritePackage(w io.Writer, p *Package) error {
	if p == nil {
		return ErrMissingManifest
	}
	return Write(w, p.Manifest, p.Files)
}

// WriteFile writes an Orvalho package zip to path on disk.
func WriteFile(path string, manifest []byte, files map[string][]byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = Write(f, manifest, files)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

// WriteDir builds a package from a directory tree and writes it to w.
func WriteDir(w io.Writer, dir string) error {
	manifest, files, err := ReadDir(dir)
	if err != nil {
		return err
	}
	return Write(w, manifest, files)
}

// ReadDir loads manifest + file map from a package directory (not a zip).
func ReadDir(dir string) (manifest []byte, files map[string][]byte, err error) {
	st, err := os.Stat(dir)
	if err != nil {
		return nil, nil, err
	}
	if !st.IsDir() {
		return nil, nil, fmt.Errorf("ovpkg: %s is not a directory", dir)
	}

	files = make(map[string][]byte)
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		clean, err := cleanArchivePath(rel)
		if err != nil {
			return fmt.Errorf("%w: %q", err, rel)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if clean == ManifestName {
			if manifest != nil {
				return fmt.Errorf("%w: %s", ErrDuplicatePath, ManifestName)
			}
			manifest = data
			return nil
		}
		if _, ok := files[clean]; ok {
			return fmt.Errorf("%w: %s", ErrDuplicatePath, clean)
		}
		files[clean] = data
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if len(manifest) == 0 {
		return nil, nil, ErrMissingManifest
	}
	if _, err := cuex.LoadPackage(manifest, ManifestName); err != nil {
		return nil, nil, fmt.Errorf("ovpkg: validate manifest: %w", err)
	}
	return manifest, files, nil
}

// PackageFromDir loads a directory into an in-memory Package.
func PackageFromDir(dir string) (*Package, error) {
	manifest, files, err := ReadDir(dir)
	if err != nil {
		return nil, err
	}
	return PackageFromMap(manifest, files)
}

// PackageFromMap builds a Package from raw manifest bytes and a file map.
func PackageFromMap(manifest []byte, files map[string][]byte) (*Package, error) {
	if len(manifest) == 0 {
		return nil, ErrMissingManifest
	}
	cfg, err := cuex.LoadPackage(manifest, ManifestName)
	if err != nil {
		return nil, fmt.Errorf("ovpkg: validate manifest: %w", err)
	}
	out := make(map[string][]byte, len(files))
	for name, data := range files {
		clean, err := cleanArchivePath(name)
		if err != nil {
			return nil, fmt.Errorf("%w: %q", err, name)
		}
		if clean == ManifestName {
			return nil, fmt.Errorf("%w: %s must be passed as manifest, not files", ErrDuplicatePath, ManifestName)
		}
		if _, ok := out[clean]; ok {
			return nil, fmt.Errorf("%w: %s", ErrDuplicatePath, clean)
		}
		cp := make([]byte, len(data))
		copy(cp, data)
		out[clean] = cp
	}
	m := make([]byte, len(manifest))
	copy(m, manifest)
	return &Package{Manifest: m, Files: out, Config: cfg}, nil
}

func writeZipFile(zw *zip.Writer, name string, data []byte, method uint16) error {
	h := &zip.FileHeader{Name: name, Method: method}
	w, err := zw.CreateHeader(h)
	if err != nil {
		return fmt.Errorf("ovpkg: create %s: %w", name, err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("ovpkg: write %s: %w", name, err)
	}
	return nil
}

func cleanArchivePath(name string) (string, error) {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimPrefix(name, "./")
	if name == "" || name == "." {
		return "", ErrInvalidPath
	}
	if name[0] == '/' || strings.HasPrefix(name, "../") || name == ".." {
		return "", ErrInvalidPath
	}
	if len(name) >= 2 && name[1] == ':' {
		return "", ErrInvalidPath
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", ErrInvalidPath
	}
	for _, seg := range strings.Split(clean, "/") {
		if seg == ".." || seg == "" {
			return "", ErrInvalidPath
		}
	}
	return clean, nil
}

// OpenPath opens a package from a zip file or a directory tree containing orvalho.cue.
func OpenPath(path string) (*Package, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		return PackageFromDir(path)
	}
	return OpenFile(path)
}

// Agent is one worker declared under agents.<name> after CUE evaluation.
type Agent struct {
	// Name is the key under agents.
	Name string
	// Entrypoint is the package-relative JS module path.
	Entrypoint string
	// Env is the concrete string bag projected onto guest env (CF vars/secrets).
	Env map[string]string
	// Bindings maps guest env names to raw CUE binding objects (type + fields).
	// Host drivers materialize these; values are not decoded here beyond type.
	Bindings map[string]BindingSpec
}

// BindingSpec is a typed host binding request from package CUE.
type BindingSpec struct {
	Type string
	// Fields is the full binding value as cue.Value (includes type).
	Value cue.Value
}

// SingleAgent returns the sole agent in the package.
// Serve requires exactly one agent; zero or more than one is an error
// (multi-agent supervisor is backlog).
func (p *Package) SingleAgent() (*Agent, error) {
	if p == nil || p.Config == nil {
		return nil, fmt.Errorf("ovpkg: package not loaded")
	}
	agents := p.Value().LookupPath(cue.ParsePath("agents"))
	if !agents.Exists() {
		return nil, fmt.Errorf("ovpkg: missing agents")
	}
	iter, err := agents.Fields()
	if err != nil {
		return nil, fmt.Errorf("ovpkg: agents: %w", err)
	}
	var names []string
	var values []cue.Value
	for iter.Next() {
		names = append(names, iter.Selector().Unquoted())
		values = append(values, iter.Value())
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("ovpkg: agents is empty (need exactly one)")
	}
	if len(names) > 1 {
		sort.Strings(names)
		return nil, fmt.Errorf("ovpkg: multiple agents %v (serve requires exactly one; supervisor is backlog)", names)
	}
	return decodeAgent(names[0], values[0])
}

func decodeAgent(name string, v cue.Value) (*Agent, error) {
	ep, err := v.LookupPath(cue.ParsePath("entrypoint")).String()
	if err != nil {
		return nil, fmt.Errorf("ovpkg: agents.%s.entrypoint: %w", name, err)
	}
	envMap, _, err := cuex.LookupStringMap(v, "env")
	if err != nil {
		return nil, fmt.Errorf("ovpkg: agents.%s.env: %w", name, err)
	}
	if envMap == nil {
		envMap = map[string]string{}
	}
	bindings := map[string]BindingSpec{}
	bv := v.LookupPath(cue.ParsePath("bindings"))
	if bv.Exists() {
		iter, err := bv.Fields()
		if err != nil {
			return nil, fmt.Errorf("ovpkg: agents.%s.bindings: %w", name, err)
		}
		for iter.Next() {
			bname := iter.Selector().Unquoted()
			bv := iter.Value()
			typ, err := bv.LookupPath(cue.ParsePath("type")).String()
			if err != nil {
				return nil, fmt.Errorf("ovpkg: agents.%s.bindings.%s.type: %w", name, bname, err)
			}
			bindings[bname] = BindingSpec{Type: typ, Value: bv}
		}
	}
	return &Agent{
		Name:       name,
		Entrypoint: ep,
		Env:        envMap,
		Bindings:   bindings,
	}, nil
}

// Entry returns the sole agent's entrypoint (see [Package.SingleAgent]).
// Deprecated path name kept for call-site churn reduction; prefer SingleAgent.
func (p *Package) Entry() (string, error) {
	a, err := p.SingleAgent()
	if err != nil {
		return "", err
	}
	return a.Entrypoint, nil
}

// WithRuntimeEnv re-validates the package manifest with outside-world env
// unified as runtime.env. Returns a new Package sharing the same payload files.
func (p *Package) WithRuntimeEnv(env map[string]string) (*Package, error) {
	if p == nil || len(p.Manifest) == 0 {
		return nil, ErrMissingManifest
	}
	cfg, err := cuex.LoadPackageWithEnv(p.Manifest, ManifestName, env)
	if err != nil {
		return nil, fmt.Errorf("ovpkg: validate with runtime.env: %w", err)
	}
	return &Package{
		Manifest: p.Manifest,
		Files:    p.Files,
		Config:   cfg,
	}, nil
}

// Port returns the optional package port, or 0 if unset.
func (p *Package) Port() (int, error) {
	if p == nil || p.Config == nil {
		return 0, fmt.Errorf("ovpkg: package not loaded")
	}
	// Prefer top-level port, then publish.port.
	for _, path := range []string{"port", "publish.port"} {
		fv := p.Value().LookupPath(cue.ParsePath(path))
		if !fv.Exists() {
			continue
		}
		n, err := fv.Int64()
		if err != nil {
			return 0, err
		}
		return int(n), nil
	}
	return 0, nil
}

// Egress returns the package outbound allowlist from the validated CUE
// manifest. Missing egress yields a nil slice (deny-all for host fetch).
func (p *Package) Egress() ([]string, error) {
	if p == nil || p.Config == nil {
		return nil, fmt.Errorf("ovpkg: package not loaded")
	}
	v := p.Value().LookupPath(cue.ParsePath("egress"))
	if !v.Exists() {
		return nil, nil
	}
	iter, err := v.List()
	if err != nil {
		return nil, fmt.Errorf("ovpkg: egress: %w", err)
	}
	var out []string
	for iter.Next() {
		s, err := iter.Value().String()
		if err != nil {
			return nil, fmt.Errorf("ovpkg: egress entry: %w", err)
		}
		out = append(out, s)
	}
	return out, nil
}
