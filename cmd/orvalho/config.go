package main

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/format"

	"github.com/lucasew/orvalho/pkg/cuex"
)

type configCmd struct {
	Validate *configValidateCmd `cmd:"validate" help:"validate host orvalho.cue against preludes"`
	Show     *configShowCmd     `cmd:"show" help:"print unified host config"`
}

type configValidateCmd struct{}

type configShowCmd struct{}

func (c *configValidateCmd) Run(context.Context) error {
	if _, err := loadHostConfig(); err != nil {
		return err
	}
	fmt.Printf("ok: host config valid (%s)\n", configDisplayPath())
	return nil
}

func (c *configShowCmd) Run(context.Context) error {
	cfg, err := loadHostConfig()
	if err != nil {
		return err
	}
	v := cfg.Value
	node := v.Syntax(cue.Final(), cue.Concrete(true))
	b, err := format.Node(node)
	if err != nil {
		fmt.Printf("%v\n", v)
		return nil
	}
	fmt.Print(string(b))
	if len(b) == 0 || b[len(b)-1] != '\n' {
		fmt.Println()
	}
	return nil
}

func configDisplayPath() string {
	return cmp.Or(configPath, filepath.Join(dataDir, cuex.InstanceFilename))
}
