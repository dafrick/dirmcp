## Recon

Checked 2026-11-19: go-sdk v1.8.0 `mcp/streamable.go` — `StreamableHTTPHandler.ServeHTTP` returns 403 "invalid Host header" when the local address is loopback and `req.Host` is not; `StreamableHTTPOptions.DisableLocalhostProtection` turns it off. agentbox: through Funnel, `POST /mcp` with the token → 403; without → 401 (auth runs first).

## Decisions

- Keep DNS-rebinding protection, configurable: our own check with an explicit list, instead of just disabling the SDK's.
- Check before auth: a wrong Host is refused without looking at tokens.
- Exact names, lowercase, port stripped; loopback IPs and `localhost` always allowed (local clients, tests).

## Threat model

A browser on the owner's machine tricked into calling a rebinding name still gets 403 (the name is not listed). Listing the public name adds nothing an internet client could not already send: the bearer token remains the gate.
