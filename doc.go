// Package cdp provides shared code for Chrome DevTools Protocol automation and
// the command-line tools in this module.
//
// The repository ships several related commands:
//
//   - cmd/cdp for general-purpose browser automation, inspection, and MCP use
//   - cmd/chrome-to-har for focused HAR and differential capture workflows
//   - cmd/churl for browser-backed fetching and extraction
//   - cmd/chdb for Chrome-oriented debugging workflows
//   - cmd/ndp for Node.js and V8 inspector workflows
//   - cmd/cdpscript and cmd/cdpscripttest for script execution and testing
//
// Most consumers will use one of those commands directly. The root package
// exists to document the module and to house shared code used by those tools.
//
// Internal packages provide most of the implementation:
//
//   - internal/browser manages browser discovery, launch, and interaction
//   - internal/recorder handles HAR and enhanced traffic capture
//   - internal/differential compares capture runs
//   - internal/chromeprofiles discovers and manages browser profiles
//
// The broader entry point is cmd/cdp. The narrower capture-oriented entry point
// is cmd/chrome-to-har.
package cdp
