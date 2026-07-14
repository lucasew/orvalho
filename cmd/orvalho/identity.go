package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"orvalho/pkg/identity"
)

var (
	identityPath  string
	identityForce bool
)

var identityCmd = &cobra.Command{
	Use:   "identity",
	Short: "Manage manager identity key material",
	Long:  `Generate and inspect the manager Ed25519 identity used as mesh deploy authority.`,
}

var identityGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Create and persist a new manager identity",
	RunE:  runIdentityGenerate,
}

var identityShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Load a manager identity and print its public id",
	RunE:  runIdentityShow,
}

func init() {
	identityCmd.AddCommand(identityGenerateCmd, identityShowCmd)

	// Defaults follow ORVALHO_MANAGER_KEY when set, else identity.DefaultKeyFile.
	def := defaultKeyPath()
	identityGenerateCmd.Flags().StringVar(&identityPath, "path", def, "path to write manager private key PEM")
	identityGenerateCmd.Flags().BoolVar(&identityForce, "force", false, "overwrite existing key file")

	identityShowCmd.Flags().StringVar(&identityPath, "path", def, "path to manager private key PEM")
}

func runIdentityGenerate(cmd *cobra.Command, args []string) error {
	abs, err := filepath.Abs(identityPath)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	m, err := identity.Generate()
	if err != nil {
		return err
	}
	if err := m.Save(abs, identityForce); err != nil {
		return err
	}

	fmt.Printf("wrote manager key: %s\n", abs)
	fmt.Printf("public id: %s\n", m.PublicID())
	return nil
}

func runIdentityShow(cmd *cobra.Command, args []string) error {
	abs, err := filepath.Abs(identityPath)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	m, err := identity.Load(abs)
	if err != nil {
		return err
	}

	fmt.Printf("key file: %s\n", abs)
	fmt.Printf("public id: %s\n", m.PublicID())
	return nil
}
