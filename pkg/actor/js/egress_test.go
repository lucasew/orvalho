package js

import "testing"

func TestEgressExactHost(t *testing.T) {
	e := EgressList{"catfact.ninja"}
	if !e.Allows("https://catfact.ninja/fact") {
		t.Fatal("expected allow")
	}
	if e.Allows("https://evil.example/fact") {
		t.Fatal("expected deny")
	}
	if e.Allows("http://catfact.ninja/fact") {
		// hostname rule allows any scheme http/https
	} else {
		t.Fatal("hostname rule should allow http too")
	}
}

func TestEgressWildcard(t *testing.T) {
	e := EgressList{"*.example.com"}
	if !e.Allows("https://a.example.com/x") {
		t.Fatal("subdomain")
	}
	if !e.Allows("https://example.com/x") {
		t.Fatal("apex convenience")
	}
	if e.Allows("https://example.org/x") {
		t.Fatal("other domain")
	}
}

func TestEgressOrigin(t *testing.T) {
	e := EgressList{"https://catfact.ninja"}
	if !e.Allows("https://catfact.ninja/fact") {
		t.Fatal("same origin")
	}
	if e.Allows("http://catfact.ninja/fact") {
		t.Fatal("scheme mismatch")
	}
}

func TestEgressEmptyDenies(t *testing.T) {
	var e EgressList
	if e.Allows("https://catfact.ninja/fact") {
		t.Fatal("empty allowlist must deny")
	}
	if err := e.CheckURL("https://catfact.ninja/fact"); err == nil {
		t.Fatal("expected error")
	}
}

func TestEgressRejectsNonHTTP(t *testing.T) {
	e := EgressList{"example.com"}
	if e.Allows("file:///etc/passwd") {
		t.Fatal("file")
	}
	if e.Allows("ftp://example.com/x") {
		t.Fatal("ftp")
	}
}
