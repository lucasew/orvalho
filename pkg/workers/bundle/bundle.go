package bundle

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
		return "", fmt.Errorf("bundle: BundleEntry requires PackageDir and Entry")
	}
	pkgDir, err := filepath.Abs(opts.PackageDir)
	if err != nil {
		return "", err
	}
	entryPath := filepath.Join(pkgDir, filepath.FromSlash(opts.Entry))
	if st, err := os.Stat(entryPath); err != nil || st.IsDir() {
		return "", fmt.Errorf("bundle: entry %s: %w", opts.Entry, err)
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
		return "", fmt.Errorf("bundle: esbuild: %w\n%s", err, stderr.String())
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

// rewriteDynamicImport replaces executable dynamic import(…) with
// __orvalhoDynamicImport( so goja can parse the server bundle.
//
// IMPORTANT: only rewrite code tokens, never string/template/comment contents.
// A blind ReplaceAll corrupts Astro island hydration runtime embedded as
// string literals in the SSR bundle (browser then sees __orvalhoDynamicImport).
func rewriteDynamicImport(src string) string {
	var b strings.Builder
	b.Grow(len(src) + 64)
	i := 0
	for i < len(src) {
		// Line comment
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '/' {
			j := i + 2
			for j < len(src) && src[j] != '\n' {
				j++
			}
			b.WriteString(src[i:j])
			i = j
			continue
		}
		// Block comment
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '*' {
			j := i + 2
			for j+1 < len(src) && !(src[j] == '*' && src[j+1] == '/') {
				j++
			}
			if j+1 < len(src) {
				j += 2
			}
			b.WriteString(src[i:j])
			i = j
			continue
		}
		// Single- or double-quoted string
		if src[i] == '\'' || src[i] == '"' {
			q := src[i]
			j := i + 1
			for j < len(src) {
				if src[j] == '\\' && j+1 < len(src) {
					j += 2
					continue
				}
				if src[j] == q {
					j++
					break
				}
				j++
			}
			b.WriteString(src[i:j])
			i = j
			continue
		}
		// Template literal: leave quasi-literals alone; rewrite only ${...} code.
		if src[i] == '`' {
			end := scanTemplateLiteral(src, i)
			b.WriteString(rewriteTemplateLiteral(src[i:end]))
			i = end
			continue
		}
		// Dynamic import( — keyword import followed by (
		if n := importCallLen(src, i); n > 0 {
			b.WriteString("__orvalhoDynamicImport(")
			i += n
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}

// importCallLen returns the length of an import( call starting at i
// (including whitespace between import and (), or 0 if not a call.
func importCallLen(src string, i int) int {
	const kw = "import"
	if i+len(kw) > len(src) || src[i:i+len(kw)] != kw {
		return 0
	}
	if i > 0 && isIdentByte(src[i-1]) {
		return 0
	}
	j := i + len(kw)
	for j < len(src) && (src[j] == ' ' || src[j] == '\t' || src[j] == '\n' || src[j] == '\r') {
		j++
	}
	if j >= len(src) || src[j] != '(' {
		return 0
	}
	return j + 1 - i // consume through '('
}

func isIdentByte(c byte) bool {
	return c == '_' || c == '$' ||
		(c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9')
}

// scanTemplateLiteral returns the index just past the closing ` starting at i.
func scanTemplateLiteral(src string, i int) int {
	if i >= len(src) || src[i] != '`' {
		return i
	}
	j := i + 1
	for j < len(src) {
		if src[j] == '\\' && j+1 < len(src) {
			j += 2
			continue
		}
		if src[j] == '`' {
			return j + 1
		}
		if src[j] == '$' && j+1 < len(src) && src[j+1] == '{' {
			j += 2
			depth := 1
			for j < len(src) && depth > 0 {
				if src[j] == '`' {
					// nested template
					j = scanTemplateLiteral(src, j)
					continue
				}
				if src[j] == '\'' || src[j] == '"' {
					q := src[j]
					j++
					for j < len(src) {
						if src[j] == '\\' && j+1 < len(src) {
							j += 2
							continue
						}
						if src[j] == q {
							j++
							break
						}
						j++
					}
					continue
				}
				if src[j] == '{' {
					depth++
				} else if src[j] == '}' {
					depth--
				}
				if depth > 0 {
					j++
				}
			}
			if depth == 0 {
				j++ // past closing }
			}
			continue
		}
		j++
	}
	return j
}

// rewriteTemplateLiteral rewrites import( only inside ${...} expressions.
func rewriteTemplateLiteral(tmpl string) string {
	if len(tmpl) < 2 || tmpl[0] != '`' {
		return tmpl
	}
	var b strings.Builder
	b.WriteByte('`')
	i := 1
	for i < len(tmpl) {
		if tmpl[i] == '\\' && i+1 < len(tmpl) {
			b.WriteByte(tmpl[i])
			b.WriteByte(tmpl[i+1])
			i += 2
			continue
		}
		if tmpl[i] == '`' {
			b.WriteByte('`')
			return b.String()
		}
		if tmpl[i] == '$' && i+1 < len(tmpl) && tmpl[i+1] == '{' {
			// find expression end
			start := i
			j := i + 2
			depth := 1
			for j < len(tmpl) && depth > 0 {
				if tmpl[j] == '`' {
					j = scanTemplateLiteral(tmpl, j)
					continue
				}
				if tmpl[j] == '\'' || tmpl[j] == '"' {
					q := tmpl[j]
					j++
					for j < len(tmpl) {
						if tmpl[j] == '\\' && j+1 < len(tmpl) {
							j += 2
							continue
						}
						if tmpl[j] == q {
							j++
							break
						}
						j++
					}
					continue
				}
				if tmpl[j] == '{' {
					depth++
				} else if tmpl[j] == '}' {
					depth--
				}
				if depth > 0 {
					j++
				}
			}
			if depth == 0 {
				// ${ expr }
				b.WriteString("${")
				expr := tmpl[start+2 : j]
				b.WriteString(rewriteDynamicImport(expr))
				b.WriteByte('}')
				i = j + 1
				continue
			}
		}
		b.WriteByte(tmpl[i])
		i++
	}
	return b.String()
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
