package js

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// Default max wait while a fetch handler Promise is pending and timers advance.
const defaultFetchWait = 30 * time.Second

// PrepareGuestScript rewrites common Workers module syntax so the script can
// run under goja via RunString. Currently: `export default` → `globalThis.default =`.
// Full ESM / esbuild downlevel is a separate pipeline.
func PrepareGuestScript(src string) string {
	// Only the first occurrence; guest code should not re-export after that.
	return strings.Replace(src, "export default", "globalThis.default =", 1)
}

// Fetch invokes the Workers-shaped entry `default.fetch(request, env, ctx)`
// and returns a host Response. The isolate script is evaluated on first use.
//
// env is built from Options.Env (strings) and Options.Bindings (host drivers).
// Returned Promises are awaited; pending work is advanced via Tick (timers)
// until the Promise settles or ctx / wait deadline expires.
func (iso *Isolate) Fetch(ctx context.Context, req HTTPRequest) (HTTPResponse, error) {
	iso.mu.Lock()
	defer iso.mu.Unlock()
	return iso.fetchLocked(ctx, req)
}

func (iso *Isolate) fetchLocked(ctx context.Context, req HTTPRequest) (HTTPResponse, error) {
	if err := ctx.Err(); err != nil {
		return HTTPResponse{}, err
	}
	iso.activeCtx = ctx
	defer func() { iso.activeCtx = nil }()

	if err := iso.ensureInitializedLocked(ctx); err != nil {
		return HTTPResponse{}, err
	}

	reqObj, err := iso.makeRequestLocked(req)
	if err != nil {
		return HTTPResponse{}, err
	}

	fetchFn, err := iso.lookupDefaultFetchLocked()
	if err != nil {
		return HTTPResponse{}, err
	}

	env, err := iso.buildEnvLocked()
	if err != nil {
		return HTTPResponse{}, err
	}
	exCtx := iso.vm.NewObject()
	result, err := fetchFn(goja.Undefined(), iso.vm.ToValue(reqObj), env, exCtx)
	if err != nil {
		return HTTPResponse{}, mapJSError(ctx, err)
	}

	result, err = iso.awaitPromiseLocked(ctx, result, defaultFetchWait)
	if err != nil {
		return HTTPResponse{}, err
	}
	return iso.readResponseLocked(result)
}

func (iso *Isolate) ensureInitializedLocked(ctx context.Context) error {
	if iso.initialized {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	iso.vm.ClearInterrupt()
	stopWatch := make(chan struct{})
	defer close(stopWatch)
	go func() {
		select {
		case <-ctx.Done():
			iso.vm.Interrupt(ctx.Err())
		case <-stopWatch:
		}
	}()

	iso.initialized = true
	_, err := iso.vm.RunString(iso.script)
	if err != nil {
		return mapJSError(ctx, err)
	}
	return nil
}

func (iso *Isolate) lookupDefaultFetchLocked() (goja.Callable, error) {
	def := iso.vm.Get("default")
	if def == nil || goja.IsUndefined(def) || goja.IsNull(def) {
		return nil, fmt.Errorf("js: missing global default export (expected default.fetch)")
	}
	obj := def.ToObject(iso.vm)
	fetchVal := obj.Get("fetch")
	fn, ok := goja.AssertFunction(fetchVal)
	if !ok {
		return nil, fmt.Errorf("js: default.fetch is not a function")
	}
	return fn, nil
}

func (iso *Isolate) awaitPromiseLocked(ctx context.Context, v goja.Value, maxWait time.Duration) (goja.Value, error) {
	deadline := iso.now().Add(maxWait)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		p, ok := exportPromise(v)
		if !ok {
			return v, nil
		}
		switch p.State() {
		case goja.PromiseStateFulfilled:
			return p.Result(), nil
		case goja.PromiseStateRejected:
			reason := p.Result()
			if reason != nil && !goja.IsUndefined(reason) {
				return nil, fmt.Errorf("js: fetch rejected: %s", reason.String())
			}
			return nil, fmt.Errorf("js: fetch rejected")
		case goja.PromiseStatePending:
			if !iso.now().Before(deadline) {
				return nil, fmt.Errorf("js: fetch promise timed out after %s", maxWait)
			}
			// Advance host-driven timers; also re-enter the VM so microtasks can run.
			if err := iso.drainOneTickLocked(ctx); err != nil {
				return nil, err
			}
		}
	}
}

func exportPromise(v goja.Value) (*goja.Promise, bool) {
	if v == nil {
		return nil, false
	}
	exported := v.Export()
	p, ok := exported.(*goja.Promise)
	return p, ok
}

// drainOneTickLocked runs due timers for one step without re-taking mu.
// Mirrors Tick's timer phase (script already initialized).
func (iso *Isolate) drainOneTickLocked(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	iso.vm.ClearInterrupt()
	stopWatch := make(chan struct{})
	defer close(stopWatch)
	go func() {
		select {
		case <-ctx.Done():
			iso.vm.Interrupt(ctx.Err())
		case <-stopWatch:
		}
	}()

	now := iso.now()
	executed := 0
	for executed < iso.opts.MaxTimersPerTick {
		if err := ctx.Err(); err != nil {
			return err
		}
		t := iso.timers.popDue(now)
		if t == nil {
			break
		}
		executed++
		_, err := t.callback(goja.Undefined(), t.args...)
		if err != nil {
			return mapJSError(ctx, err)
		}
		if t.interval > 0 {
			iso.timers.rescheduleInterval(t, now)
		}
	}
	// If nothing was due, still poke the runtime so promise jobs can settle.
	if executed == 0 {
		_, err := iso.vm.RunString("")
		if err != nil {
			return mapJSError(ctx, err)
		}
	}
	return nil
}
