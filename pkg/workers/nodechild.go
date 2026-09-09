package workers

import (
	"context"
	"io"
	"strings"

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
	mustSet(obj, "default", obj)
	n.installPromisifyCustom(obj)
	return obj
}

// installPromisifyCustom makes promisify(execFile) fulfill {stdout,stderr}
// the way Node's child_process does (generic promisify keeps only the
// first success argument).
func (n *nodeChild) installPromisifyCustom(cp *goja.Object) {
	v, err := runNamedScript(n.iso.vm, "node:child_process/promisify", `(function (cp) {
  var key = Symbol.for("nodejs.util.promisify.custom");
  function wrap(fn) {
    return function () {
      var args = [];
      for (var i = 0; i < arguments.length; i++) args.push(arguments[i]);
      return new Promise(function (resolve, reject) {
        args.push(function (err, stdout, stderr) {
          if (err) reject(err);
          else resolve({ stdout: stdout == null ? "" : stdout, stderr: stderr == null ? "" : stderr });
        });
        fn.apply(cp, args);
      });
    };
  }
  if (typeof cp.execFile === "function") cp.execFile[key] = wrap(cp.execFile);
  if (typeof cp.exec === "function") cp.exec[key] = wrap(cp.exec);
})`)
	if err != nil {
		panic("goja child_process promisify: " + err.Error())
	}
	fn, ok := goja.AssertFunction(v)
	if !ok {
		return
	}
	if _, err := fn(goja.Undefined(), cp); err != nil {
		panic(err)
	}
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
	cb := lastFunc(call)
	child := n.start("sh", []string{"-c", cmd}).(*goja.Object)
	if cb != nil {
		on := child.Get("on")
		if fn, ok := goja.AssertFunction(on); ok {
			_, _ = fn(child, n.iso.vm.ToValue("exit"), n.iso.vm.ToValue(func(goja.FunctionCall) goja.Value {
				_, _ = cb(goja.Undefined(), goja.Null(), n.iso.vm.ToValue(""), n.iso.vm.ToValue(""))
				return goja.Undefined()
			}))
		}
	}
	return child
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
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	n.attachStdio(child, stdinW, stdoutR)
	n.iso.trace("spawn %s %s", file, strings.Join(args, " "))
	h, err := n.iso.opts.Spawn(ctx, SpawnReq{
		File:   file,
		Args:   args,
		Cwd:    n.iso.cwd,
		Env:    processEnvSlice(n.iso),
		Stdin:  stdinR,
		Stdout: stdoutW,
	})
	if err != nil {
		n.iso.trace("spawn fail %s: %v", file, err)
		_ = stdinW.Close()
		_ = stdoutW.Close()
		_ = stdinR.Close()
		_ = stdoutR.Close()
		// Node spawn() returns the ChildProcess and emits 'error'
		// asynchronously (workerd stub is /dev/null → EACCES).
		n.scheduleChildError(child, err, file)
		return child
	}
	n.iso.trace("spawn ok %s pid=%d", file, h.PID())
	mustSet(child, "pid", h.PID())
	mustSet(child, "kill", func(goja.FunctionCall) bool {
		_ = h.Kill()
		return true
	})
	n.watch(child, h, func() {
		_ = stdinW.Close()
		_ = stdoutW.Close()
	})
	return child
}

func (n *nodeChild) scheduleChildError(child *goja.Object, err error, file string) {
	ctor, ok := goja.AssertConstructor(n.iso.vm.Get("Error"))
	if !ok {
		return
	}
	o, ctorErr := ctor(nil, n.iso.vm.ToValue(err.Error()))
	if ctorErr != nil {
		return
	}
	code := "ENOENT"
	if strings.Contains(err.Error(), "permission denied") {
		code = "EACCES"
	}
	_ = o.Set("code", code)
	_ = o.Set("syscall", "spawn")
	_ = o.Set("path", file)
	emit := child.Get("emit")
	fn, ok := goja.AssertFunction(emit)
	if !ok {
		return
	}
	var fire goja.Callable
	fire = func(this goja.Value, args ...goja.Value) (goja.Value, error) {
		_, e := fn(child, n.iso.vm.ToValue("error"), o)
		return goja.Undefined(), e
	}
	n.iso.timers.schedule(fire, nil, 0, 0, n.iso.now())
}

