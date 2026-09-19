package workers

import (
	"errors"
	"testing"

	"github.com/lucasew/orvalho/pkg/workers/bundle"
)

func TestRollupNativeBinding(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var n = require("@rollup/rollup-linux-x64-gnu");
		if (typeof n.parse !== "function") throw new Error("parse");
		if (typeof n.parseAsync !== "function") throw new Error("parseAsync");
		if (typeof n.xxhashBase64Url !== "function") throw new Error("xxhash");
		var buf = n.parse("export const x = 1;", false, false);
		if (typeof Buffer !== "undefined" && !Buffer.isBuffer(buf)) throw new Error("not buffer");
		var ast = { type: "Program", body: [] };
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestTransformCJSSkipsNativeAddon(t *testing.T) {
	elf := "\x7fELF" + "not javascript"
	_, err := bundle.TransformCJS(elf, "addon.node")
	if !errors.Is(err, bundle.ErrNativeAddon) {
		t.Fatalf("got %v", err)
	}
}
