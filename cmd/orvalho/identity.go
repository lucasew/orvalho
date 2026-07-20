package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/lucasew/orvalho/pkg/cuex"
	"github.com/lucasew/orvalho/pkg/identity"
)

var (
	identityPath  string
	identityForce bool
)

var identityCmd = &cobra.Command{
	Use:   "identity",
	Short: "Manage manager identity key material",
	Long: `Generate and inspect the manager Ed25519 identity.

Requires --data-dir. Key path comes from host CUE identity.keyPath when set,
or --path flag (Cobra), which overrides for this invocation.`,
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
	identityGenerateCmd.Flags().StringVar(&identityPath, "path", "", "path to write manager private key PEM (overrides CUE identity.keyPath)")
	identityGenerateCmd.Flags().BoolVar(&identityForce, "force", false, "overwrite existing key file")
	identityShowCmd.Flags().StringVar(&identityPath, "path", "", "path to manager private key PEM (overrides CUE identity.keyPath)")
}

func resolveKeyPath() (string, error) {
	if err := requireDataDir(); err != nil {
		return "", err
	}
	if identityPath != "" {
		return filepath.Abs(identityPath)
	}
	cfg, err := loadHostConfig()
	if err != nil {
		return "", err
	}
	p, ok, err := cuex.LookupString(cfg.Value, "identity.keyPath")
	if err != nil {
		return "", err
	}
	if !ok || p == "" {
		// default under data-dir
		return filepath.Abs(filepath.Join(dataDir, identity.DefaultKeyFile))
	}
	if filepath.IsAbs(p) {
		return p, nil
	}
	return filepath.Abs(filepath.Join(dataDir, p))
}

func runIdentityGenerate(cmd *cobra.Command, args []string) error {
	abs, err := resolveKeyPath()
	if err != nil {
		return err
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
	abs, err := resolveKeyPath()
	if err != nil {
		return err
	}
	m, err := identity.Load(abs)
	if err != nil {
		return err
	}
	fmt.Printf("key file: %s\n", abs)
	fmt.Printf("public id: %s\n", m.PublicID())
	return nil
}
