// Package main implements the NDP (Node Debug Protocol) CLI tool for unified
// debugging of Node.js and Chrome applications using the Chrome DevTools Protocol.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/tmc/misc/chrome-to-har/internal/targets"
)

var (
	verbose     bool
	timeout     int
	config      string
	mcpMode     bool
	nodePort    string
	apiPort     int
	targetTitle string
	targetURL   string
)

var rootCmd = &cobra.Command{
	Use:   "ndp",
	Short: "Node Debug Protocol - Unified debugger for Node.js and Chrome",
	Long: `NDP provides a powerful command-line interface for debugging Node.js
applications using the V8 Inspector Protocol.

Features:
- Attach to running Node.js processes (--inspect port)
- MCP server mode for AI agent integration (--mcp)
- Source listing, reading, and search across loaded modules
- CPU profiling, heap snapshots, and code coverage
- Console and exception capture
- Sourcemap analysis for bundled/compiled code
- Electron main process debugging (detect_electron tool)

Electron usage:
  ndp --mcp --node-port 9229   # main process
  cdp --mcp --remote-port 9222 # renderer (separate server)`,
	Version: "1.0.0",
	Run: func(cmd *cobra.Command, args []string) {
		if mcpMode {
			if err := runMCP(mcpConfig{
				NodePort:    nodePort,
				APIPort:     apiPort,
				Verbose:     verbose,
				TargetTitle: targetTitle,
				TargetURL:   targetURL,
			}); err != nil {
				log.Fatalf("MCP server: %v", err)
			}
			return
		}
		// If no subcommand, list all debug sessions
		if len(args) == 0 {
			nodeListCmd.Run(cmd, args)
		}
	},
}

var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Debug Node.js applications",
	Long:  "Commands for debugging Node.js processes using the V8 Inspector Protocol",
	Run: func(cmd *cobra.Command, args []string) {
		// If no subcommand, show sessions by default
		if len(args) == 0 {
			nodeSessionsCmd.Run(cmd, args)
		}
	},
}

var chromeCmd = &cobra.Command{
	Use:   "chrome",
	Short: "Debug Chrome/Chromium browsers",
	Long:  "Commands for debugging Chrome tabs and extensions using CDP",
}

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage debug sessions",
	Long:  "Save, load, and manage debugging sessions across targets",
}

var callCmd = &cobra.Command{
	Use:   "call <method> [json_params]",
	Short: "Execute a raw CDP method",
	Args:  cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		targetID, _ := cmd.Flags().GetString("target")

		// If target not specified, look for session or env var
		if targetID == "" {
			targetID = os.Getenv("NDP_SESSION_ID") // Simple env var for now
		}

		// Connect to target (or get existing session)
		// For now, we assume direct attach if target is provided
		// TODO: Implement cleaner session loading
		debugger := NewNodeDebugger(verbose)
		if targetID != "" {
			if err := debugger.Attach(ctx, targetID); err != nil {
				log.Fatalf("Failed to attach: %v", err)
			}
		} else {
			// Try to find a sensible default (e.g. only one node process)
			// For generic call, we might require explicit target for safety,
			// but for now let's try auto-attach if one exists?
			// Better: Assume user must provide target or use 'ndp repl' to discovery
			// log.Fatalf("Target ID required (use --target or set NDP_SESSION_ID)")
			// Auto-discovery logic similar to 'node attach'
			if err := debugger.Attach(ctx, "9229"); err != nil {
				log.Fatalf("No default target found: %v", err)
			}
		}

		method := args[0]
		var params interface{}
		if len(args) > 1 {
			if err := json.Unmarshal([]byte(args[1]), &params); err != nil {
				log.Fatalf("Invalid JSON params: %v", err)
			}
		}

		// Auto-enable domain if needed
		domain := strings.Split(method, ".")[0]
		if domain == "Debugger" || domain == "Profiler" || domain == "Runtime" {
			if verbose {
				log.Printf("Auto-enabling domain: %s", domain)
			}
			debugger.Execute(ctx, domain+".enable", nil)
		}

		result, err := debugger.Execute(ctx, method, params)
		if err != nil {
			log.Fatalf("Error: %v", err)
		}

		// Print result as JSON
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
	},
}

var runtimeCmd = &cobra.Command{
	Use:   "runtime",
	Short: "Runtime domain commands",
}

