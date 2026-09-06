package dependency

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LockfileName is the only writer in this version.
const LockfileName = "package-lock.json"

var foreignLockfiles = []string{
	"yarn.lock",
	"pnpm-lock.yaml",
	"bun.lock",
	"aube-lock.yaml",
	"npm-shrinkwrap.json",
}

// Node is one resolved Dependency in the graph.
type Node struct {
	Name                 string
	Version              string
	Resolved             string
	Integrity            string
	Dev                  bool
	Optional             bool
	CPU                  []string
	Dependencies         map[string]string
	PeerDependencies     map[string]string
	OptionalDependencies map[string]string
	Bin                  map[string]string
	LockPath             string // npm packages key, e.g. node_modules/leftpad
}

// Graph is the resolved tree plus the root manifest ranges.
type Graph struct {
	Name            string
	Version         string
	Root            map[string]string // name -> range from dependencies + devDependencies
	DevRoot         map[string]string
	Nodes           []Node
	Packages        map[string]lockPackage // npm packages map, including ""
	LockfileVersion int
}

// DetectLockfile returns the path of package-lock.json, empty if none exists,
// or ErrLockfile when a foreign lockfile is present without package-lock.json.
func DetectLockfile(dir string) (string, error) {
	ours := filepath.Join(dir, LockfileName)
	if st, err := os.Stat(ours); err == nil && !st.IsDir() {
		return ours, nil
	}
	for _, name := range foreignLockfiles {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return "", fmt.Errorf("%w: unrecognized %s", ErrLockfile, name)
		}
	}
	return "", nil
}

type lockFile struct {
	Name            string                 `json:"name"`
	Version         string                 `json:"version"`
	LockfileVersion int                    `json:"lockfileVersion"`
	Requires        bool                   `json:"requires"`
	Packages        map[string]lockPackage `json:"packages"`
	// v1/v2 nested dependencies are ignored; v2 also has packages.
	Dependencies json.RawMessage `json:"dependencies,omitempty"`
}

type lockPackage struct {
	Name                 string            `json:"name,omitempty"`
	Version              string            `json:"version,omitempty"`
	Resolved             string            `json:"resolved,omitempty"`
	Integrity            string            `json:"integrity,omitempty"`
	Link                 bool              `json:"link,omitempty"`
	Dev                  bool              `json:"dev,omitempty"`
	Optional             bool              `json:"optional,omitempty"`
	DevOptional          bool              `json:"devOptional,omitempty"`
	CPU                  []string          `json:"cpu,omitempty"`
	OS                   []string          `json:"os,omitempty"`
	Dependencies         map[string]string `json:"dependencies,omitempty"`
	DevDependencies      map[string]string `json:"devDependencies,omitempty"`
	PeerDependencies     map[string]string `json:"peerDependencies,omitempty"`
	OptionalDependencies map[string]string `json:"optionalDependencies,omitempty"`
	Bin                  any               `json:"bin,omitempty"`
}

func parseBin(v any) map[string]string {
	switch x := v.(type) {
	case string:
		base := filepath.Base(x)
		return map[string]string{base: x}
	case map[string]any:
		out := make(map[string]string, len(x))
		for k, raw := range x {
			s, ok := raw.(string)
			if ok {
				out[k] = s
			}
		}
		return out
	}
	return nil
}

// ReadLockfile parses package-lock.json v2 or v3.
func ReadLockfile(path string) (*Graph, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLockfile, err)
	}
	var raw lockFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLockfile, err)
	}
	if raw.LockfileVersion != 2 && raw.LockfileVersion != 3 {
		return nil, fmt.Errorf("%w: lockfileVersion %d", ErrLockfile, raw.LockfileVersion)
	}
	if raw.Packages == nil {
		return nil, fmt.Errorf("%w: missing packages", ErrLockfile)
	}
	g := &Graph{
		Name:            raw.Name,
		Version:         raw.Version,
		Root:            map[string]string{},
		DevRoot:         map[string]string{},
		Packages:        raw.Packages,
		LockfileVersion: raw.LockfileVersion,
	}
	if root, ok := raw.Packages[""]; ok {
		for n, r := range root.Dependencies {
			g.Root[n] = r
		}
		for n, r := range root.DevDependencies {
			g.DevRoot[n] = r
			if _, exists := g.Root[n]; !exists {
				g.Root[n] = r
			}
		}
	}
	for p, ent := range raw.Packages {
		if p == "" || ent.Link {
			continue
		}
		name := ent.Name
		if name == "" {
			name = packageNameFromPath(p)
		}
		g.Nodes = append(g.Nodes, Node{
			Name:                 name,
			Version:              ent.Version,
			Resolved:             ent.Resolved,
			Integrity:            ent.Integrity,
			Dev:                  ent.Dev,
			Optional:             ent.Optional || ent.DevOptional,
			CPU:                  ent.CPU,
			Dependencies:         ent.Dependencies,
			PeerDependencies:     ent.PeerDependencies,
			OptionalDependencies: ent.OptionalDependencies,
			Bin:                  parseBin(ent.Bin),
			LockPath:             p,
		})
	}
	return g, nil
}

// WriteLockfile writes lockfileVersion 3.
func WriteLockfile(path string, g *Graph) error {
	raw := lockFile{
		Name:            g.Name,
		Version:         g.Version,
		LockfileVersion: 3,
		Requires:        true,
		Packages:        g.Packages,
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFileAtomic(path, data, 0o644)
}

func packageNameFromPath(p string) string {
	parts := strings.Split(p, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "node_modules" || i+1 >= len(parts) {
			continue
		}
		if strings.HasPrefix(parts[i+1], "@") && i+2 < len(parts) {
			return parts[i+1] + "/" + parts[i+2]
		}
		return parts[i+1]
	}
	return p
}
