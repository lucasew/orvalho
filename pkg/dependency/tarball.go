package dependency

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// unpacker writes one npm package tarball (package/… prefix) into dest.
type unpacker struct {
	dest string
	tr   *tar.Reader
}

func unpackTarball(r io.Reader, dest string) (err error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := gz.Close(); err == nil {
			err = cerr
		}
	}()
	u := unpacker{dest: dest, tr: tar.NewReader(gz)}
	for {
		hdr, nextErr := u.tr.Next()
		if nextErr == io.EOF {
			return nil
		}
		if nextErr != nil {
			return nextErr
		}
		if err := u.write(hdr); err != nil {
			return err
		}
	}
}

func (u unpacker) write(hdr *tar.Header) error {
	name := strings.TrimPrefix(hdr.Name, "./")
	name = strings.TrimPrefix(name, "package/")
	if name == "" || name == "." {
		return nil
	}
	target, err := u.target(name)
	if err != nil {
		return err
	}
	switch hdr.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, 0o755)
	case tar.TypeReg, tar.TypeRegA:
		return u.file(target, hdr.FileInfo().Mode()&0o777)
	case tar.TypeSymlink:
		return u.symlink(target, hdr.Linkname)
	default:
		return nil
	}
}

func (u unpacker) target(name string) (string, error) {
	clean := path.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", fmt.Errorf("%w: %q", ErrTarPath, name)
	}
	target := filepath.Join(u.dest, filepath.FromSlash(clean))
	if !strings.HasPrefix(target, u.dest+string(os.PathSeparator)) && target != u.dest {
		return "", fmt.Errorf("%w: %q", ErrTarPath, name)
	}
	return target, nil
}

func (u unpacker) file(target string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, u.tr)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func (u unpacker) symlink(target, link string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if rerr := os.Remove(target); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
		return rerr
	}
	return os.Symlink(link, target)
}
