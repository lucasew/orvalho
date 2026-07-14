// Command orvalho is the Orvalho CLI entrypoint.
//
// Current surface is manager identity only (generate / show). Mesh, package
// signing, and deploy commands are intentionally out of scope here.
package main

import "os"

func main() {
	if err := Execute(); err != nil {
		os.Exit(1)
	}
}
