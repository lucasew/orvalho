package main

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/lucasew/orvalho/pkg/workers"
)

// hostTreeFS mounts a host directory as guest fs.FS + WriteFS.
// Paths cannot escape root.
type hostTreeFS struct {
	root string
}

var (
	_ fs.FS              = hostTreeFS{}
	_ workers.WriteFS    = hostTreeFS{}
	_ workers.RealpathFS = hostTreeFS{}
)

func newHostTreeFS(root string) hostTreeFS {
	return hostTreeFS{root: filepath.Clean(root)}
}

func (h hostTreeFS) Open(name string) (fs.File, error) {
	if err := validGuestPath(name); err != nil {
		return nil, err
	}
	return os.DirFS(h.root).Open(name)
}

func (h hostTreeFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	full, err := h.resolve(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, data, perm)
}

func (h hostTreeFS) Mkdir(name string, perm fs.FileMode) error {
	full, err := h.resolve(name)
	if err != nil {
		return err
	}
	return os.MkdirAll(full, perm)
}

func (h hostTreeFS) Remove(name string) error {
	full, err := h.resolve(name)
	if err != nil {
		return err
	}
	return os.Remove(full)
}

func (h hostTreeFS) Realpath(name string) (string, error) {
	full, err := h.resolve(name)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(full)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(h.root, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return name, nil
	}
	return filepath.ToSlash(rel), nil
}

func (h hostTreeFS) resolve(name string) (string, error) {
	if err := validGuestPath(name); err != nil {
		return "", err
	}
	full := filepath.Join(h.root, filepath.FromSlash(path.Clean(name)))
	rel, err := filepath.Rel(h.root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fs.ErrInvalid
	}
	return full, nil
}

func validGuestPath(name string) error {
	if name != "." && !fs.ValidPath(name) {
		return fs.ErrInvalid
	}
	cleaned := path.Clean(name)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fs.ErrInvalid
	}
	return nil
}
