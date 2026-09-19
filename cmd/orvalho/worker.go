package main

import (
	"context"

	lewcmd "github.com/lewtec/lewkit/x/cmd"
)

type workerCmd struct {
	DataDir lewcmd.DataDirArg `long:"data-dir" help:"host data directory"`
}

func (workerCmd) Run(context.Context) error {
	return ErrWorkerSkeleton
}
