package main

import (
	"github.com/spf13/cobra"
)

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Worker role (actor host on device)",
	Long: `Worker commands for phone/Linux hosts that run actors.

Skeleton: no runtime serve yet. Use global --data-dir for host state.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return ErrWorkerSkeleton
	},
}
