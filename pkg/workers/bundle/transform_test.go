package bundle_test

import (
	"strings"
	"testing"

	"github.com/lucasew/orvalho/pkg/workers/bundle"
)

func TestTransformCJSOptionalCatch(t *testing.T) {
	out, err := bundle.TransformCJS("try { x(); } catch { y(); }\nexport const n = 1;\n", "mod.mjs")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "catch {") {
		t.Fatalf("optional catch survived: %s", out)
	}
	if !strings.Contains(out, "exports") {
		t.Fatalf("expected CJS exports: %s", out)
	}
}