var runtimeEvalCmd = &cobra.Command{
	Use:     "evaluate <expression>",
	Short:   "Evaluate JavaScript expression",
	Aliases: []string{"eval"},
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Reuse logic from callCmd but pre-format params
		// ... (Simplified for brevity, implemented via shared helper in real code)
		// For implementation speed, just re-invoke callCmd logic or better yet, make callCmd run function reusable.

		callCmd.Run(cmd, []string{"Runtime.evaluate", fmt.Sprintf(`{"expression": %q, "includeCommandLineAPI": true}`, args[0])})
	},
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().IntVar(&timeout, "timeout", 60, "Operation timeout in seconds")
	rootCmd.PersistentFlags().StringVar(&config, "config", "", "Path to configuration file")
	rootCmd.Flags().BoolVar(&mcpMode, "mcp", false, "Run as MCP server (stdio transport)")
	rootCmd.Flags().StringVar(&nodePort, "node-port", "9229", "Node.js inspector port")
	rootCmd.Flags().IntVar(&apiPort, "api-port", 0, "Coverage API port (0 = auto-assign)")
	rootCmd.Flags().StringVar(&targetTitle, "target-title", "", "Connect to target whose title contains this string")
	rootCmd.Flags().StringVar(&targetURL, "target-url", "", "Connect to target whose URL contains this string")

	// Add subcommands
	rootCmd.AddCommand(nodeCmd)
	rootCmd.AddCommand(chromeCmd)
	rootCmd.AddCommand(sessionCmd)

	rootCmd.AddCommand(targetsCmd)
	rootCmd.AddCommand(replCmd)

	// Node commands
	nodeCmd.AddCommand(nodeAttachCmd)
	nodeCmd.AddCommand(nodeListCmd)
	nodeCmd.AddCommand(nodeSessionsCmd)
	nodeCmd.AddCommand(nodeStartCmd)

	nodeCmd.AddCommand(nodeWatchCmd)

	// Chrome commands
	chromeCmd.AddCommand(chromeAttachCmd)
	chromeCmd.AddCommand(chromeListCmd)
	chromeCmd.AddCommand(chromeNavigateCmd)
	chromeCmd.AddCommand(chromeConsoleCmd)

	// Session commands
	sessionCmd.AddCommand(sessionSaveCmd)
	sessionCmd.AddCommand(sessionLoadCmd)
	sessionCmd.AddCommand(sessionListCmd)

	// Call command
	rootCmd.AddCommand(callCmd)
	rootCmd.AddCommand(runtimeCmd)

	// Runtime subcommands
	runtimeCmd.AddCommand(runtimeEvalCmd)

	// Profile commands

}

// Node commands
var nodeAttachCmd = &cobra.Command{
	Use:   "attach [port]",
	Short: "Attach to a running Node.js process",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		port := "9229"
		if len(args) > 0 {
			port = args[0]
		}

		ctx := createContext()
		debugger := NewNodeDebugger(verbose)
		if err := debugger.Attach(ctx, port); err != nil {
			log.Fatalf("Failed to attach to Node.js: %v", err)
		}
	},
}

var nodeSessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "List active debug sessions",
	Run: func(cmd *cobra.Command, args []string) {
		sessions, err := ListSessionFiles()
		if err != nil {
			log.Fatalf("Failed to list sessions: %v", err)
		}

		if len(sessions) == 0 {
			fmt.Fprintln(os.Stderr, "No active sessions")
			fmt.Fprintln(os.Stderr, "Use 'ndp node attach <port>' to create a session")
			return
		}

		simple, _ := cmd.Flags().GetBool("simple")

		if simple {
			// Just output ports for scripting
			for _, s := range sessions {
				fmt.Println(s.Port)
			}
		} else {
			defaultPort, _ := GetDefaultSession()

			fmt.Println("Active sessions:")
			for _, s := range sessions {
				marker := " "
				if s.Port == defaultPort {
					marker = "*"
				}
				fmt.Printf("%s Port %s: %s (%s)\n", marker, s.Port, s.Title, s.URL)
			}
			fmt.Println("\n* = default session (used when no port specified)")
			fmt.Println("Set NDP_PORT environment variable or use -p flag to specify port")
		}
	},
}

var nodeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List active Node.js debug sessions",
	Run: func(cmd *cobra.Command, args []string) {
		showBreakpoints, _ := cmd.Flags().GetBool("breakpoints")

		// Check common Node.js debug ports
		ports := []string{"9229", "9230", "9231", "9232", "9222", "9223"}
		foundAny := false

		for _, port := range ports {
			url := fmt.Sprintf("http://localhost:%s/json/list", port)
			client := &http.Client{Timeout: 500 * time.Millisecond}
			resp, err := client.Get(url)
			if err != nil {
				continue
			}

			var targets []map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
				resp.Body.Close()
				continue
			}
			resp.Body.Close()

			if len(targets) == 0 {
				continue
			}

			foundAny = true
			for _, target := range targets {
				fmt.Printf("Port %s: %s\n", port, target["title"])
				fmt.Printf("  URL: %s\n", target["url"])
				fmt.Printf("  WebSocket: %s\n", target["webSocketDebuggerUrl"])

				if showBreakpoints {
					// Try to get breakpoints by attaching briefly
					ctx := createContext()
					debugger := NewNodeDebugger(verbose)

					// Suppress output
					oldStdout := os.Stdout
					oldStderr := os.Stderr
					os.Stdout = os.NewFile(0, os.DevNull)
					os.Stderr = os.NewFile(0, os.DevNull)

					if err := debugger.Attach(ctx, port); err == nil {
						// Successfully attached, now we could query breakpoints
						// This would need implementation in the debugger
						fmt.Fprintf(oldStdout, "  Breakpoints: (querying not yet implemented)\n")
					}

					os.Stdout = oldStdout
					os.Stderr = oldStderr
				}
				fmt.Println()
			}
		}

		if !foundAny {
			fmt.Println("No active Node.js debug sessions found")
			fmt.Println("Start Node.js with: node --inspect script.js")
		}
	},
}

var nodeStartCmd = &cobra.Command{
	Use:   "start <script.js>",
	Short: "Start Node.js script with debugging enabled",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		debugger := NewNodeDebugger(verbose)

		inspectBrk, _ := cmd.Flags().GetBool("break")
		port, _ := cmd.Flags().GetString("port")

		if err := debugger.StartScript(ctx, args[0], port, inspectBrk); err != nil {
			log.Fatalf("Failed to start script: %v", err)
		}
	},
}

var nodeWatchCmd = &cobra.Command{
	Use:   "watch <port> <expression>",
	Short: "Add a watch expression to a debug session",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		port := args[0]
		expression := args[1]

		// Simply re-attach to the port and add watch
		debugger := NewNodeDebugger(verbose)

		// Suppress duplicate output when re-attaching
		oldStdout := os.Stdout
		os.Stdout = os.Stderr // Temporarily redirect stdout to stderr

		if err := debugger.Attach(ctx, port); err != nil {
			os.Stdout = oldStdout
			log.Fatalf("Failed to attach to port %s: %v", port, err)
		}

		os.Stdout = oldStdout

		if err := debugger.AddWatch(ctx, expression); err != nil {
			log.Fatalf("Failed to add watch: %v", err)
		}
	},
}

// Chrome commands
var chromeAttachCmd = &cobra.Command{
	Use:   "attach [port]",
	Short: "Attach to Chrome/Chromium browser",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		port := "9222"
		if len(args) > 0 {
			port = args[0]
		}

		ctx := createContext()
		debugger := NewChromeDebugger(verbose)
		if err := debugger.Attach(ctx, port); err != nil {
			log.Fatalf("Failed to attach to Chrome: %v", err)
		}
	},
}

var chromeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List Chrome tabs and targets",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		debugger := NewChromeDebugger(verbose)

		port, _ := cmd.Flags().GetString("port")
		tabs, err := debugger.ListTabs(ctx, port)
		if err != nil {
			log.Fatalf("Failed to list tabs: %v", err)
		}

		fmt.Printf("Chrome tabs on port %s:\n", port)
		for i, tab := range tabs {
			fmt.Printf("  [%d] %s\n", i, tab.Title)
			fmt.Printf("      URL: %s\n", tab.URL)
			fmt.Printf("      ID: %s\n", tab.ID)
		}
	},
}

var chromeNavigateCmd = &cobra.Command{
	Use:   "navigate <url>",
	Short: "Navigate to URL",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		debugger := NewChromeDebugger(verbose)

		tabID, _ := cmd.Flags().GetString("tab")

		if err := debugger.Navigate(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to navigate: %v", err)
		}
	},
}

var chromeConsoleCmd = &cobra.Command{
	Use:   "console <javascript>",
	Short: "Execute JavaScript in console",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		debugger := NewChromeDebugger(verbose)

		tabID, _ := cmd.Flags().GetString("tab")

		result, err := debugger.EvaluateJS(ctx, args[0], tabID)
		if err != nil {
			log.Fatalf("Failed to execute: %v", err)
		}

		fmt.Printf("Result: %v\n", result)
	},
}

