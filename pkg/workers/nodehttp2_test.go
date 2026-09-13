package workers

import "testing"

func TestNodeHTTP2Http2ServerResponseInstanceof(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var http2 = require("http2");
		if (require("node:http2") !== http2) throw new Error("identity");
		if (typeof http2.Http2ServerResponse !== "function") throw new Error("Http2ServerResponse");
		if (typeof http2.Http2ServerRequest !== "function") throw new Error("Http2ServerRequest");
		if (({} instanceof http2.Http2ServerResponse) !== false) throw new Error("plain");
		var res = { statusMessage: "" };
		if (!(res instanceof http2.Http2ServerResponse)) {
			res.statusMessage = "OK";
		}
		if (res.statusMessage !== "OK") throw new Error("statusMessage");
		var web = new Response("hi", { status: 200, statusText: "OK", headers: { "content-type": "text/plain" } });
		if (!(res instanceof http2.Http2ServerResponse)) {
			res.statusMessage = web.statusText;
		}
		if (res.statusMessage !== "OK") throw new Error("web statusText");
		var h = Object.fromEntries(web.headers.entries());
		if (h["content-type"] !== "text/plain") throw new Error("entries " + JSON.stringify(h));
		if (typeof web.headers.getSetCookie !== "function") throw new Error("getSetCookie");
		if (!Array.isArray(web.headers.getSetCookie())) throw new Error("getSetCookie arr");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}
