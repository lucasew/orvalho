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
