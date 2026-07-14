package js

import (
	"container/heap"
	"time"

	"github.com/dop251/goja"
)

// timer is one scheduled setTimeout or setInterval entry.
type timer struct {
	id       int64
	deadline time.Time
	callback goja.Callable
	args     []goja.Value
	interval time.Duration // 0 = one-shot
	index    int           // heap index; -1 when not in the heap
}

type timerHeap []*timer

func (h timerHeap) Len() int           { return len(h) }
func (h timerHeap) Less(i, j int) bool { return h[i].deadline.Before(h[j].deadline) }
func (h timerHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *timerHeap) Push(x any) {
	n := len(*h)
	t := x.(*timer)
	t.index = n
	*h = append(*h, t)
}

func (h *timerHeap) Pop() any {
	old := *h
	n := len(old)
	t := old[n-1]
	old[n-1] = nil
	t.index = -1
	*h = old[:n-1]
	return t
}

// timerTable owns the pending-timer map and deadline heap.
type timerTable struct {
	byID   map[int64]*timer
	queue  timerHeap
	nextID int64
}

func newTimerTable() *timerTable {
	return &timerTable{
		byID:   make(map[int64]*timer),
		queue:  make(timerHeap, 0),
		nextID: 1,
	}
}

func (tt *timerTable) len() int { return len(tt.byID) }

func (tt *timerTable) schedule(cb goja.Callable, args []goja.Value, delay, interval time.Duration, now time.Time) int64 {
	t := &timer{
		id:       tt.nextID,
		deadline: now.Add(delay),
		callback: cb,
		args:     args,
		interval: interval,
	}
	tt.nextID++
	tt.byID[t.id] = t
	heap.Push(&tt.queue, t)
	return t.id
}

func (tt *timerTable) cancel(id int64) {
	t, ok := tt.byID[id]
	if !ok {
		return
	}
	if t.index >= 0 {
		heap.Remove(&tt.queue, t.index)
	}
	delete(tt.byID, id)
}

// peekDue returns the next due timer without removing it, or nil.
func (tt *timerTable) peekDue(now time.Time) *timer {
	if len(tt.queue) == 0 {
		return nil
	}
	t := tt.queue[0]
	if t.deadline.After(now) {
		return nil
	}
	return t
}

// popDue removes and returns the next due timer, or nil if none is due.
// One-shot timers are removed from byID; intervals stay until canceled or
// rescheduled.
func (tt *timerTable) popDue(now time.Time) *timer {
	t := tt.peekDue(now)
	if t == nil {
		return nil
	}
	heap.Pop(&tt.queue)
	if t.interval == 0 {
		delete(tt.byID, t.id)
	}
	return t
}

// rescheduleInterval pushes a repeating timer back if it was not cleared
// during the callback.
func (tt *timerTable) rescheduleInterval(t *timer, now time.Time) {
	if t.interval <= 0 {
		return
	}
	if _, exists := tt.byID[t.id]; !exists {
		return
	}
	t.deadline = now.Add(t.interval)
	heap.Push(&tt.queue, t)
}
