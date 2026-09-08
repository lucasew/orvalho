package workers

import (
	"context"
	"errors"
	"fmt"
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

	iso.activeCtx = ctx
	defer func() { iso.activeCtx = nil }()

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
	if _, err := iso.loadScript(key, source, file); err != nil {
		return iso.wrapScriptError(ctx, err)
	}

	idle := 0
	for {
		if err := iso.drainOneTickLocked(ctx); err != nil {
			return iso.wrapScriptError(ctx, err)
		}
		deadline, ok := iso.timers.nextDeadline()
		if ok {
			idle = 0
			wait := deadline.Sub(iso.now())
			if wait <= 0 {
				continue
			}
			if err := iso.waitForTimerLocked(ctx, wait); err != nil {
				return err
			}
			continue
		}
		idle++
		if idle >= 64 {
			return iso.finishScript(rejected)
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

func (iso *Isolate) finishScript(rejected error) error {
	if iso.scriptCause != nil {
		if ex := scriptExitOf(rejected); ex != nil && ex.Code == 0 {
			return nil
		}
		return iso.scriptCause
	}
	if rejected == nil {
		return nil
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
	mustSet(p, "argv", argv)
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
	mustRuntimeSet(iso.vm, "process", p)
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
