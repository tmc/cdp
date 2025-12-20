package main

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

var devtoolsCmd = &cobra.Command{
	Use:   "devtools",
	Short: "Open DevTools for a target",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := openDevTools(ctx, tabID); err != nil {
			log.Fatalf("Failed to open DevTools: %v", err)
		}
	},
}

func init() {
	devtoolsCmd.Flags().String("tab", "", "Target tab ID")
}

func openDevTools(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Launch DevTools
	if err := debugger.LaunchDevTools(ctx); err != nil {
		return err
	}

	fmt.Println("DevTools opened successfully!")

	return nil
}