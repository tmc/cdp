package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/spf13/cobra"
)

var (
	networkFormat   string
	networkThrottle string
	networkBlock    string
)

var networkCmd = &cobra.Command{
	Use:   "network",
	Short: "Monitor network traffic",
	Long:  `Streams network events (requests and responses) to stdout.`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := runNetworkMonitor(ctx, tabID); err != nil {
			log.Fatalf("Network monitor failed: %v", err)
		}
	},
}

func init() {
	networkCmd.Flags().String("tab", "", "Target tab ID")
	networkCmd.Flags().StringVarP(&networkFormat, "format", "f", "json", "Output format (json)")
	networkCmd.Flags().StringVar(&networkThrottle, "throttle", "", "Network throttle profile (offline, slow3g, fast3g)")
	networkCmd.Flags().StringVar(&networkBlock, "block", "", "URL pattern to block (e.g. '*.png')")
}

func runNetworkMonitor(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Signalling channel
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Enable Network domain
	if err := chromedp.Run(debugger.chromeCtx, network.Enable()); err != nil {
		return fmt.Errorf("failed to enable network: %w", err)
	}

	var actions []chromedp.Action

	// Apply Throttling
	if networkThrottle != "" {
		var offline bool
		var latency float64
		var downloadThroughput, uploadThroughput float64

		switch networkThrottle {
		case "offline":
			offline = true
			latency = 0
			downloadThroughput = 0
			uploadThroughput = 0
		case "slow3g":
			offline = false
			latency = 100
			downloadThroughput = 500 * 1024 / 8 // 500 kbps
			uploadThroughput = 500 * 1024 / 8
		case "fast3g":
			offline = false
			latency = 20
			downloadThroughput = 1.6 * 1024 * 1024 / 8 // 1.6 Mbps
			uploadThroughput = 750 * 1024 / 8          // 750 kbps
		default:
			log.Printf("Unknown throttle profile '%s', ignoring (use offline, slow3g, fast3g)", networkThrottle)
		}

		if networkThrottle == "offline" || latency > 0 {
			log.Printf("Applying throttle: %s", networkThrottle)
			actions = append(actions, network.OverrideNetworkState(offline, latency, downloadThroughput, uploadThroughput))
		}
	}

	// Apply Blocking
	if networkBlock != "" {
		log.Printf("Blocking URLs matching: %s", networkBlock)
		actions = append(actions, network.SetBlockedURLs().WithURLPatterns([]*network.BlockPattern{
			{URLPattern: networkBlock, Block: true},
		}))
	}

	if len(actions) > 0 {
		if err := chromedp.Run(debugger.chromeCtx, actions...); err != nil {
			return fmt.Errorf("failed to apply network conditions: %w", err)
		}
	}

	log.Println("Monitoring network traffic... Press Ctrl+C to stop.")

	// Event listener loop
	chromedp.ListenTarget(debugger.chromeCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			printEvent("request", e)
		case *network.EventResponseReceived:
			printEvent("response", e)
		case *network.EventLoadingFailed:
			printEvent("failed", e)
		}
	})

	// Wait for signal
	<-sigs
	log.Println("Stopping network monitor...")
	return nil
}

func printEvent(typeStr string, ev interface{}) {
	// Simple JSON output for now
	data := map[string]interface{}{
		"type":  typeStr,
		"event": ev,
	}
	enc := json.NewEncoder(os.Stdout)
	// enc.SetIndent("", "  ") // Keep it one lined for streaming
	if err := enc.Encode(data); err != nil {
		log.Printf("Failed to encode event: %v", err)
	}
}
