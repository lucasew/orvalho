package workers

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"math"
	"strconv"
	"time"

	"github.com/dop251/goja"
)

// wasmGoJS implements the Go wasm_exec gojs import module in-process.
// The JS wasm_exec handlers write through a cached DataView; those writes
// do not reliably land in the guest buffer, so syscall/js.Value.Get sees
// undefined (Astro compiler fs_js.go init).
type wasmGoJS struct {
	iso    *Isolate
	values []goja.Value
	ids    map[goja.Value]int
	refs   []int
	pool   []int
}

func newWasmGoJS(iso *Isolate) *wasmGoJS {
	g := &wasmGoJS{iso: iso, ids: map[goja.Value]int{}}
	g.values = []goja.Value{
		iso.vm.ToValue(math.NaN()),
		iso.vm.ToValue(0),
		goja.Null(),
		iso.vm.ToValue(true),
		iso.vm.ToValue(false),
		iso.vm.Get("globalThis"),
		g.makeGoObj(),
	}
	g.refs = []int{math.MaxInt32, math.MaxInt32, math.MaxInt32, math.MaxInt32, math.MaxInt32, math.MaxInt32, math.MaxInt32}
	g.ids[iso.vm.ToValue(0)] = 1
	g.ids[goja.Null()] = 2
	g.ids[iso.vm.ToValue(true)] = 3
	g.ids[iso.vm.ToValue(false)] = 4
	g.ids[iso.vm.Get("globalThis")] = 5
	g.ids[g.values[6]] = 6
	g.deferPromiseThen()
	return g
}

// deferPromiseThen makes Promise.then queue the callback on a timer.
// The Astro compiler Await uses an unbuffered channel: a fulfilled
// Promise that runs then() inside syscall/js.valueCall deadlocks the
// wasm guest (transform returns undefined; later checkdead).
func (g *wasmGoJS) deferPromiseThen() {
	_, err := g.iso.vm.RunString(`(function () {
  var proto = Promise && Promise.prototype;
  if (!proto || proto.__orvalhoThenDefer) return;
  var orig = proto.then;
  proto.then = function (onFulfilled, onRejected) {
    var self = this;
    return new Promise(function (resolve, reject) {
      setTimeout(function () {
        orig.call(self, function (v) {
          if (typeof onFulfilled !== "function") { resolve(v); return; }
          try { resolve(onFulfilled(v)); } catch (e) { reject(e); }
        }, function (e) {
          if (typeof onRejected !== "function") { reject(e); return; }
          try { resolve(onRejected(e)); } catch (e2) { reject(e2); }
        });
      }, 0);
    });
  };
  proto.__orvalhoThenDefer = true;
})();`)
	if err != nil {
		g.iso.trace("gojs deferPromiseThen: %v", err)
	}
}

func (g *wasmGoJS) makeGoObj() *goja.Object {
	o := g.iso.vm.NewObject()
	mustSet(o, "_makeFuncWrapper", func(call goja.FunctionCall) goja.Value {
		id := int(call.Argument(0).ToInteger())
		g.iso.trace("gojs makeFuncWrapper %d", id)
		return g.iso.vm.ToValue(func(fc goja.FunctionCall) goja.Value {
			g.iso.trace("gojs func %d call", id)
			ev := g.iso.vm.NewObject()
			mustSet(ev, "id", id)
			mustSet(ev, "this", fc.This)
			args := make([]any, len(fc.Arguments))
			for i, a := range fc.Arguments {
				args[i] = a
			}
			mustSet(ev, "args", g.iso.vm.ToValue(args))
			mustSet(o, "_pendingEvent", ev)
			g.resume()
			if v := ev.Get("result"); v != nil {
				return v
			}
			return goja.Undefined()
		})
	})
	return o
}

func (g *wasmGoJS) resume() {
	st := g.iso.wasmActive
	if st == nil || st.mod == nil {
		return
	}
	fn := st.mod.ExportedFunction("resume")
	if fn == nil {
		g.iso.trace("gojs resume missing")
		return
	}
	st.syncToWasm()
	_, err := fn.Call(context.Background())
	st.syncFromWasm()
	if err != nil {
		g.iso.trace("gojs resume: %v", err)
	}
}

func (g *wasmGoJS) getsp() uint32 {
	st := g.iso.wasmActive
	if st == nil || st.mod == nil {
		return 0
	}
	fn := st.mod.ExportedFunction("getsp")
	if fn == nil {
		return 0
	}
	st.syncToWasm()
	res, err := fn.Call(context.Background())
	st.syncFromWasm()
	if err != nil || len(res) == 0 {
		return 0
	}
	return uint32(res[0])
}

