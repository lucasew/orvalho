package workers

import "github.com/dop251/goja"

// nodeWorkerThreadsBinding materializes require("worker_threads").
// There is no second isolate: isMainThread is true and Worker does not run.
type nodeWorkerThreadsBinding struct{}

var _ Binding = nodeWorkerThreadsBinding{}

func (nodeWorkerThreadsBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	for _, key := range []string{"worker_threads", "node:worker_threads"} {
		if v, ok := iso.moduleCache[key]; ok {
			if o, ok := v.(*goja.Object); ok {
				return o, nil
			}
		}
	}
	return newNodeWorkerThreads(iso), nil
}

type nodeWorkerThreads struct {
	iso *Isolate
	env map[string]goja.Value
}

func newNodeWorkerThreads(iso *Isolate) *goja.Object {
	n := &nodeWorkerThreads{iso: iso, env: map[string]goja.Value{}}
	obj := iso.vm.NewObject()
	mustSet(obj, "isMainThread", true)
	mustSet(obj, "parentPort", goja.Null())
	mustSet(obj, "threadId", 0)
	mustSet(obj, "workerData", goja.Undefined())
	mustSet(obj, "resourceLimits", iso.vm.NewObject())
	mustSet(obj, "SHARE_ENV", workerShareEnv(iso))
	mustSet(obj, "Worker", n.jsWorker)
	mustSet(obj, "MessageChannel", n.jsMessageChannel)
	mustSet(obj, "MessagePort", n.jsMessagePort)
	mustSet(obj, "BroadcastChannel", n.jsBroadcastChannel)
	mustSet(obj, "receiveMessageOnPort", n.jsReceiveMessageOnPort)
	mustSet(obj, "markAsUntransferable", n.jsNoop)
	mustSet(obj, "markAsUncloneable", n.jsNoop)
	mustSet(obj, "moveMessagePortToContext", n.jsMoveMessagePortToContext)
	mustSet(obj, "getEnvironmentData", n.jsGetEnvironmentData)
	mustSet(obj, "setEnvironmentData", n.jsSetEnvironmentData)
	mustSet(obj, "default", obj)
	return obj
}

func workerShareEnv(iso *Isolate) goja.Value {
	v, err := iso.vm.RunString(`Symbol.for("nodejs.worker_threads.SHARE_ENV")`)
	if err != nil {
		return iso.vm.NewObject()
	}
	return v
}

func (n *nodeWorkerThreads) jsNoop(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) > 0 {
		return call.Argument(0)
	}
	return goja.Undefined()
}

func (n *nodeWorkerThreads) jsReceiveMessageOnPort(goja.FunctionCall) goja.Value {
	return goja.Undefined()
}

func (n *nodeWorkerThreads) jsMoveMessagePortToContext(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 {
		return goja.Undefined()
	}
	return call.Argument(0)
}

func (n *nodeWorkerThreads) jsGetEnvironmentData(call goja.FunctionCall) goja.Value {
	key := jsToString(call.Argument(0))
	if v, ok := n.env[key]; ok {
		return v
	}
	return goja.Undefined()
}

func (n *nodeWorkerThreads) jsSetEnvironmentData(call goja.FunctionCall) goja.Value {
	key := jsToString(call.Argument(0))
	if len(call.Arguments) < 2 || goja.IsUndefined(call.Argument(1)) {
		delete(n.env, key)
	} else {
		n.env[key] = call.Argument(1)
	}
	return goja.Undefined()
}

func (n *nodeWorkerThreads) jsWorker(call goja.ConstructorCall) *goja.Object {
	return n.initWorker(call.This)
}

func (n *nodeWorkerThreads) jsMessageChannel(call goja.ConstructorCall) *goja.Object {
	ch := call.This
	mustSet(ch, "port1", n.newPort())
	mustSet(ch, "port2", n.newPort())
	return ch
}

func (n *nodeWorkerThreads) jsMessagePort(call goja.ConstructorCall) *goja.Object {
	return n.initPort(call.This)
}

func (n *nodeWorkerThreads) jsBroadcastChannel(call goja.ConstructorCall) *goja.Object {
	return n.initPort(call.This)
}

