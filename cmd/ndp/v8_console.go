package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// V8Console provides an interactive JavaScript console/REPL
type V8Console struct {
	client    *V8InspectorClient
	runtime   *V8Runtime
	debugger  *V8Debugger
	profiler  *V8Profiler
	scanner   *bufio.Scanner
	history   []string
	multiline string
}

// NewV8Console creates a new interactive console
func NewV8Console(client *V8InspectorClient, runtime *V8Runtime, debugger *V8Debugger, profiler *V8Profiler) *V8Console {
	return &V8Console{
		client:   client,
		runtime:  runtime,
		debugger: debugger,
		profiler: profiler,
		scanner:  bufio.NewScanner(os.Stdin),
		history:  make([]string, 0),
	}
}

// Start begins the interactive console session
func (c *V8Console) Start() error {
	c.printWelcome()

	for {
		prompt := "> "
		if c.multiline != "" {
			prompt = "... "
		}

		fmt.Print(prompt)

		if !c.scanner.Scan() {
			break
		}

		line := strings.TrimSpace(c.scanner.Text())

		// Handle empty lines
		if line == "" {
			if c.multiline != "" {
				// Execute multiline
				if err := c.executeJavaScript(c.multiline); err != nil {
					fmt.Printf("Error: %v\n", err)
				}
				c.multiline = ""
			}
			continue
		}

		// Handle special commands
		if strings.HasPrefix(line, ".") {
			if err := c.handleSpecialCommand(line); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
			continue
		}

		// Handle multiline input
		if c.isIncompleteExpression(line) {
			if c.multiline == "" {
				c.multiline = line
			} else {
				c.multiline += "\n" + line
			}
			continue
		}

		// Execute single line or complete multiline
		expression := line
		if c.multiline != "" {
			expression = c.multiline + "\n" + line
			c.multiline = ""
		}

		if err := c.executeJavaScript(expression); err != nil {
			fmt.Printf("Error: %v\n", err)
		}

		// Add to history
		c.history = append(c.history, expression)
	}

	fmt.Println("\nBye!")
	return nil
}

// printWelcome prints the welcome message
func (c *V8Console) printWelcome() {
	fmt.Println("V8 Inspector Console - Interactive JavaScript REPL")
	fmt.Println("Connected to Node.js process")
	fmt.Println()
	fmt.Println("Special commands:")
	fmt.Println("  .help              Show this help")
	fmt.Println("  .exit              Exit the console")
	fmt.Println("  .break             Abort multiline input")
	fmt.Println("  .clear             Clear the console")
	fmt.Println("  .history           Show command history")
	fmt.Println("  .stack             Show current call stack")
	fmt.Println("  .breakpoints       List active breakpoints")
	fmt.Println("  .break <location>  Set a breakpoint")
	fmt.Println("  .continue          Resume execution")
	fmt.Println("  .step              Step execution")
	fmt.Println("  .profile start     Start CPU profiling")
	fmt.Println("  .profile stop      Stop CPU profiling")
	fmt.Println("  .gc                Force garbage collection")
	fmt.Println()
	fmt.Println("Type JavaScript expressions to evaluate them.")
	fmt.Println("Use empty line to execute multiline expressions.")
	fmt.Println()
}

