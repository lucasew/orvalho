package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"orvalho/pkg/manager"
)

var managerCmd = &cobra.Command{
	Use:   "manager",
	Short: "Manager role (pair, sign, deploy, daemon)",
	Long: `Manager commands for the owner's primary machine.

Skeleton only: no daemon or deploy yet. See SPEC.md for the manager role.`,
}

var managerVersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print manager package version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("orvalho manager %s\n", manager.Version)
	},
}

func init() {
	managerCmd.AddCommand(managerVersionCmd)
}