func (n *nodeWorkerThreads) newPort() *goja.Object {
	return n.initPort(n.iso.vm.NewObject())
}

func (n *nodeWorkerThreads) initPort(port *goja.Object) *goja.Object {
	attachEmitter(port)
	mustSet(port, "postMessage", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	mustSet(port, "close", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	mustSet(port, "start", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	mustSet(port, "ref", func(call goja.FunctionCall) goja.Value { return port })
	mustSet(port, "unref", func(call goja.FunctionCall) goja.Value { return port })
	return port
}

func (n *nodeWorkerThreads) initWorker(w *goja.Object) *goja.Object {
	attachEmitter(w)
	mustSet(w, "threadId", 1)
	mustSet(w, "stdin", goja.Null())
	mustSet(w, "stdout", goja.Null())
	mustSet(w, "stderr", goja.Null())
	mustSet(w, "resourceLimits", n.iso.vm.NewObject())
	mustSet(w, "postMessage", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	mustSet(w, "ref", func(call goja.FunctionCall) goja.Value { return w })
	mustSet(w, "unref", func(call goja.FunctionCall) goja.Value { return w })
	mustSet(w, "terminate", func(goja.FunctionCall) goja.Value {
		p, resolve, _ := n.iso.vm.NewPromise()
		resolve(0)
		return n.iso.vm.ToValue(p)
	})
	return w
}

type eeListener struct {
	val  *goja.Object
	fn   goja.Callable
	once bool
}

func attachEmitter(obj *goja.Object) {
	listeners := map[string][]eeListener{}
	add := func(ev string, val goja.Value, front, once bool) {
		fn, ok := goja.AssertFunction(val)
		if !ok {
			return
		}
		o, _ := val.(*goja.Object)
		l := eeListener{val: o, fn: fn, once: once}
		if front {
			listeners[ev] = append([]eeListener{l}, listeners[ev]...)
			return
		}
		listeners[ev] = append(listeners[ev], l)
	}
	drop := func(ev string, val goja.Value) {
		want, _ := val.(*goja.Object)
		arr := listeners[ev]
		for i, l := range arr {
			if l.val == want {
				listeners[ev] = append(arr[:i], arr[i+1:]...)
				return
			}
		}
	}
	on := func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) >= 2 {
			add(call.Argument(0).String(), call.Argument(1), false, false)
		}
		return obj
	}
	once := func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) >= 2 {
			add(call.Argument(0).String(), call.Argument(1), false, true)
		}
		return obj
	}
	off := func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) >= 2 {
			drop(call.Argument(0).String(), call.Argument(1))
		}
		return obj
	}
	mustSet(obj, "on", on)
	mustSet(obj, "addListener", on)
	mustSet(obj, "prependListener", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) >= 2 {
			add(call.Argument(0).String(), call.Argument(1), true, false)
		}
		return obj
	})
	mustSet(obj, "once", once)
	mustSet(obj, "prependOnceListener", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) >= 2 {
			add(call.Argument(0).String(), call.Argument(1), true, true)
		}
		return obj
	})
	mustSet(obj, "off", off)
	mustSet(obj, "removeListener", off)
	mustSet(obj, "removeAllListeners", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			clear(listeners)
			return obj
		}
		delete(listeners, call.Argument(0).String())
		return obj
	})
	mustSet(obj, "listenerCount", func(call goja.FunctionCall) int {
		if len(call.Arguments) == 0 {
			return 0
		}
		return len(listeners[call.Argument(0).String()])
	})
	mustSet(obj, "emit", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		ev := call.Argument(0).String()
		var args []goja.Value
		if len(call.Arguments) > 1 {
			args = call.Arguments[1:]
		}
		arr := append([]eeListener(nil), listeners[ev]...)
		for _, l := range arr {
			if l.once {
				drop(ev, l.val)
			}
			_, _ = l.fn(obj, args...)
		}
		return goja.Undefined()
	})
	mustSet(obj, "addEventListener", obj.Get("on"))
	mustSet(obj, "removeEventListener", obj.Get("off"))
}
