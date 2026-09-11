package workers

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
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

func TestNodeHTTPServerInstanceof(t *testing.T) {
	runNodeHTTP(t, `
		var http = require("http");
		if (typeof http.Server !== "function") throw new Error("Server");
		if (http.Server.prototype == null) throw new Error("prototype");
		if (({} instanceof http.Server) !== false) throw new Error("plain");
		var s = http.createServer();
		if (typeof s.listen !== "function") throw new Error("listen");
		if (typeof s.prependListener !== "function") throw new Error("prependListener");
	`)
}

func TestNodeHTTPAgent(t *testing.T) {
	runNodeHTTP(t, `
		var http = require("http");
		if (typeof http.Agent !== "function") throw new Error("Agent");
		var a = new http.Agent({ keepAlive: true });
		if (typeof a.on !== "function") throw new Error("on");
		if (!a.options || a.options.keepAlive !== true) throw new Error("options");
		var n = 0;
		a.on("free", function () { n = 1; });
		a.emit("free");
		if (n !== 1) throw new Error("emit " + n);
		var KA = class extends http.Agent {
			constructor(opts) {
				super(opts);
				this.on("free", function () {});
			}
		};
		var k = new KA({ keepAlive: true });
		if (typeof k.on !== "function") throw new Error("sub on");
		if (typeof k.keepSocketAlive !== "function") throw new Error("keepSocketAlive");
		if (k.keepSocketAlive({}) !== true) throw new Error("keep");
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

func TestNodeHTTPAsyncHandler(t *testing.T) {
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
			require("http").createServer(async function (req, res) {
				await new Promise(function (resolve) { setTimeout(resolve, 20); });
				res.end("later");
			}).listen(0, "127.0.0.1");
		`, "t.js")
	}()
	var resp *http.Response
	for i := 0; i < 50; i++ {
		resp, err = http.Get("http://" + addr + "/")
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
	if string(body) != "later" {
		t.Fatalf("body %q", body)
	}
	cancel()
	<-errc
}

func TestNodeHTTPRequestWithDueTimers(t *testing.T) {
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
			setInterval(function () {}, 0);
			require("http").createServer(function (req, res) {
				res.appendHeader("X-Test", "1");
				res.end("ok");
			}).listen(0, "127.0.0.1");
		`, "t.js")
	}()
	var resp *http.Response
	for i := 0; i < 50; i++ {
		resp, err = http.Get("http://" + addr + "/")
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
	if resp.Header.Get("X-Test") != "1" {
		t.Fatalf("headers %v", resp.Header)
	}
	cancel()
	<-errc
}

func TestNodeHTTPHostHeader(t *testing.T) {
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
				res.end(String(req.headers.host || ""));
			}).listen(0, "127.0.0.1");
		`, "t.js")
	}()
	var resp *http.Response
	for i := 0; i < 50; i++ {
		resp, err = http.Get("http://" + addr + "/")
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
	if string(body) == "" {
		t.Fatal("missing host header")
	}
	cancel()
	<-errc
}

func TestNodeHTTPUpgrade(t *testing.T) {
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
				res.end("no");
			}).on("upgrade", function (req, socket, head) {
				if (!socket.readable || !socket.writable) throw new Error("flags");
				if (String(req.headers.upgrade).toLowerCase() !== "websocket") throw new Error("hdr");
				if (!head || typeof head.length !== "number") throw new Error("head");
				if (typeof socket.removeListener !== "function") throw new Error("removeListener");
				if (typeof socket.cork !== "function" || typeof socket.uncork !== "function") throw new Error("cork");
				var n = 0;
				function onerr() { n++; }
				socket.on("error", onerr);
				socket.removeListener("error", onerr);
				socket.emit("error");
				if (n !== 0) throw new Error("error still bound");
				socket.write("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n");
			}).listen(0, "127.0.0.1");
		`, "t.js")
	}()
	var conn net.Conn
	for i := 0; i < 50; i++ {
		conn, err = net.Dial("tcp", addr)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	raw := "GET /?token=x HTTP/1.1\r\nHost: " + addr + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Protocol: vite-hmr\r\n\r\n"
	if _, err := conn.Write([]byte(raw)); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	got := string(buf[:n])
	if !strings.Contains(got, "101") {
		t.Fatalf("upgrade resp %q", got)
	}
	cancel()
	<-errc
}

func TestNodeHTTPSocketData(t *testing.T) {
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
				res.end("no");
			}).on("upgrade", function (req, socket) {
				socket.on("data", function (chunk) {
					socket.write(chunk);
				});
				socket.write("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n");
			}).listen(0, "127.0.0.1");
		`, "t.js")
	}()
	var conn net.Conn
	for i := 0; i < 50; i++ {
		conn, err = net.Dial("tcp", addr)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	raw := "GET / HTTP/1.1\r\nHost: " + addr + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"
	if _, err := conn.Write([]byte(raw)); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(buf[:n]), "101") {
		t.Fatalf("upgrade resp %q", buf[:n])
	}
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	n, err = conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if echo := string(buf[:n]); echo != "ping" {
		t.Fatalf("echo %q", echo)
	}
	cancel()
	<-errc
}

func TestNodeHTTPStreamPipe(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	iso := New("", Options{
		Imports: NodeScriptImports(),
		FS:      nodeFSMap(),
		Listen: func(ctx context.Context, req ListenReq) (net.Listener, error) {
			return ln, nil
		},
	})
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	errc := make(chan error, 1)
	go func() {
		errc <- iso.ScriptMain(ctx, `
			var fs = require("fs");
			require("http").createServer(function (req, res) {
				fs.createReadStream("hello.txt?v=hash").pipe(res);
			}).listen(0, "127.0.0.1");
		`, "t.js")
	}()
	var resp *http.Response
	for i := 0; i < 50; i++ {
		resp, err = http.Get("http://" + addr + "/")
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
	if string(body) != "hi" {
		t.Fatalf("body %q", body)
	}
	cancel()
	<-errc
}
