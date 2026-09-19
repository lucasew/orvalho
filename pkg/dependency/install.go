package dependency

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/lewtec/lewkit/x/taskgroup"
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

func (o Options) store() (Store, error) {
	s := o.Store
	if s.Dir != "" {
		return s, nil
	}
	if o.StoreDir != "" {
		s.Dir = o.StoreDir
		return s, nil
	}
	dir, err := DefaultStoreDir()
	if err != nil {
		return Store{}, err
	}
	s.Dir = dir
	return s, nil
}

func (o Options) registry() registry {
	return registry{base: o.Registry, client: o.HTTP}
}

// Install resolves the declared tree and writes the store, Lockfile, and isolated tree.
func (o Options) Install(ctx context.Context) error {
	dir := o.project()
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
		g, err = resolveGraph(o.registry(), man)
		if err != nil {
			return err
		}
		if err := WriteLockfile(filepath.Join(dir, LockfileName), g); err != nil {
			return err
		}
	}
	return o.materialize(ctx, g)
}

// Add puts name in dependencies, re-resolves, and materializes.
func (o Options) Add(ctx context.Context, name string) error {
	if name == "" {
		return ErrSpecifier
	}
	dir := o.project()
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
	pm, err := o.registry().packument(name)
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
	g, err := resolveGraph(o.registry(), man)
	if err != nil {
		return err
	}
	if err := WriteLockfile(filepath.Join(dir, LockfileName), g); err != nil {
		return err
	}
	return o.materialize(ctx, g)
}

// Remove drops name from the manifest, re-resolves, and materializes.
func (o Options) Remove(ctx context.Context, name string) error {
	if name == "" {
		return ErrSpecifier
	}
	dir := o.project()
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
	g, err := resolveGraph(o.registry(), man)
	if err != nil {
		return err
	}
	if err := WriteLockfile(filepath.Join(dir, LockfileName), g); err != nil {
		return err
	}
	return o.materialize(ctx, g)
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

func (o Options) materialize(ctx context.Context, g *Graph) error {
	work := func(ctx context.Context) error {
		return o.materializeSession(ctx, g)
	}
	if taskgroup.FromContext(ctx) != nil {
		return work(ctx)
	}
	return taskgroup.WithSession(ctx, work)
}

type pkgJob struct {
	idx int
	n   Node
}

func (o Options) materializeSession(ctx context.Context, g *Graph) error {
	st, err := o.store()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(st.Dir, 0o755); err != nil {
		return err
	}
	var jobs []pkgJob
	for i, n := range g.Nodes {
		if n.Optional && !keepOptional(n.CPU) {
			continue
		}
		jobs = append(jobs, pkgJob{idx: i, n: n})
	}
	out, err := taskgroup.Map[pkgJob, Node]{
		Name:     "packages",
		Items:    jobs,
		PoolKind: taskgroup.Control,
		TaskName: func(_ int, j pkgJob) string {
			if j.n.Name != "" && j.n.Version != "" {
				return j.n.Name + "@" + j.n.Version
			}
			if j.n.Name != "" {
				return j.n.Name
			}
			return j.n.LockPath
		},
		Fn: func(ctx context.Context, _ *taskgroup.Status, j pkgJob) (Node, error) {
			n := j.n
			err := taskgroup.Isolate(ctx, func(ctx context.Context) error {
				fetch := taskgroup.Go(ctx, "fetch", taskgroup.Internet, func(ctx context.Context, s *taskgroup.Status) error {
					var err error
					n, err = o.fetchOne(ctx, s, st, n)
					return err
				})
				taskgroup.Go(ctx, "unpack", taskgroup.IO, func(_ context.Context, s *taskgroup.Status) error {
					return o.unpackOne(s, st, n)
				}, fetch)
				return nil
			})
			return n, err
		},
	}.Run(ctx)
	if err != nil {
		return err
	}
	for i, n := range out {
		g.Nodes[jobs[i].idx] = n
		if ent, ok := g.Packages[n.LockPath]; ok {
			ent.Resolved = n.Resolved
			ent.Integrity = n.Integrity
			g.Packages[n.LockPath] = ent
		}
	}
	return (linker{root: o.project(), g: g}).run()
}

func (o Options) fetchOne(ctx context.Context, s *taskgroup.Status, st Store, n Node) (Node, error) {
	if err := o.ensureDist(&n); err != nil {
		return n, err
	}
	algo, hash, err := ParseIntegrity(n.Integrity)
	if err != nil {
		return n, err
	}
	if st.Has(algo, hash) {
		s.Update("cached")
		return n, nil
	}
	s.Update("fetch")
	if err := st.Fetch(ctx, algo, hash, []string{n.Resolved}); err != nil {
		return n, err
	}
	return n, nil
}

func (o Options) unpackOne(s *taskgroup.Status, st Store, n Node) error {
	algo, hash, err := ParseIntegrity(n.Integrity)
	if err != nil {
		return err
	}
	s.Update("unpack")
	dest := filepath.Join(o.project(), slotDir(n.Name, n.Version))
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
	return err
}

// ensureDist fills Resolved and Integrity from the registry when the Lockfile
// entry has a version but no dist (npm packages keys sometimes omit them).
func (o Options) ensureDist(n *Node) error {
	if n.Resolved != "" && n.Integrity != "" {
		return nil
	}
	if n.Name == "" || n.Version == "" {
		return fmt.Errorf("%w: %s: missing resolved", ErrLockfile, n.LockPath)
	}
	pm, err := o.registry().packument(n.Name)
	if err != nil {
		return err
	}
	pv, ok := pm.Versions[n.Version]
	if !ok {
		return fmt.Errorf("%w: %s@%s", ErrNotFound, n.Name, n.Version)
	}
	if n.Resolved == "" {
		n.Resolved = pv.Dist.Tarball
	}
	if n.Integrity == "" {
		n.Integrity = pv.Dist.Integrity
	}
	if n.Resolved == "" || n.Integrity == "" {
		return fmt.Errorf("%w: %s@%s: missing dist", ErrRegistry, n.Name, n.Version)
	}
	return nil
}
