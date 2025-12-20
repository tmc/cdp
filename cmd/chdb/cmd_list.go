package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/spf13/cobra"
	"github.com/tmc/misc/chrome-to-har/internal/targets"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available Chrome tabs and targets",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()

		// Use Chrome-style discovery
		discovery := targets.NewDiscovery(5 * time.Second)
		allTargets, err := discovery.DiscoverTargets(ctx)
		if err != nil {
			log.Fatalf("Failed to discover targets: %v", err)
		}

		// Filter for Chrome targets
		chromeTargets := targets.FilterChromeTargets(allTargets)

		if len(chromeTargets) == 0 {
			fmt.Println("No Chrome targets found.")
			fmt.Println("Start Chrome with: --remote-debugging-port=9222")
			return
		}

		fmt.Println("Available Chrome targets:")
		for _, target := range chromeTargets {
			fmt.Printf("  ID: %s\n", target.ID)
			fmt.Printf("  Type: %s\n", target.Type)
			fmt.Printf("  Title: %s\n", target.Title)
			fmt.Printf("  URL: %s\n", target.URL)
			fmt.Printf("  Port: %d\n", target.Port)
			fmt.Println()
		}
	},
}

func listChromeTargets(ctx context.Context) error {
	debugger := NewChromeDebugger(port, verbose)

	targets, err := debugger.ListTargets(ctx)
	if err != nil {
		return err
	}

	if len(targets) == 0 {
		fmt.Println("No Chrome targets found.")
		fmt.Println("Make sure Chrome is running with --remote-debugging-port flag")
		return nil
	}

	fmt.Printf("Found %d Chrome target(s) on port %s:\n\n", len(targets), port)

	for i, target := range targets {
		fmt.Printf("[%d] %s - %s\n", i+1, target.Type, target.Title)
		fmt.Printf("    URL: %s\n", target.URL)
		fmt.Printf("    ID: %s\n", target.ID)
		if target.DevtoolsURL != "" {
			fmt.Printf("    DevTools: %s\n", target.DevtoolsURL)
		}
		fmt.Println()
	}

	return nil
}