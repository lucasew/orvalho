package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	actorjs "orvalho/pkg/actor/js"
	"orvalho/pkg/ovpkg"
)

var (
	serveListen string
)

var serveCmd = &cobra.Command{
	Use:   "serve [path]",
	Short: "Serve one package (zip or directory) over local HTTP",
	Long: `Load an Orvalho package from a zip file or directory (orvalho.cue + entry)
and serve it on a local HTTP address by invoking default.fetch for each request.

This is a development convenience: no mesh, no manager pairing, no package
signature checks. --data-dir is not required.

Entry convention: Workers-shaped default export with fetch(request, env, ctx).
Bare "export default" in the entry file is rewritten for goja; prefer the
esbuild downlevel pipeline for full modern JS.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runServe,
}

func init() {
	serveCmd.Flags().StringVar(&serveListen, "listen", "", "listen address (default :8787, or :PORT from package port/publish.port)")
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
	entry, err := pkg.Entry()
	if err != nil {
		return err
	}
	src, err := pkg.Get(entry)
	if err != nil {
		return fmt.Errorf("entry %s: %w", entry, err)
	}

	script := actorjs.PrepareGuestScript(string(src))
	iso := actorjs.New(script, actorjs.Options{})

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
	fmt.Fprintf(cmd.OutOrStdout(), "orvalho serve: %s (entry %s) on http://%s\n", path, entry, actual)

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
