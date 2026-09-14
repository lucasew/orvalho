package imports

import (
	"encoding/json"
	"io/fs"
	"path"
	"strings"
)

// NodeModules looks up package files in FS using a Node.js CommonJS walk
// (TEC-17). From is the requiring file inside FS; empty means the FS root.
// Specifiers that start with # resolve through that package's "imports".
type NodeModules struct {
	FS   fs.FS
	From string
}

// Resolve implements Handler[any]. A hit is a Script; a miss calls next.
func (n NodeModules) Resolve(spec string, next Resolver[any]) (any, error) {
	file, ok := n.Lookup(spec)
	if !ok {
		return next(spec)
	}
	data, err := fs.ReadFile(n.FS, file)
	if err != nil {
		return nil, err
	}
	return Script{Source: string(data), File: file}, nil
}

// Lookup returns the file path inside FS for spec, or false if this
// tree does not contain it.
func (n NodeModules) Lookup(spec string) (string, bool) {
	if n.FS == nil {
		return "", false
	}
	if strings.HasPrefix(spec, "#") {
		return n.lookupImport(spec)
	}
	if p := path.Clean(spec); p != ".." && !strings.HasPrefix(p, "../") && n.isFile(p) {
		return p, true
	}
	name, sub, ok := splitPackage(spec)
	if !ok {
		return "", false
	}
	pkgDir, ok := n.packageDir(name)
	if !ok {
		return "", false
	}
	return n.packageFile(pkgDir, sub)
}

func (n NodeModules) lookupImport(spec string) (string, bool) {
	if spec == "#" || strings.HasPrefix(spec, "#/") {
		return "", false
	}
	pkgDir, data, ok := n.nearestPackage()
	if !ok {
		return "", false
	}
	target, ok := resolveImports(data, spec)
	if !ok || target == "" || strings.HasPrefix(target, "#") {
		return "", false
	}
	if strings.HasPrefix(target, ".") {
		return n.file(path.Join(pkgDir, target))
	}
	return n.Lookup(target)
}

func (n NodeModules) nearestPackage() (pkgDir string, data []byte, ok bool) {
	start := "."
	if n.From != "" {
		start = path.Dir(n.From)
	}
	for dir := start; ; dir = path.Dir(dir) {
		p := path.Join(dir, "package.json")
		b, err := fs.ReadFile(n.FS, p)
		if err == nil {
			return dir, b, true
		}
		if dir == "." || dir == "/" {
			break
		}
	}
	return "", nil, false
}

func (n NodeModules) packageDir(name string) (string, bool) {
	start := "."
	if n.From != "" {
		start = path.Dir(n.From)
	}
	for dir := start; ; dir = path.Dir(dir) {
		cand := path.Join(dir, "node_modules", name)
		if n.isDir(cand) {
			return cand, true
		}
		if dir == "." || dir == "/" {
			break
		}
	}
	if d, ok := n.orvalhoPackageDir(name); ok {
		return d, true
	}
	if n.isDir(name) {
		return name, true
	}
	return "", false
}

// orvalhoPackageDir finds name in node_modules/.orvalho/<name>@<ver>/…
// when no hoisted or slot symlink is visible from From.
func (n NodeModules) orvalhoPackageDir(name string) (string, bool) {
	start := "."
	if n.From != "" {
		start = path.Dir(n.From)
	}
	for dir := start; ; dir = path.Dir(dir) {
		if d, ok := n.orvalhoSlot(path.Join(dir, "node_modules", ".orvalho"), name); ok {
			return d, true
		}
		if dir == "." || dir == "/" {
			break
		}
	}
	return "", false
}

