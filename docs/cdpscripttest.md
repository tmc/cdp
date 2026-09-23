---
title: cdpscripttest
description: Run browser scripts under go test — as cdpscript archives through RunCDPScript, or as rsc.io/script fixtures through Test.
icon: flask
---

# cdpscripttest

`cdpscripttest` is the testing surface. Its point is that a script you already
have becomes a test without being rewritten: the archive that reproduced a bug
from the shell is the archive that fails the build when the bug returns.

Browser-backed tests sit behind a build tag, so an ordinary `go test ./...`
needs no browser at all:

```bash
go test -tags cdp -p 1 -parallel 1 ./cdpscripttest
```

`-parallel 1` matters — this package runs fixtures as parallel subtests, and
`-p 1` serializes packages rather than those subtests.

## Where to go next

| You want to | Page |
|---|---|
| Use this from your own module | [Adopting cdpscripttest](/docs/cdpscripttest/adopting) |
| Write fixtures, and pick the right dialect | [Writing fixtures](/docs/cdpscripttest/fixtures) |
| Compare screenshots across runs | [Visual testing](/docs/cdpscripttest/visual) |
| Add app-specific commands, own the server | [Extending the engine](/docs/cdpscripttest/extending) |
| Turn a run into a readable artifact | [Reports and artifacts](/docs/cdpscripttest/reports) |
| Build a product with an agent driving the browser | [Agentic workflows](/docs/cdpscripttest/agentic) |

## The shortest useful test

```go
func TestLogin(t *testing.T) {
	srv := httptest.NewServer(handler)
	defer srv.Close()

	err := cdpscripttest.RunCDPScript(t.Context(), "testdata/login.txtar",
		cdpscripttest.CDPScriptRunOptions{
			Headless: true,
			Env:      []string{"FIXTURE_BASE_URL=" + srv.URL},
		})
	if err != nil {
		t.Fatal(err)
	}
}
```

`RunCDPScript` is the path for `main.cdp` archives — the dialect `cdpscript`
runs — and it executes them through that same runtime, so the test and the
command cannot drift apart.

Passing a fixture server's address through `Env` is the normal way a script
reaches a page: there is no variable holding the archive's temporary directory,
so a script cannot `goto` a file embedded beside it.

## Two dialects, and the one that bites

`Test` and the `cdpscripttest` **command** run a different dialect from
`RunCDPScript`. Their scripts live in the txtar *comment* section and use
`navigate`, `title`, `eval`, `stdout`, `cmp`, and the `rtc-*` commands, with
`rsc.io/script` syntax: `!` expects failure, `?` allows it, `[cond]` guards a
line.

Handed a `main.cdp` archive, the `cdpscripttest` command finds an empty comment
section, runs nothing, and prints `ok`. A falsified assertion still passes. Use
`RunCDPScript` for those archives and keep the command for comment-section
fixtures. [Writing fixtures](/docs/cdpscripttest/fixtures) shows both side by
side.

## Fixtures should not need the network

A test that depends on a third-party site fails for reasons that have nothing
to do with the code. Serve what the script needs from the test, and keep
workflows against real sites as explicitly live-only examples.
[Extending the engine](/docs/cdpscripttest/extending) shows how to make the
fixture own its server so this is automatic.

## Next steps

- [Adopting cdpscripttest](/docs/cdpscripttest/adopting) — using it from another module.
- [Tutorial: your first script](/docs/tutorial-first-script) — write a script, break it, run it as a test.
- [cdpscript](/docs/scripting) — the script format and the dialect split.
- [Testing](/docs/testing) — running this repository's own suite.
- `go doc ./cdpscripttest` — the command set, conditions, and every option.
