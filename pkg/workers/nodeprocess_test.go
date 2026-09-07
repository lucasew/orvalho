package workers

import "testing"

func TestNodeProcessIdentity(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var p = require("process");
		if (p !== process) throw new Error("require(process) !== process");
		if (require("node:process") !== process) throw new Error("node:process");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeProcessEnvNotHost(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		if (process.env.PATH) throw new Error("leaked PATH");
		if (process.env.HOME) throw new Error("leaked HOME");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeProcessEnvInjected(t *testing.T) {
	iso := New("", Options{
		Imports:    NodeScriptImports(),
		ProcessEnv: map[string]string{"FOO": "bar"},
	})
	err := iso.ScriptMain(t.Context(), `
		if (process.env.FOO !== "bar") throw new Error("FOO " + process.env.FOO);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeProcessCwdInjected(t *testing.T) {
	iso := New("", Options{
		Imports: NodeScriptImports(),
		Cwd:     "/guest/root",
	})
	err := iso.ScriptMain(t.Context(), `
		if (process.cwd() !== "/guest/root") throw new Error("cwd " + process.cwd());
		process.chdir("/guest/other");
		if (process.cwd() !== "/guest/other") throw new Error("chdir " + process.cwd());
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeProcessPIDInjected(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports(), PID: 42})
	err := iso.ScriptMain(t.Context(), `
		if (process.pid !== 42) throw new Error("pid " + process.pid);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}
