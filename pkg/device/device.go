package device

// DeviceType identifies the type of hardware
type DeviceType string

const (
    DeviceTypeGPU    DeviceType = "gpu"
    DeviceTypeCamera DeviceType = "camera"
    DeviceTypeAudio  DeviceType = "audio"
)

// Device is the base interface for all hardware capabilities
type Device interface {
    ID() string
    Type() DeviceType
    DriverName() string
    Close() error
}

// Driver orchestrates discovery and lifecycle of devices
type Driver interface {
    Name() string
    Initialize() error
    Discover() ([]Device, error)
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