func (n *nodeChild) waitSync(file string, args []string) goja.Value {
	if n.iso.opts.Spawn == nil {
		n.throwDenied(file)
	}
	ctx := n.iso.activeCtx
	if ctx == nil {
		ctx = context.Background()
	}
	n.iso.trace("spawn sync %s %s", file, strings.Join(args, " "))
	h, err := n.iso.opts.Spawn(ctx, SpawnReq{
		File: file,
		Args: args,
		Cwd:  n.iso.cwd,
		Env:  processEnvSlice(n.iso),
	})
	if err != nil {
		n.iso.trace("spawn sync fail %s: %v", file, err)
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
	stub := n.newStream()
	mustSet(child, "stdin", stub)
	mustSet(child, "stdout", stub)
	mustSet(child, "stderr", stub)
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
	mustSet(child, "unref", func(goja.FunctionCall) goja.Value { return child })
	mustSet(child, "ref", func(goja.FunctionCall) goja.Value { return child })
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

func (n *nodeChild) attachStdio(child *goja.Object, stdin io.WriteCloser, stdout io.ReadCloser) {
	mustSet(child, "stdin", n.newStdin(stdin))
	mustSet(child, "stdout", n.newStdout(stdout))
	mustSet(child, "stderr", n.newStream())
}

func (n *nodeChild) newStream() *goja.Object {
	s := n.iso.vm.NewObject()
	listeners := map[string][]goja.Callable{}
	mustSet(s, "on", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) >= 2 {
			if fn, ok := goja.AssertFunction(call.Argument(1)); ok {
				ev := call.Argument(0).String()
				listeners[ev] = append(listeners[ev], fn)
			}
		}
		return s
	})
	mustSet(s, "prependListener", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) >= 2 {
			if fn, ok := goja.AssertFunction(call.Argument(1)); ok {
				ev := call.Argument(0).String()
				listeners[ev] = append([]goja.Callable{fn}, listeners[ev]...)
			}
		}
		return s
	})
	mustSet(s, "once", s.Get("on"))
	mustSet(s, "prependOnceListener", s.Get("prependListener"))
	mustSet(s, "emit", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		ev := call.Argument(0).String()
		var args []goja.Value
		if len(call.Arguments) > 1 {
			args = call.Arguments[1:]
		}
		for _, fn := range listeners[ev] {
			_, _ = fn(s, args...)
		}
		return goja.Undefined()
	})
	mustSet(s, "pipe", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			return call.Argument(0)
		}
		return s
	})
	mustSet(s, "unpipe", func(goja.FunctionCall) goja.Value { return s })
	mustSet(s, "unref", func(goja.FunctionCall) goja.Value { return s })
	mustSet(s, "ref", func(goja.FunctionCall) goja.Value { return s })
	mustSet(s, "destroy", func(goja.FunctionCall) goja.Value { return s })
	mustSet(s, "end", func(goja.FunctionCall) goja.Value { return s })
	return s
}

func (n *nodeChild) newStdin(w io.WriteCloser) *goja.Object {
	s := n.newStream()
	mustSet(s, "write", func(call goja.FunctionCall) goja.Value {
		data := valueBytes(call.Argument(0))
		var cb goja.Callable
		if len(call.Arguments) > 1 {
			if fn, ok := goja.AssertFunction(call.Argument(len(call.Arguments) - 1)); ok {
				cb = fn
			}
		}
		_, err := w.Write(data)
		if cb != nil {
			var ev goja.Value = goja.Null()
			if err != nil {
				ev = n.iso.vm.ToValue(err.Error())
			}
			n.iso.timers.schedule(cb, []goja.Value{ev}, 0, 0, n.iso.now())
		}
		return n.iso.vm.ToValue(err == nil)
	})
	mustSet(s, "end", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			if _, isFn := goja.AssertFunction(call.Argument(0)); !isFn {
				_, _ = w.Write(valueBytes(call.Argument(0)))
			}
		}
		_ = w.Close()
		return s
	})
	mustSet(s, "destroy", func(goja.FunctionCall) goja.Value {
		_ = w.Close()
		return s
	})
	return s
}

