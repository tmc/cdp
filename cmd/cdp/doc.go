/*
Cdp drives a Chrome or Brave browser over the Chrome DevTools Protocol.

Cdp launches a browser or attaches to a running one, then either runs a single
operation and exits, serves the browser to an agent over MCP, or presents an
interactive prompt for issuing CDP commands. Along the way it can capture
network traffic as HAR or streaming NDJSON, extract page content, and record
screenshots, PDFs, and page sources.

Usage:

	cdp [flags]
	cdp attach [-host host] [-port port] [-format text|json]
	cdp run [flags] script.txtar

With no -url and no -js, cdp starts in interactive shell mode.

# Testing

Tests that launch a browser run serially to avoid competing for browser
profiles and DevTools ports. Run the package's browser tests with:

	go test -p 1 ./cmd/cdp

# Connection

By default cdp discovers a running browser and prefers it, falling back to
launching one. Use -remote-host to attach to a browser elsewhere.

	-auto-discover
	    Automatically discover and prefer running browsers. (default true)
	-connect-existing
	    Prefer connecting to existing Chrome sessions.
	-remote-host host
	    Connect to remote Chrome at this host.
	-remote-port port
	    Remote Chrome debugging port. (default 9222)
	-remote-tab id|url
	    Connect to a specific tab by ID or URL.
	-tab id
	    Target a specific tab ID.
	-debug-port port
	    Chrome debugging port, 0 to choose automatically. (default 9222)
	-list-tabs
	    List available tabs on remote Chrome and exit.
	-list-browsers
	    List all discovered browsers, running or installed, and exit.

The attach subcommand probes common local DevTools ports and prints attachable
page targets, or the exact launch command to run when nothing is listening:

	cdp attach
	cdp attach -port 9222 -format json

# Launch

	-chrome-path path
	    Explicit path to the Chrome or Brave executable.
	-chrome-flags flags
	    Additional Chrome flags, space-separated.
	-show-chrome-flags
	    Print the Chrome command line used at launch.
	-headless
	    Run without a browser window.
	-background
	    Launch the browser without focusing its window.
	-new-window
	    Force a new window rather than reusing an existing one.
	-window-position x,y
	    Set window position, for example "100,100".
	-window-size w,h
	    Set window size, for example "1920,1080".
	-url url
	    Navigate here on start. (default "about:blank")
	-proxy url
	    Route browser traffic through this proxy.
	-load-extension paths
	    Comma-separated paths to unpacked extensions to load at start.
	-macos-permissions
	    On macOS, relaunch through an app bundle so the browser can request
	    camera and microphone permissions.

# Profiles and sessions

	-use-profile name
	    Copy the named Chrome profile, with its cookies and session, into a
	    temporary directory and launch against the copy. This does not attach
	    to a running browser using that profile; use -remote-host for that.
	-profile-dir dir
	    Custom profile directory, overriding the default locations.
	-cookie-domains domains
	    Comma-separated domains to include cookies from. Requires sqlite3 in
	    PATH.
	-list-profiles
	    List available Chrome profiles and exit.
	-keep-open
	    Leave a browser launched by cdp running after cdp exits. Browsers cdp
	    attached to are always left running.
	-no-quit
	    Alias for -keep-open.

# Capture

	-har file
	    Write a HAR file to this path.
	-har-mode mode
	    Either enhanced, which records complete headers, bodies, and POST
	    data, or simple, which is faster and records less. (default "enhanced")
	-harl
	    Stream HAR entries as NDJSON as they arrive.
	-harl-file file
	    Where to stream NDJSON, or "-" for stdout. (default "output.har.jsonl")
	-output-dir dir
	    Write domain-organized logs to this directory, overriding -harl-file.
	-group-by-page
	    Group capture output by the navigated page's domain rather than by
	    the domain each request went to. (default true)
	-full-capture
	    Capture complete request and response bodies in interactive mode.
	-max-body-bytes n
	    Maximum response body bytes to keep, 0 to keep whole bodies.
	-monitor-all-tabs
	    Capture network traffic from every tab, not just the current one.
	-monitor-url-pattern regexp
	    Only monitor URLs matching this pattern.
	-webrtc-capture streams
	    WebRTC streams to record under -full-capture: a comma-separated list
	    of sdp, datachannel, and ice, or all, or none.
	    (default "sdp,datachannel")
	-save-sources
	    Write all JavaScript and CSS sources, including originals recovered
	    from source maps, to disk.
	-no-scrub
	    Disable redaction of secrets in HAR and source output. Captures
	    routinely contain credentials; leave redaction on unless you have a
	    reason not to.

# Extraction and output

	-extract selector
	    Extract content matching a CSS selector and exit.
	-extract-mode mode
	    What -extract returns: text, html, attr:name, or count.
	    (default "text")
	-render [selector]
	    Render the page as Markdown. Pass a selector to limit it, or "body"
	    for the whole page.
	-screenshot spec
	    Take a screenshot and exit. The spec is "full", a selector, or either
	    of those followed by a destination path.
	-format format
	    Output and error format: text, json, or tsv. (default "text")

# Execution

	-js script
	    Evaluate JavaScript and exit. Repeat the flag to run several scripts
	    in order.
	-await
	    Await promises returned by -js scripts.
	-wait-ready
	    Wait for page load and network idle before running -js scripts.
	-command cmd
	    Execute a single CDP command and exit.
	-shell
	    Start in interactive shell mode. This is the default when neither
	    -url nor -js is given.
	-interactive
	    Keep the browser open for interaction.
	-wait policy
	    When interactive navigation is considered complete: domcontentloaded,
	    load, or networkidle. (default "domcontentloaded")
	-wait-for-url-change
	    Wait for the URL to change, then print the new URL.
	-timeout seconds
	    Maximum time for commands, 0 for no limit. (default 60)
	-navigation-timeout seconds
	    Maximum time for interactive navigation, 0 for no limit. (default 30)
	-H header
	    Set a custom HTTP header, for example
	    -H 'Authorization: Bearer token'. Repeatable.
	-headers header
	    Long form of -H.

# MCP server

Cdp can serve the browser to an agent over the Model Context Protocol, reading
and writing MCP messages on stdin and stdout.

	-mcp
	    Run as an MCP server over stdio.
	-tools-dir dir
	    Directory of .cdp tool definitions to expose over MCP and in the
	    shell.
	-enable-inspect
	    Expose the inspection and reversing tools over MCP:
	    inspect_fingerprint, inspect_walk, and inspect_ipc_*.
	-api-port port
	    Port for the coverage API server used by the DevTools extension,
	    0 to disable.

# CDP proxy

The proxy sits between a client and the browser so that the DevTools traffic
itself can be observed, which is useful when debugging automation rather than
the page.

	-cdp-proxy
	    Enable the CDP proxy, with an observer UI at
	    http://localhost:<debug-port>/_/.
	-cdp-proxy-observe-self
	    Route cdp's own CDP traffic through the proxy, so the observer also
	    shows the browser this cdp process drives. Requires -cdp-proxy.
	-cdp-proxy-self
	    Alias for -cdp-proxy-observe-self.

# Diagnostics

	-verbose
	    Enable verbose logging.
	-quiet
	    Suppress interactive startup progress.
	-console
	    Monitor and display browser console messages, including errors,
	    warnings, and uncaught exceptions.
	-console-stacks
	    Show full stack traces for console errors rather than one-line
	    summaries.
	-pprof-listen address
	    Serve net/http/pprof on this address, for example localhost:6060.

# Interactive shell

The shell presents a "cdp> " prompt. Any CDP method can be called directly by
its Domain.method name followed by JSON parameters:

	Page.navigate {"url":"https://example.com"}
	Runtime.evaluate {"expression":"document.title"}
	Network.getAllCookies {}

Shorter commands cover the common operations. Aliases follow in parentheses.

Navigation:

	navigate <url> (goto, go, nav)   Navigate to a URL
	reload [hard] (refresh, r)       Reload the page
	back (b)                         Go back in history
	forward (f, fwd)                 Go forward in history
	stop (s)                         Stop loading

DOM:

	click <selector> (c)             Click an element
	type <selector> <text> (input, fill)  Type into an element
	clear <selector> (clr)           Clear an element's value
	focus <selector>                 Focus an element
	submit <selector>                Submit a form
	text <selector>                  Read an element's text
	html [selector]                  Read HTML, whole page by default
	attr <selector> <attribute>      Read an attribute
	setattr <selector> <attribute> <value>  Set an attribute

Page:

	title                            Page title
	url                              Current URL
	screenshot [file] (snap, capture)  Screenshot the page
	pdf [file]                       Print the page to PDF
	source                           Page source
	render [selector] (md, markdown) Render the page as Markdown

Network:

	cookies                          List cookies
	setcookie <name> <value> [domain]  Set a cookie
	deletecookie <name> (delcookie)  Delete a cookie
	clearcookies                     Delete all cookies
	headers                          Response headers

Emulation:

	mobile [device]                  Emulate a mobile device
	desktop                          Clear device emulation
	viewport <width> <height> (vp, size)  Set the viewport
	offline                          Simulate being offline
	online                           Restore connectivity

Storage:

	localStorage (ls)                Dump local storage
	setLocal <key> <value>           Set a local storage key
	getLocal <key>                   Read a local storage key
	clearLocal                       Clear local storage
	sessionStorage (ss)              Dump session storage

Console:

	eval <expression> (js, exec)     Evaluate JavaScript
	log <message>                    console.log in the page
	error <message>                  console.error in the page
	warn <message>                   console.warn in the page
	clear_console                    Clear the console

Performance, security, and debugging:

	metrics (perf)                   Performance metrics
	memory (mem)                     Memory usage
	csp                              Content-Security-Policy
	origin                           Page origin
	pause                            Pause JavaScript execution
	sources                          List loaded scripts and stylesheets

Tabs:

	tabs (lt)                        List open tabs
	newtab [url] (nt)                Open a tab
	tab <n|text> (t)                 Switch to a tab by index or by a
	                                 substring of its title or URL

Capture and output context:

	context                          Show the output directory and context
	                                 stack
	push-context <name>              Push a named context, writing capture
	                                 output to a subdirectory
	pop-context                      Pop the current context
	note <text>                      Annotate the HAR. Requires
	                                 -har-mode=enhanced
	dom <description>                Capture a DOM snapshot into the HAR

Session:

	jsfile <path>                    Run JavaScript from a file
	reconnect (rc)                   Reconnect after losing the browser
	refresh-profile (rp)             Re-copy the profile and reconnect
	hup                              Detach, leaving the browser running
	help [aliases|<domain>|<command>]  Show help
	exit, quit                       Exit, closing a browser cdp launched
	                                 unless -keep-open is set

# Scripts

The run subcommand executes a repeatable script from a txtar archive
containing a main.cdp file:

	cdp run script.txtar

See skills/writing-cdp-scripts/references/script-format.md for the script
format, and the cdpscripttest package for running scripts as tests.

# Trust model

Cdp drives a real browser with the privileges of the user who starts it. It is
not a sandbox and does not try to be one.

On the command line and in .cdp scripts the operator is the author. Js, jsfile
and the shell's eval run arbitrary JavaScript in the page. Goto reaches any URL
the host can reach, including file:// and localhost; there is no allowlist here
(-allow-url and -allow-domain exist in churl and chrome-to-har, not in cdp).
Screenshot, pdf, har and download-dir write where they are pointed, taking
absolute paths verbatim and resolving relative ones against -output-dir without
confining the result. That is the intended power of a browser automation tool.
A .cdp script is not a shell, though: the engine registers only the commands
documented in the cdpscript package, not rsc.io/script's defaults, so there is
no exec, rm or cp.

Under -mcp the author changes. The caller is an agent, and every capability
above reaches it, along with three that are stronger than running JavaScript in
a page:

	evaluate, raw_cdp  the full page JS and CDP surfaces. Raw_cdp is
	                   restricted only by target and a short denylist of tab
	                   and browser lifecycle methods.
	upload_file        reads any absolute path on the host and hands it to a
	                   page the same agent can navigate.
	run_cdpscript      runs agent-supplied script text with the server's own
	                   environment exposed as ${NAME}, credentials included.

Tool and context names are validated, so define_tool and push_context stay
inside -tools-dir and -output-dir, but artifact paths are not confined. Start
the server with an environment and a working directory you would hand to
whatever is on the other end of the protocol.

The MCP transport is stdio and opens no port. The coverage API binds loopback
when enabled and answers with Access-Control-Allow-Origin: *, so any page the
browser visits can read coverage data from it. -pprof-listen is off unless
given an address.

Captures are redacted by default. Request headers and URL query parameters,
request and response bodies, WebSocket frames and saved sources all pass
through the scrubber unless -no-scrub is set. Two limits apply:
text larger than 512 KiB is passed through unscrubbed, a deliberate trade for
capture speed on large bundles, and scrubbing covers captured artifacts rather
than tool results, so get_cookies returns cookie values as they are and
save_state writes them to disk in cleartext.

# Exit status

	0  success
	1  general error
	2  usage error
	3  browser launch or connection failed
	4  navigation failed
	5  operation timed out
*/
package main
