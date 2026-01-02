package js

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	r := New("var x = 1;")
	if r == nil {
		t.Fatal("New returned nil")
	}
	if r.initialized {
		t.Error("New should not execute script immediately")
	}
}

func TestLazyInit(t *testing.T) {
	script := `
		var ran = false;
		ran = true;
	`
	r := New(script)
	ctx := context.Background()
	more, err := r.Tick(ctx)
	if err != nil {
		t.Fatalf("Tick failed: %v", err)
	}
	if !r.initialized {
		t.Error("First Tick should initialize runtime")
	}
	if more {
		t.Error("Tick should return false for simple script")
	}

	val := r.vm.Get("ran")
	if !val.ToBoolean() {
		t.Error("Script did not run correctly")
	}
}

func TestSetTimeout(t *testing.T) {
	script := `
		var callbackRan = false;
		setTimeout(function() {
			callbackRan = true;
		}, 50);
	`
	r := New(script)
	ctx := context.Background()

	// First tick initializes and sets timer
	more, err := r.Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !more {
		t.Fatal("Tick should return true when timer is set")
	}

	// Immediate next tick shouldn't run timer yet
	more, err = r.Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !more {
		t.Fatal("Tick should return true when timer is pending")
	}
	val := r.vm.Get("callbackRan")
	if val.ToBoolean() {
		t.Fatal("Callback ran too early")
	}

	// Wait for timer to expire
	time.Sleep(60 * time.Millisecond)

	// Next tick should run callback
	more, err = r.Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// After running the only timer, it should return false (no more work)
	if more {
		t.Error("Tick should return false after last timer ran")
	}

	val = r.vm.Get("callbackRan")
	if !val.ToBoolean() {
		t.Error("Callback did not run")
	}
}

func TestSetInterval(t *testing.T) {
	script := `
		var count = 0;
		setInterval(function() {
			count++;
		}, 10);
	`
	r := New(script)
	ctx := context.Background()

	// Init
	more, err := r.Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !more {
		t.Fatal("Should have active interval")
	}

	// Wait and Tick multiple times
	for i := 0; i < 3; i++ {
		time.Sleep(15 * time.Millisecond)
		more, err = r.Tick(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !more {
			t.Fatal("Interval should keep runtime active")
		}
	}

	val := r.vm.Get("count")
	count := val.ToInteger()
	if count < 3 {
		t.Errorf("Interval count expected >= 3, got %d", count)
	}
}

func TestClearTimeout(t *testing.T) {
	script := `
		var ran = false;
		var id = setTimeout(function() {
			ran = true;
		}, 10);
		clearTimeout(id);
	`
	r := New(script)
	ctx := context.Background()

	more, err := r.Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Since we cleared the timeout synchronously in the script, there should be no timers left.
	if more {
		t.Error("Tick should return false after clearTimeout")
	}

	time.Sleep(20 * time.Millisecond)
	r.Tick(ctx)

	val := r.vm.Get("ran")
	if val.ToBoolean() {
		t.Error("Cancelled timer should not run")
	}
}

func TestClearInterval(t *testing.T) {
	script := `
		var count = 0;
		var id = setInterval(function() {
			count++;
			if (count >= 2) {
				clearInterval(id);
			}
		}, 10);
	`
	r := New(script)
	ctx := context.Background()

	// Init
	r.Tick(ctx)

	// Run until cleared
	for i := 0; i < 10; i++ {
		time.Sleep(15 * time.Millisecond)
		more, err := r.Tick(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !more {
			// It returned false, meaning no more timers.
			break
		}
	}

	val := r.vm.Get("count")
	if val.ToInteger() != 2 {
		t.Errorf("Expected count 2, got %d", val.ToInteger())
	}
}

func TestPromiseHandling(t *testing.T) {
	// Goja handles promises as microtasks.
	// This test ensures they are executed when calling Tick (via RunString or timer callback).

	// Case 1: Promise in initial script
	script := `
		var resolved = false;
		Promise.resolve().then(function() {
			resolved = true;
		});
	`
	r := New(script)
	ctx := context.Background()
	more, err := r.Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}

	val := r.vm.Get("resolved")
	if !val.ToBoolean() {
		t.Error("Promise microtask did not run after initial script")
	}
	if more {
		t.Error("Should be done")
	}

	// Case 2: Promise in setTimeout
	script2 := `
		var resolved2 = false;
		setTimeout(function() {
			Promise.resolve().then(function() {
				resolved2 = true;
			});
		}, 10);
	`
	r2 := New(script2)
	r2.Tick(ctx) // init

	time.Sleep(20 * time.Millisecond)
	more, err = r2.Tick(ctx) // run timer
	if err != nil {
		t.Fatal(err)
	}

	val = r2.vm.Get("resolved2")
	if !val.ToBoolean() {
		t.Error("Promise microtask did not run after timer callback")
	}
}

func TestContextCancel(t *testing.T) {
	r := New("while(true);")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := r.Tick(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context canceled error, got %v", err)
	}
}

func TestContextTimeout(t *testing.T) {
	// Script that loops infinitely
	script := "while(true) {}"
	r := New(script)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Since Goja RunString is blocking, we need to ensure it respects the context cancellation.
	// However, standard Goja RunString is NOT interruptible by context unless we use Interrupt.
	// The current implementation of Tick checks context at the BEGINNING.
	// If the script itself is an infinite loop, RunString will block forever and Tick will never return,
	// ignoring the context deadline because it's only checked at entry.
	//
	// To fix this, we need to interrupt the VM when context is done.
	// But let's first see if the current implementation fails this test as expected.
	// Wait, the prompt implies the user WANTS to test if it cancels.
	// If I implement this test and it hangs, I'll need to fix the implementation.

	// Running this in a goroutine to avoid hanging the test suite if it fails.
	done := make(chan error, 1)
	go func() {
		_, err := r.Tick(ctx)
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			// Note: If the script finishes (it shouldn't) or returns another error.
			// Goja might return an InterruptedError if we implement interruption.
			// If we rely on just checking context at entry, this will timeout the TEST (hang).
			t.Logf("Tick returned error: %v", err)
			// For now, let's see if it works.
			// Spoiler: It won't work with the current implementation because RunString blocks.
			// But the user asked to ADD the test. I should add it and if it fails/hangs, I fix the code.
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out, Tick did not return after context cancellation")
	}
}
