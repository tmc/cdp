package main

import (
	"context"
	"log"

	"github.com/spf13/cobra"
)

// inspectCmd is the original element inspector, kept for compatibility.
var inspectCmd = &cobra.Command{
	Use:   "inspect <selector>",
	Short: "Inspect DOM element (see also dom --selector)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := inspectElement(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to inspect element: %v", err)
		}
	},
}

func init() {
	inspectCmd.Flags().String("tab", "", "Target tab ID")
}

func inspectElement(ctx context.Context, selector string, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Enable DOM domain
	if err := debugger.EnableDomains(ctx, "DOM", "Runtime"); err != nil {
		return err
	}

	// Inspect element
	return debugger.InspectElement(ctx, selector)
}
