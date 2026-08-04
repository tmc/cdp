/*
Churl fetches URLs through a real browser.

Churl is shaped like curl and wget, but every request runs in Chrome or Brave,
so JavaScript executes and single-page applications render before the content
is read. It prints the rendered page, extracts it as text, Markdown, or JSON,
mirrors a site recursively, and can write a HAR alongside any of those.

Usage:

	churl [flags] URL...

Short and long spellings of the same flag are listed together; either works.

# Output

	-o file
	    Write to this file rather than stdout.
	-output-format format
	    One of html, har, text, or json. (default "html")
	-har file
	    Also write a HAR to this path. Works with any output format.
	-verbose
	    Enable verbose logging.

# Request

	-X method
	    HTTP method. (default "GET")
	-d data
	    Body to send with POST or PUT.
	-H header
	    Add a request header. Repeatable.
	-u user:password
	    Credentials for basic authentication.
	-L
	    Follow redirects. (default true)

# Browser

	-chrome-path path
	    Path to the Chrome or Brave executable.
	-headless
	    Run without a browser window. (default true)
	-profile dir
	    Chrome profile directory to use.
	-debug-port port
	    DevTools port, 0 to choose automatically.
	-remote-host host
	    Connect to a browser on this host instead of launching one.
	-remote-port port
	    Remote debugging port. (default 9222)
	-remote-tab id|url
	    Connect to a specific tab.
	-list-tabs
	    List available tabs on remote Chrome and exit.
	-timeout seconds
	    Global timeout. (default 180)

# Waiting

A rendered page is only worth reading once it has settled.

	-wait-network-idle
	    Wait until network activity goes idle. (default true)
	-wait-for selector
	    Wait for a CSS selector to appear.
	-wait-for-challenge
	    Wait for anti-bot interstitials, such as Cloudflare's, to resolve.
	    (default true)
	-stable-timeout seconds
	    Maximum time to wait for the page to stabilize. (default 30)
	-w, -wait seconds
	    Wait between downloads.

# Recursion and mirroring

	-r, -recursive
	    Download recursively.
	-m, -mirror
	    Mirror a site. Shorthand for -r -k.
	-l, -level n
	    Maximum recursion depth, 0 for unlimited.
	-p, -page-requisites
	    Also download the assets a page needs to display.
	-np, -no-parent
	    Do not ascend above the starting directory.
	-span-hosts
	    Follow links to other domains.
	-k, -convert-links
	    Rewrite links to point at the downloaded copies.

# Where files land

	-P, -directory-prefix dir
	    Save below this directory. (default ".")
	-nd, -no-directories
	    Do not recreate the directory hierarchy.
	-nH, -no-host-directories
	    Do not create a top-level directory per host.
	-x, -force-directories
	    Always create directories, even for a single file.
	-cut-dirs n
	    Drop this many leading remote directory components.
	-nc, -no-clobber
	    Skip files that already exist.
	-N, -timestamping
	    Download only files newer than the local copy.
	-c, -continue
	    Resume partial downloads.
	-limit-rate n
	    Limit download speed to n bytes per second, 0 for unlimited.
	-Q, -quota n
	    Stop after downloading n bytes in total, 0 for unlimited.

# Choosing what to fetch

	-A, -accept extensions
	    Accept only these file extensions, comma-separated.
	-R, -reject extensions
	    Reject these file extensions, comma-separated.
	-accept-regex regexp
	    Accept only URLs matching this pattern.
	-reject-regex regexp
	    Reject URLs matching this pattern.
	-D, -domains domains
	    Accept only these domains, comma-separated.
	-exclude-domains domains
	    Reject these domains, comma-separated.
	-I, -include-directories dirs
	    Include only these directories, comma-separated.
	-exclude-directories dirs
	    Exclude these directories, comma-separated.

# Blocking

Blocking drops requests inside the browser, which speeds up rendering and keeps
third-party noise out of a capture.

	-block-enabled
	    Enable URL and domain blocking.
	-block-ads
	    Block common ad domains.
	-block-tracking
	    Block common tracking domains.
	-block-domain domain
	    Block a domain. Repeatable.
	-block-url pattern
	    Block URLs matching a pattern. Repeatable.
	-block-regex regexp
	    Block URLs matching a regular expression. Repeatable.
	-allow-domain domain
	    Allow a domain that a rule would otherwise block. Repeatable.
	-allow-url pattern
	    Allow matching URLs. Repeatable.
	-block-file file
	    Read blocking rules from a file.
	-block-verbose
	    Log what was blocked and why.

# Scripting

	-script-before script
	    JavaScript to run before the page loads. Repeatable.
	-script-after script
	    JavaScript to run after the page loads. Repeatable.
	-script-file-before file
	    Like -script-before, read from a file. Repeatable.
	-script-file-after file
	    Like -script-after, read from a file. Repeatable.

# WebSockets

Pages that talk over WebSockets carry their interesting traffic outside the
request log, so churl can watch the socket directly.

	-ws-enabled
	    Monitor WebSocket traffic.
	-ws-url-pattern pattern
	    Only monitor sockets whose URL matches. (default "*")
	-ws-data-pattern pattern
	    Only report frames whose payload matches.
	-ws-direction direction
	    Report only sent or only received frames.
	-ws-send message
	    Send a message to the socket. Repeatable.
	-ws-wait-for condition
	    Wait for a condition before finishing: open, closed, message,
	    first_message, and similar.
	-ws-timeout seconds
	    How long to wait for -ws-wait-for. (default 30)
	-ws-output file
	    Write captured WebSocket data here.
	-ws-stats
	    Print WebSocket statistics when finished.

# Proxies

	-proxy url
	    HTTP or HTTPS proxy, for example http://proxy.example.com:8080.
	-socks5-proxy url
	    SOCKS5 proxy, for example socks5://proxy.example.com:1080.
	-proxy-user user:password
	    Proxy credentials.
	-proxy-bypass hosts
	    Comma-separated hosts to reach directly.

# Examples

Print a rendered page, then convert it to Markdown:

	churl https://example.com
	churl -output-format text https://example.com

Wait for an application shell before reading the page:

	churl -wait-for '#app-root' https://app.example.com

Mirror a documentation site, staying within it:

	churl -m -np -P ./site https://example.com/docs/

Capture a HAR while fetching:

	churl -har capture.har https://example.com
*/
package main
