---
title: cdpscript
description: Write txtar automation scripts, pass them arguments, run them as Go tests, and debug the ones that fail.
icon: file-code
---

# cdpscript

New to scripts? Start with the [tutorial](/docs/tutorial-first-script), which builds
one from scratch. This page is the working reference for the workflow.

A script is a txtar archive containing a `main.cdp` file whose commands run in
order against a browser. The same archive runs three ways: from the command
line with `cdpscript`, as part of a `cdp` session with `cdp run`, and as a Go
test through the `cdpscripttest` package. That last one is the point — a script
that reproduces a bug becomes the regression test for it without being
rewritten.

For the command set and syntax, see `go doc ./cdpscript` and
[skills/writing-cdp-scripts/references/script-format.md](https://github.com/tmc/cdp/blob/main/skills/writing-cdp-scripts/references/script-format.md).
For the testing commands and conditions, see `go doc ./cdpscripttest`.

## Running a script

```bash
cdpscript script.txtar
cdpscript -o ./artifacts -headless script.txtar
```

Artifacts such as screenshots and PDFs go to `-o`, and the exit status
distinguishes a failed assertion (3) from a broken script (1) and a usage error
(2), so a shell caller can tell "the page was wrong" from "the script was
wrong". An interrupted run exits 130.

Values containing spaces must be quoted, and URLs containing spaces must be
percent-encoded — arguments split on whitespace, and an unquoted URL is
truncated at the first space rather than rejected.

## Scripts are executables

A script can carry a shebang line and run as a command in its own right. The
line lives above the first `-- file --` marker, in the txtar comment section,
so it is part of the archive rather than something stripped before running:

```
#!/usr/bin/env cdpscript
# greet - print the title of a page
#
# usage: greet.cdpscript URL

-- main.cdp --
goto $ARG1
title
```

```bash
chmod +x greet.cdpscript
./greet.cdpscript https://example.com
```

```
Example Domain
```

`env` resolves `cdpscript` through `PATH`, so this needs `$(go env GOPATH)/bin`
on your `PATH` — the [quickstart](/docs/quickstart) sets that up. Without it the
shebang fails with `env: cdpscript: No such file or directory`, which does not
obviously point back at the install step.

Name the file `.cdpscript` rather than `.txtar` when it is meant to be run this
way. The rest of the comment section is the script's help text, so
`./greet.cdpscript --help` prints the name, the description, and the usage line
without running the browser. Combined with argv and exit status, this is what
makes a script an ordinary Unix tool: it can be dropped on `$PATH`, piped, and
branched on.

To pass flags to `cdpscript` itself, use `env -S`, which splits the rest of the
shebang line into separate arguments:

```
#!/usr/bin/env -S cdpscript -headless
```

Verified on darwin. A plain `#!/usr/bin/env cdpscript` line is portable; `-S`
needs a `env` that supports it (GNU coreutils 8.30+, and the BSD `env` on
macOS).

## Fixtures travel with the script

Because a txtar archive holds several files, a script can carry what it needs.
The extra files are written to a temporary directory that becomes the script's
working directory, so file-taking commands such as `jsfile` and `upload` reach
them by relative name. The
[tutorial](/docs/tutorial-first-script) works through an example.

That covers filenames, not URLs: there is no variable holding the temporary
directory, so a script run by `cdpscript` cannot `goto` an embedded HTML file.

The repository's own fixtures reach a served page through `$FIXTURE_BASE_URL`:

```
-- main.cdp --
goto $FIXTURE_BASE_URL/interaction/pages/form-submit.html
wait '#signup-form'
fill '#name' Ada Lovelace
click '#submit'
wait '#result'
```

That is an ordinary environment variable, not a feature: the Go test that runs
these scripts starts a server and passes the address in `Env` (see below).
Under a plain `cdpscript` run it is unset, which turns the `goto` into an
invalid URL — see [Troubleshooting](/docs/troubleshooting).

A self-contained script needs no network, which is what makes it usable as a
test. Scripts that depend on a third-party site belong in the live examples,
not in the fixture suite.

## Arguments and environment

Scripts take positional arguments as `$ARG1`, `$ARG2`, and so on, with `$ARGC`
holding the count, and can read environment variables:

```
# usage: search.txtar <query>
-- main.cdp --
goto https://example.com/search?q=$ARG1
wait '#results'
```

## Control flow stays outside

The syntax comes from `rsc.io/script`: commands run in sequence, `!` expects
failure, `?` allows failure, and `[cond]` guards a command on a condition.

There are deliberately no loops or retries. Anything needing real control flow
belongs in the shell or the Go test that calls the script, rather than in a
second language embedded in the script format.

## Two archive dialects

This is the thing to get straight before running anything, because getting it
wrong produces a passing result rather than an error.

- **`main.cdp` archives** — the dialect this page describes, run by
  `cdpscript` and, from Go, by `cdpscripttest.RunCDPScript`.
- **Comment-section archives** — an older dialect where the script body is the
  txtar *comment* and the commands are `navigate`, `rtc-*`, and similar. These
  are what the `cdpscripttest` *command* and `cdpscripttest.Test` run;
  `cdpscripttest/testdata/rtc-*.txt` are examples.

The `cdpscripttest` command executes only the comment section. Handed a
`main.cdp` archive it finds an empty comment, runs nothing, and prints `ok`:

```bash
sed 's/Saved Ada/DEFINITELY WRONG/' testdata/interaction/form-submit.txtar > bogus.txtar
cdpscripttest -v bogus.txtar
```

```
    using browser: /Applications/Brave Browser.app/…
=== bogus.txtar
ok  bogus.txtar
```

The assertion was falsified and it still passed, because no command ran. Do not
use the `cdpscripttest` command to check a `main.cdp` archive.

## Running scripts as tests

Run `main.cdp` archives with `RunCDPScript`, supplying the environment
yourself. `$FIXTURE_BASE_URL` is not set for you — it is an ordinary variable
that the caller puts in `Env`, which is how the shipped fixtures reach their
test server (`cdpscripttest/interaction_test.go`):

```go
baseURL := startTestServer(t)

opts := cdpscripttest.CDPScriptRunOptions{
	Headless: true,
	Timeout:  20 * time.Second,
	Env: []string{
		"FIXTURE_BASE_URL=" + baseURL,
	},
}
if err := cdpscripttest.RunCDPScript(t.Context(), path, opts); err != nil {
	t.Fatalf("%s: %v", path, err)
}
```

`cdpscripttest.Test` is the entry point for the other dialect; it does not
start a server and does not set `$FIXTURE_BASE_URL`.

Browser fixtures are behind the `cdp` build tag so an ordinary `go test ./...`
does not need a browser:

```bash
go test -tags cdp -p 1 -parallel 1 ./cdpscripttest
```

See [Testing](/docs/testing) for why `-parallel 1` matters for fixtures.

## Working on a script that fails

For a `main.cdp` archive, use `cdpscript` — it runs the script body and reports
real failures:

```bash
# Watch it happen
cdpscript greet.txtar

# Verbose, with artifacts kept
cdpscript -v -o ./artifacts -headless greet.txtar
```

Drop `-headless` to see the browser. An assertion failure names the file, the
line, and both sides of the comparison, and exits 3.

The `cdpscripttest` command's `-i`, `-headful`, `-watch`, `-images`, and
`-update-golden` options apply to comment-section archives — the older format
used by the fixtures under `cdpscripttest/testdata/`, such as `rtc-basic.txt`.
Compare one against a `main.cdp` archive to tell which kind you have.

## Next steps

- [Tutorial: your first script](/docs/tutorial-first-script) — the guided version, if
  you skipped it.
- [Capture network traffic](/docs/capturing-traffic) — recording what the page
  requested while the script drove it.
- [Troubleshooting](/docs/troubleshooting) — the failures scripts hit most often.
- [Testing](/docs/testing) — running the fixture suite.
