package workers

import "testing"

func runNodeOS(t *testing.T, src string) {
	t.Helper()
	iso := New("", Options{Imports: NodeScriptImports()})
	if err := iso.ScriptMain(t.Context(), src, "t.js"); err != nil {
		t.Fatal(err)
	}
}

func TestNodeOSArchInjected(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports(), Arch: "x64"})
	if err := iso.ScriptMain(t.Context(), `
		if (require("os").arch() !== "x64") throw new Error("arch " + require("os").arch());
	`, "t.js"); err != nil {
		t.Fatal(err)
	}
}

func TestNodeOSIdentity(t *testing.T) {
	runNodeOS(t, `
		var os = require("os");
		if (require("node:os") !== os) throw new Error("node:os identity");
		if (os.EOL !== "\n") throw new Error("EOL " + JSON.stringify(os.EOL));
		if (typeof os.platform() !== "string") throw new Error("platform");
		if (os.arch() !== "wasm32") throw new Error("arch " + os.arch());
		if (os.arch() !== process.arch) throw new Error("os.arch !== process.arch");
		if (!os.tmpdir()) throw new Error("tmpdir");
		if (!os.homedir()) throw new Error("homedir");
		var n = os.availableParallelism();
		if (typeof n !== "number" || n < 1) throw new Error("availableParallelism " + n);
	`)
}
