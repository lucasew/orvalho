package wasm

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"

	"github.com/tetratelabs/wazero"
)

// Bin is one wasm image. Compiled compiles at most once on success.
type Bin struct {
	raw []byte
	mu  sync.Mutex
	mod wazero.CompiledModule
}

func (b *Bin) Bytes() []byte {
	if b == nil {
		return nil
	}
	return b.raw
}

func (b *Bin) Compiled(ctx context.Context, rt wazero.Runtime) (wazero.CompiledModule, error) {
	if b == nil {
		return nil, fmt.Errorf("WebAssembly: missing module")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.mod != nil {
		return b.mod, nil
	}
	mod, err := rt.CompileModule(ctx, b.raw)
	if err != nil {
		return nil, err
	}
	b.mod = mod
	return b.mod, nil
}

// Hub owns a wazero runtime and interns bins by content hash.
type Hub struct {
	ctx  context.Context
	rt   wazero.Runtime
	mu   sync.Mutex
	bins map[[32]byte]*Bin
}

func NewHub(ctx context.Context) *Hub {
	return &Hub{ctx: ctx, rt: wazero.NewRuntime(ctx)}
}

func (h *Hub) Runtime() wazero.Runtime {
	return h.rt
}

func (h *Hub) Bin(raw []byte) *Bin {
	sum := sha256.Sum256(raw)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.bins == nil {
		h.bins = make(map[[32]byte]*Bin)
	}
	if b, ok := h.bins[sum]; ok {
		return b
	}
	b := &Bin{raw: append([]byte(nil), raw...)}
	h.bins[sum] = b
	return b
}
