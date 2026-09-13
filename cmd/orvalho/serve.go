package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	lewcmd "github.com/lewtec/lewkit/x/cmd"

	"github.com/lucasew/orvalho/pkg/ovpkg"
	"github.com/lucasew/orvalho/pkg/workers"
	"github.com/lucasew/orvalho/pkg/workers/bundle"
)

type serveCmd struct {
	Addr    lewcmd.StringArg   `long:"addr" help:"listen address (default :8787, or :PORT from package)"`
	EnvFile lewcmd.StringArg   `long:"env-file" help:"path to .env / .dev.vars for runtime.env"`
	Var     []lewcmd.StringArg `long:"var" help:"runtime.env NAME=value (repeatable)"`
	Path    lewcmd.StringArg
}

func (s *serveCmd) Run(ctx context.Context) error {
	path := s.Path.Value()
	if path == "" {
		path = "."
	}

	pkg, err := ovpkg.OpenPath(path)
	if err != nil {
		return fmt.Errorf("open package: %w", err)
	}

	runtimeEnv, err := collectServeRuntimeEnv(lewcmd.Values(s.Var), s.EnvFile.Value())
	if err != nil {
		return err
	}
	if len(runtimeEnv) > 0 {
		pkg, err = pkg.WithRuntimeEnv(runtimeEnv)
		if err != nil {
			return err // never-allocate
		}
	}

	agent, err := pkg.SingleAgent()
	if err != nil {
		return err
	}

	script, err := loadGuestScript(path, pkg, agent.Entrypoint)
	if err != nil {
		return err
	}

	egress, err := pkg.Egress()
	if err != nil {
		return err
	}

	bindings, err := materializeAgentBindings(pkg, agent)
	if err != nil {
		return err // never-allocate
	}

	// Product injects fetch from package egress (empty allowlist ⇒ deny all).
	iso := workers.New(script, workers.Options{
		Env:      agent.Env,
		Bindings: bindings,
		Fetch:    workers.HTTPFetch(workers.EgressList(egress), nil, 0),
	})

	addr := s.Addr.Value()
	if addr == "" {
		if port, err := pkg.Port(); err != nil {
			return err
		} else if port > 0 {
			addr = fmt.Sprintf(":%d", port)
		} else {
			addr = ":8787"
		}
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           workers.Handler(iso),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	actual := ln.Addr().String()
	fmt.Printf("orvalho serve: %s (agent %s entry %s) on http://%s\n", path, agent.Name, agent.Entrypoint, actual)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		shutErr := srv.Shutdown(shutdownCtx)
		err := <-errCh
		if err == nil || err == http.ErrServerClosed {
			return shutErr
		}
		if shutErr != nil {
			return errors.Join(err, shutErr)
		}
		return err
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

// loadGuestScript returns goja-ready source: esbuild bundle for multi-file ESM,
// or PrepareGuestScript for single-file workers.
func loadGuestScript(path string, pkg *ovpkg.Package, entry string) (string, error) {
	src, err := pkg.Get(entry)
	if err != nil {
		return "", fmt.Errorf("entrypoint %s: %w", entry, err)
	}
	if !bundle.NeedsBundle(string(src)) {
		return workers.PrepareGuestScript(string(src)), nil
	}

	pkgDir, cleanup, err := packageWorkDir(path, pkg)
	if err != nil {
		return "", err
	}
	if cleanup != nil {
		defer cleanup()
	}

	fmt.Fprintf(os.Stderr, "orvalho serve: bundling %s with esbuild…\n", entry)
	bundled, err := bundle.BundleEntry(bundle.BundleOptions{
		PackageDir: pkgDir,
		Entry:      entry,
	})
	if err != nil {
		return "", fmt.Errorf("bundle entry %s: %w", entry, err)
	}
	return bundled, nil
}

// packageWorkDir returns a filesystem directory for the package (for esbuild).
// Directory packages use path as-is; zip packages are extracted to a temp dir.
func packageWorkDir(path string, pkg *ovpkg.Package) (dir string, cleanup func(), err error) {
	st, err := os.Stat(path)
	if err == nil && st.IsDir() {
		abs, err := filepath.Abs(path)
		return abs, nil, err
	}
	// Zip or non-dir: materialize Files + manifest into temp.
	tmp, err := os.MkdirTemp("", "orvalho-pkg-*")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() {
		if err := os.RemoveAll(tmp); err != nil {
			// Best-effort temp cleanup after serve materialize.
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, ovpkg.ManifestName), pkg.Manifest, 0o644); err != nil {
		cleanup()
		return "", nil, err
	}
	for name, data := range pkg.Files {
		full := filepath.Join(tmp, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			cleanup()
			return "", nil, err
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			cleanup()
			return "", nil, err
		}
	}
	return tmp, cleanup, nil
}

// collectServeRuntimeEnv builds runtime.env: process env (all keys), then
// env-file, then --var (highest precedence). Package CUE decides what is used.
func collectServeRuntimeEnv(varFlags []string, envFile string) (map[string]string, error) {
	out := map[string]string{}
	// Process environment: full map; CUE only keeps what it projects.
	for _, e := range os.Environ() {
		k, v, ok := strings.Cut(e, "=")
		if !ok || k == "" {
			continue
		}
		out[k] = v
	}
	if envFile != "" {
		m, err := parseEnvFile(envFile)
		if err != nil {
			return nil, err
		}
		for k, v := range m {
			out[k] = v
		}
	}
	for _, flag := range varFlags {
		k, v, ok := strings.Cut(flag, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("%w: %q", ErrInvalidVarFlag, flag)
		}
		out[k] = v
	}
	return out, nil
}

func parseEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("env-file: %w", err)
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%w: %s:%d", ErrEnvFileFormat, path, lineNo)
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if len(v) >= 2 {
			if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
				v = v[1 : len(v)-1]
			}
		}
		if k == "" {
			return nil, fmt.Errorf("%w: %s:%d", ErrEnvFileEmptyKey, path, lineNo)
		}
		out[k] = v
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
