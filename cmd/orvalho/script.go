package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/lucasew/orvalho/pkg/dependency"
	"github.com/lucasew/orvalho/pkg/imports"
	"github.com/lucasew/orvalho/pkg/workers"
	"github.com/lucasew/orvalho/pkg/workers/bundle"
)

var scriptCmd = &cobra.Command{
	Use:   "script",
	Short: "Run a Script as main (Node-compatible, not default.fetch)",
}

var scriptRunCmd = &cobra.Command{
	Use:   "run [target] [args...]",
	Short: "Evaluate a file or package.json script as main",
	Long: `Evaluate a Script target as CommonJS main. default.fetch is not required.

Target is a file path, or a package.json script name when the path is not a file.
package.json scripts run through sh -c with a scoped PATH whose node entry is
this CLI (no host Node.js) and node_modules/.bin. --data-dir is not required.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runScriptRun,
}

func init() {
	scriptCmd.AddCommand(scriptRunCmd)
	rootCmd.AddCommand(scriptCmd)
}

func runScriptRun(cmd *cobra.Command, args []string) error {
	if verbose {
		if err := os.Setenv("ORVALHO_VERBOSE", "1"); err != nil {
			return err
		}
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	file, shell, err := resolveScriptTarget(dir, args[0])
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if file != "" {
		return runScriptFile(ctx, dir, file, args[1:])
	}
	if len(args) > 1 {
		shell = shell + " " + strings.Join(args[1:], " ")
	}
	return runPackageScript(ctx, dir, shell)
}

func resolveScriptTarget(dir, target string) (file, shell string, err error) {
	if target == "" {
		return "", "", ErrScriptMissing
	}
	cand := target
	if !filepath.IsAbs(cand) {
		cand = filepath.Join(dir, target)
	}
	st, err := os.Stat(cand)
	if err == nil && !st.IsDir() {
		return cand, "", nil
	}
	scripts, err := readPackageScripts(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", fmt.Errorf("%w: %s", ErrScriptMissing, target)
		}
		return "", "", err
	}
	cmd, ok := scripts[target]
	if !ok || cmd == "" {
		return "", "", fmt.Errorf("%w: %s", ErrScriptMissing, target)
	}
	return "", cmd, nil
}

func readPackageScripts(dir string) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil, err
	}
	var meta struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("package.json: %w", err)
	}
	return meta.Scripts, nil
}

func runScriptFile(ctx context.Context, dir, file string, extra []string) error {
	src, err := os.ReadFile(file)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %v", ErrScriptMissing, err)
		}
		return err
	}
	root, rel := scriptTree(dir, file)
	rel = evalFSRel(root, rel)
	tree := newHostTreeFS(root)
	argv := append([]string{"node", file}, extra...)
	exe, err := os.Executable()
	if err != nil {
		exe = "orvalho"
	}
	opts := workers.Options{
		Argv:       argv,
		FS:         tree,
		ProcessEnv: processEnvMap(),
		Cwd:        dir,
		PID:        os.Getpid(),
		ExecPath:   exe,
		Platform:   nodeScriptPlatform(),
		Arch:       nodeScriptArch(),
		Spawn:      hostSpawn,
		Dial:       hostDial,
		Lookup:     hostLookup,
		Listen:     hostListen,
		Imports: append(workers.NodeScriptImports(),
			realpathScripts{root: root, inner: imports.NodeModules{FS: tree, From: rel}},
		),
		PrepareSource: prepareScriptSource,
	}
	if verboseOn() {
		opts.Trace = scriptTrace
	}
	iso := workers.New("", opts)
	return iso.ScriptMain(ctx, string(src), rel)
}

func verboseOn() bool {
	return verbose || os.Getenv("ORVALHO_VERBOSE") != ""
}

func scriptTrace(format string, args ...any) {
	if !verboseOn() {
		return
	}
	fmt.Fprintf(os.Stderr, "orvalho: %s ", time.Now().Format("15:04:05.000"))
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

// evalFSRel resolves symlinks so require walks isolated slot siblings (TEC-17).
func evalFSRel(root, rel string) string {
	full := filepath.Join(root, filepath.FromSlash(rel))
	real, err := filepath.EvalSymlinks(full)
	if err != nil {
		return rel
	}
	out, err := filepath.Rel(root, real)
	if err != nil || strings.HasPrefix(out, "..") {
		return rel
	}
	return filepath.ToSlash(out)
}

type realpathScripts struct {
	root  string
	inner imports.NodeModules
}

func (r realpathScripts) WithFrom(from string) imports.Handler[any] {
	r.inner.From = from
	return r
}

func (r realpathScripts) Resolve(spec string, next imports.Resolver[any]) (any, error) {
	v, err := r.inner.Resolve(spec, next)
	if err != nil {
		return v, err
	}
	s, ok := v.(imports.Script)
	if !ok || s.File == "" {
		return v, nil
	}
	s.File = evalFSRel(r.root, s.File)
	return s, nil
}

func prepareScriptSource(src, file string) (string, error) {
	return bundle.TransformCJS(src, file)
}

func hostListen(ctx context.Context, req workers.ListenReq) (net.Listener, error) {
	network := req.Network
	if network == "" {
		network = "tcp"
	}
	host, port, err := net.SplitHostPort(req.Address)
	if err != nil {
		host, port = req.Address, "0"
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() {
		return nil, workers.ErrListenDenied
	}
	var lc net.ListenConfig
	return lc.Listen(ctx, network, net.JoinHostPort(host, port))
}

func hostDial(ctx context.Context, req workers.DialReq) (net.Conn, error) {
	network := req.Network
	if network == "" {
		network = "tcp"
	}
	var d net.Dialer
	return d.DialContext(ctx, network, req.Address)
}

func hostLookup(ctx context.Context, req workers.LookupReq) ([]workers.LookupAddr, error) {
	network := req.Network
	if network == "" {
		network = "ip"
	}
	var r net.Resolver
	ips, err := r.LookupIP(ctx, network, req.Hostname)
	if err != nil {
		return nil, err
	}
	out := make([]workers.LookupAddr, 0, len(ips))
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			out = append(out, workers.LookupAddr{Address: v4.String(), Family: 4})
			continue
		}
		out = append(out, workers.LookupAddr{Address: ip.String(), Family: 6})
	}
	return out, nil
}

func hostSpawn(ctx context.Context, req workers.SpawnReq) (workers.Spawned, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, req.File, req.Args...)
	if req.Cwd != "" {
		cmd.Dir = req.Cwd
	}
	if len(req.Env) > 0 {
		cmd.Env = req.Env
	}
	if req.Stdin != nil {
		cmd.Stdin = req.Stdin
	}
	if req.Stdout != nil {
		cmd.Stdout = req.Stdout
	}
	if req.Stderr != nil {
		cmd.Stderr = req.Stderr
	} else {
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan workers.SpawnWait, 1)
	go func() {
		err := cmd.Wait()
		code := 0
		if err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				code = ee.ExitCode()
			} else {
				code = 1
			}
		}
		done <- workers.SpawnWait{Code: code}
	}()
	return hostSpawned{cmd: cmd, done: done}, nil
}

type hostSpawned struct {
	cmd  *exec.Cmd
	done chan workers.SpawnWait
}

func (h hostSpawned) PID() int {
	if h.cmd.Process != nil {
		return h.cmd.Process.Pid
	}
	return 0
}

func (h hostSpawned) Done() <-chan workers.SpawnWait { return h.done }

func (h hostSpawned) Kill() error {
	if h.cmd.Process == nil {
		return nil
	}
	return h.cmd.Process.Kill()
}

func processEnvMap() map[string]string {
	env := make(map[string]string)
	for _, e := range os.Environ() {
		k, v, ok := strings.Cut(e, "=")
		if ok && k != "" {
			env[k] = v
		}
	}
	// Guest paths stay inside the mounted tree (normalizeGuest strips /).
	env["HOME"] = "/"
	return env
}

func nodeScriptPlatform() string {
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
		return runtime.GOOS
	default:
		return "linux"
	}
}

func nodeScriptArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "arm64":
		return "arm64"
	default:
		return "x64"
	}
}

func scriptTree(dir, file string) (root, rel string) {
	rel, err := filepath.Rel(dir, file)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.Dir(file), filepath.Base(file)
	}
	return dir, filepath.ToSlash(rel)
}

func runPackageScript(ctx context.Context, dir, shellCmd string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	nodeDir, err := dependency.ScopedNodeDir(exe)
	if err != nil {
		return err
	}
	defer func() {
		if err := os.RemoveAll(nodeDir); err != nil {
			fmt.Fprintf(os.Stderr, "orvalho script: remove trampoline: %v\n", err)
		}
	}()

	scriptTrace("sh -c %q", shellCmd)
	cmd := exec.CommandContext(ctx, "sh", "-c", shellCmd)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = withPathPrefix(os.Environ(), nodeDir, filepath.Join(dir, "node_modules", ".bin"))
	return cmd.Run()
}

func withPathPrefix(env []string, dirs ...string) []string {
	prefix := strings.Join(dirs, string(os.PathListSeparator))
	out := make([]string, 0, len(env)+1)
	found := false
	for _, e := range env {
		k, v, ok := strings.Cut(e, "=")
		if !ok || k != "PATH" {
			out = append(out, e)
			continue
		}
		if prefix == "" {
			out = append(out, e)
		} else {
			out = append(out, "PATH="+prefix+string(os.PathListSeparator)+v)
		}
		found = true
	}
	if !found && prefix != "" {
		out = append(out, "PATH="+prefix)
	}
	return out
}

func rewriteNodeInvocation(args []string) []string {
	if len(args) == 0 || filepath.Base(args[0]) != "node" {
		return args
	}
	out := make([]string, 0, len(args)+2)
	out = append(out, args[0], "script", "run")
	return append(out, args[1:]...)
}
