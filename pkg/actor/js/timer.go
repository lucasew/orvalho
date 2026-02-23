package js

import (
	"container/heap"
	"time"

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
