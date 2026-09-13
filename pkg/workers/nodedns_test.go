package workers

import (
	"context"
	"testing"
)

func runNodeDNS(t *testing.T, src string) {
	t.Helper()
	iso := New("", Options{Imports: NodeScriptImports()})
	if err := iso.ScriptMain(t.Context(), src, "t.js"); err != nil {
		t.Fatal(err)
	}
}

func TestNodeDNSIdentity(t *testing.T) {
	runNodeDNS(t, `
		var dns = require("dns");
		if (require("node:dns") !== dns) throw new Error("identity");
		if (require("dns/promises") !== dns.promises) throw new Error("promises");
		if (require("node:dns/promises") !== dns.promises) throw new Error("node:promises");
		if (typeof dns.lookup !== "function") throw new Error("lookup");
		if (typeof dns.promises.lookup !== "function") throw new Error("promises.lookup");
		if (dns.NODATA !== "ENODATA") throw new Error("NODATA");
		if (dns.promises.CANCELLED !== dns.CANCELLED) throw new Error("CANCELLED");
		if (dns.default !== dns) throw new Error("default");
	`)
}

func TestNodeDNSPromisesFirst(t *testing.T) {
	runNodeDNS(t, `
		var p = require("dns/promises");
		var dns = require("dns");
		if (p !== dns.promises) throw new Error("promises first");
	`)
}

func TestNodeDNSLookupIP(t *testing.T) {
	runNodeDNS(t, `
		var dns = require("dns");
		var got = "";
		var fam = 0;
		dns.lookup("127.0.0.1", function (err, address, family) {
			if (err) throw err;
			got = address;
			fam = family;
		});
		setTimeout(function () {
			if (got !== "127.0.0.1") throw new Error("addr " + got);
			if (fam !== 4) throw new Error("fam " + fam);
		}, 0);
	`)
}

func TestNodeDNSLookupDenied(t *testing.T) {
	runNodeDNS(t, `
		var code = "";
		require("dns").lookup("localhost", function (err) {
			if (!err) throw new Error("should err");
			code = err.code;
		});
		setTimeout(function () {
			if (code !== "EPERM") throw new Error("code " + code);
		}, 0);
	`)
}

func TestNodeDNSLookupInjected(t *testing.T) {
	var got LookupReq
	iso := New("", Options{
		Imports: NodeScriptImports(),
		Lookup: func(ctx context.Context, req LookupReq) ([]LookupAddr, error) {
			got = req
			return []LookupAddr{{Address: "127.0.0.1", Family: 4}}, nil
		},
	})
	err := iso.ScriptMain(t.Context(), `
		var addr = "";
		var fam = 0;
		require("dns").promises.lookup("example.test", { verbatim: true }).then(function (r) {
			addr = r.address;
			fam = r.family;
		});
		setTimeout(function () {
			if (addr !== "127.0.0.1") throw new Error("addr " + addr);
			if (fam !== 4) throw new Error("fam " + fam);
		}, 0);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
	if got.Hostname != "example.test" {
		t.Fatalf("hostname %q", got.Hostname)
	}
	if got.Network != "ip" {
		t.Fatalf("network %q", got.Network)
	}
}

func TestNodeDNSResultOrder(t *testing.T) {
	runNodeDNS(t, `
		var dns = require("dns");
		if (dns.getDefaultResultOrder() !== "verbatim") throw new Error("default");
		dns.setDefaultResultOrder("ipv4first");
		if (dns.getDefaultResultOrder() !== "ipv4first") throw new Error("set");
		if (dns.promises.getDefaultResultOrder() !== "ipv4first") throw new Error("shared");
		try {
			dns.setDefaultResultOrder("my_order");
			throw new Error("should throw");
		} catch (e) {
			if (e.code !== "ERR_INVALID_ARG_VALUE") throw new Error("code " + e.code);
		}
	`)
}

func TestNodeDNSPromisesLookupDenied(t *testing.T) {
	runNodeDNS(t, `
		var code = "";
		require("dns").promises.lookup("localhost").then(function () {
			throw new Error("should reject");
		}, function (e) {
			code = e.code;
		});
		setTimeout(function () {
			if (code !== "EPERM") throw new Error("code " + code);
		}, 0);
	`)
}
