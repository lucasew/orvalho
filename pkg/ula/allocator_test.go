package ula

import (
	"fmt"
	"net/netip"
	"sync"
	"testing"
)

func TestAllocate_StableAndNoCollisions(t *testing.T) {
	store := NewMemoryStore()
	alloc := NewAllocator(store)

	const mesh = "mesh-stable"
	const device uint16 = 3

	keys := []AllocationKey{
		{Mesh: mesh, Device: device, Actor: "actor-a"},
		{Mesh: mesh, Device: device, Actor: "actor-b"},
		{Mesh: mesh, Device: device, Actor: "actor-c"},
	}

	seen := make(map[netip.Addr]string)
	var addrs []netip.Addr
	for _, k := range keys {
		addr, err := alloc.Allocate(k)
		if err != nil {
			t.Fatalf("allocate %q: %v", k.Actor, err)
		}
		if !addr.Is6() {
			t.Fatalf("not IPv6: %s", addr)
		}
		if owner, ok := seen[addr]; ok {
			t.Fatalf("collision: %s used by %q and %q", addr, owner, k.Actor)
		}
		seen[addr] = k.Actor
		addrs = append(addrs, addr)

		// Same key again → same address (recorded stability).
		again, err := alloc.Allocate(k)
		if err != nil {
			t.Fatalf("re-allocate %q: %v", k.Actor, err)
		}
		if again != addr {
			t.Fatalf("unstable allocation for %q: %s vs %s", k.Actor, addr, again)
		}

		got, ok, err := alloc.Lookup(k)
		if err != nil || !ok || got != addr {
			t.Fatalf("lookup %q: got %s ok=%v err=%v want %s", k.Actor, got, ok, err, addr)
		}
	}

	// All under the same device /64 and mesh /48.
	meshP := DeriveMeshPrefix(mesh)
	devP, err := DevicePrefix(meshP, device)
	if err != nil {
		t.Fatal(err)
	}
	for i, addr := range addrs {
		if !meshP.Contains(addr) {
			t.Fatalf("addr %s not in mesh %s", addr, meshP)
		}
		if !devP.Contains(addr) {
			t.Fatalf("addr %s not in device %s", addr, devP)
		}
		iid, err := InterfaceID(addr)
		if err != nil {
			t.Fatal(err)
		}
		if iid < MinActorIID {
			t.Fatalf("actor %d has reserved iid %d", i, iid)
		}
		if addr == mustDeviceHost(t, devP) {
			t.Fatal("actor collided with device host address")
		}
	}

	// Sequential first free: MinActorIID, +1, +2
	for i, addr := range addrs {
		iid, err := InterfaceID(addr)
		if err != nil {
			t.Fatalf("InterfaceID[%d]: %v", i, err)
		}
		want := MinActorIID + uint64(i)
		if iid != want {
			t.Fatalf("iid[%d]=%d want %d", i, iid, want)
		}
	}
}

func TestAllocate_IndependentDevicesAndMeshes(t *testing.T) {
	alloc := NewAllocator(NewMemoryStore())

	a1, err := alloc.Allocate(AllocationKey{Mesh: "m1", Device: 1, Actor: "same-name"})
	if err != nil {
		t.Fatal(err)
	}
	a2, err := alloc.Allocate(AllocationKey{Mesh: "m1", Device: 2, Actor: "same-name"})
	if err != nil {
		t.Fatal(err)
	}
	a3, err := alloc.Allocate(AllocationKey{Mesh: "m2", Device: 1, Actor: "same-name"})
	if err != nil {
		t.Fatal(err)
	}

	if a1 == a2 || a1 == a3 || a2 == a3 {
		t.Fatalf("expected distinct addresses across device/mesh: %s %s %s", a1, a2, a3)
	}

	// Same actor name on different devices may reuse the same interface id
	// (scope is mesh+device); addresses still differ by subnet.
	iid1, err := InterfaceID(a1)
	if err != nil {
		t.Fatal(err)
	}
	iid2, err := InterfaceID(a2)
	if err != nil {
		t.Fatal(err)
	}
	if iid1 != iid2 {
		t.Fatalf("expected parallel iid allocation per device, got %d and %d", iid1, iid2)
	}
}

func TestAllocate_FillsHolesAfterManualPut(t *testing.T) {
	store := NewMemoryStore()
	// Occupy iid 2 and 4; next free for a new actor should be 3, then 5.
	if err := store.Put(AllocationKey{Mesh: "m", Device: 0, Actor: "x"}, 2); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(AllocationKey{Mesh: "m", Device: 0, Actor: "y"}, 4); err != nil {
		t.Fatal(err)
	}

	alloc := NewAllocator(store)
	z, err := alloc.Allocate(AllocationKey{Mesh: "m", Device: 0, Actor: "z"})
	if err != nil {
		t.Fatal(err)
	}
	iid, err := InterfaceID(z)
	if err != nil {
		t.Fatal(err)
	}
	if iid != 3 {
		t.Fatalf("expected hole fill iid 3, got %d", iid)
	}
	w, err := alloc.Allocate(AllocationKey{Mesh: "m", Device: 0, Actor: "w"})
	if err != nil {
		t.Fatal(err)
	}
	iid, err = InterfaceID(w)
	if err != nil {
		t.Fatal(err)
	}
	if iid != 5 {
		t.Fatalf("expected iid 5, got %d", iid)
	}
}

func TestAllocate_ManyActorsNoCollisions(t *testing.T) {
	alloc := NewAllocator(NewMemoryStore())
	const n = 256
	seen := make(map[netip.Addr]struct{}, n)
	for i := 0; i < n; i++ {
		k := AllocationKey{Mesh: "big", Device: 9, Actor: fmt.Sprintf("a-%d", i)}
		addr, err := alloc.Allocate(k)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := seen[addr]; ok {
			t.Fatalf("collision at i=%d addr=%s", i, addr)
		}
		seen[addr] = struct{}{}
	}
	if len(seen) != n {
		t.Fatalf("unique=%d want %d", len(seen), n)
	}
}

func TestAllocate_Validation(t *testing.T) {
	alloc := NewAllocator(NewMemoryStore())
	if _, err := alloc.Allocate(AllocationKey{Mesh: "", Device: 1, Actor: "a"}); err == nil {
		t.Fatal("expected error for empty mesh")
	}
	if _, err := alloc.Allocate(AllocationKey{Mesh: "m", Device: 1, Actor: ""}); err == nil {
		t.Fatal("expected error for empty actor")
	}
	if _, ok, err := alloc.Lookup(AllocationKey{Mesh: "m", Device: 1, Actor: "missing"}); err != nil || ok {
		t.Fatalf("lookup missing: ok=%v err=%v", ok, err)
	}
}

func TestMemoryStore_PutConflicts(t *testing.T) {
	s := NewMemoryStore()
	k1 := AllocationKey{Mesh: "m", Device: 1, Actor: "a"}
	k2 := AllocationKey{Mesh: "m", Device: 1, Actor: "b"}
	if err := s.Put(k1, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(k1, 2); err != nil {
		t.Fatalf("idempotent put: %v", err)
	}
	if err := s.Put(k1, 3); err == nil {
		t.Fatal("expected error rebinding actor")
	}
	if err := s.Put(k2, 2); err == nil {
		t.Fatal("expected error for iid collision")
	}
	if err := s.Put(k2, 1); err == nil {
		t.Fatal("expected error for reserved iid")
	}
}

func TestAllocate_Concurrent(t *testing.T) {
	alloc := NewAllocator(NewMemoryStore())
	const n = 100
	addrs := make([]netip.Addr, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			k := AllocationKey{Mesh: "conc", Device: 0, Actor: fmt.Sprintf("c-%d", i)}
			addr, err := alloc.Allocate(k)
			if err != nil {
				t.Errorf("allocate: %v", err)
				return
			}
			addrs[i] = addr
		}()
	}
	wg.Wait()

	seen := make(map[netip.Addr]struct{}, n)
	for i, addr := range addrs {
		if !addr.IsValid() {
			t.Fatalf("invalid addr at %d", i)
		}
		if _, ok := seen[addr]; ok {
			t.Fatalf("collision on %s", addr)
		}
		seen[addr] = struct{}{}
	}
}

func TestPlan(t *testing.T) {
	mesh, dev, err := Plan(AllocationKey{Mesh: "p", Device: 42, Actor: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if mesh.Bits() != 48 || dev.Bits() != 64 {
		t.Fatalf("plan lengths: %s %s", mesh, dev)
	}
	if !mesh.Contains(dev.Addr()) {
		t.Fatal("device not in mesh")
	}
}

func mustDeviceHost(t *testing.T, dev netip.Prefix) netip.Addr {
	t.Helper()
	a, err := DeviceHostAddress(dev)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
