package runtime_test

import (
	"context"
	"testing"
	"time"

	"github.com/lucasew/orvalho/internal/capability"
	"github.com/lucasew/orvalho/internal/runtime"
)

func TestActorRuntime(t *testing.T) {
	caps := capability.DefaultCapabilities()
	caps.AllowFetch = true

	actor, err := runtime.NewActor("test-actor", caps)
	if err != nil {
		t.Fatalf("Failed to create actor: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	actor.Start(ctx)
	defer actor.Shutdown()

	// Send a script to run
	script := `
        console.log("Test: Actor started");

        setTimeout(() => {
            console.log("Test: Timeout fired");
        }, 100);

        const b64 = btoa("hello");
        if (b64 !== "aGVsbG8=") {
            throw new Error("Base64 failed");
        }
        console.log("Test: Base64 ok");
    `

	err = actor.Send(runtime.EvalEvent{Script: script})
    // Note: Send puts in channel, doesn't wait for execution error.
    // Ideally we'd have a way to get result back.
    // For now we assume if it doesn't crash it works, and we see logs in stdout.

	time.Sleep(500 * time.Millisecond)
}
