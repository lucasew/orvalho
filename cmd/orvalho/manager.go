package main

import "context"

type managerCmd struct{}

func (managerCmd) Run(context.Context) error {
	return ErrManagerSkeleton
}
