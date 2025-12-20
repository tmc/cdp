package main

import (
	"context"
	"log"
	"time"

	"github.com/spf13/cobra"
)

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Monitor network requests and console output",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		duration, _ := cmd.Flags().GetDuration("duration")

		if err := monitorActivity(ctx, duration); err != nil {
			log.Fatalf("Monitoring failed: %v", err)
		}
	},
}

func init() {
	monitorCmd.Flags().DurationP("duration", "d", 0, "Monitoring duration (0 for indefinite)")
}

func monitorActivity(ctx context.Context, duration time.Duration) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to first available target
	if err := debugger.Connect(ctx, ""); err != nil {
		return err
	}

	// Start network monitoring
	return debugger.MonitorNetwork(ctx, duration)
}