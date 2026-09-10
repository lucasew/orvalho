package workers

import (
	"io/fs"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/dop251/goja"
)

// nodeOxideBinding is require("@tailwindcss/oxide"). The napi/WASI
// addons do not load in this isolate; Scanner walks the guest FS.
type nodeOxideBinding struct{}

var _ Binding = nodeOxideBinding{}

func (nodeOxideBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	if v, ok := iso.moduleCache["@tailwindcss/oxide"]; ok {
		if o, ok := v.(*goja.Object); ok {
			return o, nil
		}
	}
	return newNodeOxide(iso), nil
}

func newNodeOxide(iso *Isolate) *goja.Object {
	n := &nodeOxide{iso: iso}
	obj := iso.vm.NewObject()
	mustSet(obj, "Scanner", n.jsScanner)
	mustSet(obj, "default", obj)
	return obj
}

type nodeOxide struct {
	iso *Isolate
}

type oxideSource struct {
	base, pattern string
	negated       bool
}

type oxideScan struct {
	iso     *Isolate
	sources []oxideSource
	files   []string
	scanned []string
}

func (n *nodeOxide) jsScanner(call goja.ConstructorCall) *goja.Object {
	s := &oxideScan{iso: n.iso}
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		s.sources = parseOxideSources(n.iso, call.Argument(0).ToObject(n.iso.vm))
	}
	obj := call.This
	mustSet(obj, "scan", s.jsScan)
	mustSet(obj, "scanFiles", s.jsScanFiles)
	mustSet(obj, "getCandidatesWithPositions", s.jsPositions)
	mustAccessor(obj, "files", n.iso.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return n.iso.vm.ToValue(s.files)
	}))
	mustAccessor(obj, "scannedFiles", n.iso.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return n.iso.vm.ToValue(s.scanned)
	}))
	mustAccessor(obj, "globs", n.iso.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return s.globObjs(false)
	}))
	mustAccessor(obj, "normalizedSources", n.iso.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return s.globObjs(true)
	}))
	return obj
}

func parseOxideSources(iso *Isolate, opts *goja.Object) []oxideSource {
	if opts == nil {
		return nil
	}
	raw := opts.Get("sources")
	if raw == nil || goja.IsUndefined(raw) || goja.IsNull(raw) {
		return nil
	}
	arr := raw.ToObject(iso.vm)
	n := int(arr.Get("length").ToInteger())
	out := make([]oxideSource, 0, n)
	for i := 0; i < n; i++ {
		item := arr.Get(strconv.Itoa(i))
		if item == nil || goja.IsUndefined(item) || goja.IsNull(item) {
			continue
		}
		o := item.ToObject(iso.vm)
		src := oxideSource{
			base:    guestScanBase(iso, jsToString(o.Get("base"))),
			pattern: jsToString(o.Get("pattern")),
			negated: o.Get("negated") != nil && o.Get("negated").ToBoolean(),
		}
		if src.pattern == "" {
			src.pattern = "**/*"
		}
		out = append(out, src)
	}
	return out
}

func guestScanBase(iso *Isolate, raw string) string {
	if raw == "" {
		return "."
	}
	if mapped, ok := iso.absHostSpec(raw); ok {
		return mapped
	}
	p := strings.ReplaceAll(raw, "\\", "/")
	p = strings.TrimLeft(p, "/")
	if p == "" {
		return "."
	}
	return path.Clean(p)
}

func (s *oxideScan) jsScan(goja.FunctionCall) goja.Value {
	return s.iso.vm.ToValue(s.scanAll())
}

func (s *oxideScan) jsScanFiles(call goja.FunctionCall) goja.Value {
	var cands []string
	seen := map[string]struct{}{}
	if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) {
		return s.iso.vm.ToValue(cands)
	}
	arr := call.Argument(0).ToObject(s.iso.vm)
	n := int(arr.Get("length").ToInteger())
	for i := 0; i < n; i++ {
		item := arr.Get(strconv.Itoa(i))
		if item == nil || goja.IsUndefined(item) || goja.IsNull(item) {
			continue
		}
		o := item.ToObject(s.iso.vm)
		body := jsToString(o.Get("content"))
		if body == "" {
			if f := jsToString(o.Get("file")); f != "" {
				if b, err := s.readGuest(guestScanBase(s.iso, f)); err == nil {
					body = string(b)
				}
			}
		}
		for _, tok := range extractCandidates(body) {
			if _, ok := seen[tok]; ok {
				continue
			}
			seen[tok] = struct{}{}
			cands = append(cands, tok)
		}
	}
	return s.iso.vm.ToValue(cands)
}

func (s *oxideScan) jsPositions(call goja.FunctionCall) goja.Value {
	return s.iso.vm.NewArray()
}

