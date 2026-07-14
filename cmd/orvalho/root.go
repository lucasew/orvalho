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
	Short: "Orvalho mesh manager CLI",
	Long: `orvalho is the command-line interface for Orvalho.

Current surface is manager identity only (generate / show). Mesh, package
signing, and deploy commands are intentionally out of scope here.`,
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
}

// defaultKeyPath returns ORVALHO_MANAGER_KEY when set, else the library default.
func defaultKeyPath() string {
	if p := os.Getenv("ORVALHO_MANAGER_KEY"); p != "" {
		return p
	}
	return identity.DefaultKeyFile
}
