package main

import (
	"context"
	"errors"
	"fmt"

	lewcmd "github.com/lewtec/lewkit/x/cmd"

	"github.com/lucasew/orvalho/pkg/dependency"
)

type dependencyCmd struct {
	Install *depInstallCmd `cmd:"install" help:"resolve the declared tree into the store"`
	Add     *depAddCmd     `cmd:"add" help:"add a Dependency, resolve, and install"`
	Remove  *depRemoveCmd  `cmd:"remove" help:"drop a Dependency, resolve, and install"`
}

type depInstallCmd struct {
	StoreDir lewcmd.StringArg `long:"store-dir" help:"content store directory"`
	Dir      lewcmd.StringArg
}

type depAddCmd struct {
	StoreDir lewcmd.StringArg `long:"store-dir" help:"content store directory"`
	Name     lewcmd.StringArg
}

type depRemoveCmd struct {
	StoreDir lewcmd.StringArg `long:"store-dir" help:"content store directory"`
	Name     lewcmd.StringArg
}

func dependencyOptions(dir, store string) dependency.Options {
	if dir == "" {
		dir = "."
	}
	return dependency.Options{Dir: dir, StoreDir: store}
}

func (c *depInstallCmd) Run(ctx context.Context) error {
	return wrapDepErr(dependencyOptions(c.Dir.Value(), c.StoreDir.Value()).Install(ctx))
}

func (c *depAddCmd) Run(ctx context.Context) error {
	return wrapDepErr(dependencyOptions(".", c.StoreDir.Value()).Add(ctx, c.Name.Value()))
}

func (c *depRemoveCmd) Run(ctx context.Context) error {
	return wrapDepErr(dependencyOptions(".", c.StoreDir.Value()).Remove(ctx, c.Name.Value()))
}

func wrapDepErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, dependency.ErrLockfile) ||
		errors.Is(err, dependency.ErrNotFound) ||
		errors.Is(err, dependency.ErrIntegrity) ||
		errors.Is(err, dependency.ErrManifest) ||
		errors.Is(err, dependency.ErrRegistry) ||
		errors.Is(err, dependency.ErrSpecifier) {
		return err
	}
	return fmt.Errorf("%w", err)
}
