package workers

import (
	"context"
	"net"
	"testing"
)

func runNodeNet(t *testing.T, src string) {
	t.Helper()
	iso := New("", Options{Imports: NodeScriptImports()})
	if err := iso.ScriptMain(t.Context(), src, "t.js"); err != nil {
		t.Fatal(err)
	}
}

func TestNodeNetIdentity(t *testing.T) {
	runNodeNet(t, `
		var net = require("net");
		if (require("node:net") !== net) throw new Error("identity");
		if (typeof net.isIP !== "function") throw new Error("isIP");
		if (typeof net.connect !== "function") throw new Error("connect");
		if (typeof net.createServer !== "function") throw new Error("createServer");
	`)
}

func TestNodeNetConnectDenied(t *testing.T) {
	runNodeNet(t, `
		try {
			require("net").connect({ port: 80, host: "127.0.0.1" });
			throw new Error("should throw");
		} catch (e) {
			if (e.code !== "EPERM") throw new Error("code " + e.code);
		}
	`)
}

func TestNodeNetDialInjected(t *testing.T) {
	var got DialReq
	c1, c2 := net.Pipe()
	t.Cleanup(func() { c1.Close(); c2.Close() })
	iso := New("", Options{
		Imports: NodeScriptImports(),
		Dial: func(ctx context.Context, req DialReq) (net.Conn, error) {
			got = req
			return c1, nil
		},
	})
	err := iso.ScriptMain(t.Context(), `
		var saw = false;
		var sock = require("net").connect({ port: 9, host: "example.test" });
		sock.on("connect", function () { saw = true; });
		setTimeout(function () {
			if (!saw) throw new Error("no connect");
		}, 0);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
	if got.Network != "tcp" {
		t.Fatalf("network %q", got.Network)
	}
	if got.Address != "example.test:9" {
		t.Fatalf("address %q", got.Address)
	}
}

func TestNodeNetIsIP(t *testing.T) {
	runNodeNet(t, `
		var net = require("net");
		if (net.isIP("127.0.0.1") !== 4) throw new Error("v4");
		if (net.isIP("::1") !== 6) throw new Error("v6");
		if (net.isIP("example.com") !== 0) throw new Error("name");
		if (net.isIPv4("8.8.8.8") !== true) throw new Error("isIPv4");
		if (net.isIPv6("fe80::1") !== true) throw new Error("isIPv6");
	`)
}
