package imports

import (
	"encoding/json"
	"io/fs"
	"path"
	"strings"
)

// NodeModules looks up package files in FS using a Node.js CommonJS walk
// (TEC-17). From is the requiring file inside FS; empty means the FS root.
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
	if n.isDir(name) {
		return name, true
	}
	return "", false
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
