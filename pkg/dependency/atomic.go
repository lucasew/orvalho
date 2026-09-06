package dependency

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// writeFileAtomic writes data to path via a sibling temp file and rename.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".orvalho-write-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			if rerr := os.Remove(tmpName); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
				return
			}
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		if cerr := tmp.Close(); cerr != nil {
			return errors.Join(err, cerr)
		}
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		if cerr := tmp.Close(); cerr != nil {
			return errors.Join(err, cerr)
		}
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename %s: %w", path, err)
	}
	ok = true
	return nil
}
