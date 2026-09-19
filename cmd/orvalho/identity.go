package main

import (
	"context"
	"fmt"
	"path/filepath"

	lewcmd "github.com/lewtec/lewkit/x/cmd"

	"github.com/lucasew/orvalho/pkg/cuex"
	"github.com/lucasew/orvalho/pkg/identity"
)

type identityCmd struct {
	DataDir  lewcmd.DataDirArg    `long:"data-dir" help:"host data directory"`
	Generate *identityGenerateCmd `cmd:"generate" help:"create and persist a new manager identity"`
	Show     *identityShowCmd     `cmd:"show" help:"load a manager identity and print its public id"`
}

type identityGenerateCmd struct {
	Path  lewcmd.StringArg `long:"path" help:"path to write manager private key PEM" default:""`
	Force lewcmd.Flag      `long:"force" help:"overwrite existing key file"`
}

type identityShowCmd struct {
	Path lewcmd.StringArg `long:"path" help:"path to manager private key PEM" default:""`
}

func resolveKeyPath(flagPath string) (string, error) {
	if err := requireDataDir(); err != nil {
		return "", err
	}
	if flagPath != "" {
		return filepath.Abs(flagPath)
	}
	cfg, err := loadHostConfig()
	if err != nil {
		return "", err
	}
	p, ok, err := cuex.LookupString(cfg.Value, "identity.keyPath")
	if err != nil {
		return "", err
	}
	if !ok || p == "" {
		// default under data-dir
		return filepath.Abs(filepath.Join(dataDir, identity.DefaultKeyFile))
	}
	if filepath.IsAbs(p) {
		return p, nil
	}
	return filepath.Abs(filepath.Join(dataDir, p))
}

func (c *identityGenerateCmd) Run(context.Context) error {
	abs, err := resolveKeyPath(c.Path.Value())
	if err != nil {
		return err
	}
	m, err := identity.Generate()
	if err != nil {
		return err
	}
	if err := m.Save(abs, c.Force.Value()); err != nil {
		return err
	}
	fmt.Printf("wrote manager key: %s\n", abs)
	fmt.Printf("public id: %s\n", m.PublicID())
	return nil
}

func (c *identityShowCmd) Run(context.Context) error {
	abs, err := resolveKeyPath(c.Path.Value())
	if err != nil {
		return err
	}
	m, err := identity.Load(abs)
	if err != nil {
		return err
	}
	fmt.Printf("key file: %s\n", abs)
	fmt.Printf("public id: %s\n", m.PublicID())
	return nil
}
