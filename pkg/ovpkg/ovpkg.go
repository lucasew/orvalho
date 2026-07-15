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

// Entry returns the package entry path from the validated CUE manifest.
func (p *Package) Entry() (string, error) {
	if p == nil || p.Config == nil {
		return "", fmt.Errorf("ovpkg: package not loaded")
	}
	s, ok, err := cuex.LookupString(p.Value(), "entry")
	if err != nil {
		return "", err
	}
	if !ok || s == "" {
		return "", fmt.Errorf("ovpkg: missing entry")
	}
	return s, nil
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
