package js

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed stubs/*
var stubFS embed.FS

// BundleOptions configures esbuild-on-load for multi-file Workers modules.
type BundleOptions struct {
	// PackageDir is the absolute path to the package root (orvalho.cue dir).
	PackageDir string
	// Entry is package-relative entrypoint (e.g. entry.mjs or worker.js).
	Entry string
	// Esbuild is the esbuild binary path; empty uses PATH ("esbuild").
	Esbuild string
}

// BundleEntry runs esbuild to produce a single IIFE script for goja.
// Sets globalThis.default from the Module Worker default export.
// Stubs cloudflare:workers and common node:* imports used by Astro/CF builds.
func BundleEntry(opts BundleOptions) (string, error) {
	if opts.PackageDir == "" || opts.Entry == "" {
		return "", fmt.Errorf("js: BundleEntry requires PackageDir and Entry")
	}
	pkgDir, err := filepath.Abs(opts.PackageDir)
	if err != nil {
		return "", err
	}
	entryPath := filepath.Join(pkgDir, filepath.FromSlash(opts.Entry))
	if st, err := os.Stat(entryPath); err != nil || st.IsDir() {
		return "", fmt.Errorf("js: entry %s: %w", opts.Entry, err)
	}

	stubDir, err := materializeStubs()
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stubDir)

	outFile, err := os.CreateTemp("", "orvalho-bundle-*.js")
	if err != nil {
		return "", err
	}
	outPath := outFile.Name()
	outFile.Close()
	defer os.Remove(outPath)

	bannerBytes, err := stubFS.ReadFile("stubs/banner.js")
	if err != nil {
		return "", err
	}

	bin := opts.Esbuild
	if bin == "" {
		bin = "esbuild"
	}

	args := []string{
		entryPath,
		"--bundle",
		"--format=iife",
		"--global-name=__orvalhoWorker",
		"--target=es2015",
		"--platform=neutral",
		"--outfile=" + outPath,
		"--alias:cloudflare:workers=" + filepath.Join(stubDir, "cloudflare_workers.js"),
		"--alias:node:events=" + filepath.Join(stubDir, "node_events.js"),
		"--alias:node:stream=" + filepath.Join(stubDir, "node_stream.js"),
		"--alias:node:process=" + filepath.Join(stubDir, "node_process.js"),
		"--alias:node:crypto=" + filepath.Join(stubDir, "node_crypto.js"),
		"--alias:node:async_hooks=" + filepath.Join(stubDir, "node_async_hooks.js"),
		"--alias:async_hooks=" + filepath.Join(stubDir, "node_async_hooks.js"),
		"--alias:node:buffer=" + filepath.Join(stubDir, "node_buffer.js"),
		"--alias:buffer=" + filepath.Join(stubDir, "node_buffer.js"),
		"--log-level=warning",
	}

	cmd := exec.Command(bin, args...)
	cmd.Dir = pkgDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("js: esbuild: %w\n%s", err, stderr.String())
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		return "", err
	}

	// goja has no ESM dynamic import(); rewrite leftover import(…) calls.
	// Static imports are already bundled; remaining import( are runtime-dynamic.
	code := rewriteDynamicImport(string(raw))
	// Drop eager WebAssembly.compile/instantiate chains (devalue etc.); goja has no WASM.
	code = neutralizeWebAssemblyEagerLoad(code)

	// Prepend workerd-style banner; footer exposes Module Worker default export.
	dynStub := `
if (typeof globalThis.__orvalhoDynamicImport !== "function") {
  globalThis.__orvalhoDynamicImport = function (spec) {
    return Promise.reject(new Error("dynamic import not supported in goja: " + String(spec)));
  };
}
`
	footer := "\n;if (typeof __orvalhoWorker !== 'undefined' && __orvalhoWorker) {\n" +
		"  globalThis.default = __orvalhoWorker.default !== undefined ? __orvalhoWorker.default : __orvalhoWorker;\n" +
		"}\n"
	return string(bannerBytes) + dynStub + "\n" + code + footer, nil
}

// rewriteDynamicImport replaces bare import( with __orvalhoDynamicImport(
// so goja can parse the script. Does not touch import.meta.
func rewriteDynamicImport(src string) string {
	// Avoid import.meta: only match import( optionally after yield/await/space.
	return strings.ReplaceAll(src, "import(", "__orvalhoDynamicImport(")
}

// neutralizeWebAssemblyEagerLoad strips eager WebAssembly.compile(…).then(…)
// init chains (devalue etc.). goja has no WASM; leftover binary blobs must not
// be passed into instantiate/destructuring.
func neutralizeWebAssemblyEagerLoad(src string) string {
	// After rewriteDynamicImport / esbuild, typical pattern:
	//   (function(){return Promise.resolve({})/*orvalho-no-wasm*/;})(E()).then(WebAssembly.instantiate).then((({ exports: A }) => { }));
	// or original WebAssembly.compile(E()).then(...)
	patterns := []string{
		"(function(){return Promise.resolve({})/*orvalho-no-wasm*/;})(E()).then(WebAssembly.instantiate).then((({ exports: A }) => {\n      }));",
		"WebAssembly.compile(E()).then(WebAssembly.instantiate).then((({ exports: A }) => {\n      }));",
	}
	out := src
	for _, p := range patterns {
		out = strings.ReplaceAll(out, p, "/*orvalho: stripped WebAssembly.init*/void 0;")
	}
	// Fallback: neutralize remaining compile calls.
	out = strings.ReplaceAll(out, "WebAssembly.compile(",
		"(function(){return Promise.resolve({})/*orvalho-no-wasm*/;})(")
	return out
}

// NeedsBundle reports whether source looks like a multi-module Workers entry
// (ESM import/export from) rather than a single-file script.
func NeedsBundle(src string) bool {
	// Heuristic: static import/export-from or dynamic import of relative paths.
	if strings.Contains(src, "from \"") || strings.Contains(src, "from '") {
		return true
	}
	if strings.Contains(src, "import(") || strings.Contains(src, "import \"") || strings.Contains(src, "import '") {
		return true
	}
	return false
}

func materializeStubs() (string, error) {
	dir, err := os.MkdirTemp("", "orvalho-stubs-*")
	if err != nil {
		return "", err
	}
	entries, err := stubFS.ReadDir("stubs")
	if err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := stubFS.ReadFile("stubs/" + e.Name())
		if err != nil {
			os.RemoveAll(dir)
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), data, 0o644); err != nil {
			os.RemoveAll(dir)
			return "", err
		}
	}
	return dir, nil
}
