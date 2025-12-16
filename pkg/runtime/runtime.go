package runtime

import (
    "github.com/lucasew/orvalho/pkg/actor"
    "github.com/lucasew/orvalho/pkg/binding"
    "github.com/lucasew/orvalho/pkg/platform"
    "github.com/lucasew/orvalho/pkg/policy"

    // Auto-register discoverers
    "github.com/lucasew/orvalho/pkg/gpu"
    "github.com/lucasew/orvalho/pkg/camera"
)

type Runtime struct {
    Platform *platform.Platform
}

func NewRuntime() *Runtime {
    p := platform.NewPlatform()

    // Register Discoverers
    p.RegisterDiscoverer(&gpu.Discoverer{})
    p.RegisterDiscoverer(&camera.Discoverer{})

    return &Runtime{
        Platform: p,
    }
}

func (r *Runtime) Initialize() error {
    return r.Platform.Scan()
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