func (s *oxideScan) globObjs(normalized bool) goja.Value {
	out := make([]any, 0, len(s.sources))
	for _, src := range s.sources {
		if !normalized && src.negated {
			continue
		}
		o := s.iso.vm.NewObject()
		mustSet(o, "base", src.base)
		mustSet(o, "pattern", src.pattern)
		out = append(out, o)
	}
	return s.iso.vm.ToValue(out)
}

func (s *oxideScan) scanAll() []string {
	s.files = s.files[:0]
	s.scanned = s.scanned[:0]
	var cands []string
	seen := map[string]struct{}{}
	skip := map[string]struct{}{}
	for _, src := range s.sources {
		if src.negated {
			for _, f := range s.listFiles(src) {
				skip[f] = struct{}{}
			}
		}
	}
	for _, src := range s.sources {
		if src.negated {
			continue
		}
		for _, f := range s.listFiles(src) {
			if _, banned := skip[f]; banned {
				continue
			}
			s.files = append(s.files, f)
			body, err := s.readGuest(f)
			if err != nil {
				continue
			}
			s.scanned = append(s.scanned, f)
			for _, tok := range extractCandidates(string(body)) {
				if _, ok := seen[tok]; ok {
					continue
				}
				seen[tok] = struct{}{}
				cands = append(cands, tok)
			}
		}
	}
	return cands
}

func (s *oxideScan) listFiles(src oxideSource) []string {
	if s.iso == nil || s.iso.opts.FS == nil {
		return nil
	}
	root := src.base
	if root == "" {
		root = "."
	}
	matchers := compileGlobs(src.pattern)
	var out []string
	_ = fs.WalkDir(s.iso.opts.FS, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if skipScanDir(name) {
				return fs.SkipDir
			}
			return nil
		}
		rel := p
		if root != "." && strings.HasPrefix(p, root+"/") {
			rel = p[len(root)+1:]
		}
		if matchAnyGlob(rel, matchers) {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func (s *oxideScan) readGuest(name string) ([]byte, error) {
	if s.iso == nil || s.iso.opts.FS == nil {
		return nil, fs.ErrNotExist
	}
	return fs.ReadFile(s.iso.opts.FS, name)
}

func skipScanDir(name string) bool {
	switch name {
	case "node_modules", ".git", ".orvalho", ".astro", ".vite", "dist", ".output":
		return true
	default:
		return false
	}
}

var oxideToken = regexp.MustCompile(`[A-Za-z0-9@_%./:#\[\]!&+,~=|-]+`)

func extractCandidates(src string) []string {
	raw := oxideToken.FindAllString(src, -1)
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, tok := range raw {
		if !keepCandidate(tok) {
			continue
		}
		if _, ok := seen[tok]; ok {
			continue
		}
		seen[tok] = struct{}{}
		out = append(out, tok)
	}
	return out
}

func keepCandidate(s string) bool {
	if len(s) < 2 || len(s) > 200 {
		return false
	}
	hasLetter := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
			hasLetter = true
			break
		}
	}
	return hasLetter
}

func compileGlobs(pattern string) []*regexp.Regexp {
	var out []*regexp.Regexp
	for _, p := range expandBraces(pattern) {
		if re, err := regexp.Compile(globToRegexp(p)); err == nil {
			out = append(out, re)
		}
	}
	if len(out) == 0 {
		out = append(out, regexp.MustCompile(`.*`))
	}
	return out
}

func matchAnyGlob(name string, res []*regexp.Regexp) bool {
	name = strings.ReplaceAll(name, "\\", "/")
	for _, re := range res {
		if re.MatchString(name) {
			return true
		}
	}
	return false
}

func expandBraces(p string) []string {
	i := strings.IndexByte(p, '{')
	if i < 0 {
		return []string{p}
	}
	j := strings.IndexByte(p[i:], '}')
	if j < 0 {
		return []string{p}
	}
	j += i
	var out []string
	for _, alt := range strings.Split(p[i+1:j], ",") {
		out = append(out, expandBraces(p[:i]+alt+p[j+1:])...)
	}
	return out
}

func globToRegexp(pattern string) string {
	var b strings.Builder
	b.WriteByte('^')
	i := 0
	for i < len(pattern) {
		switch {
		case strings.HasPrefix(pattern[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 3
		case strings.HasPrefix(pattern[i:], "**"):
			b.WriteString(".*")
			i += 2
		case pattern[i] == '*':
			b.WriteString("[^/]*")
			i++
		case pattern[i] == '?':
			b.WriteString("[^/]")
			i++
		case strings.ContainsRune(`.+()|[]{}^$\\`, rune(pattern[i])):
			b.WriteByte('\\')
			b.WriteByte(pattern[i])
			i++
		default:
			b.WriteByte(pattern[i])
			i++
		}
	}
	b.WriteByte('$')
	return b.String()
}
