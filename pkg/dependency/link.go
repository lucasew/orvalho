package dependency

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
)

const virtualStoreDir = ".orvalho"

func slotDir(name, version string) string {
	return filepath.Join("node_modules", virtualStoreDir, name+"@"+version, "node_modules", filepath.FromSlash(name))
}

func slotNodeModules(name, version string) string {
	return filepath.Join("node_modules", virtualStoreDir, name+"@"+version, "node_modules")
}

// linkTree writes the isolated tree and .bin symlinks under root.
func linkTree(root string, g *Graph) error {
	nm := filepath.Join(root, "node_modules")
	if err := os.MkdirAll(nm, 0o755); err != nil {
		return err
	}
	byPath := g.Packages
	slots := map[string]Node{} // name@version -> node
	for _, n := range g.Nodes {
		if n.Optional && !keepOptional(n.CPU) {
			continue
		}
		if n.Integrity == "" && n.Resolved == "" {
			continue
		}
		slots[n.Name+"@"+n.Version] = n
	}

	for _, n := range slots {
		dest := filepath.Join(root, slotDir(n.Name, n.Version))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		sib := filepath.Join(root, slotNodeModules(n.Name, n.Version))
		for depName := range n.Dependencies {
			target := resolveVisible(byPath, n.LockPath, depName)
			if target == nil {
				continue
			}
			if err := symlinkRel(
				filepath.Join(sib, filepath.FromSlash(depName)),
				filepath.Join(root, slotDir(target.Name, target.Version)),
			); err != nil {
				return err
			}
		}
		for peerName := range n.PeerDependencies {
			target := peerInGraph(g, peerName)
			if target == nil {
				continue
			}
			if err := symlinkRel(
				filepath.Join(sib, filepath.FromSlash(peerName)),
				filepath.Join(root, slotDir(target.Name, target.Version)),
			); err != nil {
				return err
			}
		}
	}

	for name := range g.Root {
		n := nodeAt(byPath, "node_modules/"+name)
		if n == nil {
			continue
		}
		if n.Optional && !keepOptional(n.CPU) {
			continue
		}
		if err := symlinkRel(
			filepath.Join(nm, filepath.FromSlash(name)),
			filepath.Join(root, slotDir(n.Name, n.Version)),
		); err != nil {
			return err
		}
	}

	return linkBins(root, g)
}

func nodeAt(packages map[string]lockPackage, lockPath string) *Node {
	ent, ok := packages[lockPath]
	if !ok || ent.Link {
		return nil
	}
	name := ent.Name
	if name == "" {
		name = packageNameFromPath(lockPath)
	}
	n := Node{
		Name:             name,
		Version:          ent.Version,
		Resolved:         ent.Resolved,
		Integrity:        ent.Integrity,
		Optional:         ent.Optional || ent.DevOptional,
		CPU:              ent.CPU,
		Dependencies:     ent.Dependencies,
		PeerDependencies: ent.PeerDependencies,
		Bin:              parseBin(ent.Bin),
		LockPath:         lockPath,
	}
	return &n
}

func resolveVisible(packages map[string]lockPackage, fromPath, name string) *Node {
	// Walk from fromPath/node_modules/name up to node_modules/name.
	cur := fromPath
	for {
		cand := path.Join(cur, "node_modules", name)
		if n := nodeAt(packages, cand); n != nil {
			return n
		}
		if cur == "" || cur == "." {
			break
		}
		parent := path.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return nodeAt(packages, "node_modules/"+name)
}

func peerInGraph(g *Graph, name string) *Node {
	for i := range g.Nodes {
		if g.Nodes[i].Name == name {
			n := g.Nodes[i]
			return &n
		}
	}
	return nil
}

func linkBins(root string, g *Graph) error {
	binDir := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	for name := range g.Root {
		n := nodeAt(g.Packages, "node_modules/"+name)
		if n == nil || len(n.Bin) == 0 {
			continue
		}
		if n.Optional && !keepOptional(n.CPU) {
			continue
		}
		pkgDir := filepath.Join(root, slotDir(n.Name, n.Version))
		for binName, rel := range n.Bin {
			if err := symlinkRel(filepath.Join(binDir, binName), filepath.Join(pkgDir, filepath.FromSlash(rel))); err != nil {
				return err
			}
		}
	}
	return nil
}

func symlinkRel(link, target string) error {
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}
	rel, err := filepath.Rel(filepath.Dir(link), target)
	if err != nil {
		return err
	}
	if rerr := os.Remove(link); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
		return rerr
	}
	if err := os.Symlink(rel, link); err != nil {
		return fmt.Errorf("symlink %s -> %s: %w", link, rel, err)
	}
	return nil
}

// ScopedNodeDir creates a directory with a `node` symlink to orvalho
// so shebangs resolve to this CLI (TEC-18).
func ScopedNodeDir(orvalho string) (dir string, err error) {
	dir, err = os.MkdirTemp("", "orvalho-node-*")
	if err != nil {
		return "", err
	}
	if err := os.Symlink(orvalho, filepath.Join(dir, "node")); err != nil {
		if rerr := os.RemoveAll(dir); rerr != nil {
			return "", errors.Join(err, rerr)
		}
		return "", err
	}
	return dir, nil
}
