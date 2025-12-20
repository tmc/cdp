package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/spf13/cobra"
)

var navigateCmd = &cobra.Command{
	Use:   "navigate <url>",
	Short: "Navigate to a URL",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := navigateToURL(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to navigate: %v", err)
		}
	},
}

func init() {
	navigateCmd.Flags().String("tab", "", "Target tab ID")
}

func navigateToURL(ctx context.Context, url string, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Navigate
	if err := debugger.Navigate(ctx, url); err != nil {
		debugger.Close()
		return err
	}

	fmt.Printf("✓ Navigated to: %s\n", url)

	// Give Chrome a moment to complete navigation
	// before closing the connection
	time.Sleep(500 * time.Millisecond)

	// Now close the debugger
	debugger.Close()

	return nil
}