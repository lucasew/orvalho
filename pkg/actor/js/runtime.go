package js

import (
	"container/heap"
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
	timers      map[int64]*timer
	timerQueue  timerHeap
	nextTimerID int64

	mutex sync.Mutex
}

// Ensure Runtime implements Actor interface.
var _ actor.Actor = (*Runtime)(nil)

// New creates a new JavaScript actor runtime.
// It prepares the environment but does not execute the script yet.
func New(script string) *Runtime {
	r := &Runtime{
		vm:          goja.New(),
		script:      script,
		timers:      make(map[int64]*timer),
		timerQueue:  make(timerHeap, 0),
		nextTimerID: 1,
	}
	r.initAPI()
	return r
}

func (r *Runtime) initAPI() {
	r.vm.Set("setTimeout", r.setTimeout)
	r.vm.Set("clearTimeout", r.clearTimeout)
	r.vm.Set("setInterval", r.setInterval)
	r.vm.Set("clearInterval", r.clearInterval)

	// Ensure console is available (basic polyfill if needed, though goja usually doesn't have it by default)
	// User didn't ask for console, but it's useful for debugging.
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
		if len(r.timers) > 0 {
			return true, nil
		}
		// If script finished and no timers, we are done.
		return false, nil
	}

	// Process timers
	now := time.Now()
	executed := 0

	// We check the heap.
	for len(r.timerQueue) > 0 {
		t := r.timerQueue[0]
		if !now.After(t.deadline) && !now.Equal(t.deadline) {
			break
		}

		// Timer is due
		heap.Pop(&r.timerQueue)
		executed++

		// Execute callback
		_, err := t.callback(goja.Undefined(), t.args...)
		if err != nil {
			// If interrupted by context, return context error
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			return false, err
		}

		// If interval, reschedule only if it hasn't been cleared
		if t.interval > 0 {
			if _, exists := r.timers[t.id]; exists {
				t.deadline = now.Add(t.interval)
				heap.Push(&r.timerQueue, t)
			}
		} else {
			delete(r.timers, t.id)
		}
	}

	// Check if we have more work
	if len(r.timers) > 0 {
		return true, nil
	}

	// If we executed something, maybe there are side effects (jobs) that were run.
	// But if no timers left, we are done.
	return false, nil
}
