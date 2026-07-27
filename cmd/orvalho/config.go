package main

import (
	"fmt"
	"path/filepath"
	"cmp"

	"github.com/spf13/cobra"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/format"

	"github.com/lucasew/orvalho/pkg/cuex"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Host CUE configuration",
	Long:  `Validate and show host orvalho.cue unified with embedded host preludes.`,
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate host orvalho.cue against preludes",
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := loadHostConfig(); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "ok: host config valid (%s)\n", configDisplayPath())
		return nil
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print unified host config (concrete fields)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadHostConfig()
		if err != nil {
			return err
		}
		// Emit only the concrete instance side (Final + Concrete).
		v := cfg.Value
		node := v.Syntax(cue.Final(), cue.Concrete(true))
		b, err := format.Node(node)
		if err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "%v\n", v)
			return nil
		}
		fmt.Fprint(cmd.OutOrStdout(), string(b))
		if len(b) == 0 || b[len(b)-1] != '\n' {
			fmt.Fprintln(cmd.OutOrStdout())
		}
		return nil
	},
}

func init() {
	configCmd.AddCommand(configValidateCmd, configShowCmd)
}

func configDisplayPath() string {
	return cmp.Or(configPath, filepath.Join(dataDir, cuex.InstanceFilename))
}
