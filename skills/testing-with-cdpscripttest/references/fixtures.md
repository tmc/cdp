# cdpscripttest fixture reference

This is a working companion to `go doc ./cdpscripttest`, which is the
authoritative command list. When the two disagree, `go doc` is right.

## The two dialects

A fixture is a txtar archive either way. What differs is which runtime reads it.

| | Native fixture | cdpscript archive |
|---|---|---|
| Entry point | `cdpscripttest.Test(t, e, allocCtx, baseURL, glob, env)` | `cdpscripttest.RunCDPScript(ctx, path, opts)` |
| Body lives in | the txtar comment section | a `-- main.cdp --` section |
| Navigate | `navigate /path` (BASE_URL-relative) | `goto <url>` |
| Wait | `wait-visible <sel>` (alias `wait`) | `wait <sel>` or `wait 2s` |
| Evaluate | `eval <js>` (alias `js`), `evalfile` | `js <code>`, `jsfile <path>` |
| Assert | `stdout '<regexp>'` after a command | `assert exists\|text\|visible\|status\|response\|header` |
| Conditions | `[headless]`, `[!rtc]`, `[element:#x]`, `[exec:jq]` | `[cond]` guards, `!`/`?` markers |
| Failure expected | `! eval 'throw 1'` | `! <command>` |
| Extras | `screenshot-compare`, `rtc-*`, `network-emulate`, `distill`, `markdown` | the cdpscript command set, incl. `click coord:x,y` and `@ref` |
| Lives in | `cdpscripttest/testdata/*.txt` | `cdpscripttest/testdata/cdpscript/*.txtar` |

Pick `RunCDPScript` when the archive is also meant to run as a command line
tool — that path executes the same runtime as `cdpscript` and `cdp run`, so the
test cannot drift from the tool. Pick a native fixture when you want the richer
assertion and comparison surface.

## Running

```bash
go test -tags cdp -p 1 -parallel 1 ./cdpscripttest              # all browser fixtures
go test -tags cdp -p 1 -parallel 1 -run TestCDP/login ./cdpscripttest
go test -tags cdp -p 1 -parallel 1 -v ./cdpscripttest           # one subtest line per fixture
```

Without `-tags cdp` the browser-backed fixtures are compiled out. A green run
without the tag says nothing about them.

`-parallel 1` matters: independent browser instances contend for resources,
and the resulting failures look like regressions. `-p 1` serializes packages,
not fixture subtests.

## Environment and flags

| Name | Effect |
|---|---|
| `WORK` | the test's working directory (set for scripts) |
| `TMPDIR` | temp dir, cleaned up after the test |
| `BASE_URL` | the base URL passed to `Test` (read-only; change with `set-base-url`) |
| `SCREENSHOT_DIR` | override the screenshot output directory |
| `CDPSCRIPTTEST_ARTIFACTS` | artifact root, bypassing `t.ArtifactDir()`; flat `<dir>/<script-name>/` layout |
| `UPDATE_GOLDEN` | overwrite baselines instead of comparing |
| `-cdp-artifacts=<dir>` | flag form of `CDPSCRIPTTEST_ARTIFACTS` |
| `-emit-artifacts` | write to `<script-dir>/artifacts/<script-name>/` |
| `-update-golden` | flag form of `UPDATE_GOLDEN` |
| `-cdp-skip-blur` | disable `--blur` processing, show raw content |
| `-cdp-emit-unblurred` | save an unmasked copy alongside each blurred capture |
| `-emit-cdp-report` | write `report.md` into the artifact directory |
| `-emit-cdp-report-combined` | merge all reports into one file |
| `-cdp-report-dir=<dir>` | write detailed reports and optional indexes below `<dir>` |
| `-emit-cdp-report-html` | write `report.html` and, with combined output, `index.html`; requires `-cdp-report-dir` |
| `CDPSCRIPTTEST_COVERAGE` | `0`/`false`/`off` disables coverage (on by default) |

