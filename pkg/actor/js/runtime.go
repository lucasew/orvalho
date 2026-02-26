package js

import (
	"context"
	"sync"
	"time"

	"orvalho/pkg/actor"

	"github.com/dop251/goja"
)

// Runtime implements the Actor interface using goja.
type Runtime struct {
	vm          *goja.Runtime
	script      string
	initialized bool

	// Timer management
	timerManager *TimerManager

	mutex sync.Mutex
}

// Ensure Runtime implements Actor interface.
var _ actor.Actor = (*Runtime)(nil)

// New creates a new JavaScript actor runtime.
// It prepares the environment but does not execute the script yet.
func New(script string) *Runtime {
	r := &Runtime{
		vm:           goja.New(),
		script:       script,
		timerManager: NewTimerManager(),
	}
	r.initAPI()
	return r
}

func (r *Runtime) initAPI() {
	r.vm.Set("setTimeout", r.setTimeout)
	r.vm.Set("clearTimeout", r.clearTimeout)
	r.vm.Set("setInterval", r.setInterval)
	r.vm.Set("clearInterval", r.clearInterval)
}

// Tick executes one step of the actor's logic.
func (r *Runtime) Tick(ctx context.Context) (bool, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Check context
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	// Setup context monitoring for interruption
	r.vm.ClearInterrupt()
	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-ctx.Done():
			r.vm.Interrupt(ctx.Err())
		case <-done:
		}
	}()

	// Lazy initialization
	if !r.initialized {
		r.initialized = true
		_, err := r.vm.RunString(r.script)
		if err != nil {
			// If interrupted by context, return context error
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			return false, err
		}

		// If timers were set, return true.
		if r.timerManager.Len() > 0 {
			return true, nil
		}
		// If script finished and no timers, we are done.
		return false, nil
	}

	// Process timers
	now := time.Now()

	for {
		t := r.timerManager.PopExpired(now)
		if t == nil {
			break
		}

		// Execute callback
		_, err := t.callback(goja.Undefined(), t.args...)
		if err != nil {
			// If interrupted by context, return context error
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			return false, err
		}

		// If interval, reschedule
		if t.interval > 0 {
			r.timerManager.Reschedule(t, now)
		}
	}

	// Check if we have more work
	if r.timerManager.Len() > 0 {
		return true, nil
	}

	// If we executed something, maybe there are side effects (jobs) that were run.
	// But if no timers left, we are done.
	return false, nil
}

func (r *Runtime) setTimeout(call goja.FunctionCall) goja.Value {
	return r.scheduleTimer(call, false)
}

func (r *Runtime) setInterval(call goja.FunctionCall) goja.Value {
	return r.scheduleTimer(call, true)
}

func (r *Runtime) clearTimeout(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 {
		return goja.Undefined()
	}
	id := call.Argument(0).ToInteger()
	r.timerManager.Cancel(id)
	return goja.Undefined()
}

func (r *Runtime) clearInterval(call goja.FunctionCall) goja.Value {
	return r.clearTimeout(call)
}

func (r *Runtime) scheduleTimer(call goja.FunctionCall, repeating bool) goja.Value {
	fn, ok := goja.AssertFunction(call.Argument(0))
	if !ok {
		// Not a function, do nothing or panic? Browsers usually throw or ignore.
		// For simplicity return undefined.
		return goja.Undefined()
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

	id := r.timerManager.Schedule(fn, args, delay, interval)
	return r.vm.ToValue(id)
}
