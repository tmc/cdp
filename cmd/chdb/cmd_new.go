package main

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

var newTargetCmd = &cobra.Command{
	Use:   "new [url]",
	Short: "Create new Chrome target/tab",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		url := "about:blank"
		if len(args) > 0 {
			url = args[0]
		}

		ctx := createContext()

		if err := createNewTarget(ctx, url); err != nil {
			log.Fatalf("Failed to create new target: %v", err)
		}
	},
}

func createNewTarget(ctx context.Context, url string) error {
	debugger := NewChromeDebugger(port, verbose)

	// Create new target
	target, err := debugger.CreateDevToolsTarget(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("✓ Created new target: %s\n", target.ID)
	fmt.Printf("  Title: %s\n", target.Title)
	fmt.Printf("  Type: %s\n", target.Type)

	// Navigate to URL if provided
	if url != "about:blank" {
		if err := debugger.Connect(ctx, target.ID); err != nil {
			return err
		}
		defer debugger.Close()

		if err := debugger.Navigate(ctx, url); err != nil {
			return err
		}

		fmt.Printf("  Navigated to: %s\n", url)
	}

	return nil
}