package workers

import (
	"os"
	"strconv"
	"testing"
)

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
		if (os.platform() !== "wasi") throw new Error("platform " + os.platform());
		if (os.platform() !== process.platform) throw new Error("os.platform !== process.platform");
		if (os.arch() !== "wasm32") throw new Error("arch " + os.arch());
		if (os.arch() !== process.arch) throw new Error("os.arch !== process.arch");
		if (!os.tmpdir()) throw new Error("tmpdir");
		if (!os.homedir()) throw new Error("homedir");
		var n = os.availableParallelism();
		if (typeof n !== "number" || n < 1) throw new Error("availableParallelism " + n);
	`)
}

func TestNodeOSHomedirNotHost(t *testing.T) {
	host, err := os.UserHomeDir()
	if err != nil || host == "" || host == "/" {
		t.Skip("no distinct host home")
	}
	t.Setenv("HOME", host)
	t.Setenv("TMPDIR", "/host/tmp/secret")
	iso := New("", Options{Imports: NodeScriptImports()})
	src := `
		var os = require("os");
		var home = os.homedir();
		if (home === ` + strconv.Quote(host) + `) throw new Error("leaked host home");
		if (home !== "/") throw new Error("homedir " + home);
		var info = os.userInfo();
		if (info.homedir === ` + strconv.Quote(host) + `) throw new Error("leaked userInfo home");
		if (info.homedir !== "/") throw new Error("userInfo " + info.homedir);
		var tmp = os.tmpdir();
		if (tmp === "/host/tmp/secret") throw new Error("leaked TMPDIR");
		if (tmp !== "/tmp") throw new Error("tmpdir " + tmp);
	`
	if err := iso.ScriptMain(t.Context(), src, "t.js"); err != nil {
		t.Fatal(err)
	}
}
