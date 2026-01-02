package gpu

import "github.com/lucasew/orvalho/pkg/device"

// Device extends the base Device interface with GPU specific capabilities
type Device interface {
    device.Device
    Dispatch(workgroups int) error
    CreateBuffer(size int) (int, error)
}
