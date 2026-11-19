## Why

The owner wants the endpoint named after what it serves (agentbox: `/obsidian`), not the protocol. The path is fixed to `/mcp` today.

## What Changes

- `--path PATH` (default `/mcp`); module option `path`. Only that path answers; every other path is 404. Paths are plain: `/name[/name…]`, letters, digits, `-_.`, no trailing slash, no `..`.
- Tests: unit (`TestPath`, `TestValidPath`), VM test with `/notes` on the writable instance.
- Bump to `0.4.0` (new option).

## Capabilities

### New Capabilities

### Modified Capabilities
- `endpoint`: configurable path.
- `nixos-module`: option `path`.

## Impact

`internal/server`, `cmd/dirmcp`, `nix/module.nix`, `nix/test.nix`, README, CHANGELOG, VERSION. Default unchanged.
