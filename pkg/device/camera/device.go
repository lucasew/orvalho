package camera

import "github.com/lucasew/orvalho/pkg/device"

// Device extends the base Device interface with Camera specific capabilities
type Device interface {
    device.Device
    Capture() (int, error)
}
