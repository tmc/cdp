package main

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

// Execution control commands
var pauseCmd = &cobra.Command{
	Use:   "pause",
	Short: "Pause JavaScript execution",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := pauseExecution(ctx, tabID); err != nil {
			log.Fatalf("Failed to pause execution: %v", err)
		}
	},
}

var resumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume JavaScript execution",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := resumeExecution(ctx, tabID); err != nil {
			log.Fatalf("Failed to resume execution: %v", err)
		}
	},
}

var stepCmd = &cobra.Command{
	Use:   "step",
	Short: "Step into next function call",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := stepInto(ctx, tabID); err != nil {
			log.Fatalf("Failed to step into: %v", err)
		}
	},
}

var nextCmd = &cobra.Command{
	Use:   "next",
	Short: "Step over to next line",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := stepOver(ctx, tabID); err != nil {
			log.Fatalf("Failed to step over: %v", err)
		}
	},
}

var outCmd = &cobra.Command{
	Use:   "out",
	Short: "Step out of current function",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := stepOut(ctx, tabID); err != nil {
			log.Fatalf("Failed to step out: %v", err)
		}
	},
}

var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Start interactive debugging session",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := startDebugging(ctx, tabID); err != nil {
			log.Fatalf("Debug session failed: %v", err)
		}
	},
}

func init() {
	// Add flags
	pauseCmd.Flags().String("tab", "", "Target tab ID")
	resumeCmd.Flags().String("tab", "", "Target tab ID")
	stepCmd.Flags().String("tab", "", "Target tab ID")
	nextCmd.Flags().String("tab", "", "Target tab ID")
	outCmd.Flags().String("tab", "", "Target tab ID")
	debugCmd.Flags().String("tab", "", "Target tab ID")
}

// Implementation functions

func pauseExecution(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create debugger controller
	debugCtrl := NewDebuggerController(debugger, verbose)

	// Pause execution
	if err := debugCtrl.Pause(ctx); err != nil {
		return err
	}

	fmt.Println("✓ Execution paused")

	return nil
}

func resumeExecution(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create debugger controller
	debugCtrl := NewDebuggerController(debugger, verbose)

	// Resume execution
	if err := debugCtrl.Resume(ctx); err != nil {
		return err
	}

	fmt.Println("✓ Execution resumed")

	return nil
}

func stepInto(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create debugger controller
	debugCtrl := NewDebuggerController(debugger, verbose)

	// Step into
	if err := debugCtrl.StepInto(ctx); err != nil {
		return err
	}

	fmt.Println("✓ Stepped into function")

	return nil
}

func stepOver(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create debugger controller
	debugCtrl := NewDebuggerController(debugger, verbose)

	// Step over
	if err := debugCtrl.StepOver(ctx); err != nil {
		return err
	}

	fmt.Println("✓ Stepped over line")

	return nil
}

func stepOut(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create debugger controller
	debugCtrl := NewDebuggerController(debugger, verbose)

	// Step out
	if err := debugCtrl.StepOut(ctx); err != nil {
		return err
	}

	fmt.Println("✓ Stepped out of function")

	return nil
}

func startDebugging(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Launch DevTools for the target
	if err := debugger.LaunchDevTools(ctx); err != nil {
		log.Printf("Warning: Could not launch DevTools GUI: %v", err)
	}

	fmt.Println("Interactive debugging session started...")
	fmt.Println("DevTools should open in your browser.")
	fmt.Println("Press Ctrl+C to exit.")

	// Keep session alive
	<-ctx.Done()

	return nil
}