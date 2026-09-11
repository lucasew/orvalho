package workers

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dop251/goja"
)

// es-module-lexer ImportType.
const (
	esmStatic     = 1
	esmDynamic    = 2
	esmImportMeta = 3
)

type esmImport struct {
	n  string
	t  int
	s  int
	e  int
	ss int
	se int
	d  int
	a  int
}

type esmExport struct {
	n  string
	ln string
	s  int
	e  int
	ls int
	le int
}

func isESMLexerFile(file string) bool {
	return strings.Contains(file, "es-module-lexer") && strings.HasSuffix(file, "lexer.js")
}

func (iso *Isolate) patchESMLexer(exports goja.Value) goja.Value {
	obj := iso.vm.NewObject()
	mustSet(obj, "parse", iso.jsESMParse)
	p, resolve, _ := iso.vm.NewPromise()
	resolve(goja.Undefined())
	mustSet(obj, "init", iso.vm.ToValue(p))
	mustSet(obj, "initSync", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	if src, ok := exports.(*goja.Object); ok && src != nil {
		if it := src.Get("ImportType"); it != nil && !goja.IsUndefined(it) {
			mustSet(obj, "ImportType", it)
		}
	}
	mustSet(obj, "default", obj)
	return obj
}

func (iso *Isolate) jsESMParse(call goja.FunctionCall) goja.Value {
	src := ""
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		src = call.Argument(0).String()
	}
	imps, exps, facade, hasMod := parseESM(src)
	impVals := make([]interface{}, 0, len(imps))
	for _, im := range imps {
		o := iso.vm.NewObject()
		if im.n != "" {
			mustSet(o, "n", im.n)
		}
		mustSet(o, "t", im.t)
		mustSet(o, "s", im.s)
		mustSet(o, "e", im.e)
		mustSet(o, "ss", im.ss)
		mustSet(o, "se", im.se)
		mustSet(o, "d", im.d)
		mustSet(o, "a", im.a)
		mustSet(o, "at", goja.Null())
		impVals = append(impVals, o)
	}
	expVals := make([]interface{}, 0, len(exps))
	for _, ex := range exps {
		o := iso.vm.NewObject()
		mustSet(o, "n", ex.n)
		mustSet(o, "s", ex.s)
		mustSet(o, "e", ex.e)
		mustSet(o, "ls", ex.ls)
		mustSet(o, "le", ex.le)
		if ex.ln != "" {
			mustSet(o, "ln", ex.ln)
		}
		expVals = append(expVals, o)
	}
	return iso.vm.NewArray(iso.vm.NewArray(impVals...), iso.vm.NewArray(expVals...), facade, hasMod)
}

func parseESM(src string) (imps []esmImport, exps []esmExport, facade, hasMod bool) {
	s := src
	n := len(s)
	i := 0
	onlyModule := true
	for i < n {
		c := s[i]
		switch c {
		case ' ', '\t', '\n', '\r', '\f', 0x0b:
			i++
			continue
		case '/':
			if i+1 < n && s[i+1] == '/' {
				i = skipLineComment(s, i+2)
				continue
			}
			if i+1 < n && s[i+1] == '*' {
				i = skipBlockComment(s, i+2)
				continue
			}
			i++
			onlyModule = false
			continue
		case '"', '\'', '`':
			i = skipString(s, i)
			onlyModule = false
			continue
		}
		if identStart(s, i) {
			start := i
			i = skipIdent(s, i)
			word := s[start:i]
			switch word {
			case "import":
				if i < n && s[i] == '.' {
					imps = append(imps, parseImportMeta(s, start, &i)...)
					hasMod = true
					continue
				}
				imps = append(imps, parseImportStmt(s, start, &i)...)
				hasMod = true
				continue
			case "export":
				exps = append(exps, parseExportStmt(s, start, &i)...)
				hasMod = true
				continue
			default:
				onlyModule = false
				continue
			}
		}
		i++
		if c != ';' && c != ',' && c != '{' && c != '}' && c != '(' && c != ')' && c != '[' && c != ']' {
			onlyModule = false
		}
	}
	facade = hasMod && onlyModule
	return
}

