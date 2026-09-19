package dependency

import "strings"

// keepOptional reports whether an optional Dependency is WASM (INV-18).
func keepOptional(cpu []string) bool {
	for _, c := range cpu {
		if strings.EqualFold(c, "wasm32") {
			return true
		}
	}
	return false
}
