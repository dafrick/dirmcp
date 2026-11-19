## Why

Behind Tailscale Funnel (agentbox), every authenticated request got 403: the MCP go-sdk's DNS-rebinding protection refuses non-localhost Host headers on a loopback listener, and a proxy forwards the public name. v0.3.0 cannot be used behind any proxy. The tests called the server directly and missed it.

## What Changes

- `--allowed-host NAME` (repeatable), module option `allowedHosts`.
- dirmcp turns the SDK's check off and does its own, before auth: localhost names and loopback IPs always, plus exactly the listed names (case and port ignored); anything else 403.
- Tests: unit test for allowed, refused and default hosts; VM test with a Host header.
- Bump to `0.3.1`.

## Capabilities

### New Capabilities

### Modified Capabilities
- `endpoint`: Host check with allowed proxy names.

## Impact

`internal/server`, `cmd/dirmcp`, `nix/module.nix`, `nix/test.nix`, README, CHANGELOG, VERSION.