Environment variables take precedence over their flag counterparts. Register
the flags by calling `flag.Parse` in `TestMain`.

## Screenshot comparison

```text
screenshot-compare --threshold 3 --blur '.timestamp' '#main' main.png
```

- On first run the capture *becomes* the baseline. A new comparison always
  passes — open the image before you trust it.
- `--blur <selector>` applies a 10px CSS blur filter to matching elements, so
  the text is unreadable but its presence still shows. Different strings can
  still differ by a few pixels, so a small `--threshold` may be needed.
  Repeatable.
- `--threshold N` is max allowed diff percent (default 5). Raising it to absorb
  churn also absorbs regressions — blur the churn instead.
- `screenshot-sel` captures one element; `screenrecord start|stop` writes GIF,
  PNG, numbered PNG frames, or WebM. WebM requires `ffmpeg` in `PATH`; the
  other formats have no external dependency. A script that fails mid-recording
  still leaves the artifact path in the command log.

## Network emulation

```text
network-emulate --latency 400 --down 51200 --up 20480 --loss 5
navigate /dashboard
wait-visible '.loaded'
network-emulate-clear
```

Flags: `--loss` (percent), `--queue`, `--reorder`, `--latency` (ms), `--down`
and `--up` (bytes/sec, `-1` for no limit). Clear it before assertions that
should not run under the emulated condition.

## WebRTC fixtures

```go
opts := append(chromedp.DefaultExecAllocatorOptions[:],
	cdpscripttest.WebRTCAllocatorOptions()...)
opts = append(opts, chromedp.Flag("headless", true))
```

```text
rtc-inject                 # must precede navigate
navigate /video-call
wait-visible '#status'
rtc-wait connected 60s
rtc-stats-video --direction outbound
stdout 'framesSent'
```

`rtc-inject` installs an `RTCPeerConnection` monkey-patch via
`Page.addScriptToEvaluateOnNewDocument`, so it only takes effect for documents
loaded *after* it runs. Without it, `rtc-state`, `rtc-peers`, and `rtc-events`
fail outright — `testdata/rtc-negative-no-inject.txt` pins exactly that with
`!` markers. Injecting twice is safe; the helper guards on
`__cdpst_rtc_initialized`. Use `[!rtc] skip 'WebRTC not injected'` when a
fixture should degrade rather than fail.

Headless loopback ICE can stall. Prefer asserting on SDP and peer state over
asserting that data-channel messages were delivered.

## Patterns for a whole-application suite

**Own the server the fixtures talk to.** A `start-server` command checks
whether `$BASE_URL` answers; if not it builds the binary, picks a free port
with `net.Listen("tcp", "127.0.0.1:0")`, starts the server, and polls a health
endpoint until ready. Fixtures then never depend on someone having started
something. A `reset-state` command that POSTs to a test-only reset endpoint so
each script starts clean pairs with it.

**Collapse multi-step setup into one command.** Instead of six lines of login
in every fixture:

```go
func signInCmd() script.Cmd {
	return script.Command(
		script.CmdUsage{Summary: "dev-mode login, wait for app shell"},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 0 {
				return nil, script.ErrUsage
			}
			cs, err := cdpscripttest.CDPState(s)
			if err != nil {
				return nil, err
			}
			return func(*script.State) (string, string, error) {
				out, err := cdpscripttest.RunWithWaitTimeout(cs, "sign-in", ".app-shell", 0,
					chromedp.Navigate(cs.BaseURL()+"/session/new"),
					chromedp.Navigate(cs.BaseURL()+"/app"),
					chromedp.WaitVisible(".app-shell", chromedp.ByQuery),
				)
				return out, "", err
			}, nil
		},
	)
}

eng := cdpscripttest.NewEngine()
eng.Cmds["sign-in"] = signInCmd()
```

Return `script.ErrUsage` for bad arguments so the failure reads like every
other command's.

