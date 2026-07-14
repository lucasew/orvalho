package ula

import (
	"net/netip"
)

// Allocator records stable actor /128 addresses under a mesh ULA plan.
// Pure prefix math lives in plan.go; this type only asks Store for an
// interface id and maps it to an address.
type Allocator struct {
	store Store
}

// NewAllocator wraps store. store must be non-nil.
func NewAllocator(store Store) *Allocator {
	if store == nil {
		panic("ula: nil store")
	}
	return &Allocator{store: store}
}

// Allocate returns a stable IPv6 /128 for key.
//
// If key was allocated before, the same address is returned (recorded
// stability). Otherwise Store.ClaimNext assigns the smallest free interface
// id >= MinActorIID under the device /64.
func (a *Allocator) Allocate(key AllocationKey) (netip.Addr, error) {
	if err := key.validate(); err != nil {
		return netip.Addr{}, err
	}

	iid, err := a.store.ClaimNext(key, MinActorIID)
	if err != nil {
		return netip.Addr{}, err
	}
	return a.addressFor(key, iid)
}

// Lookup returns a previously allocated address without creating one.
func (a *Allocator) Lookup(key AllocationKey) (netip.Addr, bool, error) {
	if err := key.validate(); err != nil {
		return netip.Addr{}, false, err
	}
	iid, ok, err := a.store.Get(key)
	if err != nil || !ok {
		return netip.Addr{}, ok, err
	}
	addr, err := a.addressFor(key, iid)
	if err != nil {
		return netip.Addr{}, false, err
	}
	return addr, true, nil
}

// Plan returns mesh /48 and device /64 for key (pure derivation; no storage).
func Plan(key AllocationKey) (mesh netip.Prefix, device netip.Prefix, err error) {
	if err := key.validate(); err != nil {
		return netip.Prefix{}, netip.Prefix{}, err
	}
	mesh = DeriveMeshPrefix(key.Mesh)
	device, err = DevicePrefix(mesh, key.Device)
	return mesh, device, err
}

func (a *Allocator) addressFor(key AllocationKey, iid uint64) (netip.Addr, error) {
	mesh := DeriveMeshPrefix(key.Mesh)
	device, err := DevicePrefix(mesh, key.Device)
	if err != nil {
		return netip.Addr{}, err
	}
	return ActorAddress(device, iid)
}