func (g *wasmGoJS) handle(name string, sp uint32) {
	st := g.iso.wasmActive
	if st == nil || st.jsBuf == nil {
		return
	}
	g.iso.trace("gojs %s", name)
	switch name {
	case "syscall/js.valueGet":
		recv := g.load(st, sp+8)
		key := g.loadString(st, sp+16)
		var got goja.Value = goja.Undefined()
		if o := g.asObject(recv); o != nil {
			got = o.Get(key)
			if got == nil {
				got = goja.Undefined()
			}
		}
		sp = g.getsp()
		g.store(st, sp+32, got)
	case "syscall/js.valueSet":
		if o, ok := g.load(st, sp+8).(*goja.Object); ok && o != nil {
			_ = o.Set(g.loadString(st, sp+16), g.load(st, sp+32))
		}
	case "syscall/js.valueDelete":
		if o, ok := g.load(st, sp+8).(*goja.Object); ok && o != nil {
			_ = o.Delete(g.loadString(st, sp+16))
		}
	case "syscall/js.valueIndex":
		recv := g.load(st, sp+8)
		idx := g.loadI64(st, sp+16)
		var got goja.Value = goja.Undefined()
		if o, ok := recv.(*goja.Object); ok && o != nil {
			got = o.Get(strconv.Itoa(int(idx)))
			if got == nil {
				got = goja.Undefined()
			}
		}
		g.store(st, sp+24, got)
	case "syscall/js.valueSetIndex":
		if o, ok := g.load(st, sp+8).(*goja.Object); ok && o != nil {
			_ = o.Set(strconv.Itoa(int(g.loadI64(st, sp+16))), g.load(st, sp+24))
		}
	case "syscall/js.valueCall":
		g.valueCall(st, sp, false)
	case "syscall/js.valueInvoke":
		g.valueInvoke(st, sp)
	case "syscall/js.valueNew":
		g.valueNew(st, sp)
	case "syscall/js.valueLength":
		v := g.load(st, sp+8)
		n := 0
		if o, ok := v.(*goja.Object); ok && o != nil {
			if ln := o.Get("length"); ln != nil && !goja.IsUndefined(ln) {
				n = int(ln.ToInteger())
			}
		}
		g.storeI64(st, sp+16, int64(n))
	case "syscall/js.stringVal":
		g.store(st, sp+24, g.iso.vm.ToValue(g.loadString(st, sp+8)))
	case "syscall/js.valuePrepareString":
		s := jsToString(g.load(st, sp+8))
		b := []byte(s)
		g.store(st, sp+16, g.iso.vm.ToValue(b))
		g.storeI64(st, sp+24, int64(len(b)))
	case "syscall/js.valueLoadString":
		g.copyToJSBytes(st, sp)
	case "syscall/js.copyBytesToGo":
		g.copyBytesToGo(st, sp)
	case "syscall/js.copyBytesToJS":
		g.copyBytesToJS(st, sp)
	case "syscall/js.valueInstanceOf":
		a, b := g.load(st, sp+8), g.load(st, sp+16)
		ok := false
		if ao, okA := a.(*goja.Object); okA {
			if ctor, okB := goja.AssertConstructor(b); okB {
				// goja has no public Instanceof; approximate via prototype.
				_ = ctor
				if proto := ao.Prototype(); proto != nil {
					if bo, ok := b.(*goja.Object); ok {
						if p := bo.Get("prototype"); p != nil {
							ok = proto == p
						}
					}
				}
			}
		}
		if ok {
			st.jsBuf[sp+24] = 1
		} else {
			st.jsBuf[sp+24] = 0
		}
	case "syscall/js.finalizeRef":
		id := int(binary.LittleEndian.Uint32(st.jsBuf[sp+8:]))
		if id >= 0 && id < len(g.refs) && g.refs[id] != math.MaxInt32 {
			g.refs[id]--
			if g.refs[id] <= 0 {
				v := g.values[id]
				delete(g.ids, v)
				g.values[id] = goja.Undefined()
				g.pool = append(g.pool, id)
			}
		}
	case "runtime.wasmWrite":
		fd := g.loadI64(st, sp+8)
		ptr := g.loadI64(st, sp+16)
		n := int32(binary.LittleEndian.Uint32(st.jsBuf[sp+24:]))
		if n > 0 && int(ptr)+int(n) <= len(st.jsBuf) {
			g.iso.writeStdio(int(fd), st.jsBuf[ptr:int(ptr)+int(n)])
		}
	case "runtime.wasmExit":
		// Guest finished; leave values in place.
	case "runtime.resetMemoryDataView":
		// JS DataView is unused for gojs now.
	case "runtime.nanotime1":
		ns := time.Now().UnixNano()
		g.storeI64(st, sp+8, ns)
	case "runtime.walltime":
		ms := time.Now().UnixMilli()
		g.storeI64(st, sp+8, ms/1000)
		binary.LittleEndian.PutUint32(st.jsBuf[sp+16:], uint32((ms%1000)*1e6))
	case "runtime.getRandomData":
		ptr := g.loadI64(st, sp+8)
		n := g.loadI64(st, sp+16)
		if n > 0 && int(ptr)+int(n) <= len(st.jsBuf) {
			_, _ = rand.Read(st.jsBuf[ptr : int(ptr)+int(n)])
		}
	case "runtime.scheduleTimeoutEvent":
		delay := time.Duration(g.loadI64(st, sp+8)+1) * time.Millisecond
		cb, _ := goja.AssertFunction(g.iso.vm.ToValue(func(goja.FunctionCall) goja.Value {
			g.resume()
			return goja.Undefined()
		}))
		id := g.iso.timers.schedule(cb, nil, delay, 0, g.iso.now())
		binary.LittleEndian.PutUint32(st.jsBuf[sp+16:], uint32(id))
	case "runtime.clearTimeoutEvent":
		id := int64(binary.LittleEndian.Uint32(st.jsBuf[sp+8:]))
		g.iso.timers.cancel(id)
	}
}

