package main

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

var attachCmd = &cobra.Command{
	Use:   "attach [port]",
	Short: "Attach to a running Chrome instance",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		targetPort := port
		if len(args) > 0 {
			targetPort = args[0]
		}

		ctx := createContext()
		if err := attachToChrome(ctx, targetPort); err != nil {
			log.Fatalf("Failed to attach to Chrome: %v", err)
		}
	},
}

func attachToChrome(ctx context.Context, port string) error {
	fmt.Printf("Attaching to Chrome on port %s...\n", port)

	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// List targets
	targets, err := debugger.ListTargets(ctx)
	if err != nil {
		return err
	}

	if len(targets) == 0 {
		return fmt.Errorf("no Chrome targets found on port %s", port)
	}

	// Use the first page target
	var targetID string
	for _, target := range targets {
		if target.Type == "page" {
			targetID = target.ID
			break
		}
	}

	if targetID == "" && len(targets) > 0 {
		targetID = targets[0].ID
	}

	// Connect to target
	if err := debugger.Connect(ctx, targetID); err != nil {
		return err
	}

	// Enable common domains
	if err := debugger.EnableDomains(ctx, "Runtime", "Page", "Network", "DOM"); err != nil {
		return err
	}

	fmt.Printf("Successfully attached to Chrome target\n")

	// Keep running until interrupted
	<-ctx.Done()

	return nil
}