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

// unpackTarball extracts an npm package tarball (package/… prefix) into dest.
func unpackTarball(r io.Reader, dest string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		name = strings.TrimPrefix(name, "package/")
		if name == "" || name == "." {
			continue
		}
		if err := unpackEntry(dest, name, hdr, tr); err != nil {
			return err
		}
	}
}

func unpackEntry(dest, name string, hdr *tar.Header, tr *tar.Reader) error {
	clean := path.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return fmt.Errorf("unsafe tar path %q", name)
	}
	target := filepath.Join(dest, filepath.FromSlash(clean))
	if !strings.HasPrefix(target, dest+string(os.PathSeparator)) && target != dest {
		return fmt.Errorf("unsafe tar path %q", name)
	}
	switch hdr.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, 0o755)
	case tar.TypeReg, tar.TypeRegA:
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, hdr.FileInfo().Mode()&0o777)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(f, tr)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	case tar.TypeSymlink:
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if rerr := os.Remove(target); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
			return rerr
		}
		return os.Symlink(hdr.Linkname, target)
	default:
		return nil
	}
}
