package imports

import (
	"encoding/json"
	"strings"
)

var exportConditions = []string{"require", "node", "default"}

func resolveExports(packageJSON []byte, sub string) (string, bool) {
	var meta struct {
		Exports json.RawMessage `json:"exports"`
	}
	if json.Unmarshal(packageJSON, &meta) != nil || len(meta.Exports) == 0 || string(meta.Exports) == "null" {
		return "", false
	}
	var v any
	if json.Unmarshal(meta.Exports, &v) != nil {
		return "", false
	}
	key := "."
	if sub != "" {
		key = "./" + strings.TrimPrefix(sub, "./")
	}
	target, ok := matchExport(v, key)
	if !ok {
		return "", false
	}
	return strings.TrimPrefix(target, "./"), true
}

func exportMiss(packageJSON []byte, sub string) bool {
	var meta struct {
		Exports json.RawMessage `json:"exports"`
	}
	if json.Unmarshal(packageJSON, &meta) != nil || len(meta.Exports) == 0 || string(meta.Exports) == "null" {
		return false
	}
	_, ok := resolveExports(packageJSON, sub)
	return !ok
}

func matchExport(v any, key string) (string, bool) {
	switch x := v.(type) {
	case string:
		if key == "." {
			return x, true
		}
		return "", false
	case map[string]any:
		if isConditionMap(x) {
			if key != "." {
				return "", false
			}
			return pickCondition(x)
		}
		if raw, ok := x[key]; ok {
			return pickTarget(raw)
		}
		if key == "." {
			if raw, ok := x["."]; ok {
				return pickTarget(raw)
			}
			return pickCondition(x)
		}
		return "", false
	default:
		return "", false
	}
}

func isConditionMap(m map[string]any) bool {
	for k := range m {
		if strings.HasPrefix(k, ".") {
			return false
		}
	}
	return len(m) > 0
}

func pickTarget(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case map[string]any:
		return pickCondition(x)
	case []any:
		for _, item := range x {
			if s, ok := pickTarget(item); ok {
				return s, true
			}
		}
		return "", false
	default:
		return "", false
	}
}

func pickCondition(m map[string]any) (string, bool) {
	for _, c := range exportConditions {
		raw, ok := m[c]
		if !ok {
			continue
		}
		if s, ok := pickTarget(raw); ok {
			return s, true
		}
	}
	return "", false
}

// resolveImports maps a #specifier through package.json "imports".
// Conditions are the same as exports (require, node, default).
func resolveImports(packageJSON []byte, spec string) (string, bool) {
	var meta struct {
		Imports json.RawMessage `json:"imports"`
	}
	if json.Unmarshal(packageJSON, &meta) != nil || len(meta.Imports) == 0 || string(meta.Imports) == "null" {
		return "", false
	}
	var v any
	if json.Unmarshal(meta.Imports, &v) != nil {
		return "", false
	}
	m, ok := v.(map[string]any)
	if !ok {
		return "", false
	}
	if raw, ok := m[spec]; ok {
		return pickTarget(raw)
	}
	return matchImportPattern(m, spec)
}

func matchImportPattern(m map[string]any, spec string) (string, bool) {
	bestKey := ""
	bestMid := ""
	var bestRaw any
	for key, raw := range m {
		star := strings.IndexByte(key, '*')
		if star < 0 {
			continue
		}
		prefix, suffix := key[:star], key[star+1:]
		if !strings.HasPrefix(spec, prefix) || !strings.HasSuffix(spec, suffix) {
			continue
		}
		if len(spec) < len(prefix)+len(suffix) {
			continue
		}
		if bestKey != "" && len(key) < len(bestKey) {
			continue
		}
		bestKey = key
		bestMid = spec[len(prefix) : len(spec)-len(suffix)]
		bestRaw = raw
	}
	if bestKey == "" {
		return "", false
	}
	target, ok := pickTarget(bestRaw)
	if !ok {
		return "", false
	}
	return strings.Replace(target, "*", bestMid, 1), true
}
