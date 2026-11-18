## Recon

Checked 2026-11-18: OpenSpec 2.0.0 (nixpkgs-unstable, `openspec init --tools claude`), git-hooks.nix (cachix) as in agentos, CI actions pinned to the same SHAs as agentos (`actions/checkout` v7.0.1, `cachix/install-nix-action` v31.11.5). Go module path `github.com/dafrick/dirmcp`; Go has no registry, versions are Git tags (proxy.golang.org caches public tags).

## Context

Copied from agentos and trimmed: no secrets, hosts, services or umbrella branches. One addition: security is a principle, and designs get a threat model section.

## Decisions

- **Same workflow as agentos**, so the same agents and habits work in both repos.
- **nixpkgs-unstable only.** No host to match; newest Go and openspec.
- **Public repo, MIT license.** Nothing secret lives here; public keeps it usable as a flake input and `go install` without credentials.
- **SECURITY.md + private vulnerability reporting**, since the repo is public and security-critical.
- **Dependabot cooldown of 7 days** before taking new action releases (supply-chain risk).
- **Go hooks later.** gofmt, go vet and `go test` join the hooks and `nix flake check` with the first Go code.

## Risks / Trade-offs

- The pre-push hook is the first gate. Unlike agentos (private), a public repo gets free rulesets: `main` requires the `checks` job and blocks force-push and deletion, so an unchecked push cannot become a release.
