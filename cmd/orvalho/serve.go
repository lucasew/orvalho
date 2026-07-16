package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	actorjs "orvalho/pkg/actor/js"
	"orvalho/pkg/ovpkg"
)

var (
	serveListen  string
	serveVarFlags []string
	serveEnvFile  string
)

var serveCmd = &cobra.Command{
	Use:   "serve [path]",
	Short: "Serve one package (zip or directory) over local HTTP",
	Long: `Load an Orvalho package from a zip file or directory (orvalho.cue + agents)
and serve it on a local HTTP address by invoking default.fetch for each request.

This is a development convenience: no mesh, no manager pairing, no package
signature checks. --data-dir is not required.

Outside values fill runtime.env (then CUE projects to agents.*.env):
  --var NAME=value (repeatable, highest precedence)
  --env-file path (.env / .dev.vars KEY=value lines)
  process environment for keys referenced by the package (declared fields only after projection)

Exactly one agent is required. CUE or binding failures never-allocate (exit before listen).

Entry convention: Workers-shaped default export with fetch(request, env, ctx).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runServe,
}

func init() {
	serveCmd.Flags().StringVar(&serveListen, "listen", "", "listen address (default :8787, or :PORT from package port/publish.port)")
	serveCmd.Flags().StringArrayVar(&serveVarFlags, "var", nil, "runtime.env entry NAME=value (repeatable; overrides env file and process env)")
	serveCmd.Flags().StringVar(&serveEnvFile, "env-file", "", "path to .env / .dev.vars (KEY=value lines) for runtime.env")
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) == 1 {
		path = args[0]
	}

	pkg, err := ovpkg.OpenPath(path)
	if err != nil {
		return fmt.Errorf("open package: %w", err)
	}

	runtimeEnv, err := collectServeRuntimeEnv(serveVarFlags, serveEnvFile)
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
	src, err := pkg.Get(agent.Entrypoint)
	if err != nil {
		return fmt.Errorf("entrypoint %s: %w", agent.Entrypoint, err)
	}

	egress, err := pkg.Egress()
	if err != nil {
		return err
	}

	bindings, err := actorjs.MaterializeAgentBindings(pkg, agent)
	if err != nil {
		return err // never-allocate
	}

	script := actorjs.PrepareGuestScript(string(src))
	iso := actorjs.New(script, actorjs.Options{
		Egress:   actorjs.EgressList(egress),
		Env:      agent.Env,
		Bindings: bindings,
	})

	listen := serveListen
	if listen == "" {
		if port, err := pkg.Port(); err != nil {
			return err
		} else if port > 0 {
			listen = fmt.Sprintf(":%d", port)
		} else {
			listen = ":8787"
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{
		Addr:              listen,
		Handler:           actorjs.Handler(iso),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	actual := ln.Addr().String()
	fmt.Fprintf(cmd.OutOrStdout(), "orvalho serve: %s (agent %s entry %s) on http://%s\n", path, agent.Name, agent.Entrypoint, actual)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		err := <-errCh
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
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
			return nil, fmt.Errorf("invalid --var %q (want NAME=value)", flag)
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
			return nil, fmt.Errorf("env-file %s:%d: want KEY=value", path, lineNo)
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if len(v) >= 2 {
			if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
				v = v[1 : len(v)-1]
			}
		}
		if k == "" {
			return nil, fmt.Errorf("env-file %s:%d: empty key", path, lineNo)
		}
		out[k] = v
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
