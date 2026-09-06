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
	for _, f := range []struct {
		key  string
		dest any
	}{
		{"name", &m.Name},
		{"version", &m.Version},
		{"dependencies", &m.Dependencies},
		{"devDependencies", &m.DevDependencies},
		{"optionalDependencies", &m.OptionalDependencies},
		{"peerDependencies", &m.PeerDependencies},
	} {
		if err := decodeField(raw, f.key, f.dest); err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrManifest, f.key, err)
		}
	}
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
	// encoding/json sorts map keys, so the written file is stable.
	data, err := json.MarshalIndent(jsonKeyed(raw), "", "  ")
	if err != nil {
		return err
	}
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
