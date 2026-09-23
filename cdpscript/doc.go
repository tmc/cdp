// Package cdpscript runs browser automation scripts over CDP.
//
// Scripts are txtar archives with a required main.cdp file. ExecuteReader runs
// an archive from any io.Reader; ExecuteTxtar runs one from disk. ExecuteScript
// runs a plain .cdp command body, which is useful for MCP-defined tools.
//
// Scripts use rsc.io/script syntax: commands run sequentially, ! expects
// failure, ? allows failure, and [cond] guards a command. cdpscript deliberately
// keeps loops and retries in the calling shell or Go test harness instead of
// adding a second control-flow language.
//
// Use WithOutputDir for artifacts, WithEnv for environment, and WithRemoteTab
// to attach to an existing DevTools tab, including one the caller already
// drives. ErrUsage and ErrAssertionFailed let
// command-line wrappers return stable Unix exit codes.
//
// # Command Reference
//
// Every command the engine accepts, with its arguments and summary. This list
// is generated from the engine's own command table and checked by
// TestDocCommandReferenceMatchesEngine, so it cannot drift from the code.
// CommandNames returns the same names at runtime.
//
//	assert exists|text|visible|status|response|header args... assert condition on page
//	back                                          go back in history
//	block pattern                                 block URLs matching pattern
//	capture screenshot|dom [description]          capture screenshot or DOM to HAR
//	click selector|@ref                           click element
//	cookie get [name] | set name value [domain] [path] | clear [name] get, set, or clear browser cookies
//	dblclick selector|coord:x,y                   double-click element
//	dialog accept|dismiss [prompt-text]           handle next JavaScript dialog
//	download-dir dir                              set download directory
//	drag source target [steps]                    drag from source to target
//	extract selector                              extract text from element
//	fill selector|@ref value                      fill input field
//	forward                                       go forward in history
//	goto url                                      navigate to URL
//	har filename                                  write HAR file
//	hover selector                                hover over element
//	js code                                       execute JavaScript
//	jsfile filename                               execute JavaScript from file
//	log message                                   log message
//	mouse down|move|up [selector|coord:x,y]       press, move, or release the mouse
//	note description                              add note to HAR
//	pdf filename                                  save page as PDF
//	press key                                     press keyboard key
//	reload                                        reload page
//	render [--term] [selector]                    render page or element as markdown
//	screenshot filename                           take screenshot
//	scroll [up|down|left|right [px]|selector]     scroll page or element
//	select selector value|text                    select dropdown option
//	set-range selector value                      set the value of a range input
//	snapshot [-i] [--depth N] [--compact] [--selector CSS] get accessibility snapshot with element refs
//	source [-x] [-as name] path                   source a CDP script file
//	tag [tag-name]                                set tag for network activity
//	title                                         get page title
//	type selector|@ref value                      fill input field
//	upload selector file...                       upload files to a file input
//	url                                           get current URL
//	viewport width height                         set viewport size
//	wait [duration|selector]                      wait for duration or selector
//	wait-download filename [timeout]              wait for downloaded file
//
// A selector argument is a CSS selector, or @ref to address an element by a
// ref returned from snapshot. type is an alias for fill. source -as name
// registers the sourced script under name, which then works as a command for
// the rest of the run.
//
// Two conditions are available to [cond] guards: headless is true when the
// engine launched a headless browser, and has-tab is true when it is attached
// to an existing DevTools tab.
//
// Arguments passed after the archive path are available as ${ARG1} through
// ${ARGN}, with ${ARGC} holding the count; the process environment plus
// anything from WithEnv is available as ${NAME}.
//
// For worked examples and the fuller prose treatment of each command group,
// see skills/writing-cdp-scripts/references/script-format.md in the module
// source.
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
