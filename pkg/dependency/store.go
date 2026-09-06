package dependency

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fetchurl/fetchurl"
)

// Store is a fetchurl-layout directory of hashed tarballs.
type Store struct {
	Dir    string
	Client *fetchurl.Fetcher
}

// DefaultStoreDir is the project-local content store when --store-dir is unset.
const DefaultStoreDir = ".orvalho/store"

// ObjectPath is {dir}/{algo}/{shard}/{hash}.
func (s Store) ObjectPath(algo, hash string) string {
	hash = strings.ToLower(hash)
	algo = strings.ToLower(algo)
	shard := hash
	if len(hash) >= 2 {
		shard = hash[:2]
	}
	return filepath.Join(s.Dir, algo, shard, hash)
}

// Has reports whether the object is already on disk.
func (s Store) Has(algo, hash string) bool {
	_, err := os.Stat(s.ObjectPath(algo, hash))
	return err == nil
}

// Open returns the stored tarball.
func (s Store) Open(algo, hash string) (*os.File, error) {
	return os.Open(s.ObjectPath(algo, hash))
}

// Fetch writes the tarball into the store. URLs are the fetchurl source list
// (registry resolved URL). Hash is lowercase hex.
func (s Store) Fetch(ctx context.Context, algo, hash string, urls []string) error {
	if s.Has(algo, hash) {
		return nil
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.Dir, "put-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if rerr := os.Remove(tmpName); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
			return
		}
	}()

	client := s.Client
	if client == nil {
		client = fetchurl.NewFetcher(nil)
	}
	err = client.Fetch(ctx, fetchurl.FetchOptions{
		Algo: algo,
		Hash: strings.ToLower(hash),
		URLs: urls,
		Out:  tmp,
	})
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		if errors.Is(err, fetchurl.ErrHashMismatch) {
			return fmt.Errorf("%w: %w", ErrIntegrity, err)
		}
		return fmt.Errorf("%w: %w", ErrRegistry, err)
	}

	final := s.ObjectPath(algo, hash)
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmpName, final); err != nil {
		// Copy across filesystems.
		return copyFile(tmpName, final)
	}
	return nil
}

func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := in.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		if cerr := out.Close(); cerr != nil {
			err = errors.Join(err, cerr)
		}
		if rerr := os.Remove(dst); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
			err = errors.Join(err, rerr)
		}
		return err
	}
	return out.Close()
}
