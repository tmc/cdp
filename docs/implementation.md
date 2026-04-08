# Implementation Notes

This repository now contains a small public Go module plus several command-line tools built on top of shared internal packages.

## Current Shape

- `main.go`: the root `chrome-to-har` capture-oriented CLI
- `cmd/cdp`: the general Chrome/CDP CLI
- `cmd/churl`: browser-backed fetch and extraction
- `cmd/chdb`: Chrome-oriented debugger workflow
- `cmd/ndp`: Node/V8 debugger workflow
- `cmd/cdpscript` and `cmd/cdpscripttest`: script execution and testing tools
- `internal/browser`: shared browser lifecycle and interaction code
- `internal/recorder`: HAR and enhanced capture support

## What Is Settled

- Browser management is shared through internal packages instead of being reimplemented per command.
- The repository supports both capture-oriented and interactive/debugger-oriented workflows.
- HAR capture has grown beyond plain network logging and now includes injected capture for traffic CDP does not expose directly, such as gRPC-Web and WebRTC data channel events.

## What Still Needs Work

- `cmd/cdp` remains too large and mixes several concerns in one package.
- `cmd/ndp` still contains a wide surface area for REPL, session, debugger, and runtime behavior.
- Some docs and command surfaces still reflect the project’s earlier extraction history.

## Direction

The likely cleanup path is:

1. Keep the current command set small and intentional.
2. Move more command logic out of `package main` files and into focused internal packages.
3. Keep shared browser and recorder primitives reusable across commands.
4. Prefer deleting dead compatibility layers over preserving parallel implementations.
