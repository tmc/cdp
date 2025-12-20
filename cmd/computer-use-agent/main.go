// computer-use-agent: AI-powered browser automation using Google Gemini.
//
// This tool uses Google's Gemini AI model to control a Chrome browser via natural
// language commands. It implements the computer use capabilities of Gemini to
// automate web interactions.
//
// Usage:
//
//	computer-use-agent --query "Search for the latest Go releases and summarize them"
//
// Environment Variables:
//
//	GEMINI_API_KEY    - Google Gemini API key (required)
//	CHROME_PATH       - Path to Chrome executable (optional)
//
// Flags:
//
//	--query string           Natural language task description (required unless --shell)
//	--model string           Gemini model name (default: "gemini-2.0-flash-exp")
//	--url string             Starting URL (default: "https://www.google.com")
//	--headless              Run browser in headless mode
//	--verbose               Enable verbose logging
//	--timeout int           Timeout in seconds (default: 300)
//	--chrome-path string    Path to Chrome/Brave executable
//	--use-profile string    Chrome/Brave profile to use (e.g., "Default", "Profile 1")
//	--profile-dir string    Custom profile directory (overrides default locations)
//	--list-profiles         List available Chrome/Brave profiles and exit
//	--format string         Output format for --list-profiles: text or json (default: "text")
//	--har string            Output HAR file path (optional)
//	--shell                 Interactive shell mode - allows multi-turn conversations
//	--interactive           Keep browser open for interaction (alias for --shell)
//	--devtools              Automatically open Chrome DevTools in browser window
//	--debug-port int        Chrome DevTools debugging port (0 = auto-assign)
//	--use-os-screenshots    Use macOS screencapture for full window capture (includes DevTools)
//
// Examples:
//
//	# List available profiles
//	computer-use-agent --list-profiles
//
//	# Use a specific profile
//	computer-use-agent --use-profile "Default" --query "Navigate to github.com"
//
//	# Search and summarize
//	computer-use-agent --query "Search Google for 'Golang 1.22 features' and summarize the top result"
//
//	# Fill a form
//	computer-use-agent --query "Go to example.com/contact and fill in the contact form" --url "https://example.com"
//
//	# Research task
//	computer-use-agent --query "Find the current price of Bitcoin and Ethereum" --verbose
//
//	# Interactive shell mode
//	computer-use-agent --shell --use-profile "Default"
//
//	# Save HAR file
//	computer-use-agent --query "Navigate to example.com" --har "output.har"
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/tmc/macgo"
	"github.com/tmc/misc/chrome-to-har/internal/browser"
	"github.com/tmc/misc/chrome-to-har/internal/chromeprofiles"
	"github.com/tmc/misc/chrome-to-har/internal/discovery"
)

var (
	query           = flag.String("query", "", "Natural language task description (required unless --shell is used)")
	model           = flag.String("model", "gemini-2.5-computer-use-preview-10-2025", "Gemini model name (use gemini-2.5-computer-use-preview-10-2025 for computer use)")
	url             = flag.String("url", "https://www.google.com", "Starting URL")
	headless        = flag.Bool("headless", false, "Run browser in headless mode")
	verbose         = flag.Bool("verbose", false, "Enable verbose logging")
	timeout         = flag.Int("timeout", 300, "Timeout in seconds")
	chromePath      = flag.String("chrome-path", "", "Path to Chrome/Brave executable")
	useProfile      = flag.String("use-profile", "", "Chrome/Brave profile name to use (e.g., 'Default', 'Profile 1')")
	profileDir      = flag.String("profile-dir", "", "Custom profile directory (overrides default locations)")
	listProfiles    = flag.Bool("list-profiles", false, "List available Chrome/Brave profiles and exit")
	outputFormat    = flag.String("format", "text", "Output format for --list-profiles: text or json")
	har             = flag.String("har", "", "Output HAR file path (optional)")
	shell           = flag.Bool("shell", false, "Interactive shell mode - allows multi-turn conversations")
	interactive     = flag.Bool("interactive", false, "Keep browser open for interaction (alias for --shell)")
	debugPort       = flag.Int("debug-port", 0, "Chrome DevTools debugging port (0 = auto-assign, default: 0)")
	devtools        = flag.Bool("devtools", false, "Automatically open Chrome DevTools in browser window")
	useOSScreenshot = flag.Bool("use-os-screenshots", false, "Use macOS screencapture for full window capture (includes DevTools)")
	workDir         = flag.String("work-dir", "", "Work directory for logs and screenshots (default: temp dir)")
	keepWorkDir     = flag.Bool("keep-work-dir", false, "Keep work directory after completion (default: cleanup on exit)")
	openWorkDir     = flag.Bool("open-work-dir", false, "Open work directory in Finder (macOS) or file explorer")
	// VM mode flags
	vmMode   = flag.Bool("vm", false, "Use VM control instead of browser (requires --vm-socket)")
	vmSocket = flag.String("vm-socket", "", "Path to VM control socket (e.g., ./vm/control.sock)")
)