// handleSpecialCommand processes special console commands
func (c *V8Console) handleSpecialCommand(line string) error {
	parts := strings.Fields(line)
	command := parts[0]

	switch command {
	case ".help":
		c.printWelcome()

	case ".exit":
		os.Exit(0)

	case ".break":
		if len(parts) > 1 {
			// Set breakpoint
			location := parts[1]
			condition := ""
			if len(parts) > 2 {
				condition = strings.Join(parts[2:], " ")
			}

			bp, err := c.debugger.SetBreakpointByLocation(location, condition)
			if err != nil {
				return err
			}

			fmt.Printf("Breakpoint set: %s at %s\n", bp.ID, location)
			if condition != "" {
				fmt.Printf("  Condition: %s\n", condition)
			}
		} else {
			// Abort multiline
			c.multiline = ""
			fmt.Println("Multiline input aborted")
		}

	case ".clear":
		// Clear console (ANSI escape sequence)
		fmt.Print("\033[H\033[2J")

	case ".history":
		fmt.Println("Command history:")
		for i, cmd := range c.history {
			fmt.Printf("  %d: %s\n", i+1, cmd)
		}

	case ".stack":
		stack := c.debugger.GetCallStack()
		if len(stack) == 0 {
			fmt.Println("No call stack available (not paused)")
		} else {
			fmt.Printf("Call stack (%d frames):\n", len(stack))
			for i, frame := range stack {
				fmt.Printf("  [%d] %s (%s)\n", i, frame.FunctionName, frame.URL)
			}
		}

	case ".breakpoints":
		breakpoints := c.debugger.ListBreakpoints()
		if len(breakpoints) == 0 {
			fmt.Println("No active breakpoints")
		} else {
			fmt.Printf("Active breakpoints (%d):\n", len(breakpoints))
			for i, bp := range breakpoints {
				fmt.Printf("  [%d] %s at line %d\n", i+1, bp.ID, bp.LineNumber)
				if bp.Condition != "" {
					fmt.Printf("      Condition: %s\n", bp.Condition)
				}
			}
		}

	case ".continue":
		if err := c.debugger.Resume(); err != nil {
			return err
		}
		fmt.Println("Execution resumed")

	case ".step":
		if len(parts) > 1 {
			switch parts[1] {
			case "into":
				if err := c.debugger.StepInto(); err != nil {
					return err
				}
				fmt.Println("Stepping into...")
			case "over":
				if err := c.debugger.StepOver(); err != nil {
					return err
				}
				fmt.Println("Stepping over...")
			case "out":
				if err := c.debugger.StepOut(); err != nil {
					return err
				}
				fmt.Println("Stepping out...")
			default:
				fmt.Println("Usage: .step [into|over|out]")
			}
		} else {
			// Default to step over
			if err := c.debugger.StepOver(); err != nil {
				return err
			}
			fmt.Println("Stepping over...")
		}

	case ".profile":
		if len(parts) > 1 {
			switch parts[1] {
			case "start":
				if err := c.profiler.EnableProfiler(); err != nil {
					return err
				}
				if err := c.profiler.StartCPUProfiling("console-profile", 1000); err != nil {
					return err
				}
				fmt.Println("CPU profiling started")

			case "stop":
				profile, err := c.profiler.StopCPUProfiling()
				if err != nil {
					return err
				}

				duration := (profile.EndTime - profile.StartTime) / 1000000
				fmt.Printf("CPU profiling stopped (%.2fs, %d samples)\n", duration, len(profile.Samples))

				// Show top functions
				fmt.Println("Top functions by hit count:")
				count := 0
				for _, node := range profile.Nodes {
					if node.HitCount > 0 && count < 5 {
						fmt.Printf("  %s: %d hits\n", node.CallFrame.FunctionName, node.HitCount)
						count++
					}
				}

			default:
				fmt.Println("Usage: .profile [start|stop]")
			}
		} else {
			fmt.Println("Usage: .profile [start|stop]")
		}

	case ".gc":
		// Force garbage collection
		_, err := c.runtime.Evaluate("global.gc && global.gc()", &EvaluateOptions{
			Silent: true,
		})
		if err != nil {
			// Try alternative methods
			c.runtime.Evaluate("if (typeof gc === 'function') gc()", &EvaluateOptions{
				Silent: true,
			})
		}
		fmt.Println("Garbage collection requested")

	default:
		fmt.Printf("Unknown command: %s\nType .help for available commands\n", command)
	}

	return nil
}

// executeJavaScript evaluates JavaScript code and displays results
func (c *V8Console) executeJavaScript(expression string) error {
	options := &EvaluateOptions{
		ReturnByValue:         false, // Keep object references for inspection
		GeneratePreview:       true,
		IncludeCommandLineAPI: true,
		ReplMode:             true,
	}

	result, err := c.runtime.Evaluate(expression, options)
	if err != nil {
		return err
	}

	// Handle exceptions
	if result.Exception != nil {
		fmt.Printf("Uncaught %s\n", result.Exception.Text)
		if result.Exception.Exception != nil {
			fmt.Printf("%s\n", c.runtime.FormatValue(result.Exception.Exception))
		}
		if result.Exception.StackTrace != nil {
			c.printStackTrace(result.Exception.StackTrace)
		}
		return nil
	}

	// Display result
	if result.Result != nil {
		// Check if result is undefined (don't print)
		if result.Result.Type == "undefined" {
			return nil
		}

		value := c.runtime.FormatValue(result.Result)
		fmt.Printf("%s\n", value)

		// For objects, show expandable preview
		if result.Result.Type == "object" && result.Result.ObjectID != "" {
			c.showObjectPreview(result.Result)
		}
	}

	return nil
}

// showObjectPreview displays an expandable object preview
func (c *V8Console) showObjectPreview(obj *RemoteObject) {
	if obj.Preview != nil {
		if properties, ok := obj.Preview["properties"].([]interface{}); ok {
			if len(properties) > 0 {
				fmt.Print("  {")
				for i, prop := range properties {
					if i >= 3 { // Limit preview to first 3 properties
						fmt.Print(" ...")
						break
					}
					if propMap, ok := prop.(map[string]interface{}); ok {
						if name, ok := propMap["name"].(string); ok {
							if value, ok := propMap["value"].(string); ok {
								if i > 0 {
									fmt.Print(",")
								}
								fmt.Printf(" %s: %s", name, value)
							}
						}
					}
				}
				fmt.Println(" }")
			}
		}
	}
}

