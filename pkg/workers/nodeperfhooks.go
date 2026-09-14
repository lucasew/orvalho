package workers

import "github.com/dop251/goja"

// nodePerfHooksBinding materializes require("perf_hooks").
// performance is the isolate global (now / timeOrigin).
type nodePerfHooksBinding struct{}

var _ Binding = nodePerfHooksBinding{}

func (nodePerfHooksBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	for _, key := range []string{"perf_hooks", "node:perf_hooks"} {
		if v, ok := iso.moduleCache[key]; ok {
			if o, ok := v.(*goja.Object); ok {
				return o, nil
			}
		}
	}
	return newNodePerfHooks(iso), nil
}

func newNodePerfHooks(iso *Isolate) *goja.Object {
	n := &nodePerf{iso: iso}
	obj := iso.vm.NewObject()
	perf := ensurePerformance(iso)
	mustSet(obj, "performance", perf)
	mustSet(obj, "PerformanceObserver", jsPerformanceObserver)
	mustSet(obj, "monitorEventLoopDelay", n.jsMonitorEventLoopDelay)
	mustSet(obj, "createHistogram", n.jsCreateHistogram)
	mustSet(obj, "constants", iso.vm.NewObject())
	mustSet(obj, "default", obj)
	return obj
}

type nodePerf struct {
	iso *Isolate
}

func ensurePerformance(iso *Isolate) *goja.Object {
	if v := iso.vm.Get("performance"); v != nil {
		if o, ok := v.(*goja.Object); ok {
			if now := o.Get("now"); now != nil && !goja.IsUndefined(now) {
				return o
			}
		}
	}
	o := iso.vm.NewObject()
	mustSet(o, "now", func(goja.FunctionCall) float64 {
		return float64(iso.now().UnixMilli())
	})
	mustSet(o, "timeOrigin", 0)
	mustRuntimeSet(iso.vm, "performance", o)
	return o
}

func jsPerformanceObserver(call goja.ConstructorCall) *goja.Object {
	mustSet(call.This, "observe", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	mustSet(call.This, "disconnect", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	mustSet(call.This, "takeRecords", func(goja.FunctionCall) []any { return nil })
	return call.This
}

func (n *nodePerf) jsMonitorEventLoopDelay(goja.FunctionCall) goja.Value {
	return emptyHistogram(n.iso.vm)
}

func (n *nodePerf) jsCreateHistogram(goja.FunctionCall) goja.Value {
	return emptyHistogram(n.iso.vm)
}

func emptyHistogram(vm *goja.Runtime) goja.Value {
	h := vm.NewObject()
	mustSet(h, "enable", func(goja.FunctionCall) goja.Value { return vm.ToValue(true) })
	mustSet(h, "disable", func(goja.FunctionCall) goja.Value { return vm.ToValue(true) })
	mustSet(h, "percentile", func(goja.FunctionCall) goja.Value { return vm.ToValue(0) })
	mustSet(h, "min", 0)
	mustSet(h, "max", 0)
	mustSet(h, "mean", 0)
	mustSet(h, "stddev", 0)
	mustSet(h, "exceeds", 0)
	return h
}