func parseImportStmt(s string, start int, i *int) []esmImport {
	skipWS(s, i)
	if *i < len(s) && s[*i] == '(' {
		return []esmImport{parseDynamicImport(s, start, i)}
	}
	// import 'x'  /  import "x"
	if *i < len(s) && (s[*i] == '\'' || s[*i] == '"') {
		spec, ss, se := readString(s, i)
		skipWS(s, i)
		eatSemi(s, i)
		return []esmImport{{n: spec, t: esmStatic, s: ss + 1, e: se - 1, ss: start, se: *i, d: -1, a: -1}}
	}
	// import type ...  (TS) — still a static import if it has from
	skipImportClause(s, i)
	skipWS(s, i)
	spec, a := "", -1
	ss, se := 0, 0
	if hasWord(s, *i, "from") {
		*i = skipIdent(s, *i)
		skipWS(s, i)
		if *i < len(s) && (s[*i] == '\'' || s[*i] == '"') {
			spec, ss, se = readString(s, i)
		}
	}
	skipWS(s, i)
	if *i < len(s) && (hasWord(s, *i, "assert") || hasWord(s, *i, "with")) {
		a = *i
		*i = skipIdent(s, *i)
		skipWS(s, i)
		skipBalanced(s, i, '{', '}')
	}
	skipWS(s, i)
	eatSemi(s, i)
	imp := esmImport{n: spec, t: esmStatic, ss: start, se: *i, d: -1, a: a}
	if se > ss {
		imp.s, imp.e = ss+1, se-1
	}
	return []esmImport{imp}
}

func parseDynamicImport(s string, start int, i *int) esmImport {
	d := *i
	*i++ // (
	skipWS(s, i)
	spec, ss, se := "", 0, 0
	if *i < len(s) && (s[*i] == '\'' || s[*i] == '"') {
		spec, ss, se = readString(s, i)
	} else if *i < len(s) && s[*i] == '`' {
		_, ss, se = readString(s, i)
	}
	skipWS(s, i)
	a := -1
	if *i < len(s) && s[*i] == ',' {
		*i++
		skipWS(s, i)
		a = *i
	}
	skipBalanced(s, i, '(', ')')
	imp := esmImport{t: esmDynamic, ss: start, se: *i, d: d, a: a}
	if spec != "" {
		imp.n = spec
		imp.s, imp.e = ss, se
	} else if se > ss {
		imp.s, imp.e = ss, se
	}
	return imp
}

func parseImportMeta(s string, start int, i *int) []esmImport {
	// import.meta or import.meta.url or import.source(
	*i++ // .
	skipWS(s, i)
	wStart := *i
	*i = skipIdent(s, *i)
	word := s[wStart:*i]
	if word == "meta" {
		return []esmImport{{t: esmImportMeta, s: start, e: *i, ss: start, se: *i, d: -2, a: -1}}
	}
	skipWS(s, i)
	if *i < len(s) && s[*i] == '(' {
		return []esmImport{parseDynamicImport(s, start, i)}
	}
	return nil
}

func parseExportStmt(s string, start int, i *int) []esmExport {
	skipWS(s, i)
	if hasWord(s, *i, "default") {
		ds := *i
		*i = skipIdent(s, *i)
		skipWS(s, i)
		skipExportDefault(s, i)
		return []esmExport{{n: "default", s: ds, e: ds + 7, ls: -1, le: -1}}
	}
	if hasWord(s, *i, "async") {
		*i = skipIdent(s, *i)
		skipWS(s, i)
	}
	if hasWord(s, *i, "function") || hasWord(s, *i, "class") {
		*i = skipIdent(s, *i)
		skipWS(s, i)
		if *i < len(s) && identStart(s, *i) {
			ns := *i
			*i = skipIdent(s, *i)
			name := s[ns:*i]
			return []esmExport{{n: name, ln: name, s: ns, e: *i, ls: ns, le: *i}}
		}
		return []esmExport{{n: "default", s: start, e: *i, ls: -1, le: -1}}
	}
	if hasWord(s, *i, "var") || hasWord(s, *i, "let") || hasWord(s, *i, "const") {
		*i = skipIdent(s, *i)
		return parseExportBindings(s, i)
	}
	if *i < len(s) && s[*i] == '{' {
		return parseExportList(s, start, i)
	}
	if *i < len(s) && s[*i] == '*' {
		*i++
		skipWS(s, i)
		if hasWord(s, *i, "as") {
			*i = skipIdent(s, *i)
			skipWS(s, i)
			if *i < len(s) && identStart(s, *i) {
				ns := *i
				*i = skipIdent(s, *i)
				name := s[ns:*i]
				skipWS(s, i)
				if hasWord(s, *i, "from") {
					*i = skipIdent(s, *i)
					skipWS(s, i)
					if *i < len(s) && (s[*i] == '\'' || s[*i] == '"') {
						readString(s, i)
					}
				}
				skipWS(s, i)
				eatSemi(s, i)
				return []esmExport{{n: name, s: ns, e: *i, ls: -1, le: -1}}
			}
		}
		skipWS(s, i)
		if hasWord(s, *i, "from") {
			*i = skipIdent(s, *i)
			skipWS(s, i)
			if *i < len(s) && (s[*i] == '\'' || s[*i] == '"') {
				readString(s, i)
			}
		}
		skipWS(s, i)
		eatSemi(s, i)
		return nil
	}
	if identStart(s, *i) {
		ns := *i
		*i = skipIdent(s, *i)
		name := s[ns:*i]
		return []esmExport{{n: name, ln: name, s: ns, e: *i, ls: ns, le: *i}}
	}
	_ = start
	return nil
}

