# cdp for Codex

Run the `cdp` Chrome DevTools Protocol toolkit as an MCP server in
[Codex](https://developers.openai.com/codex/).

## Prerequisite

Install the binary (Go 1.26+) and make sure your Go bin directory is on `PATH`:

```bash
go install github.com/tmc/cdp/cmd/cdp@latest
```

## Install

Add the server with the Codex CLI:

```bash
codex mcp add cdp -- cdp -mcp -headless
```

Or append the block in [`config.toml`](config.toml) to `~/.codex/config.toml`:

```toml
[mcp_servers.cdp]
command = "cdp"
args = ["-mcp", "-headless"]
```

Restart Codex to load the server.

## Options

`cdp -mcp` accepts the same flags as the CLI. Useful ones for the `args` list:

- drop `-headless` to watch Chrome while Codex drives it;
- `-output-dir <dir>` to persist HAR/HARL captures and saved page sources;
- `-enable-inspect` to expose the reversing/inspection tools;
- `-tools-dir <dir>` to register custom `.cdp` tool definitions;
- `-no-scrub` to disable secret redaction (on by default).

`cdp` accepts both `-flag` and `--flag`. Run `cdp -help` for the full list.
