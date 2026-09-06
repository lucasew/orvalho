package dependency

import (
	"os"
	"path/filepath"

	"github.com/lewtec/lewkit/x/io/atomic"
)

// writeFileAtomic stages data next to path and renames it into place.
func writeFileAtomic(path string, data []byte, mode os.FileMode) (err error) {
	if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	op := atomic.NewOperation(path, true)
	defer func() {
		if rerr := op.Rollback(); rerr != nil && err == nil {
			err = rerr
		}
	}()
	if err = os.WriteFile(op.StagingPath(), data, mode); err != nil {
		return err
	}
	return op.Commit()
}
