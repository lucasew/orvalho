// Command orvalho is the single product CLI entrypoint (Cobra).
//
// Manager and worker roles are subcommands of this binary, not separate mains.
package main

import "os"

func main() {
	if err := Execute(); err != nil {
		os.Exit(1)
	}
}
