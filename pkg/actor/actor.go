// Package actor defines the host-facing step API for guest isolates.
package actor

import "context"

// Actor is a host-driven unit of work. The host calls Tick; guests never own
// a free-running event-loop thread that can freeze the process.
type Actor interface {
	// Tick executes one step of the actor's logic.
	// more is true when there is still pending work (for example active timers).
	// more is false when the actor is idle with no scheduled work.
	// err is non-nil when execution failed or ctx was canceled.
	Tick(ctx context.Context) (more bool, err error)
}
