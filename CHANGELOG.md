# Changelog

All notable changes are listed here. Format: [Keep a Changelog](https://keepachangelog.com). Versioning: [SemVer](https://semver.org), see `CONTRIBUTING.md`.

## [0.2.0] - 2026-11-18

### Added

- The server: `sh` runs command lines (pipes, loops) with allowlisted GNU tools and options, each program under Landlock (directory only, no network, no foreign programs) and rlimits; built-in `rm` moves to `.trash/`.
- `more` pages output over 30 KB or still running after 15 s; `create` makes new files only (`--create-in`).
- Streamable HTTP endpoint with a bearer token (≥ 32 bytes, private file), body and concurrency limits, logs without file contents.
- Nix package that pins every tool to a store path; Go tests run in the package build. ADR 0005.

## [0.1.0] - 2026-11-18

### Added

- Repo foundations: README, CONTRIBUTING, AGENTS, SECURITY, ADRs 0001–0004, Nix flake with dev shell and git hooks (nixfmt, gitleaks, release check), CI with a release per merged change, Dependabot for GitHub Actions, OpenSpec; ruleset on `main`.
