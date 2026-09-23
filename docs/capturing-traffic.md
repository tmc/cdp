---
title: Capture network traffic
description: Record HAR and streaming captures, including authenticated sessions and full request and response bodies.
icon: network-wired
---

# Capture network traffic

Three commands in this repository capture traffic, and which one you want
depends on how much control you need over the session:

- `chrome-to-har` navigates to a URL and writes a HAR. Use it for
  one-shot, scriptable captures.
- `cdp` captures while you drive the browser, by hand or from an agent. Use it
  when getting to the traffic requires logging in, clicking through, or
  otherwise doing something a URL alone will not express.
- `churl -har` captures alongside a fetch, when the page content is what you
  are really after.

For the complete flag reference, see `go doc ./cmd/chrome-to-har`,
`go doc ./cmd/cdp`, and `go doc ./cmd/churl`. This guide covers the workflows
those flags add up to.

## A single-page capture

```bash
chrome-to-har -url https://example.com -output example.har
```

Modern pages keep loading well after the load event fires, so a capture that
stops too early is the usual reason a request you expected is missing. Against
a page that fetches `/api/items` from JavaScript after load:

```bash
chrome-to-har -headless -url http://localhost:8099/ -output demo.har
jq -r '.log.entries[] | "\(.request.method) \(.request.url) \(.response.status)"' demo.har
```

```
GET http://localhost:8099/ 200
```

One entry: the XHR never made it. Waiting for something the fetch produces
fixes it:

```bash
chrome-to-har -headless -url http://localhost:8099/ -wait-for '#done' -output demo2.har
```

```
GET http://localhost:8099/ 200
GET http://localhost:8099/api/items 200
GET http://localhost:8099/favicon.ico 404
```

`-wait-for` waits for a selector; `-wait-stable` waits for the network and DOM
to go quiet, which is what you want when you do not know a selector to wait on.
To keep the archive to what you care about, filter it with `-urls` and `-omit`,
or stop requests from being made at all with `-block`.

(The fixture server used above is the one from the
[Quickstart](/docs/quickstart).)

## Streaming instead of archiving

A HAR is written at the end of the run, which is inconvenient for a long
session and impossible for one that never ends. Both `chrome-to-har -stream`
and `cdp -harl` emit one JSON object per entry as it arrives:

```bash
chrome-to-har -url https://example.com -stream -urls 'api\.example\.com'
cdp -harl -harl-file session.har.jsonl
```

`cdp -harl` writes to `output.har.jsonl` by default. Pass `-harl-file -` to
stream to stdout, which is what you want when piping into `jq`.

`cdp -har` writes the file whether cdp launches the browser or attaches to one
already listening on the debug port. `-har-mode` selects how much it records:
`enhanced`, the default, captures roughly 3.4 KB per entry against `simple`'s
576 bytes for the same page. Reach for `chrome-to-har` when the HAR is the
deliverable rather than a sanity check.

## Authenticated sessions

This is the part that most often goes wrong. A browser `cdp` launches on its
own has an empty profile: no cookies, no session, so no access to anything
behind a login.

There are two ways to get an authenticated session, and they are not
interchangeable:

**Copy a profile.** `-use-profile` copies the named Chrome profile, with its
cookies, into a temporary directory and launches against the copy:

```bash
cdp -list-profiles
cdp -use-profile Default -har session.har -url https://example.com -interactive
```

The copy means your real profile is never written to, but also that anything
requiring a live session — a token refreshed since the copy, a device
challenge — may fail. Copying cookies out of Chrome's database needs `sqlite3`
on your `PATH` when you use `-cookie-domains`.

**Attach to a browser you already logged into.** This keeps the real session,
which is what you want for anything with a strict sign-in:

```bash
# Start the browser with debugging enabled, then log in normally.
cdp attach                      # prints attachable targets, or how to launch
cdp -remote-host localhost -remote-port 9222 -har session.har -shell
```

`cdp attach` with no browser listening prints the exact command to launch one.

## Capturing bodies

By default a capture records metadata and headers. `-full-capture` also records
complete request and response bodies, which is what you need when the answer is
in a response payload rather than in the request log:

```bash
cdp -full-capture -harl -output-dir ./capture -shell
```

Bodies make captures large. `-max-body-bytes` caps each one; the default of 0
keeps them whole.

`-output-dir` organizes output by the navigated page's domain rather than one
flat file, and saved page sources land under that domain's directory. Set
`-group-by-page=false` to group by the domain each request went to instead.

`-full-capture` also captures WebRTC activity, controlled by
`-webrtc-capture`: any of `sdp`, `datachannel`, and `ice`, or `all`, or `none`.
It defaults to `sdp,datachannel`, because ICE is noisy and rarely what you are
looking for.

## Secrets

Captures of authenticated sessions contain credentials — that is what makes
them useful and what makes them dangerous to pass around. HAR and source output
is redacted by default. `-no-scrub` turns redaction off; think about where the
file is going before you use it.

## Reading a capture

A HAR is JSON, so `jq` is usually enough:

```bash
# Requests that failed
jq '.log.entries[] | select(.response.status >= 400) | .request.url' capture.har

# Everything sent to one host
jq -r '.log.entries[] | select(.request.url | contains("api.example.com"))
	| "\(.request.method) \(.request.url) \(.response.status)"' capture.har

# Headers on a specific request, when you need to replay it
jq '.log.entries[] | select(.request.url | contains("/download"))
	| .request.headers' capture.har
```

Streamed NDJSON is one entry per line, so the same filters work with
`jq -c` over the stream, without waiting for the run to finish.

## Next steps

- [Differential capture](/docs/differential-capture) — comparing two captures of
  the same page.
- [How cdp works](/docs/how-cdp-works) — why capture stops when the tool stops, and
  why the two authentication paths differ.
- [Troubleshooting](/docs/troubleshooting) — missing requests and logged-out
  sessions.