func (n NodeModules) orvalhoSlot(store, name string) (string, bool) {
	parent, prefix := store, name+"@"
	if i := strings.IndexByte(name, '/'); i >= 0 && strings.HasPrefix(name, "@") {
		parent = path.Join(store, name[:i])
		prefix = name[i+1:] + "@"
	}
	ents, err := fs.ReadDir(n.FS, parent)
	if err != nil {
		return "", false
	}
	var best, bestVer string
	for _, e := range ents {
		en := e.Name()
		if !strings.HasPrefix(en, prefix) {
			continue
		}
		cand := path.Join(parent, en, "node_modules", name)
		if !n.isDir(cand) {
			continue
		}
		ver := slotVersion(en)
		if best == "" || cmpSlotVersion(ver, bestVer) > 0 {
			best, bestVer = cand, ver
		}
	}
	return best, best != ""
}

func slotVersion(en string) string {
	if i := strings.LastIndexByte(en, '@'); i >= 0 {
		return en[i+1:]
	}
	return en
}

// cmpSlotVersion is numeric major.minor.patch; a release beats the
// matching prerelease. Unparseable versions fall back to string order.
func cmpSlotVersion(a, b string) int {
	va, oka := parseSlotVersion(a)
	vb, okb := parseSlotVersion(b)
	if !oka || !okb {
		return strings.Compare(a, b)
	}
	if c := va.major - vb.major; c != 0 {
		return c
	}
	if c := va.minor - vb.minor; c != 0 {
		return c
	}
	if c := va.patch - vb.patch; c != 0 {
		return c
	}
	if va.pre == vb.pre {
		return 0
	}
	if va.pre == "" {
		return 1
	}
	if vb.pre == "" {
		return -1
	}
	return strings.Compare(va.pre, vb.pre)
}

type slotVer struct {
	major, minor, patch int
	pre                 string
}

func parseSlotVersion(s string) (slotVer, bool) {
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	pre := ""
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre = s[i+1:]
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return slotVer{}, false
	}
	var nums [3]int
	for i, p := range parts {
		n := 0
		if p == "" {
			return slotVer{}, false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return slotVer{}, false
			}
			n = n*10 + int(c-'0')
		}
		nums[i] = n
	}
	return slotVer{nums[0], nums[1], nums[2], pre}, true
}

func (n NodeModules) packageFile(pkgDir, sub string) (string, bool) {
	data, err := fs.ReadFile(n.FS, path.Join(pkgDir, "package.json"))
	if err == nil {
		if file, ok := resolveExports(data, sub); ok {
			return n.file(path.Join(pkgDir, file))
		}
		if exportMiss(data, sub) {
			return "", false
		}
		if sub == "" {
			var meta struct {
				Main string `json:"main"`
			}
			main := "index.js"
			if json.Unmarshal(data, &meta) == nil && meta.Main != "" {
				main = meta.Main
			}
			return n.file(path.Join(pkgDir, main))
		}
	}
	if sub != "" {
		return n.file(path.Join(pkgDir, sub))
	}
	return n.file(path.Join(pkgDir, "index.js"))
}

func (n NodeModules) file(p string) (string, bool) {
	p = path.Clean(p)
	if p == ".." || strings.HasPrefix(p, "../") {
		return "", false
	}
	for _, cand := range []string{p, p + ".js", path.Join(p, "index.js")} {
		if n.isFile(cand) {
			return cand, true
		}
	}
	return "", false
}

func (n NodeModules) isDir(p string) bool {
	st, err := fs.Stat(n.FS, p)
	return err == nil && st.IsDir()
}

func (n NodeModules) isFile(p string) bool {
	st, err := fs.Stat(n.FS, p)
	return err == nil && !st.IsDir()
}

func splitPackage(spec string) (name, sub string, ok bool) {
	if spec == "" || spec[0] == '.' {
		return "", "", false
	}
	if spec[0] == '@' {
		i := strings.IndexByte(spec[1:], '/')
		if i < 0 {
			return "", "", false
		}
		first := i + 1
		rest := spec[first+1:]
		if j := strings.IndexByte(rest, '/'); j >= 0 {
			return spec[:first+1+j], rest[j+1:], true
		}
		return spec, "", true
	}
	if i := strings.IndexByte(spec, '/'); i >= 0 {
		return spec[:i], spec[i+1:], true
	}
	return spec, "", true
}
