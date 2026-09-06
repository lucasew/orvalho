package main

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/lucasew/orvalho/pkg/dependency"
)

var storeDir string

var dependencyCmd = &cobra.Command{
	Use:   "dependency",
	Short: "Resolve and install registry Dependencies",
}

var dependencyInstallCmd = &cobra.Command{
	Use:   "install [dir]",
	Short: "Resolve the declared tree into the store and isolated node_modules",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDependencyInstall,
}

var dependencyAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a Dependency, resolve, and install",
	Args:  cobra.ExactArgs(1),
	RunE:  runDependencyAdd,
}

var dependencyRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Drop a Dependency, resolve, and install",
	Args:  cobra.ExactArgs(1),
	RunE:  runDependencyRemove,
}

func init() {
	dependencyCmd.PersistentFlags().StringVar(&storeDir, "store-dir", "", "content store directory (default: <user-cache>/orvalho)")
	dependencyCmd.AddCommand(dependencyInstallCmd, dependencyAddCmd, dependencyRemoveCmd)
}

func dependencyOptions(dir string) dependency.Options {
	if dir == "" {
		dir = "."
	}
	return dependency.Options{Dir: dir, StoreDir: storeDir}
}

func runDependencyInstall(cmd *cobra.Command, args []string) error {
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	}
	return wrapDepErr(dependencyOptions(dir).Install(cmd.Context()))
}

func runDependencyAdd(cmd *cobra.Command, args []string) error {
	return wrapDepErr(dependencyOptions(".").Add(cmd.Context(), args[0]))
}

func runDependencyRemove(cmd *cobra.Command, args []string) error {
	return wrapDepErr(dependencyOptions(".").Remove(cmd.Context(), args[0]))
}

func wrapDepErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, dependency.ErrLockfile) ||
		errors.Is(err, dependency.ErrNotFound) ||
		errors.Is(err, dependency.ErrIntegrity) ||
		errors.Is(err, dependency.ErrManifest) ||
		errors.Is(err, dependency.ErrRegistry) ||
		errors.Is(err, dependency.ErrSpecifier) {
		return err
	}
	return fmt.Errorf("%w", err)
}
