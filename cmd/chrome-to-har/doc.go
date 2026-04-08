// Command chrome-to-har records browser activity and generates HAR
// (HTTP Archive) files.
//
// This command launches Chrome or Chromium, navigates to specified URLs, and
// captures network traffic in HAR format. It remains the focused capture tool
// in this repository, while cmd/cdp provides broader CDP automation and
// debugging workflows.
//
// Usage:
//
//	chrome-to-har [flags] [URL...]
//
// Examples:
//
//	chrome-to-har https://example.com
//	chrome-to-har -profile "Default" https://github.com
//	chrome-to-har -filter "api\\." https://example.com
//	chrome-to-har -block "analytics|tracking" https://news.site.com
package main

