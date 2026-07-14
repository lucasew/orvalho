package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var managerCmd = &cobra.Command{
	Use:   "manager",
	Short: "Manager role (pair, sign, deploy, daemon)",
	Long: `Manager commands for the owner's primary machine.

Skeleton: no daemon or deploy yet. Use global --data-dir for host state.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("no manager subcommand yet (skeleton)")
	},
}
