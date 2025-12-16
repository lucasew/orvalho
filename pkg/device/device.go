package device

// DeviceType identifies the type of hardware (GPU, Camera, etc)
type DeviceType string

const (
    DeviceTypeGPU    DeviceType = "gpu"
    DeviceTypeCamera DeviceType = "camera"
)

// Device represents a hardware capability
type Device interface {
    ID() string
    Type() DeviceType
    // Close releases resources
    Close() error
}

// Registry holds all available devices
type Registry struct {
    devices map[string]Device
}

func NewRegistry() *Registry {
    return &Registry{
        devices: make(map[string]Device),
    }
}

func (r *Registry) Register(d Device) {
    r.devices[d.ID()] = d
}

func (r *Registry) List() []Device {
    var list []Device
    for _, d := range r.devices {
        list = append(list, d)
    }
    return list
}

func (r *Registry) Get(id string) Device {
    return r.devices[id]
}
