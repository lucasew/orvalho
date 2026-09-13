package workers

import (
	"testing"
	"time"
)

func TestPumpUntilDoesNotRunOtherIsolate(t *testing.T) {
	a := New("", Options{})
	b := New("", Options{})
	if err := a.ScriptStart(t.Context(), `globalThis.n = 0; setInterval(function () { globalThis.n++; }, 1);`, "a.js"); err != nil {
		t.Fatal(err)
	}
	if err := b.ScriptStart(t.Context(), `globalThis.n = 0; setTimeout(function () { globalThis.n = 1; }, 0);`, "b.js"); err != nil {
		t.Fatal(err)
	}
	more, used, err := a.PumpUntil(t.Context(), 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !more {
		t.Fatal("a still has an interval")
	}
	if used <= 0 {
		t.Fatal("used")
	}
	if b.vm.Get("n").ToInteger() != 0 {
		t.Fatal("b ran without Pump")
	}
	if _, _, err := b.Pump(t.Context()); err != nil {
		t.Fatal(err)
	}
	if b.vm.Get("n").ToInteger() != 1 {
		t.Fatal("b did not run on its own Pump")
	}
}

func TestPumpDoesNotWait(t *testing.T) {
	iso := New("", Options{})
	if err := iso.ScriptStart(t.Context(), `globalThis.n = 0; setTimeout(function () { globalThis.n = 1; }, 60 * 1000);`, "t.js"); err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	more, _, err := iso.Pump(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(t0) > 200*time.Millisecond {
		t.Fatal("Pump waited on a future timer")
	}
	if !more {
		t.Fatal("timer remains")
	}
	if iso.vm.Get("n").ToInteger() != 0 {
		t.Fatal("future timer fired")
	}
}
