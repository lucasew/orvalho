package js

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewDoesNotRunScript(t *testing.T) {
	iso := New(`globalThis.ran = true;`, Options{})
	if iso.initialized {
		t.Fatal("New must not evaluate the script")
	}
}

func TestRunScriptOnFirstTick(t *testing.T) {
	iso := New(`var ran = true;`, Options{})
	more, err := iso.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if more {
		t.Fatal("expected no pending work")
	}
	if !iso.vm.Get("ran").ToBoolean() {
		t.Fatal("script did not run")
	}
}

func TestSetTimeoutFiresUnderHostControl(t *testing.T) {
	iso := New(`
		var fired = false;
		setTimeout(function() { fired = true; }, 50);
	`, Options{})

	// Freeze wall-clock so deadlines are deterministic.
	base := time.Unix(1_700_000_000, 0)
	iso.now = func() time.Time { return base }

	more, err := iso.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !more {
		t.Fatal("expected pending timer")
	}

	// Still before deadline: callback must not run.
	iso.now = func() time.Time { return base.Add(49 * time.Millisecond) }
	more, err = iso.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !more {
		t.Fatal("timer should still be pending")
	}
	if iso.vm.Get("fired").ToBoolean() {
		t.Fatal("callback ran too early")
	}

	// At/after deadline: host Tick fires it.
	iso.now = func() time.Time { return base.Add(50 * time.Millisecond) }
	more, err = iso.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if more {
		t.Fatal("expected idle after last timer")
	}
	if !iso.vm.Get("fired").ToBoolean() {
		t.Fatal("callback did not run")
	}
}

func TestSetTimeoutPassesArgs(t *testing.T) {
	iso := New(`
		var got = null;
		setTimeout(function(a, b) { got = a + b; }, 0, 2, 3);
	`, Options{})
	base := time.Unix(0, 0)
	iso.now = func() time.Time { return base }

	if _, err := iso.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := iso.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := iso.vm.Get("got").ToInteger(); got != 5 {
		t.Fatalf("got %d, want 5", got)
	}
}

func TestSetIntervalAndClear(t *testing.T) {
	iso := New(`
		var count = 0;
		var id = setInterval(function() {
			count++;
			if (count >= 2) clearInterval(id);
		}, 10);
	`, Options{})

	base := time.Unix(0, 0)
	now := base
	iso.now = func() time.Time { return now }

	if _, err := iso.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5; i++ {
		now = now.Add(10 * time.Millisecond)
		more, err := iso.Tick(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !more && iso.vm.Get("count").ToInteger() >= 2 {
			break
		}
	}

	if c := iso.vm.Get("count").ToInteger(); c != 2 {
		t.Fatalf("count=%d, want 2", c)
	}
	if iso.PendingTimers() != 0 {
		t.Fatalf("pending=%d, want 0", iso.PendingTimers())
	}
}

func TestClearTimeoutBeforeFire(t *testing.T) {
	iso := New(`
		var ran = false;
		var id = setTimeout(function() { ran = true; }, 10);
		clearTimeout(id);
	`, Options{})

	base := time.Unix(0, 0)
	iso.now = func() time.Time { return base }

	more, err := iso.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if more {
		t.Fatal("cleared timer must leave isolate idle")
	}

	iso.now = func() time.Time { return base.Add(100 * time.Millisecond) }
	if _, err := iso.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if iso.vm.Get("ran").ToBoolean() {
		t.Fatal("cleared timer must not fire")
	}
}

func TestContextCancelBeforeTick(t *testing.T) {
	iso := New(`var x = 1;`, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := iso.Tick(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	if iso.initialized {
		t.Fatal("canceled Tick must not initialize")
	}
}

func TestContextCancelInterruptsInfiniteLoop(t *testing.T) {
	iso := New(`while (true) {}`, Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := iso.Tick(ctx)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error from canceled infinite loop")
		}
		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			t.Fatalf("want context deadline/cancel, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Tick did not return after context deadline")
	}
}

func TestMaxTimersPerTickCap(t *testing.T) {
	// Schedule more zero-delay timers than the per-tick batch cap.
	iso := New(`
		var n = 0;
		for (var i = 0; i < 50; i++) {
			setTimeout(function() { n++; }, 0);
		}
	`, Options{MaxTimersPerTick: 10, MaxPendingTimers: 100})

	base := time.Unix(0, 0)
	iso.now = func() time.Time { return base }

	if _, err := iso.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if iso.PendingTimers() != 50 {
		t.Fatalf("pending=%d, want 50", iso.PendingTimers())
	}

	more, err := iso.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !more {
		t.Fatal("expected more work after partial batch")
	}
	if n := iso.vm.Get("n").ToInteger(); n != 10 {
		t.Fatalf("n=%d, want 10 after one capped Tick", n)
	}
	if iso.PendingTimers() != 40 {
		t.Fatalf("pending=%d, want 40", iso.PendingTimers())
	}

	// Drain remaining under the same cap.
	for iso.PendingTimers() > 0 {
		more, err = iso.Tick(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	}
	if more {
		t.Fatal("expected idle when drained")
	}
	if n := iso.vm.Get("n").ToInteger(); n != 50 {
		t.Fatalf("n=%d, want 50 after drain", n)
	}
}

func TestMaxPendingTimersCap(t *testing.T) {
	iso := New(`
		var threw = false;
		var ok = 0;
		try {
			for (var i = 0; i < 5; i++) {
				setTimeout(function() {}, 1000);
				ok++;
			}
		} catch (e) {
			threw = true;
			globalThis.errMsg = String(e);
		}
		globalThis.ok = ok;
		globalThis.threw = threw;
	`, Options{MaxPendingTimers: 3, MaxTimersPerTick: 100})

	_, err := iso.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !iso.vm.Get("threw").ToBoolean() {
		t.Fatal("expected throw when exceeding MaxPendingTimers")
	}
	if ok := iso.vm.Get("ok").ToInteger(); ok != 3 {
		t.Fatalf("ok=%d, want 3 successful schedules before throw", ok)
	}
	if iso.PendingTimers() != 3 {
		t.Fatalf("pending=%d, want 3", iso.PendingTimers())
	}
	msg := iso.vm.Get("errMsg").String()
	if !strings.Contains(strings.ToLower(msg), "timer") {
		t.Fatalf("unexpected error message: %q", msg)
	}
}

func TestPromiseMicrotaskOnTick(t *testing.T) {
	iso := New(`
		var resolved = false;
		Promise.resolve().then(function() { resolved = true; });
	`, Options{})
	if _, err := iso.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !iso.vm.Get("resolved").ToBoolean() {
		t.Fatal("promise microtask did not run during Tick")
	}
}
