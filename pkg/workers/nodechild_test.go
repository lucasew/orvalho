package workers

import (
	"context"
	"testing"
)

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
