# dirmcp

An [MCP](https://modelcontextprotocol.io) server that gives a remote AI client (for example a claude.ai custom connector) shell-like access to **one directory**, using the real Unix tools from an allowlist.

Status: foundations only. The server comes in the next change (see `openspec/changes/`).

> **Warning.** dirmcp gives whoever holds its token remote access to the directory it serves, with everything the allowed tools can do there. Read "Deploying securely" (added with the server) before you expose it.

## Planned shape

- One directory, served over Streamable HTTP with a bearer token.
- `sh`: command lines with pipes and loops, but only allowlisted programs with allowlisted flags. Nothing else runs.
- `more`: next chunk of a long or still-running output.
- `create`: new files only, never overwrites.
- Each command is confined to the directory, with no network.

## Layout

```
AGENTS.md, CLAUDE.md   agent instructions (CLAUDE.md imports AGENTS.md)
CONTRIBUTING.md        how to make changes
VERSION, CHANGELOG.md  version and changelog
flake.nix              dev shell, formatter, checks
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