// Session commands
var sessionSaveCmd = &cobra.Command{
	Use:   "save <name>",
	Short: "Save current debug session",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		manager := NewSessionManager(verbose)

		if err := manager.SaveSession(ctx, args[0]); err != nil {
			log.Fatalf("Failed to save session: %v", err)
		}

		fmt.Printf("Session '%s' saved successfully\n", args[0])
	},
}

var sessionLoadCmd = &cobra.Command{
	Use:   "load <name>",
	Short: "Load debug session",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		manager := NewSessionManager(verbose)

		if err := manager.LoadSession(ctx, args[0]); err != nil {
			log.Fatalf("Failed to load session: %v", err)
		}

		fmt.Printf("Session '%s' loaded successfully\n", args[0])
	},
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved sessions",
	Run: func(cmd *cobra.Command, args []string) {
		manager := NewSessionManager(verbose)
		sessions, err := manager.ListSessions()
		if err != nil {
			log.Fatalf("Failed to list sessions: %v", err)
		}

		if len(sessions) == 0 {
			fmt.Println("No saved sessions found")
			return
		}

		fmt.Println("Saved sessions:")
		for _, s := range sessions {
			fmt.Printf("  - %s (created: %s)\n", s.Name, s.Created)
		}
	},
}

// Profile commands

// Other commands
var targetsCmd = &cobra.Command{
	Use:   "targets",
	Short: "List all debug targets",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()

		// Use Chrome-style discovery for all targets
		discovery := targets.NewDiscovery(5 * time.Second)
		allTargets, err := discovery.DiscoverTargets(ctx)
		if err != nil {
			log.Fatalf("Failed to discover targets: %v", err)
		}

		if len(allTargets) == 0 {
			fmt.Println("No debug targets found")
			fmt.Println("Ensure Chrome is running with --remote-debugging-port=9222")
			fmt.Println("Or Node.js with --inspect flag")
			return
		}

		fmt.Println("Available debug targets:")
		fmt.Println()

		// Group by type
		nodeTargets := targets.FilterNodeTargets(allTargets)
		chromeTargets := targets.FilterChromeTargets(allTargets)

		if len(nodeTargets) > 0 {
			fmt.Println("Node.js Targets:")
			for _, target := range nodeTargets {
				fmt.Printf("  [node] %s (port %d)\n", target.Title, target.Port)
				fmt.Printf("         %s\n", target.ID)
			}
			fmt.Println()
		}

		if len(chromeTargets) > 0 {
			fmt.Println("Chrome/Browser Targets:")
			for _, target := range chromeTargets {
				fmt.Printf("  [%s] %s (port %d)\n", target.Type, target.Title, target.Port)
				fmt.Printf("         %s\n", target.URL)
			}
			fmt.Println()
		}

		// Show other targets
		otherTargets := []targets.TargetInfo{}
		for _, target := range allTargets {
			if !targets.IsNodeTarget(target) && !targets.IsChromeTarget(target) {
				otherTargets = append(otherTargets, target)
			}
		}

		if len(otherTargets) > 0 {
			fmt.Println("Other Targets:")
			for _, target := range otherTargets {
				fmt.Printf("  [%s] %s (port %d)\n", target.Type, target.Title, target.Port)
			}
		}
	},
}

var replCmd = &cobra.Command{
	Use:   "repl",
	Short: "Start interactive REPL",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		repl := NewREPL(verbose)

		target, _ := cmd.Flags().GetString("target")

		if err := repl.Start(ctx, target); err != nil {
			log.Fatalf("REPL error: %v", err)
		}
	},
}

func main() {
	// Add command flags
	nodeStartCmd.Flags().BoolP("break", "b", false, "Break before first line")
	nodeStartCmd.Flags().String("port", "9229", "Debug port")

	nodeSessionsCmd.Flags().BoolP("simple", "s", false, "Simple output (just ports)")
	nodeListCmd.Flags().BoolP("breakpoints", "b", false, "Show breakpoints in each session")
	chromeNavigateCmd.Flags().String("tab", "", "Target tab ID")
	chromeConsoleCmd.Flags().String("tab", "", "Target tab ID")

	replCmd.Flags().StringP("target", "t", "", "Target to attach to")

	rootCmd.AddCommand(tuiCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func createContext() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), getTimeout())

	// Handle interrupts
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		if verbose {
			log.Println("Interrupt received, shutting down...")
		}
		cancel()
	}()

	return ctx
}

func getTimeout() time.Duration {
	return time.Duration(timeout) * time.Second
}
