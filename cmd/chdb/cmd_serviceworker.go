package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

// Service Worker commands
var swCmd = &cobra.Command{
	Use:     "sw",
	Aliases: []string{"serviceworker"},
	Short:   "Service Worker debugging and management",
	Long:    "List, inspect, and manage service workers",
}

var swListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered service workers",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := listServiceWorkers(ctx, tabID); err != nil {
			log.Fatalf("Failed to list service workers: %v", err)
		}
	},
}

var swInspectCmd = &cobra.Command{
	Use:   "inspect <scope>",
	Short: "Inspect a specific service worker",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := inspectServiceWorker(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to inspect service worker: %v", err)
		}
	},
}

var swUnregisterCmd = &cobra.Command{
	Use:   "unregister <scope>",
	Short: "Unregister a service worker",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := unregisterServiceWorker(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to unregister service worker: %v", err)
		}
	},
}

var swUpdateCmd = &cobra.Command{
	Use:   "update <scope>",
	Short: "Force update a service worker",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := updateServiceWorker(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to update service worker: %v", err)
		}
	},
}

var swCachesCmd = &cobra.Command{
	Use:   "caches",
	Short: "List caches associated with service workers",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := getServiceWorkerCaches(ctx, tabID); err != nil {
			log.Fatalf("Failed to get service worker caches: %v", err)
		}
	},
}

var swPostMessageCmd = &cobra.Command{
	Use:   "postmessage <scope> <message>",
	Short: "Send a message to a service worker",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := postMessageToServiceWorker(ctx, args[0], args[1], tabID); err != nil {
			log.Fatalf("Failed to post message to service worker: %v", err)
		}
	},
}

func init() {
	// Add subcommands to sw command
	swCmd.AddCommand(swListCmd)
	swCmd.AddCommand(swInspectCmd)
	swCmd.AddCommand(swUnregisterCmd)
	swCmd.AddCommand(swUpdateCmd)
	swCmd.AddCommand(swCachesCmd)
	swCmd.AddCommand(swPostMessageCmd)

	// Add flags
	swListCmd.Flags().String("tab", "", "Target tab ID")
	swInspectCmd.Flags().String("tab", "", "Target tab ID")
	swUnregisterCmd.Flags().String("tab", "", "Target tab ID")
	swUpdateCmd.Flags().String("tab", "", "Target tab ID")
	swCachesCmd.Flags().String("tab", "", "Target tab ID")
	swPostMessageCmd.Flags().String("tab", "", "Target tab ID")
}

// Implementation functions

func listServiceWorkers(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	swCtrl := NewServiceWorkerController(debugger, verbose)
	workers, err := swCtrl.ListServiceWorkers(ctx)
	if err != nil {
		return err
	}

	fmt.Println("Registered Service Workers:")
	if len(workers) == 0 {
		fmt.Println("  (none)")
		return nil
	}

	for i, worker := range workers {
		fmt.Printf("\n[%d] Service Worker:\n", i+1)
		if workerJSON, err := json.MarshalIndent(worker, "  ", "  "); err == nil {
			fmt.Printf("  %s\n", string(workerJSON))
		} else {
			fmt.Printf("  %v\n", worker)
		}
	}

	return nil
}

func inspectServiceWorker(ctx context.Context, scope, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	swCtrl := NewServiceWorkerController(debugger, verbose)
	workerInfo, err := swCtrl.InspectServiceWorker(ctx, scope)
	if err != nil {
		return err
	}

	fmt.Printf("Service Worker Details for scope: %s\n", scope)
	fmt.Println("=====================================")

	if workerJSON, err := json.MarshalIndent(workerInfo, "", "  "); err == nil {
		fmt.Println(string(workerJSON))
	} else {
		fmt.Printf("%v\n", workerInfo)
	}

	return nil
}

func unregisterServiceWorker(ctx context.Context, scope, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	swCtrl := NewServiceWorkerController(debugger, verbose)
	if err := swCtrl.UnregisterServiceWorker(ctx, scope); err != nil {
		return err
	}

	fmt.Printf("✓ Unregistered service worker for scope: %s\n", scope)
	return nil
}

func updateServiceWorker(ctx context.Context, scope, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	swCtrl := NewServiceWorkerController(debugger, verbose)
	if err := swCtrl.UpdateServiceWorker(ctx, scope); err != nil {
		return err
	}

	fmt.Printf("✓ Triggered update for service worker: %s\n", scope)
	return nil
}

func getServiceWorkerCaches(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	swCtrl := NewServiceWorkerController(debugger, verbose)
	caches, err := swCtrl.GetServiceWorkerCaches(ctx)
	if err != nil {
		return err
	}

	fmt.Println("Service Worker Caches:")
	if len(caches) == 0 {
		fmt.Println("  (none)")
		return nil
	}

	for i, cache := range caches {
		fmt.Printf("\n[%d] Cache:\n", i+1)
		if cacheJSON, err := json.MarshalIndent(cache, "  ", "  "); err == nil {
			fmt.Printf("  %s\n", string(cacheJSON))
		} else {
			fmt.Printf("  %v\n", cache)
		}
	}

	return nil
}

func postMessageToServiceWorker(ctx context.Context, scope, message, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	swCtrl := NewServiceWorkerController(debugger, verbose)
	if err := swCtrl.PostMessageToServiceWorker(ctx, scope, message); err != nil {
		return err
	}

	fmt.Printf("✓ Posted message to service worker (%s): %s\n", scope, message)
	return nil
}