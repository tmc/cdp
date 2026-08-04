/*
Chdb is a debugger for pages running in Chrome.

Where cmd/cdp is a general CDP driver and cmd/chrome-to-har is a capture tool,
chdb is organized around the things you do when a page misbehaves: set a
breakpoint, step, read the DOM, watch the network, take a heap snapshot.

Usage:

	chdb <command> [flags] [arguments]

Every command has its own flags and help text:

	chdb help
	chdb help <command>
	chdb <command> --help

# Connecting

	attach [port]  Attach to a running Chrome instance
	list           List available Chrome tabs and targets
	new [url]      Create a new target or tab
	navigate <url> Navigate to a URL
	devtools       Open DevTools for a target
	bridge         Start a multiplexing debug bridge, so several clients can
	               drive one browser

# Execution and breakpoints

	debug          Start an interactive debugging session
	console        Start an interactive console session
	exec <js>      Evaluate JavaScript
	break          Manage breakpoints
	pause          Pause JavaScript execution
	resume         Resume execution
	step           Step into the next call
	next           Step over to the next line
	out            Step out of the current function

# Inspecting the page

	dom            Dump the DOM tree, or one node
	inspect <sel>  Inspect an element. Superseded by dom get
	css            Inspect computed styles
	sources        Dump page sources, the resource tree and debugger scripts,
	               as a txtar archive
	unminify <url> Backfill source maps
	storage        Inspect local or session storage
	cookies        Manage cookies
	audit          Check the page for issues

# Network

	network        Monitor network traffic
	monitor        Monitor network requests and console output together
	overrides      Serve local files in place of network responses, so you can
	               edit a deployed asset without deploying
	sw             Service worker debugging and management

# Performance

	profile <type> Profile CPU or heap usage
	heap           Capture a heap snapshot
	trace          Record a performance trace
	animation      Debug and control animations
	render         Rendering and paint debugging

# Emulation

	emulate        Emulate a device, or custom metrics
	device         Device emulation and touch simulation
	screenshot     Take a screenshot
*/
package main
