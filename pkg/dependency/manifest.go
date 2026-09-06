package dependency

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Manifest is the project package.json fields we read and write.
type Manifest struct {
	Name                 string            `json:"name,omitempty"`
	Version              string            `json:"version,omitempty"`
	Dependencies         map[string]string `json:"dependencies,omitempty"`
	DevDependencies      map[string]string `json:"devDependencies,omitempty"`
	OptionalDependencies map[string]string `json:"optionalDependencies,omitempty"`
	PeerDependencies     map[string]string `json:"peerDependencies,omitempty"`

	raw map[string]json.RawMessage
}

func readManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrManifest, err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrManifest, err)
	}
	m := &Manifest{raw: raw}
	_ = decodeField(raw, "name", &m.Name)
	_ = decodeField(raw, "version", &m.Version)
	_ = decodeField(raw, "dependencies", &m.Dependencies)
	_ = decodeField(raw, "devDependencies", &m.DevDependencies)
	_ = decodeField(raw, "optionalDependencies", &m.OptionalDependencies)
	_ = decodeField(raw, "peerDependencies", &m.PeerDependencies)
	if m.Dependencies == nil {
		m.Dependencies = map[string]string{}
	}
	if m.DevDependencies == nil {
		m.DevDependencies = map[string]string{}
	}
	return m, nil
}

func decodeField(raw map[string]json.RawMessage, key string, dest any) error {
	v, ok := raw[key]
	if !ok {
		return nil
	}
	return json.Unmarshal(v, dest)
}

func writeManifest(path string, m *Manifest) error {
	raw := m.raw
	if raw == nil {
		raw = map[string]json.RawMessage{}
	}
	put := func(key string, v any, empty bool) error {
		if empty {
			delete(raw, key)
			return nil
		}
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		raw[key] = b
		return nil
	}
	if err := put("name", m.Name, m.Name == ""); err != nil {
		return err
	}
	if err := put("version", m.Version, m.Version == ""); err != nil {
		return err
	}
	if err := put("dependencies", m.Dependencies, len(m.Dependencies) == 0); err != nil {
		return err
	}
	if err := put("devDependencies", m.DevDependencies, len(m.DevDependencies) == 0); err != nil {
		return err
	}
	keys := make([]string, 0, len(raw))
	// Preserve a stable order: marshal via map (sorted by encoding/json).
	data, err := json.MarshalIndent(jsonKeyed(raw), "", "  ")
	if err != nil {
		return err
	}
	_ = keys
	data = append(data, '\n')
	return writeFileAtomic(path, data, 0o644)
}

func jsonKeyed(raw map[string]json.RawMessage) map[string]any {
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		var x any
		if err := json.Unmarshal(v, &x); err != nil {
			out[k] = json.RawMessage(v)
			continue
		}
		out[k] = x
	}
	return out
}

func manifestPath(dir string) string {
	return filepath.Join(dir, "package.json")
}