func main() {
	flag.Parse()

	// Initialize macgo if using OS screenshots on macOS
	if *useOSScreenshot && runtime.GOOS == "darwin" {
		// Enable I/O forwarding for macgo
		// if os.Getenv("MACGO_SERVICES_VERSION") == "" {
		// 	os.Setenv("MACGO_SERVICES_VERSION", "2")
		// }
		// if os.Getenv("MACGO_ENABLE_STDOUT_FORWARDING") == "" {
		// 	os.Setenv("MACGO_ENABLE_STDOUT_FORWARDING", "1")
		// }
		// if os.Getenv("MACGO_ENABLE_STDERR_FORWARDING") == "" {
		// 	os.Setenv("MACGO_ENABLE_STDERR_FORWARDING", "1")
		// }
		// // Use direct execution for better I/O handling (can be overridden with MACGO_FORCE_DIRECT=0)
		// if os.Getenv("MACGO_FORCE_DIRECT") == "" {
		// 	os.Setenv("MACGO_FORCE_DIRECT", "1")
		// }

		cfg := &macgo.Config{
			Permissions: []macgo.Permission{},
			Custom:      []string{
				// "com.apple.security.automation.apple-events", // For AppleScript/System Events
				// "com.apple.security.device.screen-capture",   // For screen-capture tool
			},
			Debug: *verbose,
		}
		if err := macgo.Start(cfg); err != nil {
			log.Fatalf("Failed to initialize macgo: %v\n", err)
		}
	}

	if err := run(); err != nil {
		log.Fatalf("Error: %v\n", err)
	}
}

