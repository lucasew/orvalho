package dependency

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// Options configure Install, Add, and Remove.
type Options struct {
	Dir      string
	StoreDir string
	Registry string
	HTTP     *http.Client
	Store    Store
}

func (o Options) project() string {
	if o.Dir != "" {
		return o.Dir
	}
	return "."
}

func (o Options) store() Store {
	s := o.Store
	if s.Dir == "" {
		if o.StoreDir != "" {
			s.Dir = o.StoreDir
		} else {
			s.Dir = filepath.Join(o.project(), DefaultStoreDir)
		}
	}
	return s
}

func (o Options) registry() registry {
	return registry{base: o.Registry, client: o.HTTP}
}

// Install resolves the declared tree and writes the store, Lockfile, and isolated tree.
func Install(ctx context.Context, opt Options) error {
	dir := opt.project()
	lockPath, err := DetectLockfile(dir)
	if err != nil {
		return err
	}
	man, err := readManifest(manifestPath(dir))
	if err != nil {
		return err
	}
	var g *Graph
	if lockPath != "" {
		g, err = ReadLockfile(lockPath)
		if err != nil {
			return err
		}
	} else {
		g, err = resolveGraph(opt.registry(), man)
		if err != nil {
			return err
		}
		if err := WriteLockfile(filepath.Join(dir, LockfileName), g); err != nil {
			return err
		}
	}
	return materialize(ctx, opt, g)
}

// Add puts name in dependencies, re-resolves, and materializes.
func Add(ctx context.Context, opt Options, name string) error {
	if name == "" {
		return ErrSpecifier
	}
	dir := opt.project()
	if _, err := DetectLockfile(dir); err != nil {
		return err
	}
	man, err := readManifest(manifestPath(dir))
	if err != nil {
		return err
	}
	rng := "latest"
	if n, r, ok := splitNameRange(name); ok {
		name, rng = n, r
	}
	pm, err := opt.registry().packument(name)
	if err != nil {
		return err
	}
	ver, ok := pickFromPackument(pm, rng)
	if !ok {
		return fmt.Errorf("%w: %s@%s", ErrNotFound, name, rng)
	}
	if man.Dependencies == nil {
		man.Dependencies = map[string]string{}
	}
	man.Dependencies[name] = "^" + ver
	delete(man.DevDependencies, name)
	if err := writeManifest(manifestPath(dir), man); err != nil {
		return err
	}
	g, err := resolveGraph(opt.registry(), man)
	if err != nil {
		return err
	}
	if err := WriteLockfile(filepath.Join(dir, LockfileName), g); err != nil {
		return err
	}
	return materialize(ctx, opt, g)
}

// Remove drops name from the manifest, re-resolves, and materializes.
func Remove(ctx context.Context, opt Options, name string) error {
	if name == "" {
		return ErrSpecifier
	}
	dir := opt.project()
	if _, err := DetectLockfile(dir); err != nil {
		return err
	}
	man, err := readManifest(manifestPath(dir))
	if err != nil {
		return err
	}
	_, inDep := man.Dependencies[name]
	_, inDev := man.DevDependencies[name]
	if !inDep && !inDev {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	delete(man.Dependencies, name)
	delete(man.DevDependencies, name)
	if err := writeManifest(manifestPath(dir), man); err != nil {
		return err
	}
	g, err := resolveGraph(opt.registry(), man)
	if err != nil {
		return err
	}
	if err := WriteLockfile(filepath.Join(dir, LockfileName), g); err != nil {
		return err
	}
	return materialize(ctx, opt, g)
}

func splitNameRange(spec string) (name, rng string, ok bool) {
	// name@version, keep scoped @scope/name@version
	if spec == "" {
		return "", "", false
	}
	if spec[0] == '@' {
		i := 1
		for i < len(spec) && spec[i] != '/' {
			i++
		}
		if i >= len(spec)-1 {
			return spec, "latest", true
		}
		rest := spec[i+1:]
		for j := 0; j < len(rest); j++ {
			if rest[j] == '@' {
				return spec[:i+1+j], rest[j+1:], true
			}
		}
		return spec, "latest", true
	}
	for i := 0; i < len(spec); i++ {
		if spec[i] == '@' {
			return spec[:i], spec[i+1:], true
		}
	}
	return spec, "latest", true
}

func materialize(ctx context.Context, opt Options, g *Graph) error {
	st := opt.store()
	if err := os.MkdirAll(st.Dir, 0o755); err != nil {
		return err
	}
	for _, n := range g.Nodes {
		if n.Optional && !keepOptional(n.CPU) {
			continue
		}
		if n.Integrity == "" || n.Resolved == "" {
			continue
		}
		algo, hash, err := ParseIntegrity(n.Integrity)
		if err != nil {
			return err
		}
		if err := st.Fetch(ctx, algo, hash, []string{n.Resolved}); err != nil {
			return err
		}
		dest := filepath.Join(opt.project(), slotDir(n.Name, n.Version))
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return err
		}
		f, err := st.Open(algo, hash)
		if err != nil {
			return err
		}
		err = unpackTarball(f, dest)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	return linkTree(opt.project(), g)
}
