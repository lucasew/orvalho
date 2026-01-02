package runtime

import (
    "github.com/lucasew/orvalho/pkg/actor"
    "github.com/lucasew/orvalho/pkg/binding"
    "github.com/lucasew/orvalho/pkg/platform"
    "github.com/lucasew/orvalho/pkg/policy"

    // Auto-register drivers
    "github.com/lucasew/orvalho/pkg/device/gpu/wgpu"
    "github.com/lucasew/orvalho/pkg/device/camera/v4l2"
)

type Runtime struct {
    Platform *platform.Platform
}

func NewRuntime() *Runtime {
    p := platform.NewPlatform()

    // Register Drivers
    p.RegisterDriver(&wgpu.Driver{})
    p.RegisterDriver(&v4l2.Driver{})

    return &Runtime{
        Platform: p,
    }
}

func (r *Runtime) Initialize() error {
    return r.Platform.Initialize()
}

func (r *Runtime) SpawnActor(id string, manifest policy.Manifest) (*actor.Actor, error) {
    // 1. Policy Evaluation
    allDevices := r.Platform.Registry.List()
    allowedDevices := policy.Evaluate(manifest, allDevices)

    // 2. Create Actor
    a, err := actor.NewActor(id, allowedDevices)
    if err != nil {
        return nil, err
    }

    // 3. Init Environment
    err = a.InitEnv(binding.InjectDevices)
    if err != nil {
        return nil, err
    }

    return a, nil
}

func (r *Runtime) SendScript(a *actor.Actor, script string) error {
    return a.Send(actor.EvalEvent{Script: script})
}
