package workers

import (
	"context"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func TestHeadersGetSetHasDeleteCaseInsensitive(t *testing.T) {
	iso := New(``, Options{})
	if _, err := iso.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}

	v, err := iso.vm.RunString(`
		var h = new Headers();
		h.set("Content-Type", "text/html");
		h.set("X-Foo", "a");
		h.append("X-Foo", "b");
		({
			getCT: h.get("content-type"),
			getFoo: h.get("x-foo"),
			hasCT: h.has("CONTENT-TYPE"),
			missing: h.get("nope"),
			hasMissing: h.has("nope")
		});
	`)
	if err != nil {
		t.Fatal(err)
	}
	obj := v.ToObject(iso.vm)
	if got := obj.Get("getCT").String(); got != "text/html" {
		t.Fatalf("content-type=%q", got)
	}
	if got := obj.Get("getFoo").String(); got != "a, b" {
		t.Fatalf("x-foo=%q, want combined", got)
	}
	if !obj.Get("hasCT").ToBoolean() {
		t.Fatal("has content-type")
	}
	if !goja.IsNull(obj.Get("missing")) {
		t.Fatalf("missing get should be null, got %v", obj.Get("missing"))
	}
	if obj.Get("hasMissing").ToBoolean() {
		t.Fatal("has missing should be false")
	}

	if _, err = iso.vm.RunString(`h.delete("content-type"); globalThis.afterDel = h.get("Content-Type");`); err != nil {
		t.Fatal(err)
	}
	if !goja.IsNull(iso.vm.Get("afterDel")) {
		t.Fatalf("after delete: %v", iso.vm.Get("afterDel"))
	}
}

func TestHeadersFromRecord(t *testing.T) {
	iso := New(``, Options{})
	if _, err := iso.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	v, err := iso.vm.RunString(`
		var h = new Headers({"X-A": "1", "X-B": "2"});
		h.get("x-a") + ":" + h.get("x-b");
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != "1:2" {
		t.Fatalf("got %q", got)
	}
}

func TestRequestConstructAndRead(t *testing.T) {
	iso := New(``, Options{})
	if _, err := iso.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}

	v, err := iso.vm.RunString(`
		var req = new Request("https://example.test/path", {
			method: "post",
			headers: {"Content-Type": "application/json"},
			body: "{\"n\":1}"
		});
		({
			method: req.method,
			url: req.url,
			ct: req.headers.get("content-type")
		});
	`)
	if err != nil {
		t.Fatal(err)
	}
	obj := v.ToObject(iso.vm)
	if got := obj.Get("method").String(); got != "POST" {
		t.Fatalf("method=%q", got)
	}
	if got := obj.Get("url").String(); got != "https://example.test/path" {
		t.Fatalf("url=%q", got)
	}
	if got := obj.Get("ct").String(); got != "application/json" {
		t.Fatalf("ct=%q", got)
	}

	// text() promise resolves; then microtasks drain at end of RunString.
	if _, err = iso.vm.RunString(`
		var gotBody = null;
		var req = new Request("https://example.test/", { method: "PUT", body: "hello" });
		req.text().then(function(t) { gotBody = t; });
	`); err != nil {
		t.Fatal(err)
	}
	if got := iso.vm.Get("gotBody").String(); got != "hello" {
		t.Fatalf("body via text()=%q", got)
	}
}

func TestResponseConstructAndHostRead(t *testing.T) {
	iso := New(``, Options{})
	if _, err := iso.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}

	v, err := iso.vm.RunString(`
		new Response("<html>ok</html>", {
			status: 201,
			statusText: "Created",
			headers: {"Content-Type": "text/html"}
		});
	`)
	if err != nil {
		t.Fatal(err)
	}

	got, err := iso.ReadResponse(v)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != 201 {
		t.Fatalf("status=%d", got.Status)
	}
	if got.StatusText != "Created" {
		t.Fatalf("statusText=%q", got.StatusText)
	}
	if got.Body != "<html>ok</html>" {
		t.Fatalf("body=%q", got.Body)
	}
	if got.Headers["content-type"] != "text/html" {
		t.Fatalf("headers=%v", got.Headers)
	}

	ok, err := iso.vm.RunString(`
		var r = new Response("x", {status: 404});
		r.ok;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if ok.ToBoolean() {
		t.Fatal("404 should not be ok")
	}
}

func TestHostMakeRequestReadableByJS(t *testing.T) {
	iso := New(`
		globalThis.handler = function(req) {
			return {
				method: req.method,
				url: req.url,
				auth: req.headers.get("authorization"),
			};
		};
	`, Options{})
	if _, err := iso.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}

	reqObj, err := iso.MakeRequest(HTTPRequest{
		Method: "GET",
		URL:    "http://[fd00::1]/",
		Headers: map[string]string{
			"Authorization": "Bearer z",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	handler, ok := goja.AssertFunction(iso.vm.Get("handler"))
	if !ok {
		t.Fatal("handler missing")
	}
	result, err := handler(goja.Undefined(), iso.vm.ToValue(reqObj))
	if err != nil {
		t.Fatal(err)
	}
	obj := result.ToObject(iso.vm)
	if got := obj.Get("method").String(); got != "GET" {
		t.Fatalf("method=%q", got)
	}
	if got := obj.Get("url").String(); got != "http://[fd00::1]/" {
		t.Fatalf("url=%q", got)
	}
	if got := obj.Get("auth").String(); got != "Bearer z" {
		t.Fatalf("auth=%q", got)
	}
}

func TestJSResponseRoundTripViaHost(t *testing.T) {
	iso := New(`
		globalThis.handle = function(req) {
			return new Response("hello " + req.method, {
				status: 200,
				headers: {"X-Echo-URL": req.url}
			});
		};
	`, Options{})
	if _, err := iso.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}

	reqObj, err := iso.MakeRequest(HTTPRequest{
		Method: "GET",
		URL:    "http://actor.test/",
	})
	if err != nil {
		t.Fatal(err)
	}

	handle, ok := goja.AssertFunction(iso.vm.Get("handle"))
	if !ok {
		t.Fatal("handle missing")
	}
	resVal, err := handle(goja.Undefined(), iso.vm.ToValue(reqObj))
	if err != nil {
		t.Fatal(err)
	}

	got, err := iso.ReadResponse(resVal)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != 200 || got.Body != "hello GET" {
		t.Fatalf("got %+v", got)
	}
	if got.Headers["x-echo-url"] != "http://actor.test/" {
		t.Fatalf("headers=%v", got.Headers)
	}
}

func TestReadResponseRejectsNonResponse(t *testing.T) {
	iso := New(``, Options{})
	if _, err := iso.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	v, err := iso.vm.RunString(`({status: 200, body: "nope"})`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = iso.ReadResponse(v)
	if err == nil {
		t.Fatal("expected error for plain object")
	}
	if !strings.Contains(err.Error(), "Response") {
		t.Fatalf("unexpected err: %v", err)
	}
}

