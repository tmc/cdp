---
title: "Tutorial: your first script"
description: Build a browser automation script from scratch, run it, make it fail on purpose, and turn it into a Go test.
icon: graduation-cap
---

# Tutorial: your first script

By the end of this you will have written a script that drives a browser,
asserted something about the page, watched the assertion fail and fixed it, and
run the same script as a Go test. It takes about ten minutes.

You need `cdpscript` installed and a Chromium-based browser. See the
[Quickstart](/docs/quickstart) if you have not done that yet.

## 1. Write the script

A script is a [txtar](https://pkg.go.dev/golang.org/x/tools/txtar) archive: a
plain text file holding one or more named files. The one named `main.cdp` is
the script; anything else is a fixture written to disk before the run.

Create `greet.txtar`:

```
-- main.cdp --
goto data:text/html,%3Ctitle%3EGreeting%3C/title%3E%3Ch1%3EHello,%20cdp%3C/h1%3E
wait h1
assert text h1 'Hello, cdp'
title
screenshot greet.png
```

Five commands: load a page, wait for an element, assert its text, print the
page title, save a screenshot.

The URL is percent-encoded for a reason. Script arguments split on whitespace,
so a URL containing a literal space is truncated at the space and you silently
load a different page than you meant to. `%20` avoids that.

## 2. Run it

```bash
cdpscript -headless -o . greet.txtar
```

```
Greeting
Saved screenshot to greet.png (4699 bytes)
```

`-o .` puts artifacts in the current directory; `-headless` keeps the window
hidden. Drop `-headless` to watch it happen — useful when a script does
something you did not expect.

The byte count depends on your fonts and screen, so expect a different number.

## 3. Make it fail

Assertions are the point of a script; a script that cannot fail is not testing
anything. Change the expected text to something wrong:

```
assert text h1 'Goodbye, cdp'
```

```
cdpscript: greet.txtar:3: assert text h1 'Goodbye, cdp': assertion failed:
text "Hello, cdp" does not contain "Goodbye, cdp"
```

```bash
echo $?
```

```
3
```

The failure names the file, the line, the command, and both sides of the
comparison. Exit status 3 means specifically "an assertion failed", as
distinct from 1 for a broken script and 2 for a usage error — so a shell
caller can tell "the page was wrong" from "the script was wrong".

Put the correct text back before continuing.

## 4. Carry a fixture in the archive

The other files in the archive are written to a temporary directory that
becomes the script's working directory, so commands that take a filename can
use a plain relative path. Create a second archive, `helper-demo.txtar`,
leaving `greet.txtar` as it is:

```
-- main.cdp --
goto data:text/html,%3Ch1%3EX%3C/h1%3E
jsfile helper.js
assert text h1 'patched'

-- helper.js --
document.querySelector('h1').textContent = 'patched';
```

```bash
cdpscript -headless helper-demo.txtar
```

```
patched
```

This is how `jsfile`, `upload`, and the other file-taking commands reach
fixtures without depending on anything outside the archive. Note that it works
for *filenames*, not URLs — there is no variable holding the temporary
directory, so you cannot `goto` an embedded HTML file. To test against real
pages, serve them, which is what the test harness in the next step does for
you.

## 5. Run it as a Go test

The same archive runs under `go test` through the `cdpscripttest` package, so
a script that reproduces a bug becomes the regression test for it without
being rewritten.

You have been working in a bare directory, so make it a module first. Fetch
`cdpscripttest` by its own path — fetching `github.com/tmc/cdp` alone leaves
the test without the dependencies that package needs:

```bash
go mod init tutdemo
go get github.com/tmc/cdp/cdpscripttest@latest
```

Then save this beside `greet.txtar`, as `greet_test.go`:

```go
//go:build cdp

package tutdemo_test

import (
	"testing"

	"github.com/tmc/cdp/cdpscripttest"
)

func TestGreet(t *testing.T) {
	err := cdpscripttest.RunCDPScript(t.Context(), "greet.txtar",
		cdpscripttest.CDPScriptRunOptions{})
	if err != nil {
		t.Fatal(err)
	}
}
```

```bash
go test -tags cdp -p 1 -parallel 1 -run TestGreet ./...
```

```
ok  	tutdemo	1.842s
```

The `tutdemo` in that line is the module name you passed to `go mod init`.

The `cdp` build tag keeps browser-dependent tests out of an ordinary
`go test ./...`. `-p 1` stops several packages from launching browsers at once;
`-parallel 1` also serializes fixtures inside one package. See
[Testing](/docs/testing) for why that matters.

For a suite of scripts rather than one, loop over a glob and call
`RunCDPScript` for each. If your fixtures need a server, start one and pass its
address in `Env` as `FIXTURE_BASE_URL` — nothing sets that variable for you.
See [cdpscript](/docs/scripting), which also explains why the
`cdpscripttest` *command* must not be used on this kind of archive.

## Next steps

- [cdpscript](/docs/scripting) — the full command set, arguments, and
  the debugging workflow for a script that misbehaves.
- [Capture network traffic](/docs/capturing-traffic) — record what the page requested
  while your script drove it.
- [How cdp works](/docs/how-cdp-works) — why scripts deliberately have no loops.
