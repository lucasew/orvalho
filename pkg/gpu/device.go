package gpu

import (
    "github.com/lucasew/orvalho/pkg/device"
    "fmt"
)

type GPUDevice struct {
    id string
    // internal wgpu handles
}

func (g *GPUDevice) ID() string { return g.id }
func (g *GPUDevice) Type() device.DeviceType { return device.DeviceTypeGPU }
func (g *GPUDevice) Close() error { return nil }

func (g *GPUDevice) Dispatch(workgroups int) error {
    fmt.Printf("GPU %s: Dispatch %d\n", g.id, workgroups)
    return nil
}

// Discoverer implementation
type Discoverer struct {}

func (d *Discoverer) Discover() ([]device.Device, error) {
    // Mock discovery
    return []device.Device{
        &GPUDevice{id: "gpu-0"},
    }, nil
}
