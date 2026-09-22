/*
Chrome-to-har records browser activity as a HAR file.

Chrome-to-har launches Chrome or Brave, navigates to a URL, and writes the
network traffic it sees as a HAR archive or as a stream of NDJSON entries. It
is the capture-focused tool in this repository; cmd/cdp covers broader CDP
automation, and cmd/churl covers fetching and extraction.

It can also compare two captures, which is how you tell what actually changed
between two runs of the same page.

Usage:

	chrome-to-har [flags]

# Capture

	-url url
	    Starting URL to navigate to.
	-output file
	    Where to write the HAR. (default "output.har")
	-stream
	    Write entries as NDJSON as they are captured, rather than one HAR at
	    the end.
	-max-body-bytes n
	    Maximum response body bytes to keep, 0 to keep whole bodies.
	-interactive
	    Drive the capture from an interactive prompt.

# Browser

	-chrome-path path
	    Path to the Chrome or Brave executable.
	-headless
	    Run without a browser window.
	-profile dir
	    Chrome profile directory to use.
	-list-profiles
	    List available Chrome profiles and exit.
	-cookie-domains domains
	    Comma-separated domains to include cookies from. Requires sqlite3 in
	    PATH.
	-debug-port port
	    DevTools port, 0 to choose automatically.
	-debug-chrome
	    Run Chrome debugging diagnostics.
	-verbose
	    Enable verbose logging.
	-timeout seconds
	    Global timeout. (default 180)

# Filtering the archive

These narrow what ends up in the output. To stop requests from being made at
all, see Blocking below.

	-filter expression
	    Filter entries with a jq expression.
	-template template
	    Transform each entry with a Go template.

# Blocking

	-block-enabled
	    Enable the rule-based blocker below.
	-block-ads
	    Block common ad domains.
	-block-tracking
	    Block common tracking domains.
	-block-domain domains
	    Comma-separated domains to block.
	-block-url patterns
	    Comma-separated URL patterns to block.
	-block-regex patterns
	    Comma-separated regular expressions to block.
	-allow-domain domains
	    Comma-separated domains to allow through a block rule.
	-allow-url urls
	    Comma-separated URLs to allow through a block rule.
	-block-file file
	    Read blocking rules from a file.
	-block-verbose
	    Log what was blocked and why.

# Waiting

A capture is only complete once the page has settled, which for most modern
pages is well after the load event.

	-wait-for selector
	    Wait for a CSS selector to appear.
	-wait-stable
	    Wait until both network and DOM go quiet.
	-wait-for-stability
	    Wait for network, DOM, and resources to settle, rather than only the network.
	-network-idle-timeout ms
	    How long to wait for the network to go idle. This is a fixed
	    sleep; network activity is not observed. (default 500)
	-stable-timeout seconds
	    Maximum time to wait for stability overall. (default 30)
	-resource-timeout seconds
	    Per-resource loading timeout. (default 10)
	-wait-for-images
	    Wait for images to load. (default true)
	-wait-for-scripts
	    Wait for scripts to load. (default true)
	-wait-for-stylesheets
	    Wait for stylesheets to load. (default true)
	-wait-for-fonts
	    Wait for fonts to load. (default true)

# Differential capture

Differential mode records named captures and reports what changed between
them, which is useful for spotting traffic a change introduced or removed.

	-diff
	    Enable differential HAR capture.
	-diff-mode
	    Enable differential capture mode.
	-capture-name name
	    Name for this capture.
	-capture-labels labels
	    Labels for this capture, as key=value,key2=value2.
	-baseline name|id
	    Capture to compare against.
	-compare-with name|id
	    Capture to compare with the baseline.
	-diff-output path
	    Where to write the report.
	-diff-format format
	    Report format: json, html, text, or csv. (default "html")
	-diff-work-dir dir
	    Working directory for differential captures.
	-list-captures
	    List available captures.
	-delete-capture id
	    Delete a capture.
	-min-significance level
	    Report only changes at or above this level: low, medium, or high.
	    (default "low")
	-track-performance
	    Report performance changes between captures. (default true)
	-track-resources
	    Report resource changes between captures. (default true)
	-track-states
	    Record page state changes during interactions.

# Examples

Capture a page:

	chrome-to-har -url https://example.com -output example.har

Stream only API traffic:

	chrome-to-har -url https://example.com -stream \
		-filter 'select(.request.url | test("api\\.example\\.com"))'

Capture with a profile's cookies, waiting for the app to render:

	chrome-to-har -profile Default -url https://app.example.com \
		-wait-for '#app-root' -wait-stable -output app.har

Record a baseline, then compare a later run against it:

	chrome-to-har -diff-mode -url https://example.com -capture-name baseline
	chrome-to-har -baseline baseline -compare-with candidate \
		-diff-output report.html

See docs/differential-capture.md for the differential workflow in full.
*/
package main
