package actor

import "context"

// Actor defines the interface for a step-based actor.
type Actor interface {
	// Tick executes one step of the actor's logic.
	// It returns 'true' if the actor has more work to do (e.g., active timers, pending callbacks).
	// It returns 'false' if the actor is idle/finished.
	// It returns error if the execution failed.
	Tick(ctx context.Context) (bool, error)
}
