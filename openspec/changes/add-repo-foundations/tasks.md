## 1. Docs

- [x] 1.1 README, CONTRIBUTING, AGENTS.md, CLAUDE.md
- [x] 1.2 ADRs 0001–0004

## 2. Flake and checks

- [x] 2.1 `flake.nix`: dev shell, formatter, git hooks; `flake.lock`
- [x] 2.2 `scripts/check-release.sh`, VERSION 0.1.0, CHANGELOG
- [x] 2.3 CI workflow and Dependabot

## 3. OpenSpec

- [x] 3.1 `openspec init --tools claude`; context and rules in `openspec/config.yaml`

## 4. GitHub and checks

- [x] 4.1 Squash merge only; ruleset on `main`; private vulnerability reporting; SECURITY.md
- [ ] 4.2 Run `nix flake check` and `scripts/check-release.sh --base origin/main` locally, then update the PR description
- [ ] 4.3 Review posted as PR comment; archive
