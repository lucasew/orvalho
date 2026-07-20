package main

import (
	"fmt"

	"cuelang.org/go/cue"

	"github.com/lucasew/orvalho/pkg/ovpkg"
	"github.com/lucasew/orvalho/pkg/workers"
)

// materializeAgentBindings builds workers.Binding map from a package agent.
// Unsupported types fail (never-allocate).
func materializeAgentBindings(pkg *ovpkg.Package, agent *ovpkg.Agent) (map[string]workers.Binding, error) {
	if agent == nil {
		return nil, fmt.Errorf("serve: nil agent")
	}
	out := make(map[string]workers.Binding, len(agent.Bindings))
	for name, spec := range agent.Bindings {
		if _, ok := agent.Env[name]; ok {
			return nil, fmt.Errorf("serve: env name %q used as both string and binding", name)
		}
		b, err := materializeSpec(pkg, name, spec)
		if err != nil {
			return nil, err
		}
		out[name] = b
	}
	return out, nil
}

func materializeSpec(pkg *ovpkg.Package, name string, spec ovpkg.BindingSpec) (workers.Binding, error) {
	switch spec.Type {
	case "assets":
		return materializeAssets(pkg, name, spec.Value)
	default:
		return nil, fmt.Errorf("unsupported binding type %q on %s (available: assets)", spec.Type, name)
	}
}

func materializeAssets(pkg *ovpkg.Package, name string, v cue.Value) (workers.Binding, error) {
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
	return workers.NewAssetBinding(pkg.PayloadFS(), root, paths...), nil
}
