package js

import (
	"context"
	"fmt"
	"sync"
	"time"

	"orvalho/pkg/actor"

	"github.com/dop251/goja"
)

// Isolate is one pure-goja VM with host-driven timers.
// Create with [New], then advance with [Isolate.Tick].
type Isolate struct {
	vm          *goja.Runtime
	script      string
	initialized bool
	opts        Options
	timers      *timerTable
	// now is time.Now by default; tests may override.
	now func() time.Time
	mu  sync.Mutex
}

// Ensure Isolate implements actor.Actor.
var _ actor.Actor = (*Isolate)(nil)

// New creates an isolate for script. The script is not executed until the
// first Tick. Zero-valued opts fields use the documented defaults.
func New(script string, opts Options) *Isolate {
	iso := &Isolate{
		vm:     goja.New(),
		script: script,
		opts:   opts.withDefaults(),
		timers: newTimerTable(),
		now:    time.Now,
	}
	iso.installTimers()
	return iso
}

func (iso *Isolate) installTimers() {
	iso.vm.Set("setTimeout", iso.jsSetTimeout)
	iso.vm.Set("clearTimeout", iso.jsClearTimeout)
	iso.vm.Set("setInterval", iso.jsSetInterval)
	iso.vm.Set("clearInterval", iso.jsClearInterval)
}

// Tick runs one host-controlled step: first-time script evaluation, then up to
// MaxTimersPerTick due timer callbacks. Returns more=true when timers remain.
// ctx cancellation interrupts the VM and stops further work.
func (iso *Isolate) Tick(ctx context.Context) (bool, error) {
	iso.mu.Lock()
	defer iso.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return false, err
	}

	iso.vm.ClearInterrupt()
	stopWatch := make(chan struct{})
	defer close(stopWatch)
	go func() {
		select {
		case <-ctx.Done():
			iso.vm.Interrupt(ctx.Err())
		case <-stopWatch:
		}
	}()

	if !iso.initialized {
		iso.initialized = true
		_, err := iso.vm.RunString(iso.script)
		if err != nil {
			return false, mapJSError(ctx, err)
		}
		return iso.timers.len() > 0, nil
	}

	now := iso.now()
	executed := 0
	for executed < iso.opts.MaxTimersPerTick {
		if err := ctx.Err(); err != nil {
			return false, err
		}

		t := iso.timers.popDue(now)
		if t == nil {
			break
		}
		executed++

		_, err := t.callback(goja.Undefined(), t.args...)
		if err != nil {
			return false, mapJSError(ctx, err)
		}

		if t.interval > 0 {
			iso.timers.rescheduleInterval(t, now)
		}
	}

	return iso.timers.len() > 0, nil
}

// PendingTimers reports how many timers are currently scheduled.
func (iso *Isolate) PendingTimers() int {
	iso.mu.Lock()
	defer iso.mu.Unlock()
	return iso.timers.len()
}

func (iso *Isolate) jsSetTimeout(call goja.FunctionCall) goja.Value {
	return iso.scheduleFromJS(call, false)
}

func (iso *Isolate) jsSetInterval(call goja.FunctionCall) goja.Value {
	return iso.scheduleFromJS(call, true)
}

func (iso *Isolate) jsClearTimeout(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 {
		return goja.Undefined()
	}
	iso.timers.cancel(call.Argument(0).ToInteger())
	return goja.Undefined()
}

func (iso *Isolate) jsClearInterval(call goja.FunctionCall) goja.Value {
	return iso.jsClearTimeout(call)
}

func (iso *Isolate) scheduleFromJS(call goja.FunctionCall, repeating bool) goja.Value {
	fn, ok := goja.AssertFunction(call.Argument(0))
	if !ok {
		panic(iso.vm.NewTypeError("callback must be a function"))
	}

	if iso.timers.len() >= iso.opts.MaxPendingTimers {
		// Prefer a JS-visible throw so guest code can catch abuse-limit errors.
		panic(iso.vm.NewTypeError(
			fmt.Sprintf("too many pending timers (max %d)", iso.opts.MaxPendingTimers),
		))
	}

	delayMs := int64(0)
	if len(call.Arguments) > 1 {
		delayMs = call.Argument(1).ToInteger()
	}
	if delayMs < 0 {
		delayMs = 0
	}
	delay := time.Duration(delayMs) * time.Millisecond

	var args []goja.Value
	if len(call.Arguments) > 2 {
		args = call.Arguments[2:]
	}

	interval := time.Duration(0)
	if repeating {
		interval = delay
	}

	id := iso.timers.schedule(fn, args, delay, interval, iso.now())
	return iso.vm.ToValue(id)
}

func mapJSError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	// Prefer the host context error when the VM was interrupted because of it.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if ie, ok := err.(*goja.InterruptedError); ok {
		if v, ok := ie.Value().(error); ok {
			return v
		}
	}
	return err
}
