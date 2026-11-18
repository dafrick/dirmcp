# dirmcp

An [MCP](https://modelcontextprotocol.io) server that gives a remote AI client (for example a claude.ai custom connector) shell-like access to **one directory**, using the real Unix tools from an allowlist.

Status: the server works (`sh`, `more`, `create`). A NixOS module with hardened instances comes next.

> **Warning.** dirmcp gives whoever holds its token remote access to the directory it serves, with everything the allowed tools can do there. Use a long random token, keep it secret, and serve read-only unless you need writes.

## What the client gets

| Tool | What it does |
|---|---|
| `sh` | Runs a command line in the directory: pipes, `for` loops, `$( )`, redirections. Only these programs run: `awk basename cat cmp comm cp cut date df diff dirname du file find fmt fold gawk grep head join ls md5sum mkdir mv nl paste readlink realpath rg sed sha256sum sort stat tac tail touch tr tree uniq wc`, plus a built-in `rm` that moves to `.trash/`. Most options work; options that run other programs or write elsewhere are refused (`find -exec`, `rg --pre`, `sort --compress-program`, …), and `sed`/`gawk` always run with `--sandbox`. No shells, `xargs` or `env`. |
| `more` | Next part of an answer that was cut at 30 KB or still running after 15 s. Output is kept in RAM for 10 minutes. |
| `create` | Only with `--create-in DIR`: makes a new file below `DIR`, never overwrites. |

Every started program runs under [Landlock](https://landlock.io): it can read only the directory and the tools' own files, write only when the server is started with `--writable`, start only the allowed programs, and use no network. It also gets CPU, memory and file-size limits and is killed after 5 minutes. Linux 6.12 or newer is needed (Landlock ABI 6); the server refuses to start otherwise.

## Usage

```sh
nix build github:dafrick/dirmcp        # or: nix run github:dafrick/dirmcp -- …
openssl rand -hex 32 > token && chmod 600 token
./result/bin/dirmcp --root ~/notes --token-file token --create-in Inbox
```

| Flag | Default | |
|---|---|---|
| `--root DIR` | required | the directory to serve |
| `--token-file FILE` | required | bearer token, ≥ 32 bytes, mode 0600 or 0400 |
| `--listen ADDR` | `127.0.0.1:8090` | MCP endpoint is `http://ADDR/mcp` (Streamable HTTP) |
| `--writable` | off | programs may change files |
| `--create-in DIR` | none | enable `create` below `DIR` (repeatable) |
| `--tools FILE` | set by the package | tools manifest: program → store path |

Clients send `Authorization: Bearer <token>`. For a claude.ai custom connector, put the URL of a public HTTPS proxy in front of `/mcp` and the header under "Request headers". The binary must come from the Nix package: it pins every tool to a store path and never searches `PATH`.

## Layout

```
AGENTS.md, CLAUDE.md   agent instructions (CLAUDE.md imports AGENTS.md)
SECURITY.md            how to report a vulnerability
CONTRIBUTING.md        how to make changes
VERSION, CHANGELOG.md  version and changelog
flake.nix              dev shell, checks, package (with the tools manifest)
cmd/dirmcp/            main: flags, startup checks, HTTP server
internal/              argcheck (option allowlists), confine (Landlock helper),
                       shell (interpreter), jobs (paging), create, server, manifest
docs/adr/              architecture decision records
openspec/              OpenSpec specs and changes
scripts/               small helper scripts (release check)
.github/               CI, Dependabot
```

## Getting started

1. Install Nix ([nixos.org/download](https://nixos.org/download)) and enable flakes.
2. Enter the dev shell: `nix develop`. It provides the tools (Go, `openspec`, `gh`) and installs the git hooks.
3. Run all checks:
   ```sh
   nix flake check                                     # pre-commit hooks
   pre-commit run --all-files --hook-stage pre-push    # pre-push hooks
   ```

Then read [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT, see [LICENSE](LICENSE).
