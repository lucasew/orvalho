package workers

import (
	"context"
	"net/http"
	"time"
)

// Server returns an *http.Server that serves iso via [Handler].
// The host still owns ListenAndServe / Serve / Shutdown.
func Server(iso *Isolate, addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           Handler(iso),
		ReadHeaderTimeout: 10 * time.Second,
	}
}

// Run advances the isolate on a fixed interval until ctx is cancelled.
// This is an optional pulse for hosts that want background timer progress
// between requests; the default embed model is freeze-between-requests
// (progress only inside Fetch). Tick still takes the isolate lock.
func (iso *Isolate) Run(ctx context.Context, every time.Duration) error {
	if every <= 0 {
		every = 10 * time.Millisecond
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if _, err := iso.Tick(ctx); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return err
			}
		}
	}
}