func parseExportList(s string, start int, i *int) []esmExport {
	*i++ // {
	var out []esmExport
	for *i < len(s) {
		skipWS(s, i)
		if *i < len(s) && s[*i] == '}' {
			*i++
			break
		}
		if *i < len(s) && (s[*i] == '\'' || s[*i] == '"') {
			name, ss, se := readString(s, i)
			skipWS(s, i)
			ln, ls, le := name, ss+1, se-1
			if hasWord(s, *i, "as") {
				*i = skipIdent(s, *i)
				skipWS(s, i)
				if *i < len(s) && (s[*i] == '\'' || s[*i] == '"') {
					name, ss, se = readString(s, i)
				} else if identStart(s, *i) {
					ns := *i
					*i = skipIdent(s, *i)
					name = s[ns:*i]
					ss, se = ns, *i
				}
			}
			out = append(out, esmExport{n: name, ln: ln, s: ss + 1, e: se - 1, ls: ls, le: le})
		} else if identStart(s, *i) {
			ns := *i
			*i = skipIdent(s, *i)
			local := s[ns:*i]
			name, s0, e0 := local, ns, *i
			skipWS(s, i)
			if hasWord(s, *i, "as") {
				*i = skipIdent(s, *i)
				skipWS(s, i)
				if *i < len(s) && (s[*i] == '\'' || s[*i] == '"') {
					name, qs, qe := readString(s, i)
					s0, e0 = qs+1, qe-1
					_ = name
					out = append(out, esmExport{n: name, ln: local, s: s0, e: e0, ls: ns, le: *i})
					// fix le
					out[len(out)-1].ls, out[len(out)-1].le = ns, ns+len(local)
				} else if identStart(s, *i) {
					as := *i
					*i = skipIdent(s, *i)
					name = s[as:*i]
					s0, e0 = as, *i
					out = append(out, esmExport{n: name, ln: local, s: s0, e: e0, ls: ns, le: ns + len(local)})
				}
			} else {
				out = append(out, esmExport{n: name, ln: local, s: s0, e: e0, ls: ns, le: e0})
			}
		}
		skipWS(s, i)
		if *i < len(s) && s[*i] == ',' {
			*i++
		}
	}
	skipWS(s, i)
	if hasWord(s, *i, "from") {
		*i = skipIdent(s, *i)
		skipWS(s, i)
		if *i < len(s) && (s[*i] == '\'' || s[*i] == '"') {
			readString(s, i)
		}
	}
	skipWS(s, i)
	eatSemi(s, i)
	_ = start
	return out
}

func parseExportBindings(s string, i *int) []esmExport {
	var out []esmExport
	for *i < len(s) {
		skipWS(s, i)
		if *i >= len(s) || s[*i] == ';' {
			eatSemi(s, i)
			break
		}
		if s[*i] == '{' || s[*i] == '[' {
			skipBalanced(s, i, s[*i], map[byte]byte{'{': '}', '[': ']'}[s[*i]])
			skipWS(s, i)
			if *i < len(s) && s[*i] == '=' {
				*i++
				skipAssign(s, i)
			}
		} else if identStart(s, *i) {
			ns := *i
			*i = skipIdent(s, *i)
			name := s[ns:*i]
			out = append(out, esmExport{n: name, ln: name, s: ns, e: *i, ls: ns, le: *i})
			skipWS(s, i)
			if *i < len(s) && s[*i] == '=' {
				*i++
				skipAssign(s, i)
			}
		} else {
			break
		}
		skipWS(s, i)
		if *i < len(s) && s[*i] == ',' {
			*i++
			continue
		}
		eatSemi(s, i)
		break
	}
	return out
}

func skipImportClause(s string, i *int) {
	if *i >= len(s) {
		return
	}
	if s[*i] == '*' {
		*i++
		skipWS(s, i)
		if hasWord(s, *i, "as") {
			*i = skipIdent(s, *i)
			skipWS(s, i)
			if identStart(s, *i) {
				*i = skipIdent(s, *i)
			}
		}
		return
	}
	if identStart(s, *i) {
		*i = skipIdent(s, *i)
		skipWS(s, i)
		if *i < len(s) && s[*i] == ',' {
			*i++
			skipWS(s, i)
		}
	}
	if *i < len(s) && s[*i] == '{' {
		skipBalanced(s, i, '{', '}')
		return
	}
	if *i < len(s) && s[*i] == '*' {
		skipImportClause(s, i)
	}
}

