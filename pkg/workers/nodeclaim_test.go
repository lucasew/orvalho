package workers

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lucasew/orvalho/pkg/imports"
)

func TestNodeClaims(t *testing.T) {
	nodeRoot, claimsFile, claimsDir := nodeTestPaths()
	if _, err := os.Stat(filepath.Join(nodeRoot, "parallel")); err != nil {
		t.Skip("testdata/node missing; workspaced codebase apply")
	}
	claims, err := readAllClaims(claimsFile, claimsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) == 0 {
		t.Fatal("testdata/claims is empty")
	}
	for _, rel := range claims {
		t.Run(rel, func(t *testing.T) {
			runClaimedNodeTest(t, nodeRoot, rel)
		})
	}
}

func runClaimedNodeTest(t *testing.T, nodeRoot, rel string) {
	t.Helper()
	file := filepath.Join(nodeRoot, filepath.FromSlash(rel))
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	iso := New("", Options{
		Argv: []string{"node", file},
		FS:   os.DirFS(nodeRoot),
		Imports: append(NodeScriptImports(), imports.NodeModules{
			FS:   os.DirFS(nodeRoot),
			From: filepath.ToSlash(rel),
		}),
	})
	if err := iso.ScriptMain(t.Context(), string(src), filepath.ToSlash(rel)); err != nil {
		t.Fatal(err)
	}
}

func nodeTestPaths() (nodeRoot, claimsFile, claimsDir string) {
	_, file, _, _ := runtime.Caller(0)
	repo := filepath.Join(filepath.Dir(file), "..", "..")
	return filepath.Join(repo, "testdata", "node"),
		filepath.Join(repo, "testdata", "claims"),
		filepath.Join(repo, "testdata", "claims.d")
}

func readAllClaims(file, dir string) ([]string, error) {
	var out []string
	if _, err := os.Stat(file); err == nil {
		got, err := readClaims(file)
		if err != nil {
			return nil, err
		}
		out = append(out, got...)
	}
	ents, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, e := range ents {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		got, err := readClaims(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, got...)
	}
	return out, nil
}

func readClaims(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out, sc.Err()
}