func (g *wasmGoJS) asObject(v goja.Value) *goja.Object {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	if o, ok := v.(*goja.Object); ok {
		return o
	}
	return v.ToObject(g.iso.vm)
}

func (g *wasmGoJS) valueCall(st *wasmInstance, sp uint32, _ bool) {
	recv := g.load(st, sp+8)
	name := g.loadString(st, sp+16)
	args := g.loadSlice(st, sp+32)
	var ret goja.Value = goja.Undefined()
	ok := byte(0)
	if o := g.asObject(recv); o != nil {
		if fn, isFn := goja.AssertFunction(o.Get(name)); isFn {
			out, err := fn(recv, args...)
			if err == nil {
				ret = out
				ok = 1
			} else {
				ret = g.iso.vm.ToValue(err.Error())
			}
		}
	}
	sp = g.getsp()
	g.store(st, sp+56, ret)
	st.jsBuf[sp+64] = ok
}

func (g *wasmGoJS) valueInvoke(st *wasmInstance, sp uint32) {
	fnv := g.load(st, sp+8)
	args := g.loadSlice(st, sp+16)
	var ret goja.Value = goja.Undefined()
	ok := byte(0)
	if fn, isFn := goja.AssertFunction(fnv); isFn {
		out, err := fn(goja.Undefined(), args...)
		if err == nil {
			ret = out
			ok = 1
		} else {
			ret = g.iso.vm.ToValue(err.Error())
		}
	}
	sp = g.getsp()
	g.store(st, sp+40, ret)
	st.jsBuf[sp+48] = ok
}

func (g *wasmGoJS) valueNew(st *wasmInstance, sp uint32) {
	ctor := g.load(st, sp+8)
	args := g.loadSlice(st, sp+16)
	var ret goja.Value = goja.Undefined()
	ok := byte(0)
	if c, isC := goja.AssertConstructor(ctor); isC {
		this, _ := ctor.(*goja.Object)
		out, err := c(this, args...)
		if err == nil {
			ret = out
			ok = 1
		} else {
			ret = g.iso.vm.ToValue(err.Error())
		}
	}
	sp = g.getsp()
	g.store(st, sp+40, ret)
	st.jsBuf[sp+48] = ok
}

func (g *wasmGoJS) loadSlice(st *wasmInstance, off uint32) []goja.Value {
	ptr := g.loadI64(st, off)
	n := g.loadI64(st, off+8)
	out := make([]goja.Value, 0, n)
	for i := int64(0); i < n; i++ {
		out = append(out, g.load(st, uint32(ptr+i*8)))
	}
	return out
}

func (g *wasmGoJS) loadString(st *wasmInstance, off uint32) string {
	ptr := g.loadI64(st, off)
	n := g.loadI64(st, off+8)
	if n <= 0 || int(ptr)+int(n) > len(st.jsBuf) {
		return ""
	}
	return string(st.jsBuf[ptr : int(ptr)+int(n)])
}

func (g *wasmGoJS) loadI64(st *wasmInstance, off uint32) int64 {
	if int(off)+8 > len(st.jsBuf) {
		return 0
	}
	lo := binary.LittleEndian.Uint32(st.jsBuf[off:])
	hi := int32(binary.LittleEndian.Uint32(st.jsBuf[off+4:]))
	return int64(lo) + int64(hi)*1<<32
}

func (g *wasmGoJS) storeI64(st *wasmInstance, off uint32, v int64) {
	if int(off)+8 > len(st.jsBuf) {
		return
	}
	binary.LittleEndian.PutUint32(st.jsBuf[off:], uint32(v))
	binary.LittleEndian.PutUint32(st.jsBuf[off+4:], uint32(v>>32))
}

