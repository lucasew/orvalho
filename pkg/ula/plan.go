// Package ula implements pure Unique Local Address (ULA) IPv6 plan math and
// a small recorded allocator for Orvalho actor addresses.
//
// Address hierarchy (RFC 4193 locally-assigned ULA under fd00::/8):
//
//	| 8 bits | 40 bits   | 16 bits    | 64 bits        |
//	| 0xfd   | Global ID | Subnet ID  | Interface ID   |
//	|-------- mesh /48 --------------|-- device /64 --|--- actor /128 ---|
//
// Mesh Global ID is derived deterministically from mesh/owner context.
// Device uses a 16-bit subnet id. Actors receive stable /128 addresses via
// recorded interface IDs (see Allocator + Store). No WireGuard or HTTP here.
package ula

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net/netip"
)

// Interface ID conventions under a device /64.
const (
	// ReservedIID is never allocated (all-zero host id).
	ReservedIID = 0
	// DeviceHostIID is reserved for the worker/device host on the overlay.
	DeviceHostIID = 1
	// MinActorIID is the first interface ID available for actors.
	MinActorIID = 2
)

// DeriveMeshPrefix returns a ULA /48 for the mesh/owner context.
// The 40-bit Global ID is SHA-256(meshContext)[:5] with the fd prefix bit set
// so the result is always inside fd00::/8.
func DeriveMeshPrefix(meshContext string) netip.Prefix {
	sum := sha256.Sum256([]byte(meshContext))
	var b [16]byte
	b[0] = 0xfd
	copy(b[1:6], sum[:5])
	addr := netip.AddrFrom16(b)
	return netip.PrefixFrom(addr, 48)
}

// DevicePrefix embeds deviceID as the 16-bit subnet id under a mesh /48,
// producing a device /64.
func DevicePrefix(mesh netip.Prefix, deviceID uint16) (netip.Prefix, error) {
	if !mesh.IsValid() || !mesh.Addr().Is6() {
		return netip.Prefix{}, ErrMeshMustIPv6
	}
	if mesh.Bits() != 48 {
		return netip.Prefix{}, fmt.Errorf("%w: mesh /48 required, got /%d", ErrMeshMustIPv6, mesh.Bits())
	}
	if !isULA(mesh.Addr()) {
		return netip.Prefix{}, fmt.Errorf("%w: mesh prefix %s is not ULA (fd00::/8)", ErrMeshMustIPv6, mesh)
	}
	b := mesh.Addr().As16()
	// Clear host bits past /48 before writing subnet.
	for i := 6; i < 16; i++ {
		b[i] = 0
	}
	b[6] = byte(deviceID >> 8)
	b[7] = byte(deviceID)
	return netip.PrefixFrom(netip.AddrFrom16(b), 64), nil
}

// HostAddress returns the /128 for a given interface ID under a device /64.
func HostAddress(device netip.Prefix, iid uint64) (netip.Addr, error) {
	if !device.IsValid() || !device.Addr().Is6() {
		return netip.Addr{}, ErrDeviceMustIPv6
	}
	if device.Bits() != 64 {
		return netip.Addr{}, fmt.Errorf("%w: /64 required, got /%d", ErrDeviceMustIPv6, device.Bits())
	}
	b := device.Addr().As16()
	// Zero interface id then write iid.
	for i := 8; i < 16; i++ {
		b[i] = 0
	}
	binary.BigEndian.PutUint64(b[8:], iid)
	return netip.AddrFrom16(b), nil
}

// DeviceHostAddress is the reserved /128 for the worker on a device /64.
func DeviceHostAddress(device netip.Prefix) (netip.Addr, error) {
	return HostAddress(device, DeviceHostIID)
}

// ActorAddress is the /128 for an actor interface ID under a device /64.
// iid must be >= MinActorIID.
func ActorAddress(device netip.Prefix, iid uint64) (netip.Addr, error) {
	if iid < MinActorIID {
		return netip.Addr{}, fmt.Errorf("%w: actor interface id %d (min %d)", ErrReservedIID, iid, MinActorIID)
	}
	return HostAddress(device, iid)
}

// InterfaceID extracts the 64-bit interface identifier from an IPv6 address.
func InterfaceID(addr netip.Addr) (uint64, error) {
	if !addr.IsValid() || !addr.Is6() {
		return 0, ErrNeedIPv6
	}
	b := addr.As16()
	return binary.BigEndian.Uint64(b[8:]), nil
}

// PrefixOf returns the /n containing addr (addr must be IPv6).
func PrefixOf(addr netip.Addr, bits int) (netip.Prefix, error) {
	if !addr.IsValid() || !addr.Is6() {
		return netip.Prefix{}, ErrNeedIPv6
	}
	if bits < 0 || bits > 128 {
		return netip.Prefix{}, fmt.Errorf("%w: invalid prefix length %d", ErrNeedIPv6, bits)
	}
	p := netip.PrefixFrom(addr, bits)
	return p.Masked(), nil
}

func isULA(addr netip.Addr) bool {
	if !addr.Is6() {
		return false
	}
	b := addr.As16()
	return b[0] == 0xfd
}
