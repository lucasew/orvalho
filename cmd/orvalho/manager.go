package main

import (
	"context"

	lewcmd "github.com/lewtec/lewkit/x/cmd"
)

type managerCmd struct {
	DataDir lewcmd.DataDirArg `long:"data-dir" help:"host data directory"`
}

func (managerCmd) Run(context.Context) error {
	return ErrManagerSkeleton
}
