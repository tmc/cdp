package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/chromedp/cdproto/heapprofiler"
	"github.com/spf13/cobra"
	"github.com/tmc/cdp/internal/chromedp"
)

var heapOutput string

var heapCmd = &cobra.Command{
	Use:   "heap",
	Short: "Capture a heap snapshot",
	Long:  `Captures a heap snapshot and saves it to a file directly usable by Chrome DevTools.`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if heapOutput == "" {
			log.Fatal("Please specify --output")
		}

		if err := runHeap(ctx, tabID); err != nil {
			log.Fatalf("Heap snapshot failed: %v", err)
		}
	},
}

func init() {
	heapCmd.Flags().String("tab", "", "Target tab ID")
	heapCmd.Flags().StringVarP(&heapOutput, "output", "o", "heap.heapsnapshot", "Output file for heap snapshot")
}

func runHeap(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	f, err := os.Create(heapOutput)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	// Chrome sends the snapshot as AddHeapSnapshotChunk events and
	// replies to TakeHeapSnapshot after the last chunk.

	chunkCount := 0

	chromedp.ListenTarget(debugger.chromeCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *heapprofiler.EventAddHeapSnapshotChunk:
			f.WriteString(e.Chunk)
			chunkCount++
		}
	})

	log.Println("Capturing heap snapshot...")

	// TakeHeapSnapshot uses reportProgress=false.
	err = chromedp.Run(debugger.chromeCtx,
		heapprofiler.Enable(),
		heapprofiler.TakeHeapSnapshot(),
	)
	if err != nil {
		return fmt.Errorf("failed to take snapshot: %w", err)
	}

	log.Printf("Snapshot captured (%d chunks). Saved to %s", chunkCount, heapOutput)
	return nil
}
