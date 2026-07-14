// Command orvalho is the Orvalho CLI entrypoint.
//
// Current surface is manager identity only (generate / show). Mesh, package
// signing, and deploy commands are intentionally out of scope here.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"orvalho/pkg/identity"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "orvalho: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 1 {
		printUsage(os.Stderr)
		return fmt.Errorf("missing command")
	}

	switch args[0] {
	case "identity":
		return runIdentity(args[1:])
	case "help", "-h", "--help":
		printUsage(os.Stdout)
		return nil
	default:
		printUsage(os.Stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage(w *os.File) {
	fmt.Fprintf(w, `Usage: orvalho <command> [flags]

Commands:
  identity generate   Create and persist a new manager identity
  identity show       Load a manager identity and print its public id

Environment:
  ORVALHO_MANAGER_KEY   Default path for the manager key file
                        (fallback: %s)

`, identity.DefaultKeyFile)
}

func defaultKeyPath() string {
	if p := os.Getenv("ORVALHO_MANAGER_KEY"); p != "" {
		return p
	}
	return identity.DefaultKeyFile
}

func runIdentity(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("identity: missing subcommand (generate|show)")
	}
	switch args[0] {
	case "generate":
		return identityGenerate(args[1:])
	case "show":
		return identityShow(args[1:])
	case "help", "-h", "--help":
		fmt.Fprint(os.Stdout, `Usage:
  orvalho identity generate [-path FILE] [-force]
  orvalho identity show [-path FILE]

`)
		return nil
	default:
		return fmt.Errorf("identity: unknown subcommand %q", args[0])
	}
}

func identityGenerate(args []string) error {
	fs := flag.NewFlagSet("identity generate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	path := fs.String("path", defaultKeyPath(), "path to write manager private key PEM")
	force := fs.Bool("force", false, "overwrite existing key file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	abs, err := filepath.Abs(*path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	m, err := identity.Generate()
	if err != nil {
		return err
	}
	if err := m.Save(abs, *force); err != nil {
		return err
	}

	fmt.Printf("wrote manager key: %s\n", abs)
	fmt.Printf("public id: %s\n", m.PublicID())
	return nil
}

func identityShow(args []string) error {
	fs := flag.NewFlagSet("identity show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	path := fs.String("path", defaultKeyPath(), "path to manager private key PEM")
	if err := fs.Parse(args); err != nil {
		return err
	}

	abs, err := filepath.Abs(*path)
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