**Allocator options that matter for local servers.**

```go
opts := chromedp.DefaultExecAllocatorOptions[:]
opts = append(opts,
	chromedp.ExecPath(browserExecPath()),      // your lookup; cmd/cdpscripttest reads CDP_BROWSER
	chromedp.Flag("no-proxy-server", true),    // HTTP_PROXY otherwise breaks localhost
	chromedp.WindowSize(1280, 800),            // stable width for screenshot baselines
)
```

`no-proxy-server` is the non-obvious one: an exported `HTTP_PROXY` sends
Chrome's localhost requests through a proxy that cannot reach the test server,
and the failure looks like the server never started.

**Run scripts sequentially.** `Test` calls `t.Parallel()` per fixture; use
`-parallel 1` because `-p 1` serializes packages, not subtests. Prefer
`RunFiles(ctx, eng, files, RunOptions{...})` with `ExpandGlobs` — it runs files
in order, owns the allocator, and reports per-file results through `OnResult`. Roll your own loop
only for something `RunOptions` does not expose:
one `chromedp.NewContext(browserCtx)` tab per script, `t.TempDir()` as the
workdir, `NewStateWithArtifactDir(tabCtx, workdir, baseURL, artifacts, env)`,
`s.ExtractFiles(a)` for the txtar sections, then `eng.Execute(s.State, file,
bufio.NewReader(bytes.NewReader(a.Comment)), logBuf)`.

Treat `ErrSkip` as `t.Skip`, `ErrStop` as success, anything else as failure —
that is what makes the `skip` and `stop` commands mean what they say.

**Reports.** `GenerateReport(path, name, a.Comment, log)` per script;
`NewCombinedReportWriter(path, names, sources)` plus `Update(ScriptReport{...})`
for one merged report updated as each script finishes, so a crashed run still
leaves something readable. For native Markdown and HTML reports, set
`RunOptions.Report` to `&report.Options{Dir: root, HTML: true, Combined: true}`.
The writer emits `index.md`, `index.html`, and per-script reports beside the
existing artifacts; no converter or asset-copy step is needed.

**Redact for shared reports.** `GenerateReport` embeds the script source and
the execution **log**, not the environment — but anything a fixture prints lands
in the log. Do not print secrets from a fixture, and filter keys containing
`TOKEN`, `SECRET`, `PASSWORD`, `COOKIE`, `PRIVATE_KEY`, or ending in
`_KEY`/`_API` down to `[redacted]` when the report will be shared.

**Committed baselines.** Keep them under `testdata/` in the repo and regenerate
deliberately with `go test -update-golden -tags cdp ...`. An artifact root that
the run clears cannot accumulate baselines — every comparison becomes a first
run, and first runs always pass.

**Coverage appears whether you asked or not.** Native execution starts CDP
coverage by default and writes `coverage.json` into each artifact directory.
`CDPSCRIPTTEST_COVERAGE=0` disables it.

## Common failures

| Symptom | Cause |
|---|---|
| Suite passes, nothing ran | glob matched no files, or `-tags cdp` missing. `Test` fails on an empty glob — check the tag first |
| Passes locally, fails in CI | headed/headless rendering difference, or missing fonts; compare the artifact images, do not raise the threshold |
| Flaky only in a full-repo run | package parallelism; re-run with `-p 1`, and add `-parallel 1` for fixtures |
| `coverage.json` appears unbidden | coverage is on by default; `CDPSCRIPTTEST_COVERAGE=0` |
| Every navigate fails against a local server | `HTTP_PROXY` is exported; add `chromedp.Flag("no-proxy-server", true)` |
| Scripts interfere with each other | `Test` runs fixtures in parallel; use `-parallel 1`, or switch to `RunFiles` and reset server state per script |
| `rtc-*` commands fail | `rtc-inject` was never called, or ran after `navigate` |
| Baseline drift after an intentional UI change | update deliberately with `-update-golden`, then look at every changed image before committing |
