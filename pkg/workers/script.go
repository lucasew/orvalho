package workers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// ScriptMain evaluates source as CommonJS main (TEC-09) then runs the
// event loop until idle or ctx is cancelled. Hosts that want to tick
// themselves should call ScriptStart and then Pump / PumpUntil.
func (iso *Isolate) ScriptMain(ctx context.Context, source, file string) error {
	iso.mu.Lock()
	defer iso.mu.Unlock()

	if err := iso.scriptStartLocked(ctx, source, file); err != nil {
		return err
	}
	if err := iso.runLoop(ctx); err != nil {
		return iso.wrapScriptError(ctx, err)
	}
	return iso.finishScript(iso.scriptRejected)
}

// ScriptStart loads CommonJS main and returns. JS does not run again
// until the host calls Pump or ScriptMain's loop.
func (iso *Isolate) ScriptStart(ctx context.Context, source, file string) error {
	iso.mu.Lock()
	defer iso.mu.Unlock()
	return iso.scriptStartLocked(ctx, source, file)
}

func (iso *Isolate) scriptStartLocked(ctx context.Context, source, file string) error {
	if err := iso.ensureInitializedLocked(ctx); err != nil {
		return iso.wrapScriptError(ctx, err)
	}

	defer iso.pushCtx(ctx)()

	stopWatch := iso.watchInterrupt(ctx)
	defer stopWatch()

	iso.scriptCause = nil
	iso.scriptRejected = nil
	iso.installProcess()
	iso.installBuffer()

	iso.vm.SetPromiseRejectionTracker(func(p *goja.Promise, op goja.PromiseRejectionOperation) {
		if op != goja.PromiseRejectionReject || p == nil {
			return
		}
		err := rejectionError(p.Result())
		if err == nil {
			return
		}
		if scriptExitOf(err) != nil {
			if iso.scriptRejected == nil {
				iso.scriptRejected = err
			}
			return
		}
		iso.noteScriptCause(err)
		if iso.scriptRejected == nil || scriptExitOf(iso.scriptRejected) != nil {
			iso.scriptRejected = err
		}
	})

	key := file
	if key == "" {
		key = "."
	}
	iso.trace("script main %s %dB", file, len(source))
	if _, err := iso.loadScript(key, source, file); err != nil {
		return iso.wrapScriptError(ctx, err)
	}
	return nil
}

// runLoop is the isolate event loop. goja_nodejs/eventloop is Start/Run
// until idle and owns an unexported VM — it is not a tick budget. We
// pump ready work, then wait on I/O the same way (job channel + wakeup).
func (iso *Isolate) runLoop(ctx context.Context) error {
	// scriptStartLocked restores activeCtx; wasm/I/O Bindings in timer
	// and plugin callbacks still need the host context (wazero panics on nil).
	defer iso.pushCtx(ctx)()
	iso.trace("loop start")
	for {
		more, _, err := iso.pumpLocked(ctx)
		if err != nil {
			iso.trace("loop stop err=%v", err)
			return err
		}
		if !more {
			iso.trace("loop idle")
			return nil
		}
		deadline, hasTimer := iso.timers.nextDeadline()
		var wait time.Duration
		if hasTimer {
			wait = deadline.Sub(iso.now())
			if wait <= 0 {
				continue
			}
		}
		if err := iso.waitForWorkLocked(ctx, wait); err != nil {
			iso.trace("loop stop err=%v", err)
			return err
		}
	}
}

// Pump runs ready work (queued HTTP, isolate jobs, due timers) and
// returns. It never waits on I/O. used is wall time spent in this call.
// more is true if timers, listeners, or in-flight work remain.
func (iso *Isolate) Pump(ctx context.Context) (more bool, used time.Duration, err error) {
	iso.mu.Lock()
	defer iso.mu.Unlock()
	return iso.pumpLocked(ctx)
}

