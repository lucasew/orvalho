package workers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lucasew/orvalho/pkg/actor"
	"github.com/lucasew/orvalho/pkg/wasm"

	"github.com/dop251/goja"
	"github.com/dop251/goja/parser"
)

// Isolate is one pure-goja VM with host-driven timers and minimal WinterTC
// web types (Headers, Request, Response). Outbound fetch is optional DI.
// Create with [New]; advance with [Isolate.Fetch] (request path) or
// [Isolate.Tick] (tests / optional pulse). Idle isolates are frozen.
type Isolate struct {
	vm          *goja.Runtime
	script      string
	initialized bool
	opts        Options
	timers      *timerTable
	// now is time.Now by default; tests may override.
	now func() time.Time
	mu  sync.Mutex

	// Registries map JS objects to Go-side state for web types.
	headersReg  map[*goja.Object]*headerBag
	requestReg  map[*goja.Object]*requestBag
	responseReg map[*goja.Object]*responseBag

	// activeCtx is the context of the current host Fetch/Tick; outbound
	// fetch inherits it for cancellation.
	activeCtx context.Context

	// moduleCache is the per-isolate require cache (specifier → exports).
	moduleCache map[string]goja.Value
	// loading is the CJS module object for scripts still evaluating, so
	// circular require sees the live module.exports (esbuild reassigns it).
	loading map[string]*goja.Object

	// importFrom is the FS path of the script currently evaluating.
	importFrom string

	// scriptCause is the first require/load error during ScriptMain.
	// Guest code may catch it and process.exit(1); we still report this.
	scriptCause error
	// scriptRejected is the last unhandled rejection during ScriptMain.
	scriptRejected error

	// cwd is the injected process.cwd(); chdir updates only this.
	cwd string

	// httpCh delivers accepted HTTP requests to the event loop.
	httpCh chan *httpJob
	// wake unblocks waitForWorkLocked when a completion is not its own channel.
	wake chan struct{}
	// listeners is how many guest servers are currently listening.
	listeners int
	// inFlight is HTTP/upgrade jobs that have not finished (res.end / upgrade return).
	inFlight int
	// httpQ is accepted requests waiting to be dispatched from runLoop.
	httpQ []*httpJob
	// pluginCh is the isolate-thread job queue (esbuild onLoad/onResolve,
	// socket data). goja is not safe on the worker/read goroutines.
	pluginCh chan func()
	// esbuildBusy is isolate-thread builds waiting off-thread (so the
	// loop does not go idle before onLoad callbacks arrive).
	esbuildBusy int

	wasm       *wasm.Hub
	wasmMods   map[*goja.Object]*wasm.Bin
	wasmSeq    uint64
	wasmActive *wasmInstance
	wasmGo     *wasmGoJS
	// esmLexer is the guest es-module-lexer wasm instance (parse/sa/ri).
	esmLexer *wasmInstance
}

// Ensure Isolate implements actor.Actor.
var _ actor.Actor = (*Isolate)(nil)

func (iso *Isolate) trace(format string, args ...any) {
	if iso == nil || iso.opts.Trace == nil {
		return
	}
	iso.opts.Trace(format, args...)
}

// New creates an isolate for script. The script is not executed until the
// first Tick. Zero-valued opts fields use the documented defaults.
func New(script string, opts Options) *Isolate {
	vm := goja.New()
	// Guest files often keep //# sourceMappingURL; goja's default
	// loader reads the host FS and fails the parse when the map is
	// absent. Maps are not a Binding.
	vm.SetParserOptions(parser.WithDisableSourceMaps)
	iso := &Isolate{
		vm:       vm,
		script:   script,
		opts:     opts.withDefaults(),
		timers:   newTimerTable(),
		now:      time.Now,
		httpCh:   make(chan *httpJob, 16),
		wake:     make(chan struct{}, 1),
		pluginCh: make(chan func(), 32),
	}
	iso.installTimers()
	iso.installWebTypes()
	iso.installOutboundFetch()
	iso.installHostPolyfills()
	iso.installModules()
	return iso
}

func (iso *Isolate) installTimers() {
	iso.vm.Set("setTimeout", iso.jsSetTimeout)
	iso.vm.Set("clearTimeout", iso.jsClearTimeout)
	iso.vm.Set("setInterval", iso.jsSetInterval)
	iso.vm.Set("clearInterval", iso.jsClearInterval)
	iso.vm.Set("setImmediate", iso.jsSetImmediate)
	iso.vm.Set("clearImmediate", iso.jsClearTimeout)
}

