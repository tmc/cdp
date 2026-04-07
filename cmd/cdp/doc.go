/*
cdp: Interactive Chrome DevTools Protocol Command-Line Tool

cdp connects to or launches a Chrome/Brave browser and provides an interactive
command-line prompt (REPL) for executing Chrome DevTools Protocol (CDP) commands,
capturing network traffic as HAR/HARL, and managing browser sessions.

Usage:

	cdp [flags]

Connection Flags:

	-remote-host <host>
	    Connect to remote Chrome at this host.
	-remote-port <port>
	    Remote Chrome debugging port. (default: 9222)
	-remote-tab <id|url>
	    Connect to a specific tab by ID or URL.
	-tab <id>
	    Target a specific tab ID.
	-list-tabs
	    List available tabs on remote Chrome and exit.
	-list-browsers
	    List all discovered browsers (running or installed) and exit.
	-connect-existing
	    Prefer connecting to existing Chrome sessions.

Launch Flags:

	-chrome-path <path>
	    Explicit path to the Chrome/Brave executable.
	-headless
	    Run Chrome in headless mode (without UI).
	-debug-port <port>
	    Chrome debugging port. (default: 9222)
	-url <url>
	    Navigate to this URL on start. (default: "about:blank")
	-window-position <x,y>
	    Set window position (e.g., "100,100").
	-window-size <w,h>
	    Set window size (e.g., "1920,1080").
	-new-window
	    Force open in new window (vs reusing existing).
	-chrome-flags <flags>
	    Additional Chrome flags (space-separated).
	-show-chrome-flags
	    Print the Chrome command-line flags used at launch.

Session & Profile Flags:

	-full-capture
	    Interactive mode with full request/response body capture.
	-use-profile <name>
	    Use Chrome profile with cookies and session data.
	-profile-dir <dir>
	    Custom profile directory (overrides default locations).
	-cookie-domains <domains>
	    Comma-separated list of domains to include cookies from
	    (requires sqlite3 in PATH).
	-list-profiles
	    List available Chrome profiles and exit.

Capture Flags:

	-har <file>
	    Save HAR file to this path.
	-har-mode <mode>
	    HAR capture mode: enhanced (complete headers/bodies) or simple. (default: "enhanced")
	-harl
	    Stream HAR entries as NDJSON.
	-harl-file <file>
	    File to stream NDJSON to (use '-' for stdout). (default: "output.har.jsonl")
	-output-dir <dir>
	    Directory to write domain-organized logs to (overrides --harl-file).
	-monitor-all-tabs
	    Monitor network traffic from all browser tabs.

Execution Flags:

	-shell
	    Start in interactive shell mode (auto if no --url or --js).
	-interactive
	    Keep browser open for interaction.
	-command <cmd>
	    Execute a single CDP command and exit.
	-js <script>
	    Execute JavaScript and exit (repeatable via -js flag).
	-wait-ready
	    Wait for page load and network idle before executing -js scripts.
	-await
	    Await Promise return values from -js scripts.
	-timeout <seconds>
	    Max seconds to wait for commands. 0 for no timeout. (default: 60)
	-screenshot <selector>
	    Take a screenshot and exit (CSS selector or 'full' for full page).

Display Flags:

	-verbose
	    Enable verbose logging.
	-console
	    Monitor and display browser console messages.
	-console-stacks
	    Show full stack traces for console errors.
	-format <format>
	    Output format: text or json. (default: "text")

Interactive Shell Commands:

Once connected, cdp presents a "cdp> " prompt. Commands fall into several categories.

CDP Commands (Domain.method format):

	Page.navigate {"url":"https://example.com"}
	Runtime.evaluate {"expression":"document.title"}
	DOM.getDocument {}
	Network.getAllCookies {}

Navigation Aliases:

	goto <url>          Navigate to URL
	reload              Reload the current page
	title               Get page title
	url                 Get current URL
	html                Get page HTML

Tab Management:

	tabs / lt           List open browser tabs
	newtab / nt [url]   Open a new tab (default: about:blank)
	tab / t <n|text>    Switch to tab by index or title/URL substring

Output Context:

	context             Show current output directory and context stack
	push-context <name> Push a named context (writes HAR to subdirectory)
	pop-context         Pop the current context

Screenshots:

	screenshot                    Full-page screenshot saved to file
	screenshot <selector>         Element screenshot (CSS selector)
	screenshot <selector> <file>  Save element screenshot to specific file
	screenshot --json             Full-page screenshot as base64 JSON

Source Extraction:

	sources                       List all JavaScript and CSS sources
	sources --save <dir>          Save all inline sources to directory
	sources --type js|css|inline  Filter by source type
	sources --get <url>           Display specific source content

File Commands:

	jsfile <path>       Execute JavaScript from a file

Annotation Commands (--har-mode=enhanced only):

	note <text>         Add a text annotation to HAR file
	dom <desc>          Capture DOM snapshot

Console Monitoring:

	console             Enable console monitoring

Session Commands:

	reconnect / rc      Reconnect to browser if connection lost
	refresh-profile / rp Re-copy user profile and reconnect
	hup                 Detach from browser (leave it running)
	help                Show help
	help aliases        List all alias commands
	exit / quit         Exit the program

Exit Codes:

	0 - Success
	1 - General error
	2 - Command line usage error
	3 - Browser launch/connection failed
	4 - Page navigation failed
	5 - Operation timed out
*/
package main
