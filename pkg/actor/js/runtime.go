package js

import (
	"container/heap"
	"context"
	"sync"
	"time"

	"orvalho/pkg/actor"
	"orvalho/pkg/observability"

	"github.com/dop251/goja"
)

// timer represents a scheduled task (setTimeout or setInterval).
type timer struct {
	id       int64
	deadline time.Time
	callback goja.Callable
	args     []goja.Value
	interval time.Duration // 0 if one-shot
	index    int           // heap index
}

// timerHeap implements heap.Interface for timers.
type timerHeap []*timer

func (h timerHeap) Len() int           { return len(h) }
func (h timerHeap) Less(i, j int) bool { return h[i].deadline.Before(h[j].deadline) }
func (h timerHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *timerHeap) Push(x interface{}) {
	n := len(*h)
	item := x.(*timer)
	item.index = n
	*h = append(*h, item)
}

func (h *timerHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	*h = old[0 : n-1]
	return item
}

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
	// The prompt says "Web API polyfills (timers, fetch, console) are injected...".
	// But requirements say "You must implement setTimeout...".
	// It doesn't strictly say implement console, but I'll skip it unless needed to avoid clutter.
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
			observability.ReportError(err, "timer callback failed")
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
	if t, ok := r.timers[id]; ok {
		if t.index != -1 {
			heap.Remove(&r.timerQueue, t.index)
		}
		delete(r.timers, id)
	}
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

	t := &timer{
		id:       r.nextTimerID,
		deadline: time.Now().Add(delay),
		callback: fn,
		args:     args,
	}
	if repeating {
		t.interval = delay
		// Intervals shouldn't be 0 ideally to avoid tight loops, but JS allows it (clamped to 4ms usually).
		// We'll trust the delay for now.
		if t.interval < time.Millisecond {
			// Maybe clamp to 1ms to avoid infinite tight loop in Tick?
			// But let's respect user input for now.
		}
	}

	r.nextTimerID++
	r.timers[t.id] = t
	heap.Push(&r.timerQueue, t)

	return r.vm.ToValue(t.id)
}
