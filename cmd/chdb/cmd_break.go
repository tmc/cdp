package main

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

// Breakpoint management commands
var breakCmd = &cobra.Command{
	Use:   "break",
	Short: "Manage breakpoints",
	Long:  "Set, list, and remove breakpoints for debugging",
}

var breakSetCmd = &cobra.Command{
	Use:   "set <location>",
	Short: "Set a breakpoint at location (file:line or URL:line)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		condition, _ := cmd.Flags().GetString("condition")
		tabID, _ := cmd.Flags().GetString("tab")

		if err := setBreakpointNew(ctx, args[0], condition, tabID); err != nil {
			log.Fatalf("Failed to set breakpoint: %v", err)
		}
	},
}

var breakListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all breakpoints",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := listBreakpoints(ctx, tabID); err != nil {
			log.Fatalf("Failed to list breakpoints: %v", err)
		}
	},
}

var breakRemoveCmd = &cobra.Command{
	Use:   "remove <id>",
	Short: "Remove a breakpoint by ID",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := removeBreakpoint(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to remove breakpoint: %v", err)
		}
	},
}

var breakClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear all breakpoints",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := clearBreakpoints(ctx, tabID); err != nil {
			log.Fatalf("Failed to clear breakpoints: %v", err)
		}
	},
}

var breakXHRCmd = &cobra.Command{
	Use:   "xhr <url-pattern>",
	Short: "Break on XHR/Fetch URL",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := setXHRBreakpoint(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to set XHR breakpoint: %v", err)
		}
	},
}

var breakEventCmd = &cobra.Command{
	Use:   "event <event-name>",
	Short: "Break on event listener (e.g. click, keydown)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := setEventBreakpoint(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to set event breakpoint: %v", err)
		}
	},
}

func init() {
	// Add subcommands to break command
	breakCmd.AddCommand(breakSetCmd)
	breakCmd.AddCommand(breakListCmd)
	breakCmd.AddCommand(breakRemoveCmd)
	breakCmd.AddCommand(breakClearCmd)
	breakCmd.AddCommand(breakXHRCmd)
	breakCmd.AddCommand(breakEventCmd)

	// Add flags
	breakSetCmd.Flags().StringP("condition", "c", "", "Conditional breakpoint expression")
	breakSetCmd.Flags().String("tab", "", "Target tab ID")
	breakListCmd.Flags().String("tab", "", "Target tab ID")
	breakRemoveCmd.Flags().String("tab", "", "Target tab ID")
	breakClearCmd.Flags().String("tab", "", "Target tab ID")
}

func setBreakpointNew(ctx context.Context, location string, condition string, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create debugger controller
	debugCtrl := NewDebuggerController(debugger, verbose)

	// Set breakpoint
	bp, err := debugCtrl.SetBreakpoint(ctx, location, condition)
	if err != nil {
		return err
	}

	fmt.Printf("✓ Breakpoint set:\n")
	fmt.Printf("  ID: %s\n", bp.ID)
	fmt.Printf("  Location: %s:%d\n", bp.URL, bp.LineNumber)
	if bp.Condition != "" {
		fmt.Printf("  Condition: %s\n", bp.Condition)
	}

	return nil
}

func listBreakpoints(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create debugger controller
	debugCtrl := NewDebuggerController(debugger, verbose)

	// List breakpoints
	breakpoints := debugCtrl.ListBreakpoints()

	if len(breakpoints) == 0 {
		fmt.Println("No breakpoints set")
		return nil
	}

	fmt.Printf("Active breakpoints (%d):\n", len(breakpoints))
	for _, bp := range breakpoints {
		fmt.Printf("\n  ID: %s\n", bp.ID)
		fmt.Printf("  Type: %s\n", bp.Type)
		fmt.Printf("  Location: %s:%d", bp.URL, bp.LineNumber)
		if bp.ColumnNumber > 0 {
			fmt.Printf(":%d", bp.ColumnNumber)
		}
		fmt.Println()
		if bp.Condition != "" {
			fmt.Printf("  Condition: %s\n", bp.Condition)
		}
		fmt.Printf("  Enabled: %v\n", bp.Enabled)
	}

	return nil
}

func removeBreakpoint(ctx context.Context, breakpointID string, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create debugger controller
	debugCtrl := NewDebuggerController(debugger, verbose)

	// Remove breakpoint
	if err := debugCtrl.RemoveBreakpoint(ctx, breakpointID); err != nil {
		return err
	}

	fmt.Printf("✓ Breakpoint removed: %s\n", breakpointID)

	return nil
}

func clearBreakpoints(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create debugger controller
	debugCtrl := NewDebuggerController(debugger, verbose)

	// Get all breakpoints
	breakpoints := debugCtrl.ListBreakpoints()

	if len(breakpoints) == 0 {
		fmt.Println("No breakpoints to clear")
		return nil
	}

	// Remove each breakpoint
	for _, bp := range breakpoints {
		if err := debugCtrl.RemoveBreakpoint(ctx, bp.ID); err != nil {
			log.Printf("Warning: Failed to remove breakpoint %s: %v", bp.ID, err)
		}
	}

	fmt.Printf("✓ Cleared %d breakpoint(s)\n", len(breakpoints))

	return nil
}

func setXHRBreakpoint(ctx context.Context, pattern string, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	ctrl := NewDebuggerController(debugger, verbose)
	if err := ctrl.SetXHRBreakpoint(ctx, pattern); err != nil {
		return err
	}

	fmt.Printf("✓ XHR Breakpoint set for: %s\n", pattern)
	return nil
}

func setEventBreakpoint(ctx context.Context, name string, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	ctrl := NewDebuggerController(debugger, verbose)
	if err := ctrl.SetEventListenerBreakpoint(ctx, name); err != nil {
		return err
	}

	fmt.Printf("✓ Event Breakpoint set for: %s\n", name)
	return nil
}
