package workers

import (
	"context"

	"github.com/dop251/goja"
)

// nodeChildBinding materializes require("child_process").
type nodeChildBinding struct{}

var _ Binding = nodeChildBinding{}

func (nodeChildBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	for _, key := range []string{"child_process", "node:child_process"} {
		if v, ok := iso.moduleCache[key]; ok {
			if o, ok := v.(*goja.Object); ok {
				return o, nil
			}
		}
	}
	return newNodeChild(iso), nil
}

type nodeChild struct {
	iso *Isolate
}

func newNodeChild(iso *Isolate) *goja.Object {
	n := &nodeChild{iso: iso}
	obj := iso.vm.NewObject()
	mustSet(obj, "spawn", n.jsSpawn)
	mustSet(obj, "exec", n.jsExec)
	mustSet(obj, "execFile", n.jsExecFile)
	mustSet(obj, "fork", n.jsFork)
	mustSet(obj, "execSync", n.jsExecSync)
	mustSet(obj, "execFileSync", n.jsExecFileSync)
	mustSet(obj, "spawnSync", n.jsSpawnSync)
	return obj
}

func (n *nodeChild) jsSpawn(call goja.FunctionCall) goja.Value {
	file, args := spawnFileArgs(call)
	if file == "" {
		n.throwArgType("file")
	}
	return n.start(file, args)
}

func (n *nodeChild) jsFork(call goja.FunctionCall) goja.Value {
	mod, args := spawnFileArgs(call)
	if mod == "" {
		n.throwArgType("modulePath")
	}
	execPath := "orvalho"
	if p, ok := n.iso.vm.Get("process").(*goja.Object); ok {
		if v := p.Get("execPath"); v != nil && !goja.IsUndefined(v) {
			execPath = v.String()
		}
	}
	return n.start(execPath, append([]string{mod}, args...))
}

func (n *nodeChild) jsExecFile(call goja.FunctionCall) goja.Value {
	file, args := spawnFileArgs(call)
	if file == "" {
		n.throwArgType("file")
	}
	cb := lastFunc(call)
	child := n.start(file, args).(*goja.Object)
	if cb != nil {
		on := child.Get("on")
		if fn, ok := goja.AssertFunction(on); ok {
			_, _ = fn(child, n.iso.vm.ToValue("exit"), n.iso.vm.ToValue(func(call goja.FunctionCall) goja.Value {
				code := 0
				if len(call.Arguments) > 0 {
					code = int(call.Argument(0).ToInteger())
				}
				_, _ = cb(goja.Undefined(), goja.Null(), n.iso.vm.ToValue(""), n.iso.vm.ToValue(""))
				_ = code
				return goja.Undefined()
			}))
		}
	}
	return child
}

func (n *nodeChild) jsExec(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) {
		n.throwArgType("command")
	}
	cmd := call.Argument(0).String()
	return n.start("sh", []string{"-c", cmd})
}

func (n *nodeChild) jsExecSync(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) {
		n.throwArgType("command")
	}
	return n.waitSync("sh", []string{"-c", call.Argument(0).String()})
}

func (n *nodeChild) jsExecFileSync(call goja.FunctionCall) goja.Value {
	file, args := spawnFileArgs(call)
	if file == "" {
		n.throwArgType("file")
	}
	return n.waitSync(file, args)
}

func (n *nodeChild) jsSpawnSync(call goja.FunctionCall) goja.Value {
	file, args := spawnFileArgs(call)
	if file == "" {
		n.throwArgType("file")
	}
	return n.waitSync(file, args)
}

func (n *nodeChild) start(file string, args []string) goja.Value {
	child := n.newChild(0)
	if n.iso.opts.Spawn == nil {
		n.throwDenied(file)
	}
	ctx := n.iso.activeCtx
	if ctx == nil {
		ctx = context.Background()
	}
	h, err := n.iso.opts.Spawn(ctx, SpawnReq{
		File: file,
		Args: args,
		Cwd:  n.iso.cwd,
		Env:  processEnvSlice(n.iso),
	})
	if err != nil {
		panic(n.iso.vm.NewGoError(err))
	}
	mustSet(child, "pid", h.PID())
	n.watch(child, h)
	return child
}

func (n *nodeChild) waitSync(file string, args []string) goja.Value {
	if n.iso.opts.Spawn == nil {
		n.throwDenied(file)
	}
	ctx := n.iso.activeCtx
	if ctx == nil {
		ctx = context.Background()
	}
	h, err := n.iso.opts.Spawn(ctx, SpawnReq{
		File: file,
		Args: args,
		Cwd:  n.iso.cwd,
		Env:  processEnvSlice(n.iso),
	})
	if err != nil {
		panic(n.iso.vm.NewGoError(err))
	}
	w := <-h.Done()
	out := n.iso.vm.NewObject()
	mustSet(out, "status", w.Code)
	mustSet(out, "pid", h.PID())
	mustSet(out, "stdout", "")
	mustSet(out, "stderr", "")
	return out
}