// PumpUntil runs Pump until budget elapses, ctx is done, or the isolate
// has no ready work. A tight guest loop cannot hold other isolates:
// each Isolate has its own lock and VM.
func (iso *Isolate) PumpUntil(ctx context.Context, budget time.Duration) (more bool, used time.Duration, err error) {
	iso.mu.Lock()
	defer iso.mu.Unlock()
	if budget <= 0 {
		return iso.pumpLocked(ctx)
	}
	deadline := iso.now().Add(budget)
	tickCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	defer iso.pushCtx(tickCtx)()
	stop := iso.watchInterrupt(tickCtx)
	defer stop()
	var total time.Duration
	for !iso.now().After(deadline) {
		var step time.Duration
		more, step, err = iso.pumpLocked(tickCtx)
		total += step
		if err != nil {
			if tickCtx.Err() != nil && ctx.Err() == nil {
				_, hasTimer := iso.timers.nextDeadline()
				return hasTimer || iso.loopHeld(), total, nil
			}
			return more, total, err
		}
		if !more {
			return false, total, nil
		}
		if d, ok := iso.timers.nextDeadline(); !ok || d.After(iso.now()) {
			if !iso.hasReadyWork() {
				return more, total, nil
			}
		}
	}
	_, hasTimer := iso.timers.nextDeadline()
	return hasTimer || iso.loopHeld(), total, nil
}

func (iso *Isolate) pumpLocked(ctx context.Context) (bool, time.Duration, error) {
	defer iso.pushCtx(ctx)()
	t0 := iso.now()
	iso.loopN++
	n := iso.loopN
	if err := ctx.Err(); err != nil {
		_, hasTimer := iso.timers.nextDeadline()
		return hasTimer || iso.loopHeld(), 0, err
	}
	httpN := iso.pollHTTP()
	jobN := iso.pollPlugins()
	timerN, err := iso.drainOneTickLocked(ctx)
	used := iso.now().Sub(t0)
	if httpN > 0 || jobN > 0 || timerN > 0 || err != nil {
		iso.trace("loop #%d pump http=%d jobs=%d timers=%d hold=%s used=%s",
			n, httpN, jobN, timerN, iso.loopHoldLabel(), used.Round(time.Millisecond))
	}
	if err != nil {
		return false, used, err
	}
	_, hasTimer := iso.timers.nextDeadline()
	return hasTimer || iso.loopHeld(), used, nil
}

func (iso *Isolate) hasReadyWork() bool {
	if len(iso.httpQ) > 0 || len(iso.httpCh) > 0 || len(iso.pluginCh) > 0 {
		return true
	}
	if d, ok := iso.timers.nextDeadline(); ok && !d.After(iso.now()) {
		return true
	}
	return false
}

func (iso *Isolate) loopHeld() bool {
	return iso.listeners > 0 || iso.inFlight > 0 || iso.esbuildBusy > 0 || len(iso.httpQ) > 0
}

func (iso *Isolate) loopHoldLabel() string {
	var p []string
	if iso.listeners > 0 {
		p = append(p, fmt.Sprintf("listen=%d", iso.listeners))
	}
	if iso.inFlight > 0 {
		p = append(p, fmt.Sprintf("inFlight=%d", iso.inFlight))
	}
	if iso.esbuildBusy > 0 {
		p = append(p, fmt.Sprintf("esbuild=%d", iso.esbuildBusy))
	}
	if len(iso.httpQ) > 0 {
		p = append(p, fmt.Sprintf("httpQ=%d", len(iso.httpQ)))
	}
	if _, ok := iso.timers.nextDeadline(); ok {
		p = append(p, "timer")
	}
	if len(p) == 0 {
		return "-"
	}
	return strings.Join(p, ",")
}

func (iso *Isolate) waitForWorkLocked(ctx context.Context, wait time.Duration) error {
	if wait > 0 {
		iso.trace("loop #%d wait timer=%s hold=%s", iso.loopN, wait.Round(time.Millisecond), iso.loopHoldLabel())
	} else {
		iso.trace("loop #%d wait io hold=%s", iso.loopN, iso.loopHoldLabel())
	}
	iso.mu.Unlock()
	var timerC <-chan time.Time
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		timerC = timer.C
	}
	var why string
	var err error
	select {
	case <-ctx.Done():
		why = "ctx"
		err = ctx.Err()
	case <-timerC:
		why = "timer"
	case <-iso.wake:
		why = "kick"
	case fn := <-iso.pluginCh:
		iso.mu.Lock()
		iso.trace("loop #%d wake job hold=%s", iso.loopN, iso.loopHoldLabel())
		fn()
		return nil
	case job := <-iso.httpCh:
		iso.mu.Lock()
		iso.httpQ = append(iso.httpQ, job)
		iso.trace("loop #%d wake http hold=%s", iso.loopN, iso.loopHoldLabel())
		return nil
	}
	iso.mu.Lock()
	iso.trace("loop #%d wake %s hold=%s", iso.loopN, why, iso.loopHoldLabel())
	return err
}

