package wasm

import (
	"encoding/hex"
	"testing"
)

// add.wasm: (module (func (export "add") (param i32 i32) (result i32) local.get 0 local.get 1 i32.add))
const addWasmHex = "0061736d0100000001070160027f7f017f030201000707010361646400000a09010700200020016a0b"

func TestBinCompilesOnce(t *testing.T) {
	raw, err := hex.DecodeString(addWasmHex)
	if err != nil {
		t.Fatal(err)
	}
	h := NewHub(t.Context())
	b1 := h.Bin(raw)
	b2 := h.Bin(append([]byte(nil), raw...))
	if b1 != b2 {
		t.Fatal("same bytes must intern to one bin")
	}
	m1, err := b1.Compiled(t.Context(), h.Runtime())
	if err != nil {
		t.Fatal(err)
	}
	m2, err := b2.Compiled(t.Context(), h.Runtime())
	if err != nil {
		t.Fatal(err)
	}
	if m1 != m2 {
		t.Fatal("Compiled is a singleton")
	}
}
