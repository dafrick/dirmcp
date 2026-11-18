# Changelog

All notable changes are listed here. Format: [Keep a Changelog](https://keepachangelog.com). Versioning: [SemVer](https://semver.org), see `CONTRIBUTING.md`.

## [0.2.1] - 2026-11-18

### Fixed

- The exec helper could fail with "out of memory" between setting the address-space limit and starting the program; it now uses raw syscalls there, with the GC off and that limit set last.
- Security: the helper drops capabilities on a locked OS thread. Before, Go could move it to another thread before `execve`, so a program could start with all capabilities inside its user namespace (still confined by Landlock and the mount namespace).

## [0.2.0] - 2026-11-18

### Added

- The server: `sh` runs command lines (pipes, loops) with allowlisted GNU tools and options. Each program gets its own root (user, mount and network namespaces: only the directory and the Nix store exist), no capabilities, Landlock (no writes unless `--writable`, no foreign programs, no network) and rlimits; built-in `rm` moves to `.trash/`. Needs Linux 6.12+ with unprivileged user namespaces.
- `more` pages output over 30 KB or still running after 15 s; `create` makes new files only (`--create-in`).
- Streamable HTTP endpoint with a bearer token (≥ 32 bytes, private file, compared by hash), body, time and concurrency limits, logs without file contents.
- Nix package that pins every tool to a store path; Go tests run in the package build. ADR 0005.

## [0.1.0] - 2026-11-18

### Added

- Repo foundations: README, CONTRIBUTING, AGENTS, SECURITY, ADRs 0001–0004, Nix flake with dev shell and git hooks (nixfmt, gitleaks, release check), CI with a release per merged change, Dependabot for GitHub Actions, OpenSpec; ruleset on `main`.
