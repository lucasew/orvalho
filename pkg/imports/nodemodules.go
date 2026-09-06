package imports

import (
	"encoding/json"
	"io/fs"
	"path"
	"strings"
)

// NodeModules looks up package files in FS. If FS has a node_modules
// directory, packages are loaded from there; otherwise the root is the
// package tree. Specifiers that already name a file are returned as-is.
type NodeModules struct {
	FS fs.FS
}

// Lookup returns the file path inside FS for spec, or false if this
// tree does not contain it.
func (n NodeModules) Lookup(spec string) (string, bool) {
	if n.FS == nil {
		return "", false
	}
	if file, ok := n.file(spec); ok {
		return file, true
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
	if n.isDir(path.Join("node_modules", name)) {
		return path.Join("node_modules", name), true
	}
	if n.isDir(name) {
		return name, true
	}
	return "", false
}

func (n NodeModules) packageFile(pkgDir, sub string) (string, bool) {
	if sub != "" {
		return n.file(path.Join(pkgDir, sub))
	}
	main := "index.js"
	if data, err := fs.ReadFile(n.FS, path.Join(pkgDir, "package.json")); err == nil {
		var meta struct {
			Main string `json:"main"`
		}
		if json.Unmarshal(data, &meta) == nil && meta.Main != "" {
			main = meta.Main
		}
	}
	return n.file(path.Join(pkgDir, main))
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