// printStackTrace prints a formatted stack trace
func (c *V8Console) printStackTrace(stackTrace map[string]interface{}) {
	if callFrames, ok := stackTrace["callFrames"].([]interface{}); ok {
		fmt.Println("    at:")
		for _, frame := range callFrames {
			if frameMap, ok := frame.(map[string]interface{}); ok {
				functionName := "anonymous"
				if name, ok := frameMap["functionName"].(string); ok && name != "" {
					functionName = name
				}

				url := ""
				if u, ok := frameMap["url"].(string); ok {
					url = u
				}

				lineNumber := 0
				if line, ok := frameMap["lineNumber"].(float64); ok {
					lineNumber = int(line)
				}

				fmt.Printf("        %s (%s:%d)\n", functionName, url, lineNumber)
			}
		}
	}
}

// isIncompleteExpression checks if the expression might need more input
func (c *V8Console) isIncompleteExpression(line string) bool {
	// Simple heuristics for detecting incomplete expressions
	trimmed := strings.TrimSpace(line)

	// Check for unclosed brackets/braces/parentheses
	openBraces := strings.Count(trimmed, "{") - strings.Count(trimmed, "}")
	openBrackets := strings.Count(trimmed, "[") - strings.Count(trimmed, "]")
	openParens := strings.Count(trimmed, "(") - strings.Count(trimmed, ")")

	if openBraces > 0 || openBrackets > 0 || openParens > 0 {
		return true
	}

	// Check for trailing operators/keywords that suggest continuation
	if strings.HasSuffix(trimmed, ",") ||
		strings.HasSuffix(trimmed, ".") ||
		strings.HasSuffix(trimmed, "&&") ||
		strings.HasSuffix(trimmed, "||") ||
		strings.HasSuffix(trimmed, "+") ||
		strings.HasSuffix(trimmed, "-") ||
		strings.HasSuffix(trimmed, "*") ||
		strings.HasSuffix(trimmed, "/") ||
		strings.HasSuffix(trimmed, "=") ||
		strings.HasSuffix(trimmed, "?") ||
		strings.HasSuffix(trimmed, ":") {
		return true
	}

	// Check for keywords that suggest more content
	if strings.HasPrefix(trimmed, "function") ||
		strings.HasPrefix(trimmed, "if") ||
		strings.HasPrefix(trimmed, "for") ||
		strings.HasPrefix(trimmed, "while") ||
		strings.HasPrefix(trimmed, "try") ||
		strings.HasPrefix(trimmed, "class") {
		// If it doesn't end with } or ;, it's probably incomplete
		if !strings.HasSuffix(trimmed, "}") && !strings.HasSuffix(trimmed, ";") {
			return true
		}
	}

	return false
}

// Enhanced console with event handling
func (c *V8Console) SetupEventHandlers() {
	// Handle debugger events
	c.client.OnEvent("Debugger.paused", func(params map[string]interface{}) {
		reason := "unknown"
		if r, ok := params["reason"].(string); ok {
			reason = r
		}

		fmt.Printf("\n[Debugger] Paused: %s\n", reason)

		// Show current location
		if callFrames, ok := params["callFrames"].([]interface{}); ok && len(callFrames) > 0 {
			if frame, ok := callFrames[0].(map[string]interface{}); ok {
				if location, ok := frame["location"].(map[string]interface{}); ok {
					if lineNumber, ok := location["lineNumber"].(float64); ok {
						if functionName, ok := frame["functionName"].(string); ok {
							fmt.Printf("[Debugger] At %s (line %.0f)\n", functionName, lineNumber+1)
						}
					}
				}
			}
		}

		fmt.Print("> ")
	})

	c.client.OnEvent("Debugger.resumed", func(params map[string]interface{}) {
		fmt.Printf("\n[Debugger] Resumed execution\n")
		fmt.Print("> ")
	})

	c.client.OnEvent("Runtime.consoleAPICalled", func(params map[string]interface{}) {
		// Handle console.log, console.error, etc.
		if args, ok := params["args"].([]interface{}); ok && len(args) > 0 {
			fmt.Print("\n[Console] ")
			for i, arg := range args {
				if i > 0 {
					fmt.Print(" ")
				}
				if argMap, ok := arg.(map[string]interface{}); ok {
					if value, ok := argMap["value"]; ok {
						fmt.Printf("%v", value)
					} else if description, ok := argMap["description"].(string); ok {
						fmt.Print(description)
					}
				}
			}
			fmt.Println()
			fmt.Print("> ")
		}
	})

	c.client.OnEvent("Runtime.exceptionThrown", func(params map[string]interface{}) {
		if exceptionDetails, ok := params["exceptionDetails"].(map[string]interface{}); ok {
			if text, ok := exceptionDetails["text"].(string); ok {
				fmt.Printf("\n[Exception] %s\n", text)
				fmt.Print("> ")
			}
		}
	})
}