package main

import (
	"context"
	"errors"
	"fmt"

	lewcmd "github.com/lewtec/lewkit/x/cmd"
	"github.com/lewtec/lewkit/x/taskgroup"
	"github.com/lewtec/lewkit/x/taskgroup/progress"

	"github.com/lucasew/orvalho/pkg/dependency"
)

type dependencyCmd struct {
	taskgroup.Arg `flatten:"" ctx:"taskgroup"`
	Install       *depInstallCmd `cmd:"install" help:"resolve the declared tree into the store"`
	Add           *depAddCmd     `cmd:"add" help:"add a Dependency, resolve, and install"`
	Remove        *depRemoveCmd  `cmd:"remove" help:"drop a Dependency, resolve, and install"`
}

type depInstallCmd struct {
	StoreDir lewcmd.StringArg  `long:"store-dir" help:"content store directory" default:""`
	Dir      lewcmd.WorkDirArg `help:"project directory"`
}

type depAddCmd struct {
	StoreDir lewcmd.StringArg `long:"store-dir" help:"content store directory" default:""`
	Name     lewcmd.StringArg
}

type depRemoveCmd struct {
	StoreDir lewcmd.StringArg `long:"store-dir" help:"content store directory" default:""`
	Name     lewcmd.StringArg
}

func dependencyOptions(dir, store string) dependency.Options {
	return dependency.Options{Dir: dir, StoreDir: store}
}

func runDepProgress(ctx context.Context, fn func(context.Context) error) error {
	var s *taskgroup.Session
	if arg, ok := lewcmd.Lookup[taskgroup.Arg](ctx, "taskgroup"); ok {
		s, ctx = arg.Enter(ctx, taskgroup.DefaultLimits())
	} else {
		s, ctx = taskgroup.New(ctx, taskgroup.DefaultLimits())
	}
	return progress.Run(s, ctx, fn)
}

func (c *depInstallCmd) Run(ctx context.Context) error {
	return runDepProgress(ctx, func(ctx context.Context) error {
		return wrapDepErr(dependencyOptions(c.Dir.Value(), c.StoreDir.Value()).Install(ctx))
	})
}

func (c *depAddCmd) Run(ctx context.Context) error {
	return runDepProgress(ctx, func(ctx context.Context) error {
		return wrapDepErr(dependencyOptions(".", c.StoreDir.Value()).Add(ctx, c.Name.Value()))
	})
}

func (c *depRemoveCmd) Run(ctx context.Context) error {
	return runDepProgress(ctx, func(ctx context.Context) error {
		return wrapDepErr(dependencyOptions(".", c.StoreDir.Value()).Remove(ctx, c.Name.Value()))
	})
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
