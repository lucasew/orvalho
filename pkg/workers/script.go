package workers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/dop251/goja"
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

	defer iso.pushCtx(ctx)()

	stopWatch := iso.watchInterrupt(ctx)
	defer stopWatch()

	iso.scriptCause = nil
	iso.installProcess()
	iso.installBuffer()

	var rejected error
	iso.vm.SetPromiseRejectionTracker(func(p *goja.Promise, op goja.PromiseRejectionOperation) {
		if op != goja.PromiseRejectionReject || p == nil {
			return
		}
		err := rejectionError(p.Result())
		if err == nil {
			return
		}
		if scriptExitOf(err) != nil {
			if rejected == nil {
				rejected = err
			}
			return
		}
		iso.noteScriptCause(err)
		if rejected == nil || scriptExitOf(rejected) != nil {
			rejected = err
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

	idle := 0
	for {
		iso.pollHTTP()
		iso.pollPlugins()
		if err := iso.drainOneTickLocked(ctx); err != nil {
			return iso.wrapScriptError(ctx, err)
		}
		deadline, hasTimer := iso.timers.nextDeadline()
		listening := iso.listeners > 0
		busy := iso.inFlight > 0 || iso.esbuildBusy > 0
		if !hasTimer && !listening && !busy {
			idle++
			if idle >= 64 {
				return iso.finishScript(rejected)
			}
			continue
		}
		idle = 0
		var wait time.Duration
		switch {
		case hasTimer:
			wait = deadline.Sub(iso.now())
			if wait <= 0 {
				continue
			}
		case busy:
			wait = scriptIdlePoll
		}
		if err := iso.waitForWorkLocked(ctx, wait); err != nil {
			return err
		}
	}
}

func (iso *Isolate) waitForWorkLocked(ctx context.Context, wait time.Duration) error {
	iso.mu.Unlock()
	var timerC <-chan time.Time
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		timerC = timer.C
	}
	if iso.dispatching > 0 {
		select {
		case <-ctx.Done():
			iso.mu.Lock()
			return ctx.Err()
		case <-timerC:
			iso.mu.Lock()
			return nil
		case <-iso.wake:
			iso.mu.Lock()
			return nil
		case fn := <-iso.pluginCh:
			iso.mu.Lock()
			fn()
			return nil
		}
	}
	select {
	case <-ctx.Done():
		iso.mu.Lock()
		return ctx.Err()
	case <-timerC:
		iso.mu.Lock()
		return nil
	case <-iso.wake:
		iso.mu.Lock()
		return nil
	case fn := <-iso.pluginCh:
		iso.mu.Lock()
		fn()
		return nil
	case job := <-iso.httpCh:
		iso.mu.Lock()
		iso.dispatchHTTP(job)
		return nil
	}
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

func (iso *Isolate) pollPlugins() {
	if iso == nil || iso.pluginCh == nil {
		return
	}
	for {
		select {
		case fn := <-iso.pluginCh:
			fn()
		default:
			return
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

func (iso *Isolate) pollHTTP() {
	if iso.dispatching > 0 {
		return
	}
	for {
		select {
		case job := <-iso.httpCh:
			iso.dispatchHTTP(job)
		default:
			return
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
