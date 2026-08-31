/*
Cdpscripttest runs CDP browser automation scripts as tests.

Each script is a txtar archive whose comment section is the script body and
whose file sections are fixtures written out before the script runs. Scripts
report pass or fail, compare screenshots against baselines, and can drop into
an interactive prompt at the point they finish, which is the fastest way to
work out why one failed.

Usage:

	cdpscripttest [flags] script.txt ...

# Browser

	-url url
	    Base URL for navigate commands. (default "http://localhost:8090")
	-browser path
	    Path to the Chrome, Brave, or Chromium binary. Auto-detected when
	    empty.
	-headful
	    Run with a visible window rather than headless.
	-window-size WxH
	    Browser window size, for example 1280x900. Defaults to the terminal
	    size.
	-debug-port port
	    Attach to an existing Chrome debug port instead of launching a
	    browser.
	-timeout duration
	    Per-script timeout. (default 2m)

# Working on a failing script

	-i, -interactive
	    Drop to a cdp> prompt after each script, with the page left as the
	    script left it.
	-watch
	    Re-run scripts when they change.
	-v
	    Show full stdout, which is what you want when a text or html command
	    is the thing under test.
	-no-color
	    Disable ANSI color. Also honors NO_COLOR.

# Artifacts

	-artifacts dir
	    Where to persist screenshots. Defaults to a screenshots directory
	    beside each script.
	-update-golden
	    Overwrite baseline screenshots instead of comparing against them.
	-images
	    Display screenshots inline, using the iTerm2 or Kitty protocol when
	    the terminal supports it.
	-emit-cdp-report
	    Write a report for each script. Defaults to the artifact directory.
	-cdp-report-dir dir
	    Write reports and their artifacts to dir.
	-emit-cdp-report-html
	    Write HTML alongside Markdown reports.
	-emit-cdp-report-combined
	    Write every report into one file. Implies -emit-cdp-report.

# Exit status

	0  all scripts passed
	1  a script failed
	2  usage error

See the cdpscripttest package for running these scripts from Go tests.
*/
package main
