package workers

import (
	"os"
	"strings"
	"testing"
	"time"
)

func loadOfficialLexer(t *testing.T, iso *Isolate) {
	t.Helper()
	raw, err := os.ReadFile("testdata/es_module_lexer.wasm")
	if err != nil {
		t.Fatal(err)
	}
	iso.vm.Set("$WASM", iso.uint8Array(raw))
	err = iso.ScriptMain(t.Context(), `
		new WebAssembly.Instance(new WebAssembly.Module($WASM));
	`, "boot-lexer.js")
	if err != nil {
		t.Fatal(err)
	}
	if iso.esmLexer == nil {
		t.Fatal("official lexer wasm not detected")
	}
}

func TestParseESMOfficialWasm(t *testing.T) {
	iso := New("", Options{})
	loadOfficialLexer(t, iso)
	defer iso.pushCtx(t.Context())()
	src := "import { a } from 'mod';\nexport var p = 5;\n"
	imps, exps, _, has, err := iso.parseESMLexer(src)
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("hasModuleSyntax")
	}
	if len(imps) != 1 || imps[0].n != "mod" {
		t.Fatalf("imports %+v", imps)
	}
	if src[imps[0].s:imps[0].e] != "mod" {
		t.Fatalf("spec slice %q", src[imps[0].s:imps[0].e])
	}
	if len(exps) != 1 || exps[0].n != "p" {
		t.Fatalf("exports %+v", exps)
	}
}

func TestParseESMOfficialLarge(t *testing.T) {
	iso := New("", Options{})
	loadOfficialLexer(t, iso)
	defer iso.pushCtx(t.Context())()
	var b strings.Builder
	b.WriteString("import { x } from './a.js';\n")
	b.WriteString(strings.Repeat("function Icon(){return 0}\n", 40000))
	b.WriteString("export { Icon };\n")
	src := b.String()
	t0 := time.Now()
	imps, exps, _, _, err := iso.parseESMLexer(src)
	if err != nil {
		t.Fatal(err)
	}
	if dt := time.Since(t0); dt > 2*time.Second {
		t.Fatalf("parse %s", dt)
	}
	if len(imps) != 1 || imps[0].n != "./a.js" {
		t.Fatalf("imports %+v", imps)
	}
	if len(exps) != 1 || exps[0].n != "Icon" {
		t.Fatalf("exports %+v", exps)
	}
}

func TestESMLexerPatch(t *testing.T) {
	iso := New("", Options{})
	loadOfficialLexer(t, iso)
	obj := iso.patchESMLexer(iso.vm.NewObject())
	iso.vm.Set("$lex", obj)
	err := iso.ScriptMain(t.Context(), `
		var r = $lex.parse("import { a } from 'mod'; export var p = 5;");
		if (r[0].length !== 1 || r[0][0].n !== "mod") throw new Error("imp");
		if (r[1].length !== 1 || r[1][0].n !== "p") throw new Error("exp");
		if (!r[3]) throw new Error("has");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}
