package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec <javascript>",
	Short: "Execute JavaScript in Chrome console",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := executeJavaScript(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to execute JavaScript: %v", err)
		}
	},
}

func init() {
	execCmd.Flags().String("tab", "", "Target tab ID")
}

func executeJavaScript(ctx context.Context, expression string, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Enable runtime
	if err := debugger.EnableDomains(ctx, "Runtime"); err != nil {
		debugger.Close()
		return err
	}

	result, err := debugger.Execute(ctx, expression)
	if err != nil {
		debugger.Close()
		return err
	}

	fmt.Printf("✓ Executed: %s\n", expression)
	fmt.Printf("Result: %v\n", result)

	// Give Chrome a moment to complete any pending operations
	// before closing the connection
	time.Sleep(100 * time.Millisecond)

	// Now close the debugger
	debugger.Close()

	return nil
}