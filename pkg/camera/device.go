package camera

import (
    "github.com/lucasew/orvalho/pkg/device"
    "fmt"
)

type CameraDevice struct {
    id string
}

func (c *CameraDevice) ID() string { return c.id }
func (c *CameraDevice) Type() device.DeviceType { return device.DeviceTypeCamera }
func (c *CameraDevice) Close() error { return nil }

func (c *CameraDevice) Capture() int {
    fmt.Printf("Camera %s: Capture\n", c.id)
    return 1
}

type Discoverer struct {}

func (d *Discoverer) Discover() ([]device.Device, error) {
    return []device.Device{
        &CameraDevice{id: "camera-0"},
    }, nil
}
