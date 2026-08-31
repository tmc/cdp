// Package cdpscripttest brings rsc.io/script txtar-script ergonomics to
// Chrome DevTools Protocol (CDP) browser testing.
//
// Browser-backed fixture tests use the cdp build tag:
//
//	go test -tags cdp -p 1 -parallel 1 ./cdpscripttest
//
// Scripts are plain-text txtar archives: the archive comment section is the
// script body, and any -- filename -- sections are files extracted to the
// test's working directory before the script runs.
//
// # Script Syntax
//
// Scripts follow the rsc.io/script language. Each line is either a comment,
// a condition guard, or a command:
//
//   - Lines starting with # are comments.
//   - A command prefixed with ! expects failure (non-zero exit).
//   - Condition guards like [headless] or [!short] make the line conditional.
//   - stdout and stderr are built-in assertions that match the previous
//     command's output against a regexp pattern.
//   - cmp compares files (often paired with txtar fixture files).
//
// Example:
//
//	# Check the page title after navigating.
//	navigate /app
//	title
//	stdout 'MyApp'
//
//	# Only run in headless mode.
//	[headless] screenshot before.png
//
//	# Expect eval to fail with bad JS.
//	! eval 'throw new Error("boom")'
//
// Prefix conditions accept a colon-separated argument. For readability,
// the engine also accepts a space separator:
//
//	[stdout pattern]         # rewritten to [stdout:pattern]
//	[element .my-selector]   # rewritten to [element:.my-selector]
//	[title *App*]            # rewritten to [title:*App*]
//	[rtc-state connected]    # rewritten to [rtc-state:connected]
//
// # Txtar Format
//
// Test files use the txtar format from golang.org/x/tools/txtar. The
// comment section (before any -- filename -- line) is the script body.
// Named sections are extracted as files into the test's working directory.
// This lets scripts reference fixture files (HTML, JS, JSON) without
// external files:
//
//	navigate /app
//	evalfile setup.js
//	text '#result'
//	stdout 'ok'
//	-- setup.js --
//	document.getElementById('result').textContent = 'ok';
//
// For cdpscript-format txtars containing main.cdp, use RunCDPScript. That path
// delegates to the real cdpscript runtime instead of the legacy cdpscripttest
// command table, so runtime behavior stays aligned. Set
// CDPScriptRunOptions.TabID and Port to run a fixture against an already
// running DevTools tab.
// Checked-in fixtures live under testdata/, testdata/interaction/, and
// testdata/cdpscript/.
//
// RunCDPScript uses the cdpscript runtime command surface:
//
//	goto, back, forward, reload, wait
//	click, dblclick, fill, type, drag, mouse, set-range, hover, press,
//	scroll, select, upload
//	dialog, viewport
//	js, jsfile
//	extract, title, url, render, snapshot
//	assert
//	screenshot, pdf, log, download-dir, wait-download
//	block, cookie
//	source
//	tag, har, note, capture
//
// # Commands
//
// The default command set includes scripttest's defaults (env, echo, exec,
// exists, grep, mkdir, rm, cp, cmp, chmod, cd, replace, stdin) plus the
// following CDP-aware commands.
//
// Navigation and waiting:
//
//	navigate <path>              navigate to BASE_URL+path, wait for <body>
//	wait-visible [opts] <sel>     wait for a CSS selector to become visible
//	wait-not-visible [opts] <sel>  wait for a CSS selector to disappear
//	timeout <duration>           set default wait timeout (default 10s)
//	sleep <duration>             pause (e.g. "500ms", "2s")
//
// wait-visible/wait-not-visible options:
//
//	--timeout <duration>   override the default wait timeout for this command
//
// Interaction:
//
//	click <selector>             click a CSS selector
//	send-keys <selector> <text>   send keystrokes to a CSS selector
//
// Evaluation:
//
//	eval <js>                    evaluate JavaScript; result → stdout
//	evalfile <filename>          evaluate a JS file from the working directory
//
// Content extraction:
//
//	text <selector>              trimmed inner text → stdout
//	html <selector>              inner HTML → stdout
//	title                        page title → stdout
//	url                          current page URL → stdout
//
// Screenshots:
//
//	screenshot [--blur <sel>]... [filename]                    full-page PNG
//	screenshot-sel [--padding N] [--blur <sel>]... <sel> [fn]  element PNG
//	screenshot-compare [opts] [--blur <sel>]... <sel> [fn]     diff vs baseline
//	screenrecord start [options] [filename]                    begin tab recording
//	screenrecord stop                                          write recording artifact
//	screen-record start [options] [filename]                   alias for screenrecord
//	video start [options] [filename]                           alias for screenrecord
//
// All screenshot commands support --blur <selector> to mask dynamic content
// before capture. Blurred elements have their text replaced with a fixed
// placeholder and a mild CSS blur applied, producing identical pixels across
// runs while still showing that content was present. Repeatable.
//
// screenrecord captures Chrome screencast frames from the active tab. It writes
// GIF, PNG, or numbered PNG frame artifacts. --selector crops every frame to a
// fixed element border box. If a script fails while recording, cdpscripttest
// stops the recording during cleanup and leaves the artifact path in the command
// log for reports.
//
// screenshot-compare options:
//
//	--threshold N    max allowed diff percent (default 5)
//	--update         overwrite baseline with current capture
//
// Script injection:
//
//	inject <js>            install JS on every new document (persists across nav)
//	inject-clear           remove all scripts installed with inject
//
// Content extraction via external tools:
//
//	distill [selector]     extract main content via htmldistill → stdout
//	markdown [selector]    convert HTML to Markdown via html2md → stdout
//
// Gate these with [exec:htmldistill] or [exec:html2md] conditions.
//
// Control flow:
//
//	skip [msg]             skip the current script (not a failure)
//	stop [msg]             halt the script early without failure or skip
//	set-base-url <url>       override BASE_URL for subsequent commands
//
// # Command Aliases
//
// Aliases mirror the interactive cmd/cdp shell for familiarity:
//
//	goto   → navigate
//	wait   → wait-visible
//	js     → eval
//	jsfile → evalfile
//	pause  → sleep
//	type   → send-keys
//	fill   → send-keys
//
// Compatibility aliases are also accepted for older scripts:
//
//	waitVisible     → wait-visible
//	waitNotVisible  → wait-not-visible
//	sendKeys        → send-keys
//	setBaseURL      → set-base-url
//
// # WebRTC Commands
//
// WebRTC monitoring requires calling rtc-inject before navigate to install
// an RTCPeerConnection monkey-patch via Page.addScriptToEvaluateOnNewDocument.
//
// Setup:
//
//	rtc-inject                   inject WebRTC monitoring script
//	rtc-select <id>              set current peer by stable ID
//
// State inspection:
//
//	rtc-peers                    print peer count and states
//	rtc-state                    print connection state of current peer
//	rtc-wait <state> [timeout]   wait for peer to reach state (default 30s)
//	rtc-sdp                      print SDP for current peer
//	rtc-ice                      print ICE candidates (local and remote)
//	rtc-events                   print event log for current peer
//	rtc-tracks                   print active tracks for current peer
//	rtc-devices                  print available media devices
//
// Stats:
//
//	rtc-stats                           print all stats for current peer
//	rtc-stats-video [--direction D]     video RTP stats (inbound|outbound)
//	rtc-stats-audio [--direction D]     audio RTP stats (inbound|outbound)
//	rtc-stats-transport                 transport and candidate-pair stats
//	rtc-stats-poll --duration D --interval I   poll stats over time
//
// Testing helpers:
//
//	rtc-mock-screenshare [--width W] [--height H] [--fps F]
//	    mock getDisplayMedia with a canvas test pattern
//
// # Network Emulation Commands
//
//	network-emulate [flags]    emulate network conditions
//	network-emulate-clear      reset all network emulation
//
// network-emulate flags:
//
//	--loss N       packet loss percent (0-100)
//	--queue N      packet queue length
//	--reorder      enable packet reordering
//	--latency N    HTTP latency in ms
//	--down N       download throughput bytes/sec (-1 = no limit)
//	--up N         upload throughput bytes/sec (-1 = no limit)
//
// # Conditions
//
// Boolean conditions (true/false):
//
//	headless          Chrome is running in headless mode
//	rtc               WebRTC monitoring is active
//	short             testing.Short() is true (go test only)
//	verbose           testing.Verbose() is true (go test only)
//
// Prefix conditions (accept a colon-separated or space-separated argument):
//
//	exec:<name>           executable is in PATH
//	element:<selector>    querySelector returns non-null (instant, no waiting)
//	title:<glob>          page title matches a glob pattern
//	stdout:<pattern>      last command's stdout matches a regexp
//	rtc-state:<state>     selected peer connection is in <state>
//	rtc-dc:<label>        data channel with <label> exists
//	rtc-codec:<name>      codec <name> appears in cached stats (e.g. VP8, opus)
//
// All conditions can be negated with !:
//
//	[!headless] skip 'requires headed mode'
//	[!rtc] skip 'WebRTC not injected'
//	[!element:#status] skip 'status element missing'
//
// # Environment Variables
//
// The following environment variables are available in scripts:
//
//	WORK          the test's working directory
//	TMPDIR        a temporary directory (cleaned up after the test)
//	BASE_URL      the base URL passed to Test() (read-only in env; use set-base-url)
//	SCREENSHOT_DIR  override the screenshot output directory
//
// Additional variables used by the test runner:
//
//	CDPSCRIPTTEST_ARTIFACTS   artifact root directory, bypassing t.ArtifactDir()
//	UPDATE_GOLDEN             set to update baselines instead of comparing
//
// When CDPSCRIPTTEST_ARTIFACTS is set, screenshots and baselines are saved to
// <dir>/<script-name>/ with no _artifacts/<hash> nesting. For example:
//
//	CDPSCRIPTTEST_ARTIFACTS=./out go test -tags cdp ./...
//	# produces: ./out/fleet-view/fleet-full-page.png
//
// # Flags
//
// The package registers test flags (call flag.Parse in TestMain):
//
//	-cdp-artifacts=<dir>    set the artifact root (bypasses t.ArtifactDir)
//	-emit-artifacts         save to <script-dir>/artifacts/<script-name>/
//	-update-golden          update golden baselines instead of comparing
//
// -emit-artifacts derives the output path from the script file location. For
// example, testdata/interaction/viewport.txtar produces
// testdata/interaction/artifacts/viewport/*.png.
// No path argument needed.
//
// All screenshot commands (screenshot, screenshot-sel, screenshot-compare)
// write to the same artifact directory. screenshot-compare reads baselines
// from the artifact directory and compares against them. On first run, the
// capture becomes the baseline. Use -update-golden to overwrite baselines.
//
// Environment variables take precedence over their flag counterparts.
//
// # Usage
//
// Typical test setup:
//
//	func TestCDP(t *testing.T) {
//	    // Set up a chromedp allocator (headless Chrome).
//	    ctx, cancel := chromedp.NewExecAllocator(context.Background(),
//	        append(chromedp.DefaultExecAllocatorOptions[:],
//	            chromedp.Flag("headless", true),
//	        )...,
//	    )
//	    defer cancel()
//
//	    e := cdpscripttest.NewEngine()
//	    cdpscripttest.Test(t, e, ctx, "http://localhost:8080", "testdata/*.txt", nil)
//	}
//
// For CLI (non-test) usage:
//
//	e := cdpscripttest.NewCLIEngine()
//
// # Example Script
//
//	# Navigate and verify the page title.
//	navigate /app
//	wait-visible h1
//	title
//	stdout 'MyApp'
//
//	# Navigate to settings, check heading.
//	navigate /app/settings
//	wait-visible h1
//	text h1
//	stdout 'Settings'
//
//	# Screenshot comparison against baseline.
//	screenshot-compare --threshold 3 '#main-content' settings.png
//
// # WebRTC Example
//
//	# Inject before navigate so the monkey-patch runs at page load.
//	rtc-inject
//	navigate /video-call
//	wait-visible '#status'
//	rtc-wait connected 60s
//
//	# Inspect connection details.
//	rtc-peers
//	stdout 'connected'
//
//	rtc-stats-video --direction outbound
//	stdout 'framesSent'
//
//	# Conditional check with rtc-state.
//	[rtc-state connected] rtc-ice
//	stdout 'candidate:'
package cdpscripttest
