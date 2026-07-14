package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"orvalho/pkg/identity"
)

// rootCmd is the base command for the orvalho CLI.
var rootCmd = &cobra.Command{
	Use:   "orvalho",
	Short: "Orvalho personal mesh runtime",
	Long: `orvalho is the single product CLI for Orvalho.

All commands use Cobra. Manager and worker are subcommands of this binary
(not separate entrypoints). Host and package configuration use CUE (see SPEC).

Current surface: identity, manager (skeleton), worker (skeleton).`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "orvalho: %v\n", err)
		return err
	}
	return nil
}

func init() {
	rootCmd.AddCommand(identityCmd)
	rootCmd.AddCommand(managerCmd)
	rootCmd.AddCommand(workerCmd)
}

// defaultKeyPath returns ORVALHO_MANAGER_KEY when set, else the library default.
func defaultKeyPath() string {
	if p := os.Getenv("ORVALHO_MANAGER_KEY"); p != "" {
		return p
	}
	return identity.DefaultKeyFile
}
