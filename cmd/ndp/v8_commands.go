package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

// Global V8 client instance for commands
var v8Client *V8InspectorClient
var v8Debugger *V8Debugger
var v8Runtime *V8Runtime
var v8Profiler *V8Profiler

// V8 command group - comprehensive Node.js debugging commands
var v8Cmd = &cobra.Command{
	Use:   "v8 [command]",
	Short: "Advanced V8 Inspector debugging (Chrome DevTools compatible)",
	Long: `Advanced Node.js debugging using the V8 Inspector Protocol.
This provides comprehensive debugging capabilities matching Chrome DevTools:
- Breakpoint management and stepping
- Runtime evaluation and object inspection
- CPU and memory profiling
- Code coverage analysis
- Interactive console/REPL`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Initialize V8 client for all subcommands
		host := "127.0.0.1"
		v8Client = NewV8InspectorClient(host, "", verbose)
		v8Debugger = NewV8Debugger(v8Client)
		v8Runtime = NewV8Runtime(v8Client)
		v8Profiler = NewV8Profiler(v8Client)
	},
}

// Connect command
var v8ConnectCmd = &cobra.Command{
	Use:   "connect <port>",
	Short: "Connect to a Node.js debugging session",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		port := args[0]

		if err := v8Client.ConnectByPort(ctx, port); err != nil {
			log.Fatalf("Failed to connect: %v", err)
		}

		// Enable debugging domains
		if err := v8Runtime.EnableRuntime(); err != nil {
			log.Fatalf("Failed to enable runtime: %v", err)
		}

		if err := v8Debugger.EnableDebugger(); err != nil {
			log.Fatalf("Failed to enable debugger: %v", err)
		}

		fmt.Printf("Connected to Node.js process on port %s\n", port)

		// Show target info
		info := v8Client.GetTargetInfo()
		if infoJSON, err := json.MarshalIndent(info, "", "  "); err == nil {
			fmt.Printf("Target info:\n%s\n", infoJSON)
		}
	},
}

// Targets command
var v8TargetsCmd = &cobra.Command{
	Use:   "targets [port]",
	Short: "List available Node.js debugging targets",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()

		port := "9229" // default port
		if len(args) > 0 {
			port = args[0]
		}

		v8Client.port = port
		targets, err := v8Client.DiscoverTargets(ctx)
		if err != nil {
			log.Fatalf("Failed to discover targets: %v", err)
		}

		if len(targets) == 0 {
			fmt.Printf("No debugging targets found on port %s\n", port)
			return
		}

		fmt.Printf("Found %d debugging target(s) on port %s:\n\n", len(targets), port)
		for i, target := range targets {
			fmt.Printf("[%d] %s\n", i+1, target.Title)
			fmt.Printf("    ID: %s\n", target.ID)
			fmt.Printf("    Type: %s\n", target.Type)
			fmt.Printf("    URL: %s\n", target.URL)
			fmt.Printf("    WebSocket: %s\n", target.WebSocketDebuggerURL)
			if target.Description != "" {
				fmt.Printf("    Description: %s\n", target.Description)
			}
			fmt.Println()
		}
	},
}

// Breakpoint commands
var v8BreakCmd = &cobra.Command{
	Use:   "break <location> [condition]",
	Short: "Set a breakpoint at the specified location",
	Long: `Set a breakpoint at the specified location.
Location format: file:line (e.g., script.js:25)
Optionally provide a condition for conditional breakpoints.`,
	Args: cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		connectAndRun(func() error {
			location := args[0]
			condition := ""
			if len(args) > 1 {
				condition = args[1]
			}

			bp, err := v8Debugger.SetBreakpointByLocation(location, condition)
			if err != nil {
				return err
			}

			fmt.Printf("Breakpoint set: %s\n", bp.ID)
			fmt.Printf("  Location: %s\n", location)
			if condition != "" {
				fmt.Printf("  Condition: %s\n", condition)
			}
			fmt.Printf("  Resolved: %t\n", bp.Resolved)

			return nil
		})
	},
}

