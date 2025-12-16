package main

import (
    "context"
    "time"
    "github.com/lucasew/orvalho/internal/runtime"
    "github.com/lucasew/orvalho/internal/capability"
)

func main() {
    caps := capability.DefaultCapabilities()
    caps.AllowFetch = true

    actor, err := runtime.NewActor("test-actor", caps)
    if err != nil {
        panic(err)
    }

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    actor.Start(ctx)
    // defer actor.Shutdown()

    // Send a script to run
    script := `
        console.log("Hello from Actor!");

        setTimeout(() => {
            console.log("Timeout fired!");
        }, 500);

        setInterval(() => {
             console.log("Interval tick");
        }, 200);

        fetch("https://httpbin.org/get").then(res => {
            console.log("Fetch result:", res);
        }).catch(err => {
            console.log("Fetch error:", err);
        });

        console.log("Base64:", btoa("hello"));
    `

    actor.Send(runtime.EvalEvent{Script: script})

    time.Sleep(3 * time.Second)
}
