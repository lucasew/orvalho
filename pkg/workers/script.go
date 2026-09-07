package workers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
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

	iso.installProcess()

	var rejected error
	iso.vm.SetPromiseRejectionTracker(func(p *goja.Promise, op goja.PromiseRejectionOperation) {
		if op != goja.PromiseRejectionReject || p == nil {
			return
		}
		res := p.Result()
		if res != nil {
			if ex := scriptExitOf(errorOf(res.Export())); ex != nil {
				rejected = ex
				return
			}
		}
		rejected = fmt.Errorf("%w: %v", ErrScriptThrow, res)
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
			if rejected != nil {
				if ex := scriptExitOf(rejected); ex != nil {
					if ex.Code == 0 {
						return nil
					}
					return ex
				}
				return rejected
			}
			return nil
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

func (iso *Isolate) installProcess() {
	argv := iso.opts.Argv
	if len(argv) == 0 {
		argv = []string{"orvalho"}
	}
	p := iso.vm.NewObject()
	mustSet(p, "argv", argv)
	mustSet(p, "platform", runtime.GOOS)
	mustSet(p, "arch", runtime.GOARCH)
	mustSet(p, "title", "orvalho")
	mustSet(p, "pid", os.Getpid())
	versions := iso.vm.NewObject()
	mustSet(versions, "node", "24.0.0")
	mustSet(p, "versions", versions)
	env := iso.vm.NewObject()
	for _, e := range os.Environ() {
		k, v, ok := strings.Cut(e, "=")
		if ok && k != "" {
			mustSet(env, k, v)
		}
	}
	mustSet(p, "env", env)
	mustSet(p, "cwd", func() (string, error) { return os.Getwd() })
	mustSet(p, "chdir", func(dir string) error { return os.Chdir(dir) })
	mustSet(p, "exit", iso.jsProcessExit)
	mustSet(p, "nextTick", iso.jsNextTick)
	mustRuntimeSet(iso.vm, "process", p)
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
