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
