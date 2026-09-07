package workers

import (
	"context"
	"fmt"
	"time"
)

// ScriptMain evaluates source as CommonJS main (TEC-09). default.fetch is
// not required. file is the path inside the import tree, used as require
// parent. Pending timers are drained until none remain or ctx is cancelled.
func (iso *Isolate) ScriptMain(ctx context.Context, source, file string) error {
	iso.mu.Lock()
	defer iso.mu.Unlock()

	if err := iso.ensureInitializedLocked(ctx); err != nil {
		return iso.wrapScriptError(ctx, err)
	}

	iso.activeCtx = ctx
	defer func() { iso.activeCtx = nil }()

	stopWatch := iso.watchInterrupt(ctx)
	defer stopWatch()

	key := file
	if key == "" {
		key = "."
	}
	if _, err := iso.loadScript(key, source, file); err != nil {
		return iso.wrapScriptError(ctx, err)
	}

	for {
		if err := iso.drainOneTickLocked(ctx); err != nil {
			return iso.wrapScriptError(ctx, err)
		}
		deadline, ok := iso.timers.nextDeadline()
		if !ok {
			return nil
		}
		wait := deadline.Sub(iso.now())
		if wait <= 0 {
			continue
		}
		if err := iso.waitForTimerLocked(ctx, wait); err != nil {
			return err
		}
	}
}

func (iso *Isolate) waitForTimerLocked(ctx context.Context, wait time.Duration) error {
	iso.mu.Unlock()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		iso.mu.Lock()
		return ctx.Err()
	case <-timer.C:
		iso.mu.Lock()
		return nil
	}
}

func (iso *Isolate) wrapScriptError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	err = mapJSError(ctx, err)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return fmt.Errorf("%w: %v", ErrScriptThrow, err)
}
