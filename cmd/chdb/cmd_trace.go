package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/chromedp/cdproto/tracing"
	"github.com/chromedp/chromedp"
	"github.com/spf13/cobra"
)

var (
	traceOutput   string
	traceDuration time.Duration
)

var traceCmd = &cobra.Command{
	Use:   "trace",
	Short: "Record a performance trace",
	Long:  `Records a performance trace (CPU, Memory, etc.) for a specified duration and saves it to a JSON file.`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if traceOutput == "" {
			log.Fatal("Please specify --output")
		}

		if err := runTrace(ctx, tabID); err != nil {
			log.Fatalf("Trace failed: %v", err)
		}
	},
}

func init() {
	traceCmd.Flags().String("tab", "", "Target tab ID")
	traceCmd.Flags().StringVarP(&traceOutput, "output", "o", "trace.json", "Output trace file (JSON)")
	traceCmd.Flags().DurationVarP(&traceDuration, "duration", "d", 5*time.Second, "Trace duration")
}

func runTrace(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Capture trace events
	var traceEvents []interface{} // Using interface{} effectively captures raw JSON events
	// Or we can use []tracing.Event if we want typed events.
	// The Chrome trace viewer expects an array of event objects or a specific JSON structure.
	// tracing.DataCollected provides []Event.

	traceDone := make(chan struct{})

	chromedp.ListenTarget(debugger.chromeCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *tracing.EventDataCollected:
			// Append events
			// Note: EventDataCollected contains []tracing.Event.
			// We need to store them.
			// However, simple appending might copy a lot.
			// For CLI tool, this is fine.
			for _, event := range e.Value {
				traceEvents = append(traceEvents, event)
			}
		case *tracing.EventTracingComplete:
			close(traceDone)
		}
	})

	log.Printf("Recording trace for %v...", traceDuration)

	err := chromedp.Run(debugger.chromeCtx,
		tracing.Start(), // Default categories
		chromedp.Sleep(traceDuration),
		tracing.End(),
	)
	if err != nil {
		return fmt.Errorf("tracing failed: %w", err)
	}

	log.Println("Waiting for trace data collection...")
	<-traceDone

	// Write trace to file
	// Format: {"traceEvents": [...]} or just [...] depending on viewer support.
	// Standard trace format wraps events in "traceEvents".
	outputData := map[string]interface{}{
		"traceEvents": traceEvents,
	}

	f, err := os.Create(traceOutput)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	if err := enc.Encode(outputData); err != nil {
		return fmt.Errorf("failed to write trace data: %w", err)
	}

	log.Printf("Trace saved to %s", traceOutput)
	return nil
}
