package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

var (
	es6UnicodeEscape  = regexp.MustCompile(`\\u\{([0-9a-fA-F]{1,6})\}`)
	unicodePropEscape = regexp.MustCompile(`\\([pP])\{([^}]+)\}`)
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
	return rewriteUnicodeProperties(rewriteES6UnicodeEscapes(src))
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
