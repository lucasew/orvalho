// Command orvalho is the single product CLI entrypoint (lewkit x/cmd).
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "orvalho: %v\n", err)
		os.Exit(1)
	}
}