func (n *nodeChild) newStdout(r io.ReadCloser) *goja.Object {
	s := n.newStream()
	ch := make(chan []byte, 8)
	go func() {
		defer close(ch)
		buf := make([]byte, 64*1024)
		for {
			nr, err := r.Read(buf)
			if nr > 0 {
				cp := make([]byte, nr)
				copy(cp, buf[:nr])
				ch <- cp
			}
			if err != nil {
				return
			}
		}
	}()
	mustSet(s, "destroy", func(goja.FunctionCall) goja.Value {
		_ = r.Close()
		return s
	})
	n.pump(s, ch)
	return s
}

func (n *nodeChild) pump(s *goja.Object, ch <-chan []byte) {
	emit := s.Get("emit")
	fn, ok := goja.AssertFunction(emit)
	if !ok {
		return
	}
	var poll goja.Callable
	poll = func(this goja.Value, args ...goja.Value) (goja.Value, error) {
		for {
			select {
			case b, ok := <-ch:
				if !ok {
					_, err := fn(s, n.iso.vm.ToValue("end"))
					return goja.Undefined(), err
				}
				if _, err := fn(s, n.iso.vm.ToValue("data"), n.bytesToJS(b)); err != nil {
					return goja.Undefined(), err
				}
			default:
				n.iso.timers.schedule(poll, nil, 0, 0, n.iso.now())
				return goja.Undefined(), nil
			}
		}
	}
	n.iso.timers.schedule(poll, nil, 0, 0, n.iso.now())
}

func (n *nodeChild) bytesToJS(b []byte) goja.Value {
	cp := append([]byte(nil), b...)
	ab := n.iso.vm.NewArrayBuffer(cp)
	if buf := n.iso.vm.Get("Buffer"); buf != nil && !goja.IsUndefined(buf) {
		if o, ok := buf.(*goja.Object); ok {
			if from, ok := goja.AssertFunction(o.Get("from")); ok {
				if v, err := from(buf, n.iso.vm.ToValue(ab)); err == nil {
					return v
				}
			}
		}
	}
	ctor, ok := goja.AssertConstructor(n.iso.vm.Get("Uint8Array"))
	if !ok {
		return n.iso.vm.ToValue(ab)
	}
	v, err := ctor(nil, n.iso.vm.ToValue(ab))
	if err != nil {
		return n.iso.vm.ToValue(ab)
	}
	return v
}

func (n *nodeChild) watch(child *goja.Object, h Spawned, onDone func()) {
	emit := child.Get("emit")
	fn, ok := goja.AssertFunction(emit)
	if !ok {
		return
	}
	var poll goja.Callable
	poll = func(this goja.Value, args ...goja.Value) (goja.Value, error) {
		select {
		case w := <-h.Done():
			if onDone != nil {
				onDone()
			}
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

func spawnFileName(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return ""
	}
	if o, ok := v.(*goja.Object); ok {
		if d := o.Get("default"); d != nil && !goja.IsUndefined(d) && !goja.IsNull(d) {
			if s := d.Export(); s != nil {
				if str, ok := s.(string); ok && str != "" {
					return str
				}
			}
		}
	}
	s := v.String()
	if s == "[object Object]" {
		return ""
	}
	return s
}

func spawnFileArgs(call goja.FunctionCall) (file string, args []string) {
	if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) {
		return "", nil
	}
	file = spawnFileName(call.Argument(0))
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
