package workers

import "testing"

func runNodeTTY(t *testing.T, src string) {
	t.Helper()
	iso := New("", Options{Imports: NodeScriptImports()})
	if err := iso.ScriptMain(t.Context(), src, "t.js"); err != nil {
		t.Fatal(err)
	}
}

func TestNodeTTYIdentity(t *testing.T) {
	runNodeTTY(t, `
		var tty = require("tty");
		if (require("node:tty") !== tty) throw new Error("identity");
		if (typeof tty.isatty !== "function") throw new Error("isatty type");
		if (tty.isatty(1) !== false) throw new Error("isatty(1)");
		if (tty.isatty() !== false) throw new Error("isatty()");
		if (tty.isatty(null) !== false) throw new Error("isatty(null)");
		if (tty.isatty(undefined) !== false) throw new Error("isatty(undefined)");
		if (typeof tty.ReadStream !== "function") throw new Error("ReadStream");
		if (typeof tty.WriteStream !== "function") throw new Error("WriteStream");
		if (new tty.WriteStream(1).isTTY !== false) throw new Error("WriteStream isTTY");
		if (new tty.ReadStream(0).isTTY !== false) throw new Error("ReadStream isTTY");
		if (tty.default !== tty) throw new Error("default");
	`)
}
