package js

import (
	"fmt"

	"cuelang.org/go/cue"

	"orvalho/pkg/ovpkg"
)

// MaterializeAgentBindings builds host Binding map from a package agent.
// Unsupported types fail (never-allocate).
func MaterializeAgentBindings(pkg *ovpkg.Package, agent *ovpkg.Agent) (map[string]Binding, error) {
	if agent == nil {
		return nil, fmt.Errorf("js: nil agent")
	}
	out := make(map[string]Binding, len(agent.Bindings))
	for name, spec := range agent.Bindings {
		if _, ok := agent.Env[name]; ok {
			return nil, fmt.Errorf("js: env name %q used as both string and binding", name)
		}
		b, err := materializeSpec(pkg, name, spec)
		if err != nil {
			return nil, err
		}
		out[name] = b
	}
	return out, nil
}

func materializeSpec(pkg *ovpkg.Package, name string, spec ovpkg.BindingSpec) (Binding, error) {
	switch spec.Type {
	case "assets":
		return materializeAssets(pkg, name, spec.Value)
	default:
		return nil, fmt.Errorf("unsupported binding type %q on %s (available: assets)", spec.Type, name)
	}
}

func materializeAssets(pkg *ovpkg.Package, name string, v cue.Value) (Binding, error) {
	rootV := v.LookupPath(cue.ParsePath("root"))
	if !rootV.Exists() {
		return nil, fmt.Errorf("assets binding %q: missing root", name)
	}
	root, err := rootV.String()
	if err != nil {
		return nil, fmt.Errorf("assets binding %q: root: %w", name, err)
	}
	var paths []string
	pv := v.LookupPath(cue.ParsePath("paths"))
	if pv.Exists() {
		iter, err := pv.List()
		if err != nil {
			return nil, fmt.Errorf("assets binding %q: paths: %w", name, err)
		}
		for iter.Next() {
			s, err := iter.Value().String()
			if err != nil {
				return nil, fmt.Errorf("assets binding %q: paths entry: %w", name, err)
			}
			paths = append(paths, s)
		}
	}
	return &AssetBinding{
		Root:  root,
		Paths: paths,
		Read: func(path string) ([]byte, bool) {
			if pkg == nil {
				return nil, false
			}
			b, err := pkg.Get(path)
			if err != nil {
				return nil, false
			}
			return b, true
		},
	}, nil
}
