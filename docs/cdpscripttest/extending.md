---
title: Extending the engine
description: Add app-specific commands, let fixtures own the server they test, and run scripts sequentially when they share mutable state.
icon: puzzle-piece
---

# Extending the engine

`Engine` embeds `*script.Engine`, so anything your fixtures repeat can become a
command instead. This is where a fixture suite stops being a pile of scripts and
starts being a language for your application.

```go
eng := cdpscripttest.NewEngine()
eng.Cmds["start-server"] = startServerCmd()
eng.Cmds["reset-state"] = resetStateCmd()
eng.Cmds["sign-in"] = signInCmd()
```

A fixture then opens with three words instead of twenty lines:

```text
start-server
reset-state
sign-in
navigate /app
wait-visible '.dashboard'
```

`NewEngine` is test-only — `DefaultConds` calls `testing.Short()`, which panics
outside a test binary. Use `NewCLIEngine` for standalone tools.

## Writing a command

`cdpscripttest.CDPState(s)` recovers the CDP state from the `*script.State`, and
`RunWithWaitTimeout(cs, name, readySelector, timeout, actions...)` runs chromedp
actions and waits for a readiness selector afterwards.

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
```

Return `script.ErrUsage` for bad arguments so the failure reads like every other
command's. The outer function validates and the returned `WaitFunc` does the
work, which is what lets the engine log the command before it runs.

`State` exposes `BaseURL`, `CDPCtx`, `Headless`, `ScreenshotDir`, `WaitTimeout`,
and the `rtc-*` bookkeeping — see `go doc ./cdpscripttest State`.

## Let the fixtures own the server

The rule that fixtures should not depend on third-party network is easy to
state and easy to violate by accident, because "the dev server is already
running" is true on your machine. Make it structurally true instead: a command
that checks whether `$BASE_URL` answers and starts one if not.

```go
func startServerCmd() script.Cmd {
	return script.Command(
		script.CmdUsage{Summary: "ensure the demo server is running"},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			baseURL, _ := s.LookupEnv("BASE_URL")
			url, err := ensureServer(baseURL) // build, pick a free port, poll health
			if err != nil {
				return nil, fmt.Errorf("start-server: %w", err)
			}
			return func(*script.State) (string, string, error) {
				return url + "\n", "", nil
			}, nil
		},
	)
}
```

Pick the port with `net.Listen("tcp", "127.0.0.1:0")` rather than hardcoding
one, start the server at most once with a `sync.Once`, and poll a health
endpoint until it answers before returning. A `reset-state` command that clears
accumulated state between scripts pairs with it.

**Set `no-proxy-server`.** An exported `HTTP_PROXY` makes Chrome route even
localhost requests through a proxy that cannot reach your test server, and the
failure presents as though the server never started:

```go
opts = append(opts,
	chromedp.Flag("no-proxy-server", true),
	chromedp.WindowSize(1280, 800),
)
```

## Running fixtures sequentially

`Test` calls `t.Parallel()` for every fixture, so they run concurrently against
one browser. That is right when fixtures are independent and wrong when they
share mutable server state — one script's writes become another's flake.

**`-p 1` does not fix this.** It serializes *packages*, not the subtests inside
one. Use `-parallel 1` to serialize test fixtures, or reach for `RunFiles`,
which runs files in order and owns the allocator lifecycle:

```go
files, err := cdpscripttest.ExpandGlobs([]string{"testdata/cdp/*.txt"})
if err != nil {
	t.Fatal(err)
}

res, err := cdpscripttest.RunFiles(t.Context(), eng, files, cdpscripttest.RunOptions{
	BaseURL:     srv.URL,
	ArtifactDir: artifacts,
	EmitReport:  true,
	OnResult: func(sr cdpscripttest.ScriptResult) {
		if sr.Err != nil {
			t.Errorf("%s: %v", sr.File, sr.Err)
		}
	},
})
if err != nil {
	t.Fatal(err)
}
t.Logf("%d passed, %d failed", res.Passed(), res.Failed())
```

`RunOptions` also carries `AllocatorOpts` (defaulting to headless) and `Env`.
`ExpandGlobs` accepts several patterns and handles `...`-style recursion, which
plain `filepath.Glob` — what `Test` uses — does not.

For a single script against a `State` you already built, `Run(t, e, s, filename,
r)` is the smallest entry point.

### Rolling your own loop

Only worth it when you need something `RunOptions` does not expose — a shared
browser context across scripts, custom per-script report handling, or
`ErrSkip`/`ErrStop` mapped to your own semantics:

```go
for _, file := range files {
	name := strings.TrimSuffix(filepath.Base(file), ".txt")
	t.Run(name, func(t *testing.T) {
		tabCtx, cancel := chromedp.NewContext(browserCtx)
		t.Cleanup(cancel)

		workdir := t.TempDir()
		s, err := cdpscripttest.NewStateWithArtifactDir(tabCtx, workdir, baseURL,
			filepath.Join(artifactRoot, name), env)
		if err != nil {
			t.Fatal(err)
		}

		a, err := txtar.ParseFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.ExtractFiles(a); err != nil {
			t.Fatal(err)
		}

		log := new(strings.Builder)
		err = eng.Execute(s.State, file, bufio.NewReader(bytes.NewReader(a.Comment)), log)
		switch {
		case err == nil:
		case errors.Is(err, cdpscripttest.ErrSkip):
			t.Skip(err)
		case errors.Is(err, cdpscripttest.ErrStop):
		default:
			t.Errorf("FAIL: %v", err)
		}
	})
}
```

One browser, a fresh tab per script. Handling `ErrSkip` as `t.Skip` and
`ErrStop` as success is what makes the `skip` and `stop` commands mean what they
say — treat them as failures and fixtures that deliberately bail will look
broken.

## Next steps

- [Reports and artifacts](/docs/cdpscripttest/reports) — capturing what a run did.
- [Agentic workflows](/docs/cdpscripttest/agentic) — why this shape suits agent-driven development.
- `go doc ./cdpscripttest` — `Engine`, `State`, and the default command set.
