package workers

import (
	"context"
	"io"
	"testing"

	"github.com/lucasew/orvalho/pkg/workers/bundle"
)

func TestNodeChildPromisifyExecFile(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports(), PrepareSource: bundle.TransformCJS})
	err := iso.ScriptMain(t.Context(), `
		import childProcess, { exec, execFile, execSync } from "node:child_process";
		import { promisify } from "node:util";
		if (typeof execFile !== "function") throw new Error("named " + typeof execFile);
		if (typeof childProcess.execFile !== "function") throw new Error("ns " + typeof childProcess.execFile);
		var execFileAsync = promisify(childProcess.execFile);
		if (typeof execFileAsync !== "function") throw new Error("async");
	`, "t.mjs")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeChildPromisifyExecFileResult(t *testing.T) {
	done := make(chan SpawnWait, 1)
	done <- SpawnWait{Code: 0}
	iso := New("", Options{
		Imports:       NodeScriptImports(),
		PrepareSource: bundle.TransformCJS,
		Spawn: func(ctx context.Context, req SpawnReq) (Spawned, error) {
			return stubSpawned{pid: 3, done: done}, nil
		},
	})
	err := iso.ScriptMain(t.Context(), `
		import { execFile } from "node:child_process";
		import { promisify } from "node:util";
		var saw;
		promisify(execFile)("echo").then(function (r) {
			if (!r || typeof r !== "object") throw new Error("shape " + r);
			if (r.stdout !== "") throw new Error("stdout " + r.stdout);
			if (r.stderr !== "") throw new Error("stderr " + r.stderr);
			saw = true;
		});
		setTimeout(function () {
			if (!saw) throw new Error("no result");
		}, 0);
	`, "t.mjs")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeChildPropNames(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var cp = require("child_process");
		var names = Object.getOwnPropertyNames(cp);
		if (names.indexOf("execFile") < 0) throw new Error("names " + names);
		var d = Object.getOwnPropertyDescriptor(cp, "execFile");
		if (!d || typeof d.value !== "function") throw new Error("desc " + d);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeChildESMExecFile(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports(), PrepareSource: bundle.TransformCJS})
	err := iso.ScriptMain(t.Context(), `
		import cp from "node:child_process";
		if (typeof cp.execFile !== "function") throw new Error("default.execFile " + typeof cp.execFile + " keys " + Object.getOwnPropertyNames(cp));
		import { execFile } from "node:child_process";
		if (typeof execFile !== "function") throw new Error("named execFile");
	`, "t.mjs")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeChildIdentity(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var cp = require("child_process");
		if (require("node:child_process") !== cp) throw new Error("identity");
		if (typeof cp.spawn !== "function") throw new Error("spawn");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeChildSpawnDenied(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var cp = require("child_process");
		try {
			cp.spawn("echo", ["hi"]);
			throw new Error("should throw");
		} catch (e) {
			if (e.code !== "EPERM") throw new Error("code " + e.code);
		}
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeChildSpawnInjected(t *testing.T) {
	done := make(chan SpawnWait, 1)
	done <- SpawnWait{Code: 0}
	var got SpawnReq
	iso := New("", Options{
		Imports: NodeScriptImports(),
		Cwd:     "/guest",
		Spawn: func(ctx context.Context, req SpawnReq) (Spawned, error) {
			got = req
			return stubSpawned{pid: 7, done: done}, nil
		},
	})
	err := iso.ScriptMain(t.Context(), `
		var saw = false;
		var child = require("child_process").spawn("echo", ["hi"]);
		if (child.pid !== 7) throw new Error("pid " + child.pid);
		if (typeof child.unref !== "function") throw new Error("unref");
		child.unref();
		if (typeof child.stdout.pipe !== "function") throw new Error("pipe");
		child.stdout.pipe({});
		child.stderr.pipe({});
		child.on("exit", function (code) {
			if (code !== 0) throw new Error("code " + code);
			saw = true;
		});
		setTimeout(function () {
			if (!saw) throw new Error("no exit");
		}, 0);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
	if got.File != "echo" || len(got.Args) != 1 || got.Args[0] != "hi" {
		t.Fatalf("req %+v", got)
	}
	if got.Cwd != "/guest" {
		t.Fatalf("cwd %q", got.Cwd)
	}
}

func TestNodeChildSpawnExecErrorIsEvent(t *testing.T) {
	iso := New("", Options{
		Imports: NodeScriptImports(),
		Spawn: func(ctx context.Context, req SpawnReq) (Spawned, error) {
			return nil, ErrSpawnDenied
		},
	})
	err := iso.ScriptMain(t.Context(), `
		var saw = false;
		var child = require("child_process").spawn("/dev/null");
		child.on("error", function (e) {
			if (!e) throw new Error("no err");
			saw = true;
		});
		setTimeout(function () {
			if (!saw) throw new Error("no error event");
		}, 0);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeChildStdioPipe(t *testing.T) {
	iso := New("", Options{
		Imports: NodeScriptImports(),
		Spawn: func(ctx context.Context, req SpawnReq) (Spawned, error) {
			if req.Stdin == nil || req.Stdout == nil {
				t.Fatal("missing stdio pipes")
			}
			done := make(chan SpawnWait, 1)
			go func() {
				_, _ = io.Copy(req.Stdout, req.Stdin)
				if c, ok := req.Stdout.(io.Closer); ok {
					_ = c.Close()
				}
				done <- SpawnWait{Code: 0}
			}()
			return stubSpawned{pid: 9, done: done}, nil
		},
	})
	err := iso.ScriptMain(t.Context(), `
		var child = require("child_process").spawn("cat", []);
		if (typeof child.stdin.write !== "function") throw new Error("write");
		if (child.stdin === child.stdout) throw new Error("shared stdio");
		var got = [];
		var ended = false;
		child.stdout.on("data", function (chunk) {
			for (var i = 0; i < chunk.length; i++) got.push(chunk[i]);
		});
		child.stdout.on("end", function () { ended = true; });
		var payload = new Uint8Array([1, 2, 3, 4]);
		child.stdin.write(payload, function () {
			child.stdin.end();
		});
		setTimeout(function () {
			if (got.join(",") !== "1,2,3,4") throw new Error("got " + got);
			if (!ended) throw new Error("no end");
		}, 20);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeChildSpawnArgType(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		try {
			require("child_process").spawn();
			throw new Error("should throw");
		} catch (e) {
			if (e.code !== "ERR_INVALID_ARG_TYPE") throw new Error("code " + e.code);
		}
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

type stubSpawned struct {
	pid  int
	done chan SpawnWait
}

func (s stubSpawned) PID() int               { return s.pid }
func (s stubSpawned) Done() <-chan SpawnWait { return s.done }
func (s stubSpawned) Kill() error            { return nil }
