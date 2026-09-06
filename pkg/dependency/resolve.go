package dependency

import (
	"fmt"
	"path"
)

func resolveGraph(reg registry, m *Manifest) (*Graph, error) {
	g := &Graph{
		Name:            m.Name,
		Version:         m.Version,
		Root:            map[string]string{},
		DevRoot:         map[string]string{},
		Packages:        map[string]lockPackage{},
		LockfileVersion: 3,
	}
	for n, r := range m.Dependencies {
		g.Root[n] = r
	}
	for n, r := range m.DevDependencies {
		g.DevRoot[n] = r
		g.Root[n] = r
	}

	var q []resolveJob
	for n, r := range m.Dependencies {
		q = append(q, resolveJob{n, r, "", false})
	}
	for n, r := range m.DevDependencies {
		q = append(q, resolveJob{n, r, "", true})
	}
	for n, r := range m.OptionalDependencies {
		q = append(q, resolveJob{n, r, "", false})
	}

	placed := map[string]string{} // name@version -> lock path
	seenJob := map[string]bool{}

	g.Packages[""] = lockPackage{
		Name:                 m.Name,
		Version:              m.Version,
		Dependencies:         copyMap(m.Dependencies),
		DevDependencies:      copyMap(m.DevDependencies),
		OptionalDependencies: copyMap(m.OptionalDependencies),
		PeerDependencies:     copyMap(m.PeerDependencies),
	}

	for len(q) > 0 {
		j := q[0]
		q = q[1:]
		key := j.parentPath + "|" + j.name + "|" + j.rng
		if seenJob[key] {
			continue
		}
		seenJob[key] = true

		pm, err := reg.packument(j.name)
		if err != nil {
			return nil, err
		}
		ver, ok := pickFromPackument(pm, j.rng)
		if !ok {
			return nil, fmt.Errorf("%w: %s@%s", ErrNotFound, j.name, j.rng)
		}
		pv := pm.Versions[ver]

		if optOf(j, m) && !keepOptional(pv.CPU) {
			continue
		}

		id := j.name + "@" + ver
		lockPath, already := placed[id]
		if !already {
			lockPath = npmLockPath(j.parentPath, j.name, g.Packages)
			placed[id] = lockPath
			ent := lockPackage{
				Name:                 j.name,
				Version:              ver,
				Resolved:             pv.Dist.Tarball,
				Integrity:            integrityOf(pv),
				Dev:                  j.dev,
				Optional:             optOf(j, m),
				CPU:                  pv.CPU,
				OS:                   pv.OS,
				Dependencies:         copyMap(pv.Dependencies),
				PeerDependencies:     copyMap(pv.PeerDependencies),
				OptionalDependencies: copyMap(pv.OptionalDependencies),
				Bin:                  pv.Bin,
			}
			g.Packages[lockPath] = ent
			g.Nodes = append(g.Nodes, Node{
				Name:                 j.name,
				Version:              ver,
				Resolved:             ent.Resolved,
				Integrity:            ent.Integrity,
				Dev:                  j.dev,
				Optional:             ent.Optional,
				CPU:                  pv.CPU,
				Dependencies:         copyMap(pv.Dependencies),
				PeerDependencies:     copyMap(pv.PeerDependencies),
				OptionalDependencies: copyMap(pv.OptionalDependencies),
				Bin:                  parseBin(pv.Bin),
				LockPath:             lockPath,
			})
			for dep, rng := range pv.Dependencies {
				q = append(q, resolveJob{dep, rng, lockPath, j.dev})
			}
			for dep, rng := range pv.OptionalDependencies {
				q = append(q, resolveJob{dep, rng, lockPath, j.dev})
			}
		}
		_ = already
	}
	return g, nil
}

type resolveJob struct {
	name, rng, parentPath string
	dev                   bool
}

func optOf(j resolveJob, m *Manifest) bool {
	if j.parentPath == "" {
		_, ok := m.OptionalDependencies[j.name]
		return ok
	}
	return false
}

func pickFromPackument(p *packument, rng string) (string, bool) {
	rng = trimRange(rng)
	if tag, ok := p.DistTags[rng]; ok {
		if _, exists := p.Versions[tag]; exists {
			return tag, true
		}
	}
	if rng == "latest" || rng == "" {
		if tag, ok := p.DistTags["latest"]; ok {
			return tag, true
		}
	}
	vers := make([]string, 0, len(p.Versions))
	for v := range p.Versions {
		vers = append(vers, v)
	}
	return pickLatest(vers, rng)
}

func trimRange(rng string) string {
	return rng
}

func integrityOf(pv packumentVersion) string {
	return pv.Dist.Integrity
}

func npmLockPath(parentPath, name string, existing map[string]lockPackage) string {
	want := path.Join("node_modules", name)
	if parentPath == "" {
		if _, ok := existing[want]; !ok {
			return want
		}
	}
	nested := path.Join(parentPath, "node_modules", name)
	if _, ok := existing[nested]; !ok {
		// Hoist when the top-level name is free or already this package.
		if parentPath != "" {
			if _, taken := existing[want]; !taken {
				return want
			}
		}
		return nested
	}
	return nested
}

func copyMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