func (n *nodeChild) newChild(pid int) *goja.Object {
	vm := n.iso.vm
	child := vm.NewObject()
	listeners := map[string][]goja.Callable{}
	mustSet(child, "pid", pid)
	mustSet(child, "connected", false)
	stdio := vm.NewObject()
	mustSet(stdio, "on", func(goja.FunctionCall) goja.Value { return stdio })
	mustSet(child, "stdin", stdio)
	mustSet(child, "stdout", stdio)
	mustSet(child, "stderr", stdio)
	mustSet(child, "on", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return child
		}
		ev := call.Argument(0).String()
		fn, ok := goja.AssertFunction(call.Argument(1))
		if ok {
			listeners[ev] = append(listeners[ev], fn)
		}
		return child
	})
	mustSet(child, "kill", func(goja.FunctionCall) bool { return true })
	mustSet(child, "send", func(goja.FunctionCall) bool { return false })
	mustSet(child, "emit", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		ev := call.Argument(0).String()
		var args []goja.Value
		if len(call.Arguments) > 1 {
			args = call.Arguments[1:]
		}
		for _, fn := range listeners[ev] {
			_, _ = fn(child, args...)
		}
		return goja.Undefined()
	})
	return child
}

func (n *nodeChild) watch(child *goja.Object, h Spawned) {
	emit := child.Get("emit")
	fn, ok := goja.AssertFunction(emit)
	if !ok {
		return
	}
	var poll goja.Callable
	poll = func(this goja.Value, args ...goja.Value) (goja.Value, error) {
		select {
		case w := <-h.Done():
			_, err := fn(child, n.iso.vm.ToValue("exit"), n.iso.vm.ToValue(w.Code), goja.Null())
			if err != nil {
				return goja.Undefined(), err
			}
			_, err = fn(child, n.iso.vm.ToValue("close"), n.iso.vm.ToValue(w.Code), goja.Null())
			return goja.Undefined(), err
		default:
			n.iso.timers.schedule(poll, nil, 0, 0, n.iso.now())
			return goja.Undefined(), nil
		}
	}
	n.iso.timers.schedule(poll, nil, 0, 0, n.iso.now())
}

func (n *nodeChild) throwDenied(file string) {
	ctor, ok := goja.AssertConstructor(n.iso.vm.Get("Error"))
	if !ok {
		panic(n.iso.vm.NewGoError(ErrSpawnDenied))
	}
	o, err := ctor(nil, n.iso.vm.ToValue("spawn EPERM "+file))
	if err != nil {
		panic(err)
	}
	_ = o.Set("code", "EPERM")
	_ = o.Set("syscall", "spawn")
	_ = o.Set("path", file)
	panic(o)
}

func (n *nodeChild) throwArgType(name string) {
	e := n.iso.vm.NewTypeError("The \"" + name + "\" argument must be of type string")
	_ = e.Set("code", "ERR_INVALID_ARG_TYPE")
	panic(e)
}

func spawnFileArgs(call goja.FunctionCall) (file string, args []string) {
	if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) {
		return "", nil
	}
	file = call.Argument(0).String()
	if len(call.Arguments) < 2 {
		return file, nil
	}
	if arr, ok := call.Argument(1).Export().([]any); ok {
		for _, a := range arr {
			args = append(args, stringifyArg(a))
		}
	}
	return file, args
}

func stringifyArg(a any) string {
	if a == nil {
		return ""
	}
	switch x := a.(type) {
	case string:
		return x
	default:
		return ""
	}
}

func lastFunc(call goja.FunctionCall) goja.Callable {
	if len(call.Arguments) == 0 {
		return nil
	}
	fn, ok := goja.AssertFunction(call.Argument(len(call.Arguments) - 1))
	if !ok {
		return nil
	}
	return fn
}

func processEnvSlice(iso *Isolate) []string {
	p, ok := iso.vm.Get("process").(*goja.Object)
	if !ok {
		return nil
	}
	envv := p.Get("env")
	env, ok := envv.(*goja.Object)
	if !ok {
		return nil
	}
	var out []string
	for _, k := range env.Keys() {
		v := env.Get(k)
		if v == nil || goja.IsUndefined(v) {
			continue
		}
		out = append(out, k+"="+v.String())
	}
	return out
}
