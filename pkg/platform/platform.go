package platform

import (
    "github.com/lucasew/orvalho/pkg/device"
)

// Discoverer finds devices of a specific type
type Discoverer interface {
    Discover() ([]device.Device, error)
}

// Platform manages device discovery and health
type Platform struct {
    Registry *device.Registry
    discoverers []Discoverer
}

func NewPlatform() *Platform {
    return &Platform{
        Registry: device.NewRegistry(),
    }
}

func (p *Platform) RegisterDiscoverer(d Discoverer) {
    p.discoverers = append(p.discoverers, d)
}

func (p *Platform) Scan() error {
    for _, d := range p.discoverers {
        devs, err := d.Discover()
        if err != nil {
            return err
        }
        for _, dev := range devs {
            p.Registry.Register(dev)
        }
    }
    return nil
}
