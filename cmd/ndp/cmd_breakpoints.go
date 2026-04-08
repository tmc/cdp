package main

import (
	"fmt"
	"log"

	"github.com/chromedp/cdproto/domdebugger"
	"github.com/chromedp/chromedp"
	"github.com/spf13/cobra"
)

var nodeBreakpointsCmd = &cobra.Command{
	Use:   "breakpoints <port>",
	Short: "List breakpoints in a debug session",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		port := args[0]

		debugger := NewNodeDebugger(verbose)
		if err := debugger.Attach(ctx, port); err != nil {
			log.Fatalf("Failed to attach to Node.js process: %v", err)
		}

		session := globalSessionTracker.GetCurrentSession()
		if session == nil {
			log.Fatalf("No active debug session")
		}
		if globalSessionTracker.GetNodeDebugger() == nil {
			log.Fatalf("Node debugger is not available")
		}
		session, err := debugger.manager.GetSession(session.ID)
		if err != nil {
			log.Fatalf("Failed to resolve session: %v", err)
		}

		manager := NewBreakpointManager(verbose)
		manager.SetSession(session)
		if err := manager.LoadBreakpoints(""); err != nil {
			log.Fatalf("Failed to load breakpoints: %v", err)
		}

		breakpoints := manager.ListBreakpoints()
		if len(breakpoints) == 0 {
			setter := NewSimpleBreakpointSetter(port)
			if err := setter.ListBreakpoints(ctx); err != nil {
				log.Fatalf("Failed to list breakpoints: %v", err)
			}
			return
		}

		fmt.Printf("Saved breakpoints (%d):\n", len(breakpoints))
		for _, bp := range breakpoints {
			if bp.Condition != "" {
				fmt.Printf("  %s  %s if %s\n", bp.ID, bp.Location, bp.Condition)
				continue
			}
			fmt.Printf("  %s  %s\n", bp.ID, bp.Location)
		}
	},
}

var nodeBreakCmd = &cobra.Command{
	Use:   "break <port> <file:line>",
	Short: "Set a breakpoint in a debug session",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		port := args[0]
		location := args[1]
		condition, _ := cmd.Flags().GetString("condition")

		// Try using chromedp BreakpointManager first, fall back to simple approach
		debugger := NewNodeDebugger(verbose)
		if err := debugger.Attach(ctx, port); err != nil {
			log.Fatalf("Failed to attach to Node.js process: %v", err)
		}

		// Get the session and create breakpoint manager
		session := globalSessionTracker.GetCurrentSession()
		if session == nil {
			log.Fatalf("No active debug session")
		}
		if globalSessionTracker.GetNodeDebugger() == nil {
			log.Fatalf("Node debugger is not available")
		}
		session, err := debugger.manager.GetSession(session.ID)
		if err != nil {
			log.Fatalf("Failed to resolve session: %v", err)
		}

		manager := NewBreakpointManager(verbose)
		manager.SetSession(session)

		if err := manager.SetBreakpoint(ctx, location, condition); err != nil {
			log.Printf("Failed to set breakpoint via manager: %v. Trying simple fallback...", err)
			setter := NewSimpleBreakpointSetter(port)
			if err := setter.SetBreakpoint(ctx, location, condition); err != nil {
				log.Fatalf("Failed to set breakpoint: %v", err)
			}
		}
	},
}

var nodeBreakRemoveCmd = &cobra.Command{
	Use:   "break-remove <port> <breakpoint-id>",
	Short: "Remove a saved breakpoint from a debug session",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		port := args[0]
		breakpointID := args[1]

		debugger := NewNodeDebugger(verbose)
		if err := debugger.Attach(ctx, port); err != nil {
			log.Fatalf("Failed to attach to Node.js process: %v", err)
		}

		session := globalSessionTracker.GetCurrentSession()
		if session == nil {
			log.Fatalf("No active debug session")
		}
		if globalSessionTracker.GetNodeDebugger() == nil {
			log.Fatalf("Node debugger is not available")
		}
		session, err := debugger.manager.GetSession(session.ID)
		if err != nil {
			log.Fatalf("Failed to resolve session: %v", err)
		}

		manager := NewBreakpointManager(verbose)
		manager.SetSession(session)
		if err := manager.LoadBreakpoints(""); err != nil {
			log.Fatalf("Failed to load breakpoints: %v", err)
		}
		bp, err := manager.GetBreakpoint(breakpointID)
		if err != nil {
			log.Fatalf("Failed to find breakpoint: %v", err)
		}
		if err := manager.RemoveBreakpoint(ctx, breakpointID); err != nil {
			log.Fatalf("Failed to remove breakpoint: %v", err)
		}
		fmt.Printf("Removed breakpoint %s at %s\n", bp.ID, bp.Location)
	},
}

var nodeBreakXhrCmd = &cobra.Command{
	Use:   "break-xhr <port> [url-substring]",
	Short: "Set an XHR/Fetch breakpoint",
	Long:  "Break when an XHR or Fetch request URL contains the substring. If empty, breaks on all.",
	Args:  cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		port := args[0]
		urlPattern := ""
		if len(args) > 1 {
			urlPattern = args[1]
		}

		debugger := NewNodeDebugger(verbose)
		if err := debugger.Attach(ctx, port); err != nil {
			log.Fatalf("Failed to attach to Node.js process: %v", err)
		}

		session := globalSessionTracker.GetCurrentSession()
		if session == nil {
			log.Fatalf("No active debug session")
		}

		log.Printf("Setting XHR breakpoint for pattern: %q...", urlPattern)
		err := chromedp.Run(session.ChromeCtx,
			domdebugger.SetXHRBreakpoint(urlPattern),
		)
		if err != nil {
			log.Fatalf("Failed to set XHR breakpoint: %v", err)
		}
		log.Println("XHR breakpoint set successfully.")
	},
}

func init() {
	nodeCmd.AddCommand(nodeBreakCmd)
	nodeCmd.AddCommand(nodeBreakpointsCmd)
	nodeCmd.AddCommand(nodeBreakRemoveCmd)
	nodeCmd.AddCommand(nodeBreakXhrCmd)

	nodeBreakCmd.Flags().StringP("condition", "c", "", "Conditional breakpoint")
}
