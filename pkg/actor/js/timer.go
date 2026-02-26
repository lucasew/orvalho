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

// TimerManager manages scheduling and cancellation of timers.
type TimerManager struct {
	timers     map[int64]*timer
	timerQueue timerHeap
	nextTimerID int64
}

// NewTimerManager creates a new TimerManager.
func NewTimerManager() *TimerManager {
	return &TimerManager{
		timers:      make(map[int64]*timer),
		timerQueue:  make(timerHeap, 0),
		nextTimerID: 1,
	}
}

// Schedule adds a new timer.
func (tm *TimerManager) Schedule(callback goja.Callable, args []goja.Value, delay time.Duration, interval time.Duration) int64 {
	t := &timer{
		id:       tm.nextTimerID,
		deadline: time.Now().Add(delay),
		callback: callback,
		args:     args,
		interval: interval,
	}
	tm.nextTimerID++
	tm.timers[t.id] = t
	heap.Push(&tm.timerQueue, t)
	return t.id
}

// Cancel removes a timer by ID.
func (tm *TimerManager) Cancel(id int64) {
	if t, ok := tm.timers[id]; ok {
		if t.index != -1 {
			heap.Remove(&tm.timerQueue, t.index)
		}
		delete(tm.timers, id)
	}
}

// PopExpired returns the next timer if it's due.
// If repeating, caller is responsible for calling Reschedule.
func (tm *TimerManager) PopExpired(now time.Time) *timer {
	if len(tm.timerQueue) == 0 {
		return nil
	}
	t := tm.timerQueue[0]
	if !now.After(t.deadline) && !now.Equal(t.deadline) {
		return nil
	}
	heap.Pop(&tm.timerQueue)
	// If it's one-shot, remove from map. If repeating, keep it or handle in Reschedule.
	// Logic in Runtime was: execute, then if interval, reschedule.
	// If reschedule, we need it in map? No, map is for cancellation.
	// If one-shot, we should remove from map BEFORE execution or AFTER?
	// If executed, maybe remove from map?
	// In Runtime: "Pop" happens. "Execute". "Reschedule or Delete".
	// If I pop here, it's removed from heap. It's still in map?
	// If canceled during execution?
	// If I pop, heap integrity is maintained. Map still has it.
	// If I don't remove from map, Cancel(id) might try to remove from heap.
	// t.index is -1 after Pop. So Cancel checks index != -1. Safe.

	// If it's not repeating, we should remove from map eventually.
	// But let's let caller decide or handle it here.
	// If I return it, caller executes it.
	// If caller doesn't reschedule, we should ensure it's gone from map?
	// Let's remove from map if interval is 0.
	if t.interval == 0 {
		delete(tm.timers, t.id)
	}
	// If interval > 0, we keep it in map because it might be rescheduled.

	return t
}

// Reschedule updates a repeating timer's deadline and pushes it back to heap.
func (tm *TimerManager) Reschedule(t *timer, now time.Time) {
	if t.interval > 0 {
		// Only if it's still in map (not canceled during execution)
		if _, exists := tm.timers[t.id]; exists {
			t.deadline = now.Add(t.interval)
			heap.Push(&tm.timerQueue, t)
		}
	}
}

// Len returns the number of active timers.
func (tm *TimerManager) Len() int {
	return len(tm.timers)
}
