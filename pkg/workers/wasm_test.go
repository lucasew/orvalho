package workers

import (
	"encoding/hex"
	"testing"
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
