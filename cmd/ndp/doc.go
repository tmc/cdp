/*
Ndp debugs Node.js processes and Chrome browsers over the inspector protocol.

Node and Chrome speak the same wire protocol, so ndp drives both: attach to a
running Node process or a browser tab, set watches, evaluate expressions,
profile, and inspect the V8 runtime. Use it for server-side JavaScript, where
cmd/cdp and cmd/chdb are aimed at pages.

Usage:

	ndp <command> [flags] [arguments]

Every command has its own flags and help text:

	ndp help
	ndp help <command>
	ndp <command> --help

# Node.js

	node start <script.js>       Start a script with debugging enabled
	node attach [port]           Attach to a running process
	node list                    List active Node.js debug sessions
	node watch <port> <expr>     Add a watch expression
	node sessions                List active debug sessions

# Chrome

	chrome attach [port]         Attach to a browser
	chrome list                  List tabs and targets
	chrome navigate <url>        Navigate to a URL
	chrome console <js>          Evaluate JavaScript in the console

# Targets and sessions

	targets                      List all debug targets
	session list                 List attach session files
	repl                         Start an interactive REPL
	tui                          Start the terminal UI
	search                       Search loaded scripts

# The V8 runtime

	v8 [command]                         V8 Inspector debugging,
	                                     compatible with Chrome DevTools
	runtime evaluate <expr>              Evaluate an expression
	runtime compile <port> <expr>        Compile a script and print its ID
	runtime run <port> <script-id>       Run a compiled script
	runtime call <port> <object-id> <fn> Call a function on a remote object
	runtime release-object <port> <id>   Release a remote object
	runtime release-group <port> <group> Release a remote object group
	runtime disable <port>               Disable the runtime domain

# Profiling and raw access

	profile cpu [duration]       Profile the CPU for a duration (default 10s)
	                             and write the profile to -o
	                             (default cpu-profile.json)
	profile heap                 Take a heap snapshot and write it to -o
	                             (default heap-snapshot.json)
	call <method> [json]         Execute a raw CDP method, for anything the
	                             commands above do not cover
	proxy                        Start a WebSocket proxy between a CDP client
	                             and the target, to inspect the DevTools
	                             traffic itself
*/
package main