// Tick runs one host-controlled step: first-time script evaluation, then up to
// MaxTimersPerTick due timer callbacks. Returns more=true when timers remain.
// ctx cancellation interrupts the VM and stops further work.
func (iso *Isolate) Tick(ctx context.Context) (bool, error) {
	iso.mu.Lock()
	defer iso.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return false, err
	}
	defer iso.pushCtx(ctx)()

	stopWatch := iso.watchInterrupt(ctx)
	defer stopWatch()

	if !iso.initialized {
		iso.initialized = true
		_, err := iso.vm.RunString(iso.script)
		if err != nil {
			return false, mapJSError(ctx, err)
		}
		iso.bindConsole()
		return iso.timers.len() > 0, nil
	}

	now := iso.now()
	executed := 0
	for executed < iso.opts.MaxTimersPerTick {
		if err := ctx.Err(); err != nil {
			return false, err
		}

		t := iso.timers.popDue(now)
		if t == nil {
			break
		}
		executed++

		_, err := t.callback(goja.Undefined(), t.args...)
		if err != nil {
			return false, mapJSError(ctx, err)
		}

		if t.interval > 0 {
			iso.timers.rescheduleInterval(t, now)
		}
	}

	return iso.timers.len() > 0, nil
}

// PendingTimers reports how many timers are currently scheduled.
func (iso *Isolate) PendingTimers() int {
	iso.mu.Lock()
	defer iso.mu.Unlock()
	return iso.timers.len()
}

func (iso *Isolate) jsSetTimeout(call goja.FunctionCall) goja.Value {
	return iso.scheduleFromJS(call, false)
}

func (iso *Isolate) jsSetImmediate(call goja.FunctionCall) goja.Value {
	// Node setImmediate(fn[, ...args]) has no delay slot.
	args := []goja.Value{call.Argument(0), iso.vm.ToValue(0)}
	if len(call.Arguments) > 1 {
		args = append(args, call.Arguments[1:]...)
	}
	return iso.scheduleFromJS(goja.FunctionCall{This: call.This, Arguments: args}, false)
}

func (iso *Isolate) jsSetInterval(call goja.FunctionCall) goja.Value {
	return iso.scheduleFromJS(call, true)
}

func (iso *Isolate) jsClearTimeout(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 {
		return goja.Undefined()
	}
	iso.timers.cancel(call.Argument(0).ToInteger())
	return goja.Undefined()
}

func (iso *Isolate) jsClearInterval(call goja.FunctionCall) goja.Value {
	return iso.jsClearTimeout(call)
}

func (iso *Isolate) scheduleFromJS(call goja.FunctionCall, repeating bool) goja.Value {
	fn, ok := goja.AssertFunction(call.Argument(0))
	if !ok {
		panic(iso.vm.NewTypeError("callback must be a function"))
	}

	if iso.timers.len() >= iso.opts.MaxPendingTimers {
		// Prefer a JS-visible throw so guest code can catch abuse-limit errors.
		panic(iso.vm.NewTypeError(
			fmt.Sprintf("too many pending timers (max %d)", iso.opts.MaxPendingTimers),
		))
	}

	delayMs := int64(0)
	if len(call.Arguments) > 1 {
		delayMs = call.Argument(1).ToInteger()
	}
	if delayMs < 0 {
		delayMs = 0
	}
	delay := time.Duration(delayMs) * time.Millisecond

	var args []goja.Value
	if len(call.Arguments) > 2 {
		args = call.Arguments[2:]
	}

	interval := time.Duration(0)
	if repeating {
		interval = delay
	}

	id := iso.timers.schedule(fn, args, delay, interval, iso.now())
	return iso.vm.ToValue(id)
}

// watchInterrupt clears any prior VM interrupt and starts a goroutine that
// interrupts the VM when ctx is done. Call the returned stop func (typically
// via defer) to end the watcher.
func (iso *Isolate) pushCtx(ctx context.Context) func() {
	prev := iso.activeCtx
	iso.activeCtx = ctx
	return func() { iso.activeCtx = prev }
}

func (iso *Isolate) watchInterrupt(ctx context.Context) (stop func()) {
	iso.vm.ClearInterrupt()
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			iso.vm.Interrupt(ctx.Err())
		case <-done:
		}
	}()
	return func() { close(done) }
}

func mapJSError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	// Prefer the host context error when the VM was interrupted because of it.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if ie, ok := err.(*goja.InterruptedError); ok {
		if v, ok := ie.Value().(error); ok {
			return v
		}
	}
	return err
}
