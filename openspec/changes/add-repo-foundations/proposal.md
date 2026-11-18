## Why

dirmcp is a new repo. Before any server code lands, it needs the same way of working as `dafrick/agentos`: written intent per change, local checks, one release per change, CI as a backstop.

## What Changes

- Docs: README (purpose, warning, getting started), CONTRIBUTING (workflow, checks, versions, Dependabot), AGENTS.md, CLAUDE.md.
- ADRs 0001–0004: Y-statement ADRs, OpenSpec per change, Go module with a Nix flake, release per change.
- Nix flake: dev shell (Go, gopls, gh, openspec), formatter, git hooks (nixfmt, gitleaks staged and history, release check).
- `scripts/check-release.sh`, VERSION `0.1.0`, CHANGELOG.
- CI: hooks on every PR and `main`, version check on PRs, tag and GitHub Release on `main`.
- Dependabot for GitHub Actions.
- OpenSpec initialised (Claude skills), project context and rules in `openspec/config.yaml`.
- GitHub: squash merge only.

## Capabilities

### New Capabilities
- `contribution-workflow`: OpenSpec per change, draft PR at propose, tests first, review as PR comment, squash merge.
- `release-management`: VERSION and CHANGELOG per change, checked locally and in CI, tag and release on `main`.

### Modified Capabilities

## Impact

New repo content only. Go hooks (gofmt, vet, tests) come with the first Go code.