func (iso *Isolate) kick() {
	if iso == nil || iso.wake == nil {
		return
	}
	select {
	case iso.wake <- struct{}{}:
	default:
	}
}

// postJob queues fn on the isolate thread and wakes the loop. It does
// not wait. I/O completions use this; runOnIsolate waits for a result.
func (iso *Isolate) postJob(fn func()) {
	if iso == nil || fn == nil || iso.pluginCh == nil {
		return
	}
	iso.pluginCh <- fn
	iso.kick()
}

func (iso *Isolate) pollPlugins() int {
	if iso == nil || iso.pluginCh == nil {
		return 0
	}
	n := 0
	for {
		select {
		case fn := <-iso.pluginCh:
			n++
			fn()
		default:
			return n
		}
	}
}

func (iso *Isolate) runOnIsolate(fn func()) {
	if iso == nil || fn == nil {
		return
	}
	if iso.pluginCh == nil {
		fn()
		return
	}
	done := make(chan struct{})
	iso.pluginCh <- func() {
		fn()
		close(done)
	}
	iso.kick()
	<-done
}

func (iso *Isolate) pollHTTP() int {
	n := 0
	for {
		select {
		case job := <-iso.httpCh:
			iso.httpQ = append(iso.httpQ, job)
		default:
			for len(iso.httpQ) > 0 {
				job := iso.httpQ[0]
				iso.httpQ = iso.httpQ[1:]
				iso.dispatchHTTP(job)
				n++
			}
			return n
		}
	}
}

func (iso *Isolate) finishScript(rejected error) error {
	if rejected == nil {
		return nil
	}
	if iso.scriptCause != nil {
		if ex := scriptExitOf(rejected); ex != nil && ex.Code == 0 {
			return nil
		}
		return iso.scriptCause
	}
	if ex := scriptExitOf(rejected); ex != nil {
		if ex.Code == 0 {
			return nil
		}
		return ex
	}
	return fmt.Errorf("%w: %v", ErrScriptThrow, rejected)
}

func rejectionError(v goja.Value) error {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	if err := errorOf(v.Export()); err != nil {
		return err
	}
	return fmt.Errorf("%s", v.String())
}

func (iso *Isolate) wrapScriptError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	err = mapJSError(ctx, err)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if ex := scriptExitOf(err); ex != nil {
		if ex.Code == 0 {
			return nil
		}
		return ex
	}
	return fmt.Errorf("%w: %v", ErrScriptThrow, err)
}

func scriptExitOf(err error) *ScriptExitError {
	var ex *ScriptExitError
	if errors.As(err, &ex) {
		return ex
	}
	var ge *goja.Exception
	if errors.As(err, &ge) && ge != nil {
		if v := ge.Value(); v != nil {
			if exported := v.Export(); exported != nil {
				if errors.As(errorOf(exported), &ex) {
					return ex
				}
			}
		}
	}
	return nil
}

func errorOf(v any) error {
	if err, ok := v.(error); ok {
		return err
	}
	return nil
}

func (iso *Isolate) installBuffer() {
	v, err := iso.loadModule("node:buffer")
	if err != nil {
		return
	}
	o, ok := v.(*goja.Object)
	if !ok {
		return
	}
	if b := o.Get("Buffer"); b != nil && !goja.IsUndefined(b) {
		mustRuntimeSet(iso.vm, "Buffer", b)
	}
}

