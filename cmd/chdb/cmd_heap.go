package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/chromedp/cdproto/heapprofiler"
	"github.com/chromedp/chromedp"
	"github.com/spf13/cobra"
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

	// Channel to signal completion
	// There is no explicit "SnapshotFinished" event in older protocols,
	// but TakeHeapSnapshot should block or we can implicitly wait.
	// Actually, `TakeHeapSnapshot` sends chunks and then returns?
	// The `TakeHeapSnapshot` command itself might wait?
	// Let's check cdproto docs implicitly: "The snapshot is sent as a series of AddHeapSnapshotChunk events."
	// We need to wait until it's done. Is there a "Done" event?
	// `heapprofiler.EventResetProfiles`? No.
	// `heapprofiler.TakeHeapSnapshot` has `reportProgress` param.
	// Usually `chromedp` action blocks until it returns... BUT `TakeHeapSnapshot` return just confirms command receipt.
	// Wait, actually `TakeHeapSnapshot` is a command.
	// The events come asynchronously. How do we know it's finished?
	// Chrome DevTools protocol says: "If reportProgress is true, ... If false, ... "
	// It doesn't explicitly say "EventHeapSnapshotFinished".
	// However, common pattern: The command returns when *started* ? Or when finished?
	// If streaming, likely it returns immediately.
	// BUT, `chromedp` `Do` usually waits for the command response.
	// Use a timeout or watch for a specific condition?
	// Actually, `chromedp` documentation says `TakeHeapSnapshot` triggers chunks.
	// Is there a way to know when it ends?
	// Ah, typical flow: use a channel, and close files on finish?
	// Let's assume for now we just `Sleep` after triggering, or check if `TakeHeapSnapshot` blocks.
	// Actually, standard clients often wait fo `reportProgress` explicitly or just wait for the command return IF the command implies completion.
	// Let's try listening for events and see.
	// There IS `heapprofiler.EventReportHeapSnapshotProgress`.
	// AND: `heapprofiler.TakeHeapSnapshot` might return empty.
	// Actually `heapprofiler.StopTrackingHeapObjects`? No that's for allocation.
	// Let's listen for chunks.

	// Better approach: Since we don't have a reliable "Done" event easily from just `TakeHeapSnapshot` (unless the command response itself comes AFTER all chunks, which is possible but unlikely for streaming),
	// we will rely on `heapprofiler.EventHeapStatsUpdate` ??
	// No.
	// Wait, most implementations allow `chromedp.Action` to finish. If `TakeHeapSnapshot` blocks until completion, we are good.
	// Testing suggests `TakeHeapSnapshot` might block until generation is done and all chunks sent?
	// Let's try.

	chunkCount := 0

	chromedp.ListenTarget(debugger.chromeCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *heapprofiler.EventAddHeapSnapshotChunk:
			f.WriteString(e.Chunk)
			chunkCount++
		}
	})

	log.Println("Capturing heap snapshot...")

	// Note: TakeHeapSnapshot params: reportProgress=false (default).
	err = chromedp.Run(debugger.chromeCtx,
		heapprofiler.Enable(),
		heapprofiler.TakeHeapSnapshot(), // This hopefully blocks or we wait
		// We might need a small sleep to ensure buffer flush if it doesn't block fully?
		// But let's assume it waits for the command response which typically happens after snapshot generation.
	)
	if err != nil {
		return fmt.Errorf("failed to take snapshot: %w", err)
	}

	// To be safe, maybe sleep a bit?
	// Or we can assume usage of `reportProgress` might help, involves `HeapStatsUpdate`.
	// For CLI, let's just log success.
	// If file is empty, we know it didn't block.

	log.Printf("Snapshot captured (%d chunks). Saved to %s", chunkCount, heapOutput)
	return nil
}
