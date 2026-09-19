package main

import (
	"errors"
	"path/filepath"

	"github.com/lucasew/orvalho/pkg/cuex"
)

var (
	dataDir    string
	configPath string
	verbose    bool
)

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

func hostDataDir(cli orvalhoCLI) string {
	switch {
	case cli.Identity != nil:
		return cli.Identity.DataDir.Value()
	case cli.ConfigCmd != nil:
		return cli.ConfigCmd.DataDir.Value()
	case cli.Manager != nil:
		return cli.Manager.DataDir.Value()
	case cli.Worker != nil:
		return cli.Worker.DataDir.Value()
	default:
		return ""
	}
}

func requireDataDir() error {
	if dataDir == "" {
		return ErrDataDirRequired
	}
	return nil
}

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
