/*
Cdpscript runs a browser automation script.

A script is a txtar archive containing a main.cdp file, whose commands run in
order against a browser cdpscript launches or attaches to. The same archives
run as tests through the cdpscripttest package, so a script that reproduces a
bug can become the regression test for it.

Usage:

	cdpscript [options] <script.txtar>
	cdpscript [options] -

With - as the script path, cdpscript reads the archive from standard input.

Options:

	-v, -verbose
	    Enable verbose logging.
	-o, -output dir
	    Write screenshots, PDFs, and other artifacts here.
	-headless
	    Run a launched browser without a window.
	-timeout duration
	    Default timeout for browser startup and selector waits.
	    (default 30s)
	-tab id
	    Attach to an existing browser tab by its ID, as reported by
	    /json/list, instead of launching one.
	-port port
	    Chrome remote debugging port. (default 9222)

# Exit status

	0    success
	1    script error
	2    usage error
	3    an assert command failed
	130  interrupted by SIGINT or SIGTERM

See the cdpscript package for the command set and the script syntax, and
skills/writing-cdp-scripts/references/script-format.md for the format
reference.
*/
package main