func run() error {
	// Handle profile listing (doesn't require query or API key)
	if *listProfiles {
		pmOpts := []chromeprofiles.Option{
			chromeprofiles.WithVerbose(*verbose),
		}

		// Use custom profile directory if specified
		if *profileDir != "" {
			pmOpts = append(pmOpts, chromeprofiles.WithProfileDir(*profileDir))
		}

		pm, err := chromeprofiles.NewProfileManager(pmOpts...)
		if err != nil {
			return fmt.Errorf("failed to create profile manager: %w", err)
		}

		profiles, err := pm.ListProfiles()
		if err != nil {
			return fmt.Errorf("failed to list profiles: %w", err)
		}

		// Handle different output formats
		if *outputFormat == "json" {
			jsonData, err := json.MarshalIndent(profiles, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal profiles to JSON: %w", err)
			}
			fmt.Println(string(jsonData))
		} else {
			// Default text format
			fmt.Println("Available Chrome/Brave profiles:")
			fmt.Println("==================================")
			for i, profile := range profiles {
				fmt.Printf("[%d] %s\n", i+1, profile)
			}

			if len(profiles) == 0 {
				fmt.Println("No Chrome/Brave profiles found.")
				fmt.Println("Suggestion: Create a profile first by opening Chrome/Brave and going to Settings > People")
			} else {
				fmt.Printf("\nUse with: computer-use-agent --use-profile \"%s\" --query \"your task\"\n", profiles[0])
			}
		}
		return nil
	}

	// Handle interactive flag (alias for shell)
	if *interactive {
		*shell = true
	}

	// Validate flags
	if *query == "" && !*shell {
		return fmt.Errorf("--query is required (unless --shell mode is used)")
	}

	// Get API key
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("GEMINI_API_KEY environment variable is required")
	}

	// Setup work directory
	var workDirPath string
	if *workDir != "" {
		workDirPath = *workDir
	} else {
		// Create temp work directory
		tmpDir, err := os.MkdirTemp("", "computer-use-agent-*")
		if err != nil {
			return fmt.Errorf("creating work directory: %w", err)
		}
		workDirPath = tmpDir
	}

	// Create work directory if it doesn't exist
	if err := os.MkdirAll(workDirPath, 0755); err != nil {
		return fmt.Errorf("creating work directory: %w", err)
	}

	// Cleanup work directory on exit unless --keep-work-dir is set
	if !*keepWorkDir {
		defer func() {
			if *verbose {
				log.Printf("Cleaning up work directory: %s\n", workDirPath)
			}
			os.RemoveAll(workDirPath)
		}()
	} else {
		fmt.Printf("Work directory: %s\n", workDirPath)
	}

	// Setup logging to file
	logFile := filepath.Join(workDirPath, "agent.log")
	logF, err := os.Create(logFile)
	if err != nil {
		return fmt.Errorf("creating log file: %w", err)
	}
	defer logF.Close()

	// Set log output to both file and stderr
	log.SetOutput(io.MultiWriter(os.Stderr, logF))

	if *verbose {
		log.Printf("Work directory: %s\n", workDirPath)
		log.Printf("Log file: %s\n", logFile)
	}

	// Open work directory in Finder/file explorer if requested
	if *openWorkDir {
		if err := openFileExplorer(workDirPath); err != nil {
			if *verbose {
				log.Printf("Warning: failed to open work directory: %v\n", err)
			}
		}
	}

	// Setup context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
	defer cancel()

	// Handle interrupt signals - allow graceful shutdown on first signal, force on second
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Track if we've already received one signal
	var shutdownInitiated bool
	go func() {
		for sig := range sigChan {
			if shutdownInitiated {
				// Second signal - force exit
				fmt.Fprintf(os.Stderr, "\nReceived second interrupt signal (%v), forcing exit...\n", sig)
				os.Exit(1)
			}
			// First signal - graceful shutdown
			shutdownInitiated = true
			fmt.Fprintf(os.Stderr, "\nReceived interrupt signal (%v), shutting down gracefully... (press Ctrl-C again to force)\n", sig)
			cancel()
		}
	}()

	// Configure browser options
	browserOpts := []browser.Option{
		browser.WithVerbose(*verbose),
		browser.WithHeadless(*headless), // Default to non-headless (visible browser)
	}

	// Determine browser path
	var browserPath string
	if *chromePath != "" {
		browserPath = *chromePath
	} else if chromeEnv := os.Getenv("CHROME_PATH"); chromeEnv != "" {
		browserPath = chromeEnv
	} else {
		// Auto-discover browser (Brave first, then Chrome)
		browserPath = discovery.FindBestBrowser()
		if browserPath == "" {
			return fmt.Errorf("no Chrome/Brave browser found - please install one or use --chrome-path flag")
		}
		if *verbose {
			log.Printf("Auto-detected browser: %s", browserPath)
		}
	}

	browserOpts = append(browserOpts, browser.WithChromePath(browserPath))

	// Use existing profile if specified
	if *useProfile != "" {
		browserOpts = append(browserOpts, browser.WithProfile(*useProfile))
	}

	// Set viewport size and timeouts
	browserOpts = append(browserOpts,
		browser.WithTimeout(60),
		browser.WithWaitNetworkIdle(true),
	)

	// Set debug port if specified
	if *debugPort > 0 {
		browserOpts = append(browserOpts, browser.WithDebugPort(*debugPort))
		if *verbose {
			log.Printf("Chrome DevTools will be available on port %d", *debugPort)
		}
	}

	// Auto-open DevTools if requested
	if *devtools {
		// Note: DevTools opens docked by default. To undock, user can press Ctrl+Shift+D
		// or click the three dots menu > Dock side > Undock into separate window
		browserOpts = append(browserOpts, browser.WithChromeFlags([]string{
			"auto-open-devtools-for-tabs",
		}))
		if *verbose {
			log.Printf("DevTools will automatically open in browser window")
			log.Printf("Note: DevTools opens docked. To undock to separate window, press Ctrl+Shift+D in the browser")
		}
		fmt.Println("\n💡 Tip: DevTools will open docked. Press Ctrl+Shift+D to undock it to a separate window for better visibility.")
	}

	if *verbose {
		log.Printf("Initializing browser...")
	}

	// Create profile manager if profile is specified
	var profileMgr chromeprofiles.ProfileManager
	if *useProfile != "" {
		pmOpts := []chromeprofiles.Option{
			chromeprofiles.WithVerbose(*verbose),
		}
		if *profileDir != "" {
			pmOpts = append(pmOpts, chromeprofiles.WithProfileDir(*profileDir))
		}

		pm, err := chromeprofiles.NewProfileManager(pmOpts...)
		if err != nil {
			return fmt.Errorf("creating profile manager: %w", err)
		}
		profileMgr = pm

		if *verbose {
			log.Printf("Profile manager created for profile '%s'", *useProfile)
		}
	}

	// Create computer (browser or VM)
	var computer Computer
	if *vmMode {
		// VM mode
		if *vmSocket == "" {
			return fmt.Errorf("--vm-socket is required when using --vm mode")
		}
		if *verbose {
			log.Printf("Creating VM computer with socket: %s", *vmSocket)
		}
		vzComputer, err := NewVZComputer(*vmSocket, *verbose, workDirPath)
		if err != nil {
			return fmt.Errorf("creating VM computer: %w", err)
		}
		computer = vzComputer
		if *verbose {
			log.Printf("VM computer initialized successfully")
		}
	} else {
		// Browser mode
		browserComputer, err := NewBrowserComputer(ctx, profileMgr, *har, *useOSScreenshot, *verbose, workDirPath, browserOpts...)
		if err != nil {
			return fmt.Errorf("creating browser computer: %w", err)
		}
		computer = browserComputer
		if *verbose {
			log.Printf("Browser initialized successfully")
		}
	}
	defer computer.Close()

	// Print DevTools URL if debug port is set (browser mode only)
	if *debugPort > 0 {
		devtoolsURL := fmt.Sprintf("http://localhost:%d", *debugPort)
		fmt.Printf("\n🔧 Chrome DevTools available at: %s\n", devtoolsURL)
		fmt.Printf("   Open this URL in Chrome to inspect the browser session\n\n")
	}

	// Navigate to initial URL
	if *verbose {
		log.Printf("Navigating to %s...", *url)
	}
	if _, err := computer.OpenWebBrowser(*url); err != nil {
		return fmt.Errorf("opening browser: %w", err)
	}

	if *verbose {
		log.Printf("Creating agent with model %s...", *model)
	}

	// Create agent
	initialQuery := *query
	if *shell && initialQuery == "" {
		// In shell mode with no initial query, use empty string
		// The shell will wait for user input
		initialQuery = ""
	}

	agent, err := NewBrowserAgent(ctx, computer, apiKey, *model, initialQuery, *verbose, *vmMode)
	if err != nil {
		return fmt.Errorf("creating agent: %w", err)
	}

	// Shell mode: interactive conversation
	if *shell {
		// Ensure agent client is closed on exit
		defer agent.Close()
		return runShellMode(ctx, agent)
	}

	// Single query mode (Run() handles client.Close via defer)
	fmt.Printf("\n🤖 Starting agent with query: %s\n\n", *query)

	if err := agent.Run(ctx); err != nil {
		return fmt.Errorf("running agent: %w", err)
	}

	fmt.Println("\n✅ Task completed successfully!")
	return nil
}

