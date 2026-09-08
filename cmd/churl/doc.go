/*
Churl fetches URLs through a real browser.

Churl is shaped like curl and wget, but every request runs in Chrome or Brave,
so JavaScript executes and single-page applications render before the content
is read. It prints the rendered page, extracts it as text, Markdown, or JSON,
and can write a HAR alongside any of those.

Usage:

	churl [flags] URL...

Short and long spellings of the same flag are listed together; either works.

# Output

	-o file
	    Write to this file rather than stdout.
	-output-format format
	    One of html, har, text, json, or pdf. (default "html")
	    pdf renders the loaded page with Page.printToPDF. It is binary, so
	    churl refuses to write it to a terminal: pass -o or redirect.

# PDF settings

The -pdf flag carries the print settings as one comma-separated string, so the
whole of printToPDF is reachable without a flag each:

	-pdf settings
	    page=<name|WxH>   letter (default), legal, tabloid, ledger, a0..a6,
	                      or explicit dimensions such as 8.5x11 or 210mmx297mm
	    margin=<lengths>  1, 2, or 4 space-separated lengths, in CSS order
	                      (default 0.4in, Chrome's own default)
	    scale=<n>         render scale (default 1)
	    ranges=<pages>    print only these one-based pages, e.g. ranges=1-5 8
	    landscape         landscape orientation
	    outline           embed PDF bookmarks built from the document headings;
	                      implies tagged, which Chrome needs to derive them
	    tagged            emit a tagged (accessible) PDF
	    css-page-size     honour @page size from the document's own CSS

	    Lengths accept in, mm, cm, or px suffixes and are inches when
	    unsuffixed. Values may not contain commas, which is why margin and
	    ranges take space-separated lists.

	    For example:
	        -pdf 'page=a4,margin=0.75,landscape'
	        -pdf 'page=210mmx297mm,margin=20mm 15mm,outline'

	-pdf-header html
	-pdf-footer html
	    HTML templates rendered in the top and bottom margins. Elements with
	    class date, title, url, pageNumber, or totalPages are substituted.
	    The margin on that edge must be large enough to show the template.
	    These stay separate flags because the templates are HTML and would
	    not survive a comma-separated list.

A document that declares @page size in its own CSS overrides page= and
landscape even without css-page-size.

There is no image-quality or resampling parameter: Page.printToPDF does not
expose one, and deviceScaleFactor does not affect its output. Images are
embedded at their source resolution, so the lever is the resolution of the
images the page loads.

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

# Mirroring

Mirroring is not implemented. Churl accepts wget's mirroring flags so that the
implementation, when it lands, keeps their spelling, and rejects any command
that sets one: a command asking for a copy of a site on disk should fail rather
than print a single page and exit successfully.

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
	-P, -directory-prefix dir
	    Save below this directory.
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
	-w, -wait seconds
	    Wait between downloads.
	-limit-rate n
	    Limit download speed to n bytes per second, 0 for unlimited.
	-Q, -quota n
	    Stop after downloading n bytes in total, 0 for unlimited.
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

Capture a HAR while fetching:

	churl -har capture.har https://example.com
*/
package main