var v8BreakListCmd = &cobra.Command{
	Use:   "break-list",
	Short: "List all active breakpoints",
	Run: func(cmd *cobra.Command, args []string) {
		connectAndRun(func() error {
			breakpoints := v8Debugger.ListBreakpoints()

			if len(breakpoints) == 0 {
				fmt.Println("No active breakpoints")
				return nil
			}

			fmt.Printf("Active breakpoints (%d):\n\n", len(breakpoints))
			for i, bp := range breakpoints {
				fmt.Printf("[%d] %s\n", i+1, bp.ID)
				if bp.Location != "" {
					fmt.Printf("    Location: %s\n", bp.Location)
				}
				if bp.URL != "" {
					fmt.Printf("    URL: %s\n", bp.URL)
				}
				if bp.URLRegex != "" {
					fmt.Printf("    URL Regex: %s\n", bp.URLRegex)
				}
				fmt.Printf("    Line: %d\n", bp.LineNumber)
				if bp.Condition != "" {
					fmt.Printf("    Condition: %s\n", bp.Condition)
				}
				fmt.Printf("    Resolved: %t\n", bp.Resolved)
				fmt.Println()
			}

			return nil
		})
	},
}

var v8BreakRemoveCmd = &cobra.Command{
	Use:   "break-remove <breakpoint-id>",
	Short: "Remove a breakpoint by ID",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		connectAndRun(func() error {
			breakpointID := args[0]

			if err := v8Debugger.RemoveBreakpoint(breakpointID); err != nil {
				return err
			}

			fmt.Printf("Breakpoint removed: %s\n", breakpointID)
			return nil
		})
	},
}

// Stepping commands
var v8ResumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume execution",
	Run: func(cmd *cobra.Command, args []string) {
		connectAndRun(func() error {
			return v8Debugger.Resume()
		})
	},
}

var v8StepIntoCmd = &cobra.Command{
	Use:   "step-into",
	Short: "Step into the next function call",
	Run: func(cmd *cobra.Command, args []string) {
		connectAndRun(func() error {
			return v8Debugger.StepInto()
		})
	},
}

var v8StepOverCmd = &cobra.Command{
	Use:   "step-over",
	Short: "Step over the next line",
	Run: func(cmd *cobra.Command, args []string) {
		connectAndRun(func() error {
			return v8Debugger.StepOver()
		})
	},
}

var v8StepOutCmd = &cobra.Command{
	Use:   "step-out",
	Short: "Step out of the current function",
	Run: func(cmd *cobra.Command, args []string) {
		connectAndRun(func() error {
			return v8Debugger.StepOut()
		})
	},
}

var v8PauseCmd = &cobra.Command{
	Use:   "pause",
	Short: "Pause execution",
	Run: func(cmd *cobra.Command, args []string) {
		connectAndRun(func() error {
			return v8Debugger.Pause()
		})
	},
}