func (iso *Isolate) installProcess() {
	argv := iso.opts.Argv
	if len(argv) == 0 {
		argv = []string{"orvalho"}
	}
	platform := iso.opts.Platform
	if platform == "" {
		platform = "wasi"
	}
	arch := iso.opts.Arch
	if arch == "" {
		arch = "wasm32"
	}
	iso.cwd = iso.opts.Cwd
	if iso.cwd == "" {
		iso.cwd = "."
	}
	p := iso.vm.NewObject()
	attachEmitter(p)
	mustSet(p, "argv", jsStrings(iso.vm, argv))
	mustSet(p, "execArgv", jsStrings(iso.vm, nil))
	mustSet(p, "version", "v24.0.0")
	mustSet(p, "platform", platform)
	mustSet(p, "arch", arch)
	mustSet(p, "title", "orvalho")
	mustSet(p, "pid", iso.opts.PID)
	execPath := iso.opts.ExecPath
	if execPath == "" {
		execPath = "orvalho"
	}
	mustSet(p, "execPath", execPath)
	versions := iso.vm.NewObject()
	mustSet(versions, "node", "24.0.0")
	mustSet(p, "versions", versions)
	env := iso.vm.NewObject()
	for k, v := range iso.opts.ProcessEnv {
		if k != "" {
			mustSet(env, k, v)
		}
	}
	mustSet(p, "env", env)
	mustSet(p, "cwd", iso.jsProcessCwd)
	mustSet(p, "chdir", iso.jsProcessChdir)
	mustSet(p, "exit", iso.jsProcessExit)
	mustSet(p, "nextTick", iso.jsNextTick)
	mustSet(p, "stdin", iso.newStdio(0))
	mustSet(p, "stdout", iso.newStdio(1))
	mustSet(p, "stderr", iso.newStdio(2))
	mustRuntimeSet(iso.vm, "process", p)
}

func jsStrings(rt *goja.Runtime, xs []string) *goja.Object {
	items := make([]any, len(xs))
	for i, s := range xs {
		items[i] = s
	}
	return rt.NewArray(items...)
}

func (iso *Isolate) writeStdio(fd int, b []byte) int {
	var w *os.File
	switch fd {
	case 1:
		w = os.Stdout
	case 2:
		w = os.Stderr
	default:
		return 0
	}
	n, _ := w.Write(b)
	return n
}

func (iso *Isolate) newStdio(fd int) *goja.Object {
	s := iso.vm.NewObject()
	attachEmitter(s)
	mustSet(s, "fd", fd)
	mustSet(s, "isTTY", false)
	mustSet(s, "getColorDepth", func(goja.FunctionCall) goja.Value {
		return iso.vm.ToValue(1)
	})
	mustSet(s, "hasColors", func(goja.FunctionCall) goja.Value {
		return iso.vm.ToValue(false)
	})
	mustSet(s, "write", func(call goja.FunctionCall) goja.Value {
		iso.writeStdio(fd, valueBytes(call.Argument(0)))
		if cb := lastFunc(call); cb != nil {
			iso.timers.schedule(cb, []goja.Value{goja.Null()}, 0, 0, iso.now())
		}
		return iso.vm.ToValue(true)
	})
	mustSet(s, "end", func(call goja.FunctionCall) goja.Value { return s })
	mustSet(s, "cork", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	mustSet(s, "uncork", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	return s
}

func (iso *Isolate) jsProcessCwd(goja.FunctionCall) string {
	return iso.cwd
}

func (iso *Isolate) jsProcessChdir(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) {
		panic(iso.vm.NewTypeError("The \"directory\" argument must be of type string"))
	}
	iso.cwd = call.Argument(0).String()
	return goja.Undefined()
}

func (iso *Isolate) jsProcessExit(call goja.FunctionCall) goja.Value {
	code := 0
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
		code = int(call.Argument(0).ToInteger())
	}
	panic(iso.vm.NewGoError(&ScriptExitError{Code: code}))
}

func (iso *Isolate) jsNextTick(call goja.FunctionCall) goja.Value {
	fn, ok := goja.AssertFunction(call.Argument(0))
	if !ok {
		panic(iso.vm.NewTypeError("callback must be a function"))
	}
	var args []goja.Value
	if len(call.Arguments) > 1 {
		args = call.Arguments[1:]
	}
	iso.timers.schedule(fn, args, 0, 0, iso.now())
	return goja.Undefined()
}