func skipExportDefault(s string, i *int) {
	skipWS(s, i)
	if hasWord(s, *i, "async") {
		*i = skipIdent(s, *i)
		skipWS(s, i)
	}
	if hasWord(s, *i, "function") || hasWord(s, *i, "class") {
		*i = skipIdent(s, *i)
		skipWS(s, i)
		if identStart(s, *i) {
			*i = skipIdent(s, *i)
		}
		return
	}
	skipAssign(s, i)
}

func skipAssign(s string, i *int) {
	depth := 0
	for *i < len(s) {
		c := s[*i]
		if depth == 0 && (c == ';' || c == ',' || c == '\n') {
			return
		}
		switch c {
		case '"', '\'', '`':
			*i = skipString(s, *i)
		case '/':
			if *i+1 < len(s) && s[*i+1] == '/' {
				*i = skipLineComment(s, *i+2)
			} else if *i+1 < len(s) && s[*i+1] == '*' {
				*i = skipBlockComment(s, *i+2)
			} else {
				*i++
			}
		case '{', '(', '[':
			depth++
			*i++
		case '}', ')', ']':
			if depth == 0 {
				return
			}
			depth--
			*i++
		default:
			*i++
		}
	}
}

func skipWS(s string, i *int) {
	for *i < len(s) {
		c := s[*i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == 0x0b {
			*i++
			continue
		}
		if c == '/' && *i+1 < len(s) && s[*i+1] == '/' {
			*i = skipLineComment(s, *i+2)
			continue
		}
		if c == '/' && *i+1 < len(s) && s[*i+1] == '*' {
			*i = skipBlockComment(s, *i+2)
			continue
		}
		return
	}
}

func eatSemi(s string, i *int) {
	skipWS(s, i)
	if *i < len(s) && s[*i] == ';' {
		*i++
	}
}

func skipLineComment(s string, i int) int {
	for i < len(s) && s[i] != '\n' {
		i++
	}
	return i
}

func skipBlockComment(s string, i int) int {
	for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
		i++
	}
	if i+1 < len(s) {
		return i + 2
	}
	return len(s)
}

func skipString(s string, i int) int {
	q := s[i]
	i++
	if q == '`' {
		for i < len(s) {
			if s[i] == '\\' {
				i += 2
				continue
			}
			if s[i] == '`' {
				return i + 1
			}
			if s[i] == '$' && i+1 < len(s) && s[i+1] == '{' {
				i += 2
				depth := 1
				for i < len(s) && depth > 0 {
					if s[i] == '"' || s[i] == '\'' || s[i] == '`' {
						i = skipString(s, i)
						continue
					}
					if s[i] == '{' {
						depth++
					} else if s[i] == '}' {
						depth--
					}
					i++
				}
				continue
			}
			i++
		}
		return i
	}
	for i < len(s) {
		if s[i] == '\\' {
			i += 2
			continue
		}
		if s[i] == q {
			return i + 1
		}
		i++
	}
	return i
}

func readString(s string, i *int) (val string, start, end int) {
	start = *i
	*i = skipString(s, *i)
	end = *i
	if end-start >= 2 {
		val = s[start+1 : end-1]
	}
	return
}

func skipIdent(s string, i int) int {
	for i < len(s) {
		r, w := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && w == 1 {
			break
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$' || r == '\u200c' || r == '\u200d' {
			i += w
			continue
		}
		break
	}
	return i
}

func identStart(s string, i int) bool {
	if i >= len(s) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return unicode.IsLetter(r) || r == '_' || r == '$'
}

func hasWord(s string, i int, w string) bool {
	if i+len(w) > len(s) || s[i:i+len(w)] != w {
		return false
	}
	end := i + len(w)
	if end < len(s) && identStart(s, end) {
		return false
	}
	if i > 0 {
		r, _ := utf8.DecodeLastRuneInString(s[:i])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$' {
			return false
		}
	}
	return true
}

func skipBalanced(s string, i *int, open, close byte) {
	if *i >= len(s) || s[*i] != open {
		return
	}
	depth := 0
	for *i < len(s) {
		c := s[*i]
		if c == '"' || c == '\'' || c == '`' {
			*i = skipString(s, *i)
			continue
		}
		if c == '/' && *i+1 < len(s) && s[*i+1] == '/' {
			*i = skipLineComment(s, *i+2)
			continue
		}
		if c == '/' && *i+1 < len(s) && s[*i+1] == '*' {
			*i = skipBlockComment(s, *i+2)
			continue
		}
		if c == open {
			depth++
		} else if c == close {
			depth--
			*i++
			if depth == 0 {
				return
			}
			continue
		}
		*i++
	}
}
