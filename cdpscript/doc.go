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
// attach to an existing DevTools tab, and WithBrowserFromContext when the
// caller already owns a browser. ErrUsage and ErrAssertionFailed let
// command-line wrappers return stable Unix exit codes.
//
// # Stability and Compatibility
//
// The module is pre-v1, so nothing here carries a v1 compatibility promise
// yet. The three surfaces are not equally settled:
//
// The script command vocabulary is the most stable surface. Command names,
// their argument order, and the meaning of ErrUsage and ErrAssertionFailed are
// treated as a contract: existing commands gain optional flags rather than
// changing meaning, and a renamed command keeps its old name as an alias.
// Scripts written today are expected to keep running.
//
// The Go API is stable in shape but not frozen. Engine, New, the Execute
// methods, and the Option constructors are unlikely to move; individual
// options may be added, and an option that takes a type from an internal
// package may be removed outright, since no code outside this module can call
// it. Deprecations are marked in godoc for at least one release before removal.
//
// The MCP tool set is the least stable surface. Tool names, schemas, and
// result shapes follow what the hosts need and may change without notice.
// Pin a commit if you depend on a specific schema.
package cdpscript
