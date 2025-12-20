package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/spf13/cobra"
)

var screenshotCmd = &cobra.Command{
	Use:   "screenshot [filename]",
	Short: "Take a screenshot",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filename := fmt.Sprintf("screenshot-%d.png", time.Now().Unix())
		if len(args) > 0 {
			filename = args[0]
		}

		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := takeScreenshot(ctx, filename, tabID); err != nil {
			log.Fatalf("Failed to take screenshot: %v", err)
		}
	},
}

func init() {
	screenshotCmd.Flags().String("tab", "", "Target tab ID")
}

func takeScreenshot(ctx context.Context, filename string, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Enable page domain
	if err := debugger.EnableDomains(ctx, "Page"); err != nil {
		debugger.Close()
		return err
	}

	// Take screenshot
	if err := debugger.TakeScreenshot(ctx, filename); err != nil {
		debugger.Close()
		return err
	}

	fmt.Printf("✓ Screenshot saved: %s\n", filename)

	// Give Chrome a moment to complete any pending operations
	// before closing the connection
	time.Sleep(100 * time.Millisecond)

	// Now close the debugger
	debugger.Close()

	return nil
}