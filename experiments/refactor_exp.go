package main

import (
    "context"
    "fmt"
    "time"

    "github.com/lucasew/orvalho/pkg/runtime"
    "github.com/lucasew/orvalho/pkg/policy"
)

func main() {
    rt := runtime.NewRuntime()
    if err := rt.Initialize(); err != nil {
        panic(err)
    }

    manifest := policy.Manifest{
        Capabilities: map[string]policy.CapabilityReq{
            "gpu": {Required: true},
            "camera": {Required: true},
        },
    }

    actor, err := rt.SpawnActor("test-actor", manifest)
    if err != nil {
        panic(err)
    }

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    actor.Start(ctx)

    script := `
    async function main() {
        console.log("Listing devices...");
        const gpus = env.DEVICES.list("gpu");
        console.log("GPUs:", gpus);

        if (gpus.length > 0) {
            const gpu = await env.DEVICES.get(gpus[0]);
            console.log("Dispatching to GPU...");
            await gpu.dispatch({workgroups: 10});
        }

        const cameras = env.DEVICES.list("camera");
        if (cameras.length > 0) {
            const cam = await env.DEVICES.get(cameras[0]);
            console.log("Capturing...");
            await cam.capture({});
        }
    }
    main();
    `

    rt.SendScript(actor, script)

    time.Sleep(1 * time.Second)
    fmt.Println("Done")
}
