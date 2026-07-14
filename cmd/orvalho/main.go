// Command orvalho is the single product CLI entrypoint (Cobra).
//
// All CLI parsing is Cobra. Configuration is CUE (pkg/cuex).
package main

import "os"

func main() {
	if err := Execute(); err != nil {
		os.Exit(1)
	}
}
