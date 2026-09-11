package workers

import (
	"encoding/hex"
	"testing"

	"github.com/dop251/goja"
)

// add.wasm: (module (func (export "add") (param i32 i32) (result i32) local.get 0 local.get 1 i32.add))
const addWasmHex = "0061736d0100000001070160027f7f017f030201000707010361646400000a09010700200020016a0b"

func TestWebAssemblyAdd(t *testing.T) {
	raw, err := hex.DecodeString(addWasmHex)
	if err != nil {
		t.Fatal(err)
	}
	iso := New("", Options{})
	err = iso.ScriptMain(t.Context(), `
		var bytes = new Uint8Array([`+bytesToJS(raw)+`]);
		var n = 0;
		WebAssembly.instantiate(bytes).then(function (r) {
			if (!r.instance || typeof r.instance.exports.add !== "function") {
				throw new Error("exports " + Object.keys(r.instance && r.instance.exports || {}));
			}
			var got = r.instance.exports.add(2, 3);
			if (got !== 5) throw new Error("add " + got);
			n = 1;
		});
		setTimeout(function () {
			if (n !== 1) throw new Error("no result");
		}, 0);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestWebAssemblyModuleConstructor(t *testing.T) {
	raw, err := hex.DecodeString(addWasmHex)
	if err != nil {
		t.Fatal(err)
	}
	iso := New("", Options{})
	err = iso.ScriptMain(t.Context(), `
		var bytes = new Uint8Array([`+bytesToJS(raw)+`]);
		var mod = new WebAssembly.Module(bytes);
		var inst = new WebAssembly.Instance(mod);
		if (inst.exports.add(1, 2) !== 3) throw new Error("ctor add");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestWebAssemblyMemExportName(t *testing.T) {
	// (module (memory (export "mem") 1)) — xxhash-wasm names memory "mem".
	raw, err := hex.DecodeString("0061736d010000000503010001070701036d656d0200")
	if err != nil {
		t.Fatal(err)
	}
	iso := New("", Options{})
	err = iso.ScriptMain(t.Context(), `
		var bytes = new Uint8Array([`+bytesToJS(raw)+`]);
		var inst = new WebAssembly.Instance(new WebAssembly.Module(bytes));
		if (!inst.exports.mem) throw new Error("no mem");
		if (!inst.exports.mem.buffer) throw new Error("no buffer");
		if (inst.exports.mem.buffer.byteLength < 65536) throw new Error("size");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestGojaArrowThisFromGo(t *testing.T) {
	iso := New("", Options{})
	err := iso.ScriptMain(t.Context(), `
		class C {
			constructor() {
				this.x = 7;
				globalThis._arrow = () => this.x;
			}
		}
		new C();
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
	fn, ok := goja.AssertFunction(iso.vm.Get("_arrow"))
	if !ok {
		t.Fatal("no arrow")
	}
	v, err := fn(goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	if v.ToInteger() != 7 {
		t.Fatalf("arrow this from Go: got %v", v.Export())
	}
}

func TestWebAssemblyMemoryViewSurvivesGrow(t *testing.T) {
	// (module (memory (export "mem") 1))
	raw, err := hex.DecodeString("0061736d010000000503010001070701036d656d0200")
	if err != nil {
		t.Fatal(err)
	}
	iso := New("", Options{})
	err = iso.ScriptMain(t.Context(), `
		var bytes = new Uint8Array([`+bytesToJS(raw)+`]);
		var inst = new WebAssembly.Instance(new WebAssembly.Module(bytes));
		var before = inst.exports.mem.buffer.byteLength;
		var dv = new DataView(inst.exports.mem.buffer);
		dv.setUint8(0, 42);
		var prev = inst.exports.mem.grow(1);
		if (typeof prev !== "number") throw new Error("grow return " + prev);
		var after = inst.exports.mem.buffer.byteLength;
		if (after <= before) throw new Error("buffer did not grow " + before + " -> " + after);
		var fresh = new DataView(inst.exports.mem.buffer);
		if (fresh.getUint8(0) !== 42) throw new Error("content lost after grow");
		fresh.setUint8(before, 7);
		if (fresh.getUint8(before) !== 7) throw new Error("write into grown pages");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestWebAssemblyMemoryGrowCap(t *testing.T) {
	raw, err := hex.DecodeString("0061736d010000000503010001070701036d656d0200")
	if err != nil {
		t.Fatal(err)
	}
	iso := New("", Options{})
	err = iso.ScriptMain(t.Context(), `
		var bytes = new Uint8Array([`+bytesToJS(raw)+`]);
		var inst = new WebAssembly.Instance(new WebAssembly.Module(bytes));
		var threw = false;
		try { inst.exports.mem.grow(100000); } catch (e) { threw = true; }
		if (!threw) throw new Error("huge grow should fail");
		if (inst.exports.mem.buffer.byteLength > 256 * 1024 * 1024) throw new Error("buffer exploded");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestWebAssemblyInstanceof(t *testing.T) {
	raw, err := hex.DecodeString(addWasmHex)
	if err != nil {
		t.Fatal(err)
	}
	iso := New("", Options{})
	err = iso.ScriptMain(t.Context(), `
		var bytes = new Uint8Array([`+bytesToJS(raw)+`]);
		var n = 0;
		WebAssembly.instantiate(bytes).then(function (r) {
			if (!(r.instance instanceof WebAssembly.Instance)) throw new Error("instance");
			if (!(r.module instanceof WebAssembly.Module)) throw new Error("module");
			n = 1;
		});
		setTimeout(function () {
			if (n !== 1) throw new Error("no result");
		}, 0);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestWasmActiveKeepsResumeModule(t *testing.T) {
	// (module (func (export "resume")))
	resumeHex := "0061736d0100000001040160000003020100070a0106726573756d6500000a040102000b"
	rawAdd, err := hex.DecodeString(addWasmHex)
	if err != nil {
		t.Fatal(err)
	}
	rawResume, err := hex.DecodeString(resumeHex)
	if err != nil {
		t.Fatal(err)
	}
	iso := New("", Options{})
	err = iso.ScriptMain(t.Context(), `
		var add = new WebAssembly.Instance(new WebAssembly.Module(new Uint8Array([`+bytesToJS(rawAdd)+`])));
		if (add.exports.add(2, 3) !== 5) throw new Error("add");
		var goLike = new WebAssembly.Instance(new WebAssembly.Module(new Uint8Array([`+bytesToJS(rawResume)+`])));
		goLike.exports.resume();
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
	if iso.wasmActive == nil || !iso.wasmActive.hasResume() {
		t.Fatal("wasmActive lost resume module after a prior instantiate")
	}
}

func TestGojsPromiseThenIsAsync(t *testing.T) {
	iso := New("", Options{})
	iso.wasmGo = newWasmGoJS(iso)
	err := iso.ScriptMain(t.Context(), `
		var sync = false;
		Promise.resolve(1).then(function () { sync = true; });
		if (sync) throw new Error("then ran synchronously");
		setTimeout(function () {
			if (!sync) throw new Error("then never ran");
		}, 0);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestGojsPromiseThenUngatedIsNotTimer(t *testing.T) {
	iso := New("", Options{})
	iso.wasmGo = newWasmGoJS(iso)
	err := iso.ScriptMain(t.Context(), `
		if (typeof Promise.prototype.then !== "function") throw new Error("then");
		if (!Promise.prototype.__orvalhoThenHold) throw new Error("hold missing");
		var n = 0;
		Promise.resolve(2).then(function (v) { n = v; });
		setTimeout(function () {
			if (n !== 2) throw new Error("n " + n);
		}, 0);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestWebAssemblyCompileThenInstantiate(t *testing.T) {
	raw, err := hex.DecodeString(addWasmHex)
	if err != nil {
		t.Fatal(err)
	}
	iso := New("", Options{})
	err = iso.ScriptMain(t.Context(), `
		var bytes = new Uint8Array([`+bytesToJS(raw)+`]);
		var n = 0;
		WebAssembly.compile(bytes).then(function (mod) {
			return WebAssembly.instantiate(mod);
		}).then(function (inst) {
			if (typeof inst.exports.add !== "function") throw new Error("no add");
			if (inst.exports.add(10, 32) !== 42) throw new Error("sum");
			n = 1;
		});
		setTimeout(function () {
			if (n !== 1) throw new Error("no result");
		}, 0);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func bytesToJS(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	s := ""
	for i, v := range b {
		if i > 0 {
			s += ","
		}
		s += itoa(int(v))
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
