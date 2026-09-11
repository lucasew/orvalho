package workers

import (
	"strings"
	"testing"
	"time"
)

func TestParseESMStaticImport(t *testing.T) {
	src := "import { a } from 'mod';\nexport var p = 5;\n"
	imps, exps, _, has := parseESM(src)
	if !has {
		t.Fatal("hasModuleSyntax")
	}
	if len(imps) != 1 || imps[0].n != "mod" {
		t.Fatalf("imports %+v", imps)
	}
	if src[imps[0].s:imps[0].e] != "mod" {
		t.Fatalf("spec slice %q", src[imps[0].s:imps[0].e])
	}
	if !strings.HasPrefix(src[imps[0].ss:imps[0].se], "import") {
		t.Fatalf("stmt %q", src[imps[0].ss:imps[0].se])
	}
	if len(exps) != 1 || exps[0].n != "p" {
		t.Fatalf("exports %+v", exps)
	}
}

func TestParseESMSideEffectAndExportStar(t *testing.T) {
	src := "import './chunk.js';\nexport * from './icons';\n"
	imps, _, _, _ := parseESM(src)
	if len(imps) != 1 || imps[0].n != "./chunk.js" {
		t.Fatalf("imports %+v", imps)
	}
}

func TestParseESMDynamicAndMeta(t *testing.T) {
	src := "import.meta.url; import('x');\n"
	imps, _, _, _ := parseESM(src)
	if len(imps) < 2 {
		t.Fatalf("imports %+v", imps)
	}
	var sawMeta, sawDyn bool
	for _, im := range imps {
		if im.t == esmImportMeta {
			sawMeta = true
		}
		if im.t == esmDynamic && im.n == "x" {
			sawDyn = true
		}
	}
	if !sawMeta || !sawDyn {
		t.Fatalf("meta=%v dyn=%v %+v", sawMeta, sawDyn, imps)
	}
}

func TestParseESMLarge(t *testing.T) {
	var b strings.Builder
	b.Grow(2_500_000)
	b.WriteString("import { x } from './a.js';\n")
	for i := 0; i < 60000; i++ {
		b.WriteString("export function Icon")
		b.WriteByte('A')
		b.WriteString("(){return 0}\n")
	}
	src := b.String()
	t0 := time.Now()
	imps, exps, _, _ := parseESM(src)
	if dt := time.Since(t0); dt > 2*time.Second {
		t.Fatalf("parse %s", dt)
	}
	if len(imps) != 1 || imps[0].n != "./a.js" {
		t.Fatalf("imports %+v", imps)
	}
	if len(exps) < 1000 {
		t.Fatalf("exports %d", len(exps))
	}
}

func TestESMLexerPatch(t *testing.T) {
	iso := New("", Options{})
	obj := iso.patchESMLexer(iso.vm.NewObject())
	iso.vm.Set("$lex", obj)
	err := iso.ScriptMain(t.Context(), `
		var r = $lex.parse("import { a } from 'mod'; export var p = 5;");
		if (r[0].length !== 1 || r[0][0].n !== "mod") throw new Error("imp");
		if (r[0][0].s === undefined || r[0][0].e === undefined) throw new Error("span");
		if (r[1].length !== 1 || r[1][0].n !== "p") throw new Error("exp");
		if (!r[3]) throw new Error("has");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}
