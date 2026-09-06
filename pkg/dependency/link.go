package dependency

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
)

const virtualStoreDir = ".orvalho"

type linker struct {
	root string
	g    *Graph
}

func slotDir(name, version string) string {
	return filepath.Join("node_modules", virtualStoreDir, name+"@"+version, "node_modules", filepath.FromSlash(name))
}

func slotNodeModules(name, version string) string {
	return filepath.Join("node_modules", virtualStoreDir, name+"@"+version, "node_modules")
}

func (l linker) run() error {
	nm := filepath.Join(l.root, "node_modules")
	if err := os.MkdirAll(nm, 0o755); err != nil {
		return err
	}
	slots := map[string]Node{}
	for _, n := range l.g.Nodes {
		if n.Optional && !keepOptional(n.CPU) {
			continue
		}
		if n.Integrity == "" && n.Resolved == "" {
			continue
		}
		slots[n.Name+"@"+n.Version] = n
	}

	for _, n := range slots {
		dest := filepath.Join(l.root, slotDir(n.Name, n.Version))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		sib := filepath.Join(l.root, slotNodeModules(n.Name, n.Version))
		for depName := range n.Dependencies {
			target := l.visible(n.LockPath, depName)
			if target == nil {
				continue
			}
			if err := symlinkRel(
				filepath.Join(sib, filepath.FromSlash(depName)),
				filepath.Join(l.root, slotDir(target.Name, target.Version)),
			); err != nil {
				return err
			}
		}
		for peerName := range n.PeerDependencies {
			target := l.peer(peerName)
			if target == nil {
				continue
			}
			if err := symlinkRel(
				filepath.Join(sib, filepath.FromSlash(peerName)),
				filepath.Join(l.root, slotDir(target.Name, target.Version)),
			); err != nil {
				return err
			}
		}
	}

	for name := range l.g.Root {
		n := l.nodeAt("node_modules/" + name)
		if n == nil {
			continue
		}
		if n.Optional && !keepOptional(n.CPU) {
			continue
		}
		if err := symlinkRel(
			filepath.Join(nm, filepath.FromSlash(name)),
			filepath.Join(l.root, slotDir(n.Name, n.Version)),
		); err != nil {
			return err
		}
	}

	return l.bins()
}

func (l linker) nodeAt(lockPath string) *Node {
	ent, ok := l.g.Packages[lockPath]
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

func (l linker) visible(fromPath, name string) *Node {
	cur := fromPath
	for {
		cand := path.Join(cur, "node_modules", name)
		if n := l.nodeAt(cand); n != nil {
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
	return l.nodeAt("node_modules/" + name)
}

func (l linker) peer(name string) *Node {
	for i := range l.g.Nodes {
		if l.g.Nodes[i].Name == name {
			n := l.g.Nodes[i]
			return &n
		}
	}
	return nil
}

func (l linker) bins() error {
	binDir := filepath.Join(l.root, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	for name := range l.g.Root {
		n := l.nodeAt("node_modules/" + name)
		if n == nil || len(n.Bin) == 0 {
			continue
		}
		if n.Optional && !keepOptional(n.CPU) {
			continue
		}
		pkgDir := filepath.Join(l.root, slotDir(n.Name, n.Version))
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
