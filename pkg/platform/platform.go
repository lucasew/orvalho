package platform

import (
    "github.com/lucasew/orvalho/pkg/device"
)

// Platform manages device discovery and health
type Platform struct {
    Registry *device.Registry
    drivers  []device.Driver
}

func NewPlatform() *Platform {
    return &Platform{
        Registry: device.NewRegistry(),
    }
}

func (p *Platform) RegisterDriver(d device.Driver) {
    p.drivers = append(p.drivers, d)
}

func (p *Platform) Initialize() error {
    for _, d := range p.drivers {
        if err := d.Initialize(); err != nil {
            return err
        }

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
