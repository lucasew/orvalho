package workers

import (
	"strings"
	"testing"
)

func TestTextEncoderRoundTrip(t *testing.T) {
	iso := New("", Options{})
	err := iso.ScriptMain(t.Context(), `
		var u = new TextEncoder();
		var b = u.encode("hi");
		if (!(b instanceof Uint8Array)) throw new Error("not Uint8Array " + b);
		if (b.length !== 2 || b[0] !== 104 || b[1] !== 105) throw new Error("encode " + b);
		if (new TextDecoder().decode(b) !== "hi") throw new Error("decode");
		var dest = new Uint8Array(8);
		var r = u.encodeInto("hi", dest);
		if (r.read !== 2 || r.written !== 2) throw new Error("encodeInto " + r.read + " " + r.written);
		if (dest[0] !== 104 || dest[1] !== 105) throw new Error("dest");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestTextEncoderLarge(t *testing.T) {
	iso := New("", Options{})
	// 2.5MiB of ASCII — the old JS polyfill push()'d every byte.
	src := strings.Repeat("a", 2500000)
	iso.vm.Set("$SRC", src)
	err := iso.ScriptMain(t.Context(), `
		var b = new TextEncoder().encode($SRC);
		if (b.length !== 2500000) throw new Error("len " + b.length);
		if (b[0] !== 97 || b[2499999] !== 97) throw new Error("bytes");
		if (new TextDecoder().decode(b).length !== 2500000) throw new Error("decode len");
		var dest = new Uint8Array(2500000);
		var r = new TextEncoder().encodeInto($SRC, dest);
		if (r.read !== 2500000 || r.written !== 2500000) throw new Error("into " + r.read + " " + r.written);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}