// Evaluation commands
var v8EvalCmd = &cobra.Command{
	Use:   "eval <expression>",
	Short: "Evaluate JavaScript expression",
	Long: `Evaluate a JavaScript expression in the global context.
Supports all JavaScript syntax and returns formatted results.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		connectAndRun(func() error {
			expression := args[0]

			options := &EvaluateOptions{
				ReturnByValue:    true,
				GeneratePreview:  true,
				IncludeCommandLineAPI: true,
			}

			result, err := v8Runtime.Evaluate(expression, options)
			if err != nil {
				return err
			}

			if result.Exception != nil {
				fmt.Printf("Exception: %s\n", result.Exception.Text)
				if result.Exception.Exception != nil {
					fmt.Printf("Details: %s\n", v8Runtime.FormatValue(result.Exception.Exception))
				}
				return nil
			}

			if result.Result != nil {
				fmt.Printf("Result: %s\n", v8Runtime.FormatValue(result.Result))
				if result.Result.Type == "object" && result.Result.ObjectID != "" {
					// Show object properties for objects
					props, err := v8Runtime.GetProperties(result.Result.ObjectID, true, false)
					if err == nil && len(props) > 0 {
						fmt.Println("Properties:")
						for _, prop := range props {
							if prop.Value != nil {
								fmt.Printf("  %s: %s\n", prop.Name, v8Runtime.FormatValue(prop.Value))
							}
						}
					}
				}
			}

			return nil
		})
	},
}

// Call stack command
var v8StackCmd = &cobra.Command{
	Use:   "stack",
	Short: "Show current call stack",
	Run: func(cmd *cobra.Command, args []string) {
		connectAndRun(func() error {
			stack := v8Debugger.GetCallStack()

			if len(stack) == 0 {
				fmt.Println("No call stack available (not paused)")
				return nil
			}

			fmt.Printf("Call stack (%d frames):\n\n", len(stack))
			for i, frame := range stack {
				fmt.Printf("[%d] %s\n", i, frame.FunctionName)
				fmt.Printf("    URL: %s\n", frame.URL)
				if location, ok := frame.Location["lineNumber"].(float64); ok {
					fmt.Printf("    Line: %.0f\n", location)
				}
				fmt.Printf("    Frame ID: %s\n", frame.CallFrameID)
				fmt.Println()
			}

			return nil
		})
	},
}

// Profiling commands
var v8ProfileCmd = &cobra.Command{
	Use:   "profile",
	Short: "CPU profiling commands",
}

var v8ProfileStartCmd = &cobra.Command{
	Use:   "start [title] [interval]",
	Short: "Start CPU profiling",
	Long: `Start CPU profiling with optional title and sampling interval.
Interval is in microseconds (default: 1000μs = 1ms).`,
	Args: cobra.MaximumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		connectAndRun(func() error {
			// Enable profiler first
			if err := v8Profiler.EnableProfiler(); err != nil {
				return err
			}

			title := "profile-" + time.Now().Format("20060102-150405")
			if len(args) > 0 {
				title = args[0]
			}

			interval := 1000 // 1ms default
			if len(args) > 1 {
				if i, err := strconv.Atoi(args[1]); err == nil {
					interval = i
				}
			}

			if err := v8Profiler.StartCPUProfiling(title, interval); err != nil {
				return err
			}

			fmt.Printf("CPU profiling started: %s (interval: %dμs)\n", title, interval)
			fmt.Println("Use 'v8 profile stop' to stop profiling and get results")

			return nil
		})
	},
}

var v8ProfileStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop CPU profiling and show results",
	Run: func(cmd *cobra.Command, args []string) {
		connectAndRun(func() error {
			profile, err := v8Profiler.StopCPUProfiling()
			if err != nil {
				return err
			}

			// Display profile summary
			duration := (profile.EndTime - profile.StartTime) / 1000000 // Convert to seconds
			fmt.Printf("CPU Profile Results:\n")
			fmt.Printf("  Duration: %.2f seconds\n", duration)
			fmt.Printf("  Nodes: %d\n", len(profile.Nodes))
			fmt.Printf("  Samples: %d\n", len(profile.Samples))
			fmt.Println()

			// Show top functions by hit count
			if len(profile.Nodes) > 0 {
				fmt.Println("Top functions by hit count:")
				for i, node := range profile.Nodes {
					if i >= 10 { // Limit to top 10
						break
					}
					if node.HitCount > 0 {
						fmt.Printf("  %s (%s:%d) - %d hits\n",
							node.CallFrame.FunctionName,
							node.CallFrame.URL,
							node.CallFrame.LineNumber,
							node.HitCount)
					}
				}
			}

			// Optionally save to file
			filename := fmt.Sprintf("cpu-profile-%d.json", time.Now().Unix())
			if profileJSON, err := json.MarshalIndent(profile, "", "  "); err == nil {
				if err := os.WriteFile(filename, profileJSON, 0644); err == nil {
					fmt.Printf("\nProfile saved to: %s\n", filename)
				}
			}

			return nil
		})
	},
}

// Coverage commands
var v8CoverageCmd = &cobra.Command{
	Use:   "coverage",
	Short: "Code coverage commands",
}

var v8CoverageStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start code coverage collection",
	Run: func(cmd *cobra.Command, args []string) {
		connectAndRun(func() error {
			// Enable profiler first
			if err := v8Profiler.EnableProfiler(); err != nil {
				return err
			}

			if err := v8Profiler.StartPreciseCoverage(true, true); err != nil {
				return err
			}

			fmt.Println("Code coverage collection started")
			fmt.Println("Use 'v8 coverage take' to collect coverage data")

			return nil
		})
	},
}

var v8CoverageTakeCmd = &cobra.Command{
	Use:   "take",
	Short: "Collect current coverage data",
	Run: func(cmd *cobra.Command, args []string) {
		connectAndRun(func() error {
			coverage, err := v8Profiler.TakePreciseCoverage()
			if err != nil {
				return err
			}

			fmt.Printf("Coverage data for %d scripts:\n\n", len(coverage))

			for _, script := range coverage {
				if script.URL == "" {
					continue // Skip internal scripts
				}

				fmt.Printf("Script: %s\n", script.URL)
				fmt.Printf("  Functions: %d\n", len(script.Functions))

				totalRanges := 0
				executedRanges := 0
				for _, function := range script.Functions {
					totalRanges += len(function.Ranges)
					for _, rangeItem := range function.Ranges {
						if rangeItem.Count > 0 {
							executedRanges++
						}
					}
				}

				if totalRanges > 0 {
					coverage := float64(executedRanges) / float64(totalRanges) * 100
					fmt.Printf("  Coverage: %.1f%% (%d/%d ranges)\n", coverage, executedRanges, totalRanges)
				}
				fmt.Println()
			}

			return nil
		})
	},
}

var v8CoverageStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop code coverage collection",
	Run: func(cmd *cobra.Command, args []string) {
		connectAndRun(func() error {
			if err := v8Profiler.StopPreciseCoverage(); err != nil {
				return err
			}

			fmt.Println("Code coverage collection stopped")
			return nil
		})
	},
}

// Console/REPL command
var v8ConsoleCmd = &cobra.Command{
	Use:   "console",
	Short: "Interactive JavaScript console/REPL",
	Run: func(cmd *cobra.Command, args []string) {
		connectAndRun(func() error {
			return startInteractiveConsole()
		})
	},
}

// Helper functions

func connectAndRun(fn func() error) {
	if !v8Client.IsConnected() {
		// Try to find any existing session or use default port
		var port string
		var found bool

		// Check if there's a default session
		if defaultPort, err := GetDefaultSession(); err == nil {
			port = defaultPort
			found = true
		} else {
			// Try to find any session
			if sessions, err := ListSessionFiles(); err == nil && len(sessions) > 0 {
				port = sessions[0].Port
				found = true
			} else {
				port = "9229" // fallback
			}
		}

		ctx := createContext()
		if err := v8Client.ConnectByPort(ctx, port); err != nil {
			if found {
				log.Fatalf("Failed to connect to saved session on port %s: %v", port, err)
			} else {
				log.Fatalf("Not connected. Use 'v8 connect <port>' first. Error: %v", err)
			}
		}

		// Enable domains
		v8Runtime.EnableRuntime()
		v8Debugger.EnableDebugger()
	}

	if err := fn(); err != nil {
		log.Fatalf("Command failed: %v", err)
	}
}

func startInteractiveConsole() error {
	console := NewV8Console(v8Client, v8Runtime, v8Debugger, v8Profiler)
	console.SetupEventHandlers()
	return console.Start()
}

func init() {
	// Add all V8 commands to the tree
	v8Cmd.AddCommand(v8ConnectCmd)
	v8Cmd.AddCommand(v8TargetsCmd)
	v8Cmd.AddCommand(v8BreakCmd)
	v8Cmd.AddCommand(v8BreakListCmd)
	v8Cmd.AddCommand(v8BreakRemoveCmd)
	v8Cmd.AddCommand(v8ResumeCmd)
	v8Cmd.AddCommand(v8StepIntoCmd)
	v8Cmd.AddCommand(v8StepOverCmd)
	v8Cmd.AddCommand(v8StepOutCmd)
	v8Cmd.AddCommand(v8PauseCmd)
	v8Cmd.AddCommand(v8EvalCmd)
	v8Cmd.AddCommand(v8StackCmd)
	v8Cmd.AddCommand(v8ConsoleCmd)

	// Profile subcommands
	v8ProfileCmd.AddCommand(v8ProfileStartCmd)
	v8ProfileCmd.AddCommand(v8ProfileStopCmd)
	v8Cmd.AddCommand(v8ProfileCmd)

	// Coverage subcommands
	v8CoverageCmd.AddCommand(v8CoverageStartCmd)
	v8CoverageCmd.AddCommand(v8CoverageTakeCmd)
	v8CoverageCmd.AddCommand(v8CoverageStopCmd)
	v8Cmd.AddCommand(v8CoverageCmd)

	// Add to main root command
	rootCmd.AddCommand(v8Cmd)
}