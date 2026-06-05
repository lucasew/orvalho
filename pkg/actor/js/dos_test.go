package js

import (
	"context"
	"testing"
	"time"
)

func TestInfiniteTimersBatchHuge(t *testing.T) {
	script := `
		for (var i = 0; i < 100000; i++) {
			setTimeout(function() {}, 0);
		}
	`
	r := New(script)
	ctx := context.Background()

	// Init
	more, err := r.Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !more {
		t.Fatal("Should have active timers")
	}

	done := make(chan struct{})
	go func() {
		r.Tick(ctx)
		close(done)
	}()

	select {
	case <-done:
		// success if it returns
	case <-time.After(1 * time.Second):
		t.Fatal("Tick hung indefinitely on infinite timers")
	}
}