func runShellMode(ctx context.Context, agent *BrowserAgent) error {
	// Check if running through macgo bundle which causes terminal job control issues
	if os.Getenv("MACGO_IN_BUNDLE") != "" {
		return fmt.Errorf("shell mode is incompatible with --use-os-screenshots\n" +
			"Reason: macgo creates a subprocess that cannot read from stdin due to Unix job control\n" +
			"Solutions:\n" +
			"  1. Use shell mode WITHOUT --use-os-screenshots: ./computer-use-agent --shell\n" +
			"  2. Use single query mode WITH --use-os-screenshots: ./computer-use-agent --use-os-screenshots --query \"your command\"")
	}

	fmt.Println("\n🤖 Interactive Shell Mode")
	fmt.Println("=" + strings.Repeat("=", 50))
	fmt.Println("Type your commands in natural language.")
	fmt.Println("Type 'exit' or 'quit' to end the session.")
	fmt.Println("Press Ctrl+C to abort current task.")
	fmt.Println(strings.Repeat("=", 51) + "\n")

	scanner := bufio.NewScanner(os.Stdin)

	for {
		// Check if context is cancelled before prompting
		select {
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "\n\n👋 Shutting down...\n")
			return ctx.Err()
		default:
		}

		fmt.Print("\n> ")

		// Read input synchronously - scanner.Scan() blocks until input is available
		if !scanner.Scan() {
			// Check for error or EOF
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("reading input: %w", err)
			}
			// EOF
			return nil
		}

		input := strings.TrimSpace(scanner.Text())

		if input == "" {
			continue
		}

		// Check for exit commands
		if input == "exit" || input == "quit" {
			fmt.Println("\n👋 Goodbye!")
			return nil
		}

		// Process the query
		fmt.Printf("\n🤖 Processing: %s\n\n", input)

		if err := agent.ProcessQuery(ctx, input); err != nil {
			// Check if error is due to context cancellation
			if ctx.Err() != nil {
				fmt.Fprintf(os.Stderr, "\n\n👋 Shutting down...\n")
				return ctx.Err()
			}
			fmt.Fprintf(os.Stderr, "\n❌ Error: %v\n", err)
			// Continue in shell mode even if there's an error
			continue
		}

		fmt.Println("\n✅ Done")
	}
}

// openFileExplorer opens a directory in the system file explorer
func openFileExplorer(path string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	case "windows":
		cmd = exec.Command("explorer", path)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return cmd.Run()
}
