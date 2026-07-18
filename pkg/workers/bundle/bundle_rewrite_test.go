package bundle

import "testing"

func TestRewriteDynamicImportSkipsStrings(t *testing.T) {
	src := `
const island = "await import(e); import(foo)";
const island2 = 'import(x)';
const island3 = ` + "`" + `await import(y)` + "`" + `;
await import("./real.js");
yield import(
  /* @vite-ignore */
  loggerConfig.entrypoint
);
fooimport(1);
import.meta.url;
`
	out := rewriteDynamicImport(src)
	if !contains(out, `"await import(e); import(foo)"`) {
		t.Fatalf("rewrote double-quoted string:\n%s", out)
	}
	if !contains(out, `'import(x)'`) {
		t.Fatalf("rewrote single-quoted string:\n%s", out)
	}
	if !contains(out, "`await import(y)`") {
		t.Fatalf("rewrote template without expr:\n%s", out)
	}
	if !contains(out, `__orvalhoDynamicImport("./real.js")`) {
		t.Fatalf("did not rewrite real dynamic import:\n%s", out)
	}
	if !contains(out, `__orvalhoDynamicImport(`) || contains(out, "yield import(") {
		// vite-ignore form should still rewrite the import(
		if contains(out, "yield import(") {
			t.Fatalf("did not rewrite multi-line import(:\n%s", out)
		}
	}
	if contains(out, "foo__orvalhoDynamicImport") {
		t.Fatalf("rewrote identifier suffix:\n%s", out)
	}
	if !contains(out, "import.meta.url") {
		t.Fatalf("broke import.meta:\n%s", out)
	}
}

func TestRewriteDynamicImportTemplateExpr(t *testing.T) {
	src := "`x ${import('./a.js')} y`"
	out := rewriteDynamicImport(src)
	if !contains(out, "__orvalhoDynamicImport('./a.js')") {
		t.Fatalf("expected rewrite inside ${}:\n%s", out)
	}
	if !contains(out, "`x ${") {
		t.Fatalf("broke template:\n%s", out)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}
