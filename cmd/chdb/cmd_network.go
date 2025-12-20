package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// Network commands
var networkCmd = &cobra.Command{
	Use:   "network",
	Short: "Network monitoring and interception",
	Long:  "Monitor, intercept, and analyze network requests",
}

var networkMonitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Monitor network requests",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")
		duration, _ := cmd.Flags().GetDuration("duration")

		if err := monitorNetwork(ctx, tabID, duration); err != nil {
			log.Fatalf("Failed to monitor network: %v", err)
		}
	},
}

var networkInterceptCmd = &cobra.Command{
	Use:   "intercept <patterns...>",
	Short: "Intercept requests matching patterns",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := interceptRequests(ctx, args, tabID); err != nil {
			log.Fatalf("Failed to intercept requests: %v", err)
		}
	},
}

var networkBlockCmd = &cobra.Command{
	Use:   "block <patterns...>",
	Short: "Block requests matching patterns",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := blockRequests(ctx, args, tabID); err != nil {
			log.Fatalf("Failed to block requests: %v", err)
		}
	},
}

var networkThrottleCmd = &cobra.Command{
	Use:   "throttle <profile>",
	Short: "Apply network throttling (offline, slow-3g, fast-3g, 4g, none)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := throttleNetwork(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to throttle network: %v", err)
		}
	},
}

var networkHarCmd = &cobra.Command{
	Use:   "har",
	Short: "Export network activity as HAR",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")
		output, _ := cmd.Flags().GetString("output")

		if err := exportHAR(ctx, tabID, output); err != nil {
			log.Fatalf("Failed to export HAR: %v", err)
		}
	},
}

var networkClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear network cache and storage",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := clearNetworkCache(ctx, tabID); err != nil {
			log.Fatalf("Failed to clear network cache: %v", err)
		}
	},
}

func init() {
	// Add subcommands to network command
	networkCmd.AddCommand(networkMonitorCmd)
	networkCmd.AddCommand(networkInterceptCmd)
	networkCmd.AddCommand(networkBlockCmd)
	networkCmd.AddCommand(networkThrottleCmd)
	networkCmd.AddCommand(networkHarCmd)
	networkCmd.AddCommand(networkClearCmd)

	// Add flags
	networkMonitorCmd.Flags().String("tab", "", "Target tab ID")
	networkMonitorCmd.Flags().DurationP("duration", "d", 0, "Monitoring duration (0 for indefinite)")

	networkInterceptCmd.Flags().String("tab", "", "Target tab ID")

	networkBlockCmd.Flags().String("tab", "", "Target tab ID")

	networkThrottleCmd.Flags().String("tab", "", "Target tab ID")

	networkHarCmd.Flags().String("tab", "", "Target tab ID")
	networkHarCmd.Flags().StringP("output", "o", "", "Output file path (default: har-<timestamp>.json)")

	networkClearCmd.Flags().String("tab", "", "Target tab ID")
}

// Implementation functions

func monitorNetwork(ctx context.Context, tabID string, duration time.Duration) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create network controller
	netCtrl := NewNetworkController(debugger, verbose)

	// Start monitoring
	return netCtrl.StartMonitoring(ctx, duration)
}

func interceptRequests(ctx context.Context, patterns []string, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create network controller
	netCtrl := NewNetworkController(debugger, verbose)

	// Enable interception
	if err := netCtrl.EnableInterception(ctx, patterns); err != nil {
		return err
	}

	fmt.Println("Request interception enabled. Press Ctrl+C to stop.")
	<-ctx.Done()

	return nil
}

func blockRequests(ctx context.Context, patterns []string, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create network controller
	netCtrl := NewNetworkController(debugger, verbose)

	// Block requests
	return netCtrl.BlockRequests(ctx, patterns)
}

func throttleNetwork(ctx context.Context, profile string, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create network controller
	netCtrl := NewNetworkController(debugger, verbose)

	// Apply throttling
	return netCtrl.SetThrottling(ctx, profile)
}

func exportHAR(ctx context.Context, tabID string, outputPath string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create network controller
	netCtrl := NewNetworkController(debugger, verbose)

	// Get HAR data
	harData, err := netCtrl.GetHAR(ctx)
	if err != nil {
		return err
	}

	// Generate output filename if not provided
	if outputPath == "" {
		outputPath = fmt.Sprintf("har-%d.json", time.Now().Unix())
	}

	// Write to file
	if err := os.WriteFile(outputPath, []byte(harData), 0644); err != nil {
		return err
	}

	fmt.Printf("✓ HAR exported to: %s\n", outputPath)
	return nil
}

func clearNetworkCache(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Use JavaScript to clear various caches
	clearScript := `
		(async function() {
			const results = [];

			// Clear browser cache
			try {
				if ('caches' in window) {
					const cacheNames = await caches.keys();
					await Promise.all(cacheNames.map(name => caches.delete(name)));
					results.push('Service Worker caches cleared');
				}
			} catch (e) {
				results.push('Failed to clear SW caches: ' + e.message);
			}

			// Clear localStorage
			try {
				localStorage.clear();
				results.push('localStorage cleared');
			} catch (e) {
				results.push('Failed to clear localStorage: ' + e.message);
			}

			// Clear sessionStorage
			try {
				sessionStorage.clear();
				results.push('sessionStorage cleared');
			} catch (e) {
				results.push('Failed to clear sessionStorage: ' + e.message);
			}

			// Clear IndexedDB (simplified)
			try {
				const databases = await indexedDB.databases();
				for (const db of databases) {
					indexedDB.deleteDatabase(db.name);
				}
				results.push('IndexedDB databases cleared');
			} catch (e) {
				results.push('Failed to clear IndexedDB: ' + e.message);
			}

			return results;
		})()
	`

	result, err := debugger.Execute(ctx, clearScript)
	if err != nil {
		return err
	}

	fmt.Println("Network cache clearing results:")
	if resultArray, ok := result.([]interface{}); ok {
		for _, msg := range resultArray {
			if str, ok := msg.(string); ok {
				fmt.Printf("  %s\n", str)
			}
		}
	} else {
		fmt.Printf("  %v\n", result)
	}

	// Also clear network cache via CDP if possible
	expression := `
		(function() {
			// Trigger a hard reload to clear network cache
			location.reload(true);
			return "Page reloaded to clear network cache";
		})()
	`

	reloadResult, err := debugger.Execute(ctx, expression)
	if err == nil {
		if msg, ok := reloadResult.(string); ok {
			fmt.Printf("  %s\n", msg)
		}
	}

	fmt.Println("✓ Network cache clearing completed")
	return nil
}