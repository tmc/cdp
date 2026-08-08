// Package cdpscript runs browser automation scripts over CDP.
//
// Scripts are txtar archives with a required main.cdp file. ExecuteReader runs
// an archive from any io.Reader; ExecuteTxtar runs one from disk. ExecuteScript
// runs a plain .cdp command body, which is useful for MCP-defined tools.
//
// Common commands:
//
//	goto, back, forward, reload, wait
//	click, dblclick, fill, type, drag, mouse, set-range, hover, press,
//	scroll, select, upload
//	dialog, viewport
//	js, jsfile
//	extract, title, url, render, snapshot
//	assert, screenshot, pdf, log, download-dir, wait-download
//	block, cookie
//	source, tag, har, note, capture
//
// Scripts use rsc.io/script syntax: commands run sequentially, ! expects
// failure, ? allows failure, and [cond] guards a command. cdpscript deliberately
// keeps loops and retries in the calling shell or Go test harness instead of
// adding a second control-flow language.
//
// Use WithOutputDir for artifacts, WithEnv for environment, WithRemoteTab to
// attach to an existing DevTools tab, and WithBrowser when the caller already
// owns a chromedp context. ErrUsage and ErrAssertionFailed let command-line
// wrappers return stable Unix exit codes.
package cdpscript
