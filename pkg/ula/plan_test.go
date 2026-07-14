package ula

import (
	"net/netip"
	"testing"
)

func TestDeriveMeshPrefix_ULA48(t *testing.T) {
	p := DeriveMeshPrefix("owner-mesh-1")
	if p.Bits() != 48 {
		t.Fatalf("bits: got /%d want /48", p.Bits())
	}
	if !p.Addr().Is6() {
		t.Fatal("expected IPv6")
	}
	b := p.Addr().As16()
	if b[0] != 0xfd {
		t.Fatalf("not ULA fd00::/8: %s", p)
	}
	// Host bits past /48 must be zero in the prefix address form.
	for i := 6; i < 16; i++ {
		if b[i] != 0 {
			t.Fatalf("non-zero bits past /48 at byte %d: %s", i, p)
		}
	}
}

func TestDeriveMeshPrefix_DeterministicAndDistinct(t *testing.T) {
	a := DeriveMeshPrefix("mesh-a")
	b := DeriveMeshPrefix("mesh-a")
	c := DeriveMeshPrefix("mesh-b")
	if a != b {
		t.Fatalf("same mesh must yield same prefix: %s vs %s", a, b)
	}
	if a == c {
		t.Fatalf("different meshes must yield different prefixes: both %s", a)
	}
}

func TestDevicePrefix_EmbedsDeviceID(t *testing.T) {
	mesh := DeriveMeshPrefix("mesh")
	d0, err := DevicePrefix(mesh, 0)
	if err != nil {
		t.Fatal(err)
	}
	d1, err := DevicePrefix(mesh, 1)
	if err != nil {
		t.Fatal(err)
	}
	d0xABCD, err := DevicePrefix(mesh, 0xABCD)
	if err != nil {
		t.Fatal(err)
	}

	if d0.Bits() != 64 || d1.Bits() != 64 {
		t.Fatalf("want /64, got %s and %s", d0, d1)
	}
	if d0 == d1 {
		t.Fatal("different device ids must differ")
	}

	// Device prefix must be contained in mesh /48.
	if !mesh.Contains(d0.Addr()) || !mesh.Contains(d1.Addr()) {
		t.Fatalf("device not under mesh: mesh=%s d0=%s d1=%s", mesh, d0, d1)
	}

	b := d0xABCD.Addr().As16()
	if b[6] != 0xAB || b[7] != 0xCD {
		t.Fatalf("device id bytes: got %02x%02x want ABCD", b[6], b[7])
	}
	for i := 8; i < 16; i++ {
		if b[i] != 0 {
			t.Fatalf("non-zero interface bits in device prefix at %d", i)
		}
	}
}

func TestDevicePrefix_RejectsBadMesh(t *testing.T) {
	v4 := netip.MustParsePrefix("10.0.0.0/8")
	if _, err := DevicePrefix(v4, 1); err == nil {
		t.Fatal("expected error for IPv4 mesh")
	}
	slash64 := DeriveMeshPrefix("m")
	// Force wrong length by re-prefixing.
	bad := netip.PrefixFrom(slash64.Addr(), 64)
	if _, err := DevicePrefix(bad, 1); err == nil {
		t.Fatal("expected error for non-/48 mesh")
	}
}

func TestHostAndActorAddress(t *testing.T) {
	mesh := DeriveMeshPrefix("mesh")
	dev, err := DevicePrefix(mesh, 7)
	if err != nil {
		t.Fatal(err)
	}

	host, err := DeviceHostAddress(dev)
	if err != nil {
		t.Fatal(err)
	}
	iid, err := InterfaceID(host)
	if err != nil {
		t.Fatal(err)
	}
	if iid != DeviceHostIID {
		t.Fatalf("host iid: got %d want %d", iid, DeviceHostIID)
	}
	if !dev.Contains(host) {
		t.Fatalf("host %s not in device %s", host, dev)
	}

	if _, err := ActorAddress(dev, ReservedIID); err == nil {
		t.Fatal("expected error for reserved iid 0")
	}
	if _, err := ActorAddress(dev, DeviceHostIID); err == nil {
		t.Fatal("expected error for device host iid as actor")
	}

	actor, err := ActorAddress(dev, MinActorIID)
	if err != nil {
		t.Fatal(err)
	}
	if !dev.Contains(actor) {
		t.Fatalf("actor %s not in device %s", actor, dev)
	}
	if actor == host {
		t.Fatal("actor must not collide with device host")
	}

	// High iid encodes in big-endian interface id.
	big, err := ActorAddress(dev, 0x0102030405060708)
	if err != nil {
		t.Fatal(err)
	}
	b := big.As16()
	want := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	for i := 0; i < 8; i++ {
		if b[8+i] != want[i] {
			t.Fatalf("iid encoding mismatch at %d: %s", i, big)
		}
	}
}

func TestPrefixOf(t *testing.T) {
	addr := netip.MustParseAddr("fd12:3456:789a:0001::2")
	p48, err := PrefixOf(addr, 48)
	if err != nil {
		t.Fatal(err)
	}
	if p48.String() != "fd12:3456:789a::/48" {
		t.Fatalf("got %s", p48)
	}
	p64, err := PrefixOf(addr, 64)
	if err != nil {
		t.Fatal(err)
	}
	if p64.String() != "fd12:3456:789a:1::/64" {
		t.Fatalf("got %s", p64)
	}
}
