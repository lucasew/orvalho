package dependency

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fetchurl/fetchurl"
	"github.com/lewtec/lewkit/x/io/atomic"
)

// Store is a fetchurl-layout directory of hashed tarballs.
type Store struct {
	Dir    string
	Client *fetchurl.Fetcher
}

// DefaultStoreDir is the user cache content store when --store-dir is unset.
// It is <os.UserCacheDir>/orvalho (on Linux, $XDG_CACHE_HOME/orvalho or ~/.cache/orvalho).
func DefaultStoreDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "orvalho"), nil
}

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

// Fetch writes the tarball into the store via fetchurl.Fetcher
// (FETCHURL_SERVER, then the registry URL). Hash is lowercase hex.
func (s Store) Fetch(ctx context.Context, algo, hash string, urls []string) (err error) {
	if s.Has(algo, hash) {
		return nil
	}
	final := s.ObjectPath(algo, hash)
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return err
	}
	op := atomic.NewOperation(final, true)
	defer func() {
		if rerr := op.Rollback(); rerr != nil && err == nil {
			err = rerr
		}
	}()
	w, err := os.Create(op.StagingPath())
	if err != nil {
		return err
	}
	client := s.Client
	if client == nil {
		client = fetchurl.NewFetcher(nil)
	}
	err = client.Fetch(ctx, fetchurl.FetchOptions{
		Algo: algo,
		Hash: strings.ToLower(hash),
		URLs: urls,
		Out:  w,
	})
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		if errors.Is(err, fetchurl.ErrHashMismatch) {
			return fmt.Errorf("%w: %w", ErrIntegrity, err)
		}
		return fmt.Errorf("%w: %w", ErrRegistry, err)
	}
	return op.Commit()
}
