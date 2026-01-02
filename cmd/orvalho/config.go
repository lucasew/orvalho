package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(configCmd)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Generate initial configuration",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Generating config...")
		// TODO: Implement config generation logic
	},
}
