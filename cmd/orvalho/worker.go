package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"orvalho/pkg/worker"
)

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Worker role (actor host on device)",
	Long: `Worker commands for phone/Linux hosts that run actors.

Skeleton only: no runtime serve yet. See SPEC.md for the worker role.`,
}

var workerVersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print worker package version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("orvalho worker %s\n", worker.Version)
	},
}

func init() {
	workerCmd.AddCommand(workerVersionCmd)
}
