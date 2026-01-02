package policy

import (
    "github.com/lucasew/orvalho/pkg/device"
)

type Manifest struct {
    Capabilities map[string]CapabilityReq `json:"capabilities"`
}

type CapabilityReq struct {
    Required bool `json:"required"`
}

// Evaluate filters available devices based on the manifest
func Evaluate(m Manifest, allDevices []device.Device) []device.Device {
    var allowed []device.Device

    // Very basic policy: if capability is listed, allow all devices of that type
    for _, dev := range allDevices {
        _, ok := m.Capabilities[string(dev.Type())]
        if ok {
            // In a real impl, we'd check selectors
            allowed = append(allowed, dev)
        } else {
            // Also allow if not explicitly forbidden? Or explicit allowlist?
            // "Actor layer filters devices based on manifest"
            // Let's assume strict allowlist for now based on types present in manifest
        }
    }

    return allowed
}
