package bundle

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

//go:embed es_module_lexer.asm.js
var esmLexerASM string

var (
	es6UnicodeEscape     = regexp.MustCompile(`\\u\{([0-9a-fA-F]{1,6})\}`)
	unicodePropEscape    = regexp.MustCompile(`\\([pP])\{([^}]+)\}`)
	emptyImportMeta      = regexp.MustCompile(`\b(import_meta\d*)\s*=\s*\{\s*\}`)
	awaitImportToRequire = regexp.MustCompile(`\bawait\s+import\s*\(`)
	awaitBareCall        = regexp.MustCompile(`(?m)^await ([A-Za-z_$][\w$]*\(\);)\s*$`)
)

// pAtom maps property names regexp2 rejects under Unicode (the /u flag)
// to a single category or POSIX class it accepts. Keys are lower-case.
var pAtom = map[string]string{
	"id_continue": "L",
	"idc":         "L",
	"id_start":    "L",
	"ids":         "L",
	"word":        `\w`,
	"alnum":       "L",
	"blank":       `\s`,
	"ahex":        "ASCII_Hex_Digit",
	"alphabetic":  "L",
	"alpha":       "L",
	"cased":       "L",
	"lower":       "Ll",
	"upper":       "Lu",
	"space":       `\s`,
	"print":       "C",
	"rgi_emoji":   "Emoji",
	"ascii":       "Cc",
}

// CompileCJS compiles source to CommonJS ES2015 in memory (ADR-0017).
// On-disk files are bundled so imports resolve. Other sources are transformed.
func CompileCJS(source, file string) (string, error) {
	if dir, ok := resolveDir(file); ok {
		return buildCJS(source, file, dir)
	}
	return TransformCJS(source, file)
}

// TransformCJS downlevels one file to CommonJS ES2015 via the esbuild Go API.
func TransformCJS(source, file string) (string, error) {
	if !needsCJSTransform(source, file) {
		return rewritePlainCJS(source), nil
	}
	source = stripImportCallOptions(source)
	source = awaitImportToRequire.ReplaceAllString(source, "require(")
	source = awaitBareCall.ReplaceAllString(source, "$1")
	result := api.Transform(source, api.TransformOptions{
		Loader:     loaderFor(file),
		Sourcefile: file,
		Format:     api.FormatCommonJS,
		Target:     api.ES2015,
		Platform:   api.PlatformNeutral,
		Charset:    api.CharsetASCII,
		Supported:  map[string]bool{"dynamic-import": false},
	})
	if len(result.Errors) > 0 {
		return "", fmt.Errorf("bundle: transform %s: %s", file, result.Errors[0].Text)
	}
	return finishCJS(string(result.Code)), nil
}

func buildCJS(source, file, dir string) (string, error) {
	name := filepath.Base(file)
	if name == "" || name == "." {
		name = "script.js"
	}
	result := api.Build(api.BuildOptions{
		Stdin: &api.StdinOptions{
			Contents:   source,
			Sourcefile: name,
			ResolveDir: dir,
			Loader:     loaderFor(file),
		},
		Bundle:        true,
		Format:        api.FormatCommonJS,
		Target:        api.ES2015,
		Platform:      api.PlatformNode,
		Charset:       api.CharsetASCII,
		Supported:     map[string]bool{"dynamic-import": false},
		Write:         false,
		LogLevel:      api.LogLevelSilent,
		AbsWorkingDir: dir,
	})
	if len(result.Errors) > 0 {
		e := result.Errors[0]
		loc := file
		if e.Location != nil && e.Location.File != "" {
			loc = fmt.Sprintf("%s:%d", e.Location.File, e.Location.Line)
		}
		return "", fmt.Errorf("bundle: %s: %s", loc, e.Text)
	}
	if len(result.OutputFiles) == 0 {
		return "", fmt.Errorf("bundle: %s: empty output", file)
	}
	return finishCJS(string(result.OutputFiles[0].Contents)), nil
}