func (g *wasmGoJS) load(st *wasmInstance, off uint32) goja.Value {
	if int(off)+8 > len(st.jsBuf) {
		return goja.Undefined()
	}
	u := binary.LittleEndian.Uint64(st.jsBuf[off:])
	if u == 0 {
		return goja.Undefined()
	}
	f := math.Float64frombits(u)
	if !math.IsNaN(f) {
		return g.iso.vm.ToValue(f)
	}
	id := int(binary.LittleEndian.Uint32(st.jsBuf[off:]))
	if id < 0 || id >= len(g.values) {
		return goja.Undefined()
	}
	return g.values[id]
}

func (g *wasmGoJS) store(st *wasmInstance, off uint32, v goja.Value) {
	if int(off)+8 > len(st.jsBuf) {
		return
	}
	if v == nil || goja.IsUndefined(v) {
		binary.LittleEndian.PutUint64(st.jsBuf[off:], 0)
		return
	}
	if goja.IsNull(v) {
		g.storeRef(st, off, v, 1)
		return
	}
	if b, ok := v.Export().(bool); ok {
		g.storeRef(st, off, v, 0)
		_ = b
		return
	}
	if n, ok := v.Export().(float64); ok && n != 0 && !math.IsNaN(n) {
		binary.LittleEndian.PutUint64(st.jsBuf[off:], math.Float64bits(n))
		return
	}
	if n, ok := v.Export().(int64); ok && n != 0 {
		binary.LittleEndian.PutUint64(st.jsBuf[off:], math.Float64bits(float64(n)))
		return
	}
	kind := 0
	switch t := v.Export().(type) {
	case string:
		kind = 2
		_ = t
	default:
		if _, ok := goja.AssertFunction(v); ok {
			kind = 4
		} else if v != nil && !goja.IsNull(v) {
			kind = 1
		}
	}
	g.storeRef(st, off, v, kind)
}

func (g *wasmGoJS) storeRef(st *wasmInstance, off uint32, v goja.Value, kind int) {
	id, ok := g.ids[v]
	if !ok {
		if len(g.pool) > 0 {
			id = g.pool[len(g.pool)-1]
			g.pool = g.pool[:len(g.pool)-1]
			g.values[id] = v
			g.refs[id] = 0
		} else {
			id = len(g.values)
			g.values = append(g.values, v)
			g.refs = append(g.refs, 0)
		}
		g.ids[v] = id
	}
	g.refs[id]++
	binary.LittleEndian.PutUint32(st.jsBuf[off:], uint32(id))
	binary.LittleEndian.PutUint32(st.jsBuf[off+4:], uint32(0x7FF80000|kind))
}

func (g *wasmGoJS) copyBytesToGo(st *wasmInstance, sp uint32) {
	dstPtr := g.loadI64(st, sp+8)
	dstLen := g.loadI64(st, sp+16)
	src := g.load(st, sp+32)
	b := valueBytes(src)
	n := int(dstLen)
	if n > len(b) {
		n = len(b)
	}
	if n > 0 && int(dstPtr)+n <= len(st.jsBuf) {
		copy(st.jsBuf[dstPtr:int(dstPtr)+n], b[:n])
	}
	g.storeI64(st, sp+40, int64(n))
	st.jsBuf[sp+48] = 1
}

func (g *wasmGoJS) copyBytesToJS(st *wasmInstance, sp uint32) {
	dst := g.load(st, sp+8)
	srcPtr := g.loadI64(st, sp+16)
	srcLen := g.loadI64(st, sp+24)
	n := int(srcLen)
	if int(srcPtr)+n > len(st.jsBuf) {
		n = len(st.jsBuf) - int(srcPtr)
	}
	if n < 0 {
		n = 0
	}
	src := st.jsBuf[srcPtr : int(srcPtr)+n]
	if o, ok := dst.(*goja.Object); ok {
		if buf := typedArrayBytes(o); buf != nil {
			if n > len(buf) {
				n = len(buf)
			}
			copy(buf, src[:n])
		}
	}
	g.storeI64(st, sp+40, int64(n))
	st.jsBuf[sp+48] = 1
}

func (g *wasmGoJS) copyToJSBytes(st *wasmInstance, sp uint32) {
	// valueLoadString: dest slice at sp+16, source is JS value at sp+8 (bytes)
	src := valueBytes(g.load(st, sp+8))
	dstPtr := g.loadI64(st, sp+16)
	dstLen := g.loadI64(st, sp+24)
	n := int(dstLen)
	if n > len(src) {
		n = len(src)
	}
	if n > 0 && int(dstPtr)+n <= len(st.jsBuf) {
		copy(st.jsBuf[dstPtr:int(dstPtr)+n], src[:n])
	}
}
