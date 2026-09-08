package workers

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func runNodeHTTP(t *testing.T, src string) {
	t.Helper()
	iso := New("", Options{Imports: NodeScriptImports()})
	if err := iso.ScriptMain(t.Context(), src, "t.js"); err != nil {
		t.Fatal(err)
	}
}

func TestNodeHTTPIdentity(t *testing.T) {
	runNodeHTTP(t, `
		var http = require("http");
		if (require("node:http") !== http) throw new Error("identity");
		if (http.STATUS_CODES["200"] !== "OK") throw new Error("STATUS_CODES");
		if (typeof http.createServer !== "function") throw new Error("createServer");
		if (require("https").STATUS_CODES["404"] !== "Not Found") throw new Error("https");
	`)
}

func TestNodeHTTPListenDenied(t *testing.T) {
	runNodeHTTP(t, `
		try {
			require("http").createServer().listen(0);
			throw new Error("should throw");
		} catch (e) {
			if (e.code !== "EPERM") throw new Error("code " + e.code);
		}
	`)
}

func TestNodeHTTPListenClose(t *testing.T) {
	iso := New("", Options{
		Imports: NodeScriptImports(),
		Listen: func(ctx context.Context, req ListenReq) (net.Listener, error) {
			return net.Listen("tcp", "127.0.0.1:0")
		},
	})
	err := iso.ScriptMain(t.Context(), `
		var s = require("http").createServer();
		s.listen(0, "127.0.0.1", function () {
			var a = s.address();
			if (!a || !a.port) throw new Error("address");
			s.close();
		});
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeHTTPRequest(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	iso := New("", Options{
		Imports: NodeScriptImports(),
		Listen: func(ctx context.Context, req ListenReq) (net.Listener, error) {
			return ln, nil
		},
	})
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	errc := make(chan error, 1)
	go func() {
		errc <- iso.ScriptMain(ctx, `
			require("http").createServer(function (req, res) {
				res.writeHead(200, { "Content-Type": "text/plain" });
				res.end("ok");
			}).listen(0, "127.0.0.1");
		`, "t.js")
	}()
	var resp *http.Response
	for i := 0; i < 50; i++ {
		resp, err = http.Get("http://" + addr + "/x")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Fatalf("body %q", body)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	cancel()
	<-errc
}
