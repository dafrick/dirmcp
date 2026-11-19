## Recon

Checked 2026-11-19: `http.ServeMux` pattern without a trailing slash matches exactly that path (no subtree), so `/notes/x` stays 404. go-sdk's Streamable HTTP handler does not care about the path.

## Decisions

- Validate the path at startup (flag) and in Nix (`types.strMatching`), so a typo fails early instead of serving a surprising route.
- No prefix/subtree routing: one exact path.

## Threat model

No change in who can do what: the host check, the token and the tools are the same on any path. A non-default path is not a secret and is not treated as one.
