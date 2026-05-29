// Package cdpscript runs txtar-backed browser automation scripts over CDP.
//
// The root package provides the script engine reused by the `cdpscript` and
// `cdp` commands. For test-driven browser automation, use the sibling
// `cdpscripttest` package.
//
// Scripts are txtar archives with a required main.cdp file. Callers can pass
// argv, environment, an output directory, and an existing DevTools tab. The
// command-line wrappers classify usage and assertion failures with sentinel
// errors from this package.
package cdpscript
