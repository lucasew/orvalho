package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
    "github.com/lucasew/orvalho/pkg/platform"
    "github.com/lucasew/orvalho/pkg/gpu"
    "github.com/lucasew/orvalho/pkg/camera"
)

func init() {
	rootCmd.AddCommand(devicesCmd)
}

var devicesCmd = &cobra.Command{
	Use:   "devices",
	Short: "List available devices",
	Run: func(cmd *cobra.Command, args []string) {
        p := platform.NewPlatform()

        // Register Discoverers
        p.RegisterDiscoverer(&gpu.Discoverer{})
        p.RegisterDiscoverer(&camera.Discoverer{})

        if err := p.Scan(); err != nil {
            fmt.Fprintf(os.Stderr, "Error scanning devices: %v\n", err)
            os.Exit(1)
        }

        devices := p.Registry.List()
        if len(devices) == 0 {
            fmt.Println("No devices found.")
            return
        }

        fmt.Printf("Found %d devices:\n", len(devices))
        for _, dev := range devices {
            fmt.Printf("- [%s] %s\n", dev.Type(), dev.ID())
        }
	},
}
