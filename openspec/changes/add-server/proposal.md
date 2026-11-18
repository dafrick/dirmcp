## Why

Claude sessions without a shell (claude.ai on a phone or the web) cannot reach files on our machines. dirmcp gives such a client shell-like access to one directory, using the real Unix tools the model already knows, without letting it run anything else.

## What Changes

- `dirmcp` binary: an MCP server over Streamable HTTP (official go-sdk), bearer token from a file.
- Tool `sh`: runs a command line (pipes, `for` loops, `$( )`, redirections) through the `mvdan/sh` interpreter. Only programs from an allowlist run, each with an allowlist of flags; dangerous flags are not on it, and `sed`/`gawk` always run in their `--sandbox` modes. `rm` is built in and moves to `.trash/`.
- Every program starts confined by Landlock: reads only the served directory and its own Nix store closure, writes only when the server is started writable, executes only allowlisted binaries, no network. Plus rlimits and a hard time limit.
- Tool `more`: long or still-running output is kept in RAM for a while and paged in chunks.
- Tool `create` (only with `--create-in`): writes a new file, never overwrites.
- Nix package with the tool allowlist resolved to store paths at build time.
- Bump to `0.2.0`.

Out of scope (next change): NixOS module with instances and systemd hardening, the README section "Deploying securely", downloading binary files.

## Capabilities

### New Capabilities
- `endpoint`: transport, auth, request limits, logging.
- `shell-tool`: the `sh` tool, program and flag allowlists, built-ins.
- `confinement`: what a started program can read, write, execute and connect to.
- `output-paging`: truncation and the `more` tool.
- `create-tool`: additive writes.

### Modified Capabilities

## Impact

New Go code, `go.mod`, Go hooks (gofmt, go vet, tests) in the flake and CI, `packages.default` in the flake. README usage section.
