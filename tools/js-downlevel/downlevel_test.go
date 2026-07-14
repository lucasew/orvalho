package jsdownlevel_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func toolDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}

func TestFixtureExists(t *testing.T) {
	path := filepath.Join(toolDir(t), "fixtures", "modern.js")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
	src := string(data)
	// Fixture must exercise syntax that esbuild rewrites at --target=es2015.
	if !strings.Contains(src, "?.") {
		t.Error("fixture should use optional chaining (?.); downlevel smoke needs modern syntax")
	}
	if !strings.Contains(src, "??") {
		t.Error("fixture should use nullish coalescing (??); downlevel smoke needs modern syntax")
	}
}

func TestGoldenExistsAndIsDownleveled(t *testing.T) {
	path := filepath.Join(toolDir(t), "golden", "bundle.js")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden missing (run: mise run js:downlevel:golden): %v", err)
	}
	out := string(data)
	if len(strings.TrimSpace(out)) == 0 {
		t.Fatal("golden bundle is empty")
	}
	// esbuild --target=es2015 should eliminate these operators.
	if strings.Contains(out, "?.") {
		t.Error("golden still contains optional chaining (?.); expected ES2015 downlevel")
	}
	if strings.Contains(out, "??") {
		t.Error("golden still contains nullish coalescing (??); expected ES2015 downlevel")
	}
	// Sanity: fixture markers should survive the IIFE bundle.
	for _, want := range []string{"__orvalhoFixture", "hello, ", "anonymous"} {
		if !strings.Contains(out, want) {
			t.Errorf("golden missing expected substring %q", want)
		}
	}
}
