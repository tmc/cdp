# CDP Script Format

CDP scripts are txtar archives executed by `cdpscript` or `cdp run`.
The implemented command surface is the one registered in `cdpscript/engine.go`.

## Running Scripts

```bash
# Build either entry point.
go build -o cdp ./cmd/cdp
go build -o cdpscript ./cmd/cdpscript

# Run a script.
cdp run script.txtar
cdpscript script.txtar
cdpscript script.txtar one two

# Common options.
cdp run -v -o /tmp/output script.txtar
cdpscript --tab <id> --port 9222 script.txtar
```

You can also run a no-argument script directly with a shebang:

```bash
#!/usr/bin/env cdpscript
-- meta.yaml --
name: example
description: Capture Example Domain
headless: true

-- main.cdp --
goto https://example.com
wait h1
assert text h1 Example Domain
```

`#!/usr/bin/env -S cdp run` also works on systems with `env -S`.

When `--help` appears after the script path, the runner prints the script's
name, description, and `Usage: <name> [args...]`.

## File Layout

A script is a txtar archive with `-- filename --` sections.
The engine reads:

- `main.cdp`: required
- `meta.yaml`: optional
- any other file: extracted into a temporary work directory before execution

Lines before the first file marker are ignored by the engine. That makes the
shebang form above work.

```text
#!/usr/bin/env cdpscript
-- meta.yaml --
name: my-script
description: Minimal example
headless: true
timeout: 20s
env:
  TARGET: https://example.com

-- main.cdp --
goto ${TARGET}
wait h1
title
screenshot example.png

-- helper.js --
document.title
```

`jsfile` resolves relative paths from the extracted script work directory.

## `meta.yaml`

Only these fields are read today:

```yaml
name: Script Name
description: What the script does
version: "1.0"
browser: brave
profile: "Profile 1"
headless: true
timeout: 30s
env:
  BASE_URL: "https://example.com"
  USERNAME: "test@example.com"
```

Field meanings:

- `name`: optional display name
- `description`: optional description
- `version`: optional version string
- `browser`: browser hint used during browser discovery
- `profile`: Chrome profile name to copy before launch
- `headless`: run without a visible window
- `timeout`: default timeout for browser launch and selector waits
- `env`: initial environment variables available as `${NAME}`

## Commands

### Navigation

```text
goto <url>
back
forward
reload
```

### Waiting

```text
wait <duration>
wait <selector>
```

Examples:

```text
wait 500ms
wait 2s
wait h1
wait #login-form
```

`wait` does not have separate `for` or `until` forms.

### Interaction

```text
click <selector|@ref>
fill <selector|@ref> <value>
type <selector|@ref> <value>
hover <selector>
press <key>
```

`type` is an alias for `fill`.

### JavaScript

```text
js <code>
jsfile <filename>
```

`js` executes JavaScript in the current page. `jsfile` reads a file from disk,
executes it, and prints a non-nil return value.

### Extraction and Rendering

```text
extract <selector>
title
url
render [--term] [selector]
```

Environment variables set by these commands:

- `extract` prints the extracted text and sets `${EXTRACTED}`
- `title` prints the page title and sets `${TITLE}`
- `url` prints the current URL and sets `${URL}`
- `render` prints markdown and sets `${RENDERED}`

### Assertions

```text
assert exists <selector>
assert text <selector> <expected>
assert visible <selector>
```

`assert text` checks substring containment, not exact match.

### Output

```text
screenshot <file>
pdf <file>
log <message>
```

`screenshot` captures a full-page PNG. `pdf` writes a page PDF. `log` prints to
stdout.

### Network and HAR

```text
block <pattern>
tag [name]
note <description>
capture screenshot [description]
capture dom [description]
har <file>
```

Notes:

- `block` blocks URLs matching the pattern
- the first `tag` command starts HAR recording if it is not already active
- `tag` with no argument clears the current tag and sets `${CURRENT_TAG}` to an empty string
- `note`, `capture`, and `har` require HAR recording to be active
- `har` writes the captured session to disk

### Scripting

```text
source [-x] [-as <name>] <path>
```

`source` reads another `.cdp` file from the filesystem.

- `-x`: trace each command before running it
- `-as <name>`: register the sourced file as a new command instead of running it immediately

Commands registered through `source -as` receive `${ARG1}` through `${ARGN}`
and `${ARGC}`.

### Accessibility Snapshots and Refs

```text
snapshot
snapshot -i
snapshot --compact
snapshot --depth 3
snapshot --selector #main
```

`snapshot` prints an accessibility tree and sets `${SNAPSHOT_REFS}` to the
number of generated refs. Output includes refs such as `@e1` that `click`,
`fill`, and `type` can consume.

Example:

```text
snapshot -i
# Output includes refs such as:
#   - button "Submit" [ref=e1]
#   - textbox "Email" [ref=e2]
click @e1
fill @e2 test@example.com
```

## Conditions

Scripts support these conditions:

```text
[headless] screenshot headless.png
[has-tab] log Attached to existing tab
```

## Environment Variables

Use `${NAME}` syntax in `main.cdp`:

```text
goto ${BASE_URL}
fill #email ${USERNAME}
log ${TITLE}
```

Values can come from:

- `meta.yaml` `env:`
- extracted command output such as `${EXTRACTED}`, `${TITLE}`, `${URL}`, `${RENDERED}`
- HAR and snapshot state such as `${CURRENT_TAG}` and `${SNAPSHOT_REFS}`
- script arguments such as `${ARG1}` and `${ARGC}`
- sourced-command arguments such as `${ARG1}` and `${ARGC}`

## Worked Example

```text
#!/usr/bin/env cdpscript
-- meta.yaml --
name: example-domain
description: Capture Example Domain and write artifacts
headless: true
timeout: 20s
env:
  TARGET: https://example.com

-- main.cdp --
goto ${TARGET}
wait h1
title
assert text h1 Example Domain
snapshot -i
render
screenshot example.png
log finished
```

## Current Limits

- Only the commands listed in this document are implemented by `cdpscript`
- `meta.yaml` supports only the fields shown above

For the shorter agent-facing reference, see
`skills/writing-cdp-scripts/references/script-format.md`.
