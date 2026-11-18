## Why

dirmcp runs on NixOS hosts (first: agentbox, serving an Obsidian vault to claude.ai). Every host would otherwise repeat the same unit, user, token handling and systemd hardening, and get the parts that must stay open for the sandbox (user namespaces, mount syscalls, two capabilities in the bounding set) wrong.

## What Changes

- `nixosModules.default`: `services.dirmcp.instances.<name>` with `root`, `listen`, `tokenFile`, `writable`, `createIn`, `user`/`group` (created unless set to an existing one), `memoryMax`, `tasksMax`.
- One service `dirmcp-<name>` per instance: token via `LoadCredential` (never in the Nix store), systemd hardening, everything except the sandbox's own needs closed.
- NixOS VM test with a read-only instance (`createIn`) and a writable one: auth, reads, no access outside, no writes, create, trash.
- README section "Deploying securely": what the token grants, how to expose it (HTTPS proxy, Funnel), token handling, how to switch it off.
- Bump to `0.3.0`.

## Capabilities

### New Capabilities
- `nixos-module`: instances, unit, hardening, credentials.

### Modified Capabilities

## Impact

`nix/module.nix`, `nix/test.nix`, `flake.nix` (output and check), README, CHANGELOG, VERSION.
