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

func TestNodeProcessOnce(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		if (typeof process.once !== "function") throw new Error("once");
		if (typeof process.on !== "function") throw new Error("on");
		if (typeof process.off !== "function") throw new Error("off");
		var n = 0;
		process.once("SIGTERM", function () { n++; });
		process.emit("SIGTERM");
		if (n !== 1) throw new Error("emit " + n);
		if (typeof process.stdin.on !== "function") throw new Error("stdin.on");
		process.stdin.on("end", function () {});
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

func TestNodeProcessPlatformDefault(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		if (process.platform !== "wasi") throw new Error("platform " + process.platform);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeProcessPlatformInjected(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports(), Platform: "linux"})
	err := iso.ScriptMain(t.Context(), `
		if (process.platform !== "linux") throw new Error("platform " + process.platform);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeProcessArchDefault(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		if (process.arch !== "wasm32") throw new Error("arch " + process.arch);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeProcessArchInjected(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports(), Arch: "x64"})
	err := iso.ScriptMain(t.Context(), `
		if (process.arch !== "x64") throw new Error("arch " + process.arch);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeProcessStdio(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		if (!process.stdout || process.stdout.isTTY !== false) throw new Error("stdout");
		if (process.stderr.getColorDepth() !== 1) throw new Error("depth");
		if (process.stderr.hasColors() !== false) throw new Error("colors");
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
