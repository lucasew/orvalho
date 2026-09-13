package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/lewtec/lewkit/x/profile"
	"github.com/spf13/cobra"

	"github.com/lucasew/orvalho/pkg/cuex"
	"github.com/lucasew/orvalho/pkg/version"
)

var (
	dataDir     string
	configPath  string
	verbose     bool
	pprofDir    string
	profileStop context.CancelFunc
	profileDone chan struct{}
)

// rootCmd is the base command for the orvalho CLI.
var rootCmd = &cobra.Command{
	Use:   "orvalho",
	Short: "Orvalho personal mesh runtime",
	Long: `orvalho is the single product CLI for Orvalho.

All commands use Cobra. Host and package configuration use CUE
(embedded preludes + orvalho.cue instances). data-dir is always an explicit
flag when host state is required — there is no implicit discovery path.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command.
func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "orvalho: %v\n", err)
		return err
	}
	return nil
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dataDir, "data-dir", "", "host data directory (required for host commands; always explicit)")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "host orvalho.cue path (default: <data-dir>/orvalho.cue)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "print host progress to stderr")
	rootCmd.PersistentFlags().StringVar(&pprofDir, "pprof-dir", "", "when set, write runtime/pprof snapshots into this directory")
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if pprofDir == "" {
			return nil
		}
		pctx, cancel := context.WithCancel(cmd.Context())
		profileStop = cancel
		profileDone = make(chan struct{})
		p := profile.NewProfile(pprofDir)
		go func() {
			defer close(profileDone)
			_ = p.Run(pctx)
		}()
		return nil
	}
	rootCmd.PersistentPostRun = func(cmd *cobra.Command, args []string) {
		if profileStop != nil {
			profileStop()
		}
		if profileDone != nil {
			<-profileDone
		}
	}

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(identityCmd)
	rootCmd.AddCommand(managerCmd)
	rootCmd.AddCommand(workerCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(dependencyCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print orvalho version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("orvalho %s\n", version.Version())
	},
}

// CLI sentinel errors (Err* names so errors.Is / error tables stay consistent).
var (
	ErrDataDirRequired   = errors.New("--data-dir is required")
	ErrNilAgent          = errors.New("serve: nil agent")
	ErrManagerSkeleton   = errors.New("no manager subcommand yet (skeleton)")
	ErrWorkerSkeleton    = errors.New("no worker subcommand yet (skeleton)")
	ErrEnvNameClash      = errors.New("serve: env name used as both string and binding")
	ErrUnsupportedBind   = errors.New("unsupported binding type")
	ErrAssetsMissingRoot = errors.New("assets binding missing root")
	ErrInvalidVarFlag    = errors.New("invalid --var (want NAME=value)")
	ErrEnvFileFormat     = errors.New("env-file: want KEY=value")
	ErrEnvFileEmptyKey   = errors.New("env-file: empty key")
	ErrScriptMissing     = errors.New("script: missing file")
)

// requireDataDir returns an error if --data-dir was not set.
func requireDataDir() error {
	if dataDir == "" {
		return ErrDataDirRequired
	}
	return nil
}

// loadHostConfig loads host CUE from --config or <data-dir>/orvalho.cue.
func loadHostConfig() (*cuex.Config, error) {
	if err := requireDataDir(); err != nil {
		return nil, err
	}
	path := configPath
	if path == "" {
		path = filepath.Join(dataDir, cuex.InstanceFilename)
	}
	return cuex.LoadHostFile(path)
}
