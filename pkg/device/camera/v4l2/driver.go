package v4l2

import (
    "fmt"
    "github.com/lucasew/orvalho/pkg/device"
    cameraDevice "github.com/lucasew/orvalho/pkg/device/camera"
)

type Driver struct {}

func (d *Driver) Name() string { return "v4l2" }

func (d *Driver) Initialize() error {
    fmt.Println("[V4L2] Initializing driver...")
    return nil
}

func (d *Driver) Discover() ([]device.Device, error) {
    // Mock discovery
    return []device.Device{
        &CameraDevice{id: "camera-v4l2-0"},
    }, nil
}

func (d *Driver) Close() error { return nil }

type CameraDevice struct {
    id string
}

func (c *CameraDevice) ID() string { return c.id }
func (c *CameraDevice) Type() device.DeviceType { return device.DeviceTypeCamera }
func (c *CameraDevice) DriverName() string { return "v4l2" }
func (c *CameraDevice) Close() error { return nil }

func (c *CameraDevice) Capture() (int, error) {
    fmt.Printf("[V4L2] Capture on %s\n", c.id)
    return 1, nil
}

var _ cameraDevice.Device = (*CameraDevice)(nil)
