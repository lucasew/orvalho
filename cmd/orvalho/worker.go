package main

import "context"

type workerCmd struct{}

func (workerCmd) Run(context.Context) error {
	return ErrWorkerSkeleton
}