func finishCJS(src string) string {
	src = rewriteES6UnicodeEscapes(src)
	src = rewriteUnicodeProperties(src)
	src = rewriteRegexpFlags(src)
	src = rewriteCopyProps(src)
	src = rewriteArgumentsCapture(src)
	src = rewriteImportMeta(src)
	src = rewriteESMLexer(src)
	return rewriteWasmMemoryCache(src)
}

// stripImportCallOptions drops import(x, { with: ... }) options so
// esbuild ES2015 can rewrite the call (dynamic-import is disabled).
func stripImportCallOptions(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	i := 0
	for i < len(src) {
		if n := importCallLen(src, i); n > 0 {
			b.WriteString(src[i : i+n])
			i += n
			arg, next := firstImportArg(src, i)
			b.WriteString(arg)
			i = next
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}

func firstImportArg(src string, i int) (string, int) {
	start := i
	depth := 1
	comma := -1
	for i < len(src) && depth > 0 {
		if src[i] == '"' || src[i] == '\'' {
			q := src[i]
			i++
			for i < len(src) {
				if src[i] == '\\' && i+1 < len(src) {
					i += 2
					continue
				}
				if src[i] == q {
					i++
					break
				}
				i++
			}
			continue
		}
		if src[i] == '`' {
			i = scanTemplateLiteral(src, i)
			continue
		}
		switch src[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				if comma >= 0 {
					return strings.TrimRight(src[start:comma], " \t\n\r") + ")", i + 1
				}
				return src[start : i+1], i + 1
			}
		case ',':
			if depth == 1 && comma < 0 {
				comma = i
			}
		}
		i++
	}
	return src[start:], len(src)
}

// wasm-bindgen caches Uint8Array(memory.buffer) until byteLength === 0
// (browser detach-on-grow). goja copies; always re-read the live buffer.
func rewriteWasmMemoryCache(src string) string {
	old := `if (cachedUint8ArrayMemory0 === null || cachedUint8ArrayMemory0.byteLength === 0) {
        cachedUint8ArrayMemory0 = new Uint8Array(wasm.memory.buffer);
    }`
	neu := `cachedUint8ArrayMemory0 = new Uint8Array(wasm.memory.buffer);`
	src = strings.ReplaceAll(src, old, neu)
	old2 := `if (cachedDataViewMemory0 === null || cachedDataViewMemory0.buffer.byteLength === 0) {
        cachedDataViewMemory0 = new DataView(wasm.memory.buffer);
    }`
	neu2 := `cachedDataViewMemory0 = new DataView(wasm.memory.buffer);`
	return strings.ReplaceAll(src, old2, neu2)
}

// esmLexerWait is the inlined es-module-lexer WASM gate. goja has no
// WASM, so parse must use the asm.js implementation instead.
const esmLexerWait = `if (!C) return init.then((() => parse(E$1)));`

func rewriteESMLexer(src string) string {
	if !strings.Contains(src, esmLexerWait) {
		return src
	}
	body := strings.Replace(esmLexerASM, "export function parse", "function parse", 1)
	prelude := "var __orvalhoESMParse = (function () {\n" + body + "\nreturn parse;\n})();\n"
	repl := "if (typeof __orvalhoESMParse === \"function\") return __orvalhoESMParse(E$1, g);\n" + esmLexerWait
	return prelude + strings.Replace(src, esmLexerWait, repl, 1)
}

// funcOpen matches esbuild's function heads. Arrows are skipped so we
// do not invent an arguments object they do not have.
var funcOpen = regexp.MustCompile(`\bfunction(?:\s+[A-Za-z_$][\w$]*)?\s*\([^)]*\)\s*\{`)

// rewriteArgumentsCapture lets goja arrows see the enclosing arguments.
// goja does not implement lexical arguments, so CJS→ESM proxies such as
// `() => fn.apply(this, arguments)` would call fn with no args.
func rewriteArgumentsCapture(src string) string {
	src = funcOpen.ReplaceAllString(src, "${0}var __orvalhoArguments = arguments;")
	src = strings.ReplaceAll(src, ".apply(this, arguments)", ".apply(this, __orvalhoArguments)")
	src = strings.ReplaceAll(src, ".apply(null, arguments)", ".apply(null, __orvalhoArguments)")
	src = strings.ReplaceAll(src, ".apply(void 0, arguments)", ".apply(void 0, __orvalhoArguments)")
	return src
}

// rewriteCopyProps replaces esbuild's for-of __copyProps with an index
// loop. goja can fail to iterate getOwnPropertyNames; each getter must
// bind its key so every name does not resolve to the last property.
func rewriteCopyProps(src string) string {
	old := `var __copyProps = (to, from, except, desc) => {
  if (from && typeof from === "object" || typeof from === "function") {
    for (let key of __getOwnPropNames(from))
      if (!__hasOwnProp.call(to, key) && key !== except)
        __defProp(to, key, { get: () => from[key], enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable });
  }
  return to;
};`
	neu := `var __copyProps = (to, from, except, desc) => {
  if (from && typeof from === "object" || typeof from === "function") {
    var names = __getOwnPropNames(from);
    for (var i = 0; i < names.length; i++) {
      var key = names[i];
      if (!__hasOwnProp.call(to, key) && key !== except)
        __defProp(to, key, { get: ((k) => from[k]).bind(null, key), enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable });
    }
  }
  return to;
};`
	return strings.ReplaceAll(src, old, neu)
}

// regexpFlagLiteral matches esbuild's `new RegExp(..., "flags")`.
var regexpFlagLiteral = regexp.MustCompile(`, ("|')([gimsuyadv]*)("|')\)`)

// rewriteRegexpFlags drops flags goja rejects (hasIndices `d`, unicodeSets `v`).
func rewriteRegexpFlags(src string) string {
	return regexpFlagLiteral.ReplaceAllStringFunc(src, func(m string) string {
		sub := regexpFlagLiteral.FindStringSubmatch(m)
		if len(sub) < 4 || sub[1] != sub[3] {
			return m
		}
		q, flags := sub[1], sub[2]
		var b strings.Builder
		for _, r := range flags {
			if r != 'd' && r != 'v' {
				b.WriteRune(r)
			}
		}
		return ", " + q + b.String() + q + ")"
	})
}

// rewriteImportMeta fills esbuild's empty import_meta stub from the wrap
// parameter __orvalhoFilename (guest files may declare their own __filename).
func rewriteImportMeta(src string) string {
	return emptyImportMeta.ReplaceAllString(src, `$1 = { url: __orvalhoFileURL(__orvalhoFilename) }`)
}

// rewriteUnicodeProperties rewrites \p{…} names regexp2 rejects when /u is set.
func rewriteUnicodeProperties(src string) string {
	return unicodePropEscape.ReplaceAllStringFunc(src, func(m string) string {
		sub := unicodePropEscape.FindStringSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		kind, name := sub[1], sub[2]
		if strings.Contains(name, "$") {
			return m
		}
		if rest, ok := strings.CutPrefix(name, "Script="); ok {
			return `\` + kind + `{` + rest + `}`
		}
		alt, ok := pAtom[strings.ToLower(name)]
		if !ok {
			return m
		}
		if alt == `\w` || alt == `\s` {
			if kind == "P" {
				if alt == `\w` {
					return `\W`
				}
				return `\S`
			}
			return alt
		}
		if strings.ToLower(name) == "print" {
			if kind == "p" {
				return `\P{C}`
			}
			return `\p{C}`
		}
		if strings.ToLower(name) == "ascii" {
			if kind == "p" {
				return `[\x00-\x7F]`
			}
			return `[^\x00-\x7F]`
		}
		return `\` + kind + `{` + alt + `}`
	})
}

// rewriteES6UnicodeEscapes turns \u{...} into UTF-16 \uXXXX so goja can parse.
func rewriteES6UnicodeEscapes(src string) string {
	return es6UnicodeEscape.ReplaceAllStringFunc(src, func(m string) string {
		sub := es6UnicodeEscape.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		n, err := strconv.ParseInt(sub[1], 16, 32)
		if err != nil || n < 0 || n > 0x10FFFF {
			return m
		}
		r := rune(n)
		if r <= 0xFFFF {
			return fmt.Sprintf(`\u%04X`, uint16(r))
		}
		r -= 0x10000
		hi := 0xD800 + (r >> 10)
		lo := 0xDC00 + (r & 0x3FF)
		return fmt.Sprintf(`\u%04X\u%04X`, hi, lo)
	})
}

func resolveDir(file string) (string, bool) {
	if file == "" {
		return "", false
	}
	p := filepath.FromSlash(file)
	cands := []string{p}
	if !filepath.IsAbs(p) {
		if wd, err := os.Getwd(); err == nil {
			cands = append(cands, filepath.Join(wd, p))
		}
	}
	for _, c := range cands {
		st, err := os.Stat(c)
		if err == nil && !st.IsDir() {
			abs, err := filepath.Abs(c)
			if err != nil {
				return filepath.Dir(c), true
			}
			return filepath.Dir(abs), true
		}
	}
	return "", false
}

// needsCJSTransform is true when esbuild can change the file (ESM
// syntax, or a loader that is not already JS).
func needsCJSTransform(src, file string) bool {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".ts", ".mts", ".cts", ".json", ".mjs":
		return true
	}
	return hasESMSyntax(src)
}

// rewritePlainCJS applies the goja-only regex patches that do not
// need esbuild. Tokens that are absent leave src unchanged.
func rewritePlainCJS(src string) string {
	if strings.Contains(src, "await") && awaitImportToRequire.MatchString(src) {
		src = awaitImportToRequire.ReplaceAllString(src, "require(")
	}
	if strings.Contains(src, "RegExp") && regexpFlagLiteral.MatchString(src) {
		src = rewriteRegexpFlags(src)
	}
	if strings.Contains(src, ".apply(") && strings.Contains(src, "arguments") {
		src = rewriteArgumentsCapture(src)
	}
	if strings.Contains(src, esmLexerWait) {
		src = rewriteESMLexer(src)
	}
	if strings.Contains(src, `\u{`) {
		src = rewriteES6UnicodeEscapes(src)
	}
	if strings.Contains(src, `\p{`) || strings.Contains(src, `\P{`) {
		src = rewriteUnicodeProperties(src)
	}
	if strings.Contains(src, "cachedUint8ArrayMemory0") || strings.Contains(src, "cachedDataViewMemory0") {
		src = rewriteWasmMemoryCache(src)
	}
	return src
}

func hasESMSyntax(src string) bool {
	for i := 0; i < len(src); {
		c := src[i]
		if c == '/' && i+1 < len(src) {
			switch src[i+1] {
			case '/':
				if j := strings.IndexByte(src[i+2:], '\n'); j >= 0 {
					i += 2 + j + 1
				} else {
					return false
				}
				continue
			case '*':
				if j := strings.Index(src[i+2:], "*/"); j >= 0 {
					i += 2 + j + 2
				} else {
					return false
				}
				continue
			}
		}
		if c == '"' || c == '\'' {
			i = skipQuoted(src, i)
			continue
		}
		if c == '`' {
			i = scanTemplateLiteral(src, i)
			continue
		}
		if isIdentStart(c) {
			start := i
			i++
			for i < len(src) && isIdentCont(src[i]) {
				i++
			}
			w := src[start:i]
			if w == "import" || w == "export" {
				return true
			}
			continue
		}
		i++
	}
	return false
}

func skipQuoted(src string, i int) int {
	q := src[i]
	i++
	for i < len(src) {
		if src[i] == '\\' && i+1 < len(src) {
			i += 2
			continue
		}
		if src[i] == q {
			return i + 1
		}
		i++
	}
	return len(src)
}

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isIdentCont(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

func loaderFor(file string) api.Loader {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".ts", ".mts", ".cts":
		return api.LoaderTS
	case ".json":
		return api.LoaderJSON
	default:
		return api.LoaderJS
	}
}
