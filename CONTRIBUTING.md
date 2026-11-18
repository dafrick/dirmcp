# Contributing

## Principles

- **Avoid custom things.** Write our own only when nothing standard fits.
- **Use standard, widely used tooling** whenever possible.
- **Understand the problem first**, then design the solution.
- **Security first.** dirmcp exposes a directory to remote clients. Every change to tools, argument checks, paths, auth or confinement comes with a test that tries to break it.
- **No fluff.** Keep code, docs and decisions short and simple.

## Every change is an OpenSpec change

1. **Explore**: skill `openspec-explore`.
2. **Propose** on a new branch: skill `openspec-propose`. Start with recon: check the current docs and versions of what the change uses, and record findings in its design. Set the version (see [Versions](#versions-and-releases)), commit, push, and open a **draft PR** into `main` right away.
3. **Apply**: skill `openspec-apply-change`. Write the test first. Commit each task or logical piece on its own and push often. Then update the PR description.
4. **Review**: run an independent review of the implementation. Post its findings as a PR comment, with what was fixed and what was not. Fix in separate commits.
5. **Archive**: skill `openspec-archive-change`. Commit, push, and update the PR description.
6. **Merge**: `git fetch --tags`, rebase onto `main`, run `scripts/check-release.sh --base origin/main`, push, mark the PR ready, get it green, squash merge.

The PR description is the living record of the change: what it does, its current status, how it was verified. Keep it current at every step.

The only exception: Dependabot PRs (see [Dependency updates](#dependency-updates)).

Every change must also:

- Update `README.md` and `CONTRIBUTING.md` if they are affected.
- Set the version (see [Versions](#versions-and-releases)).
- Add an ADR in `docs/adr/` if it makes a structural decision.

## Branches and commits

- Branch: `<type>/<openspec-change-name>`, e.g. `feat/add-server`.
- Commits and PR titles: [Conventional Commits](https://www.conventionalcommits.org), `<type>: <summary>`. The PR title becomes the squash commit.
- Types: the standard set (`feat`, `fix`, `docs`, `chore`, `refactor`, `test`, `ci`, …).
- Before merging, rebase onto `main`. No merge commits.

## Dependency updates

Dependabot (`.github/dependabot.yml`) opens update PRs against `main`, `ci(deps): …`. They are not OpenSpec changes, but follow the release rules, so CI fails on them until they bump `VERSION`. To take one: review the diff and release notes, check out its branch inside `nix develop`, bump `VERSION` (PATCH), add a `CHANGELOG.md` section, commit `chore: release X.Y.Z`, push, wait for green, squash merge.

`flake.lock` is updated by hand with `nix flake update` in a normal change.

## Architecture decisions

ADRs live in `docs/adr/NNNN-title.md`, one Y-statement each:

> In the context of …, facing …, we decided for … and neglected …, to achieve …, accepting ….

Only structural decisions become ADRs. Everything else stays in the OpenSpec design.

## Checks run locally

Work inside `nix develop`. It installs git hooks:

- pre-commit: formatting (`nixfmt`) and secret scanning of staged changes (`gitleaks`). `nix flake check` runs these.
- pre-push: secret scanning of the full history (`gitleaks`) and the release check (`scripts/check-release.sh`).

CI runs the same checks as a backstop, not as the first line. A ruleset on `main` requires the `checks` job and blocks force-pushes and deletion.

## Security issues

See [SECURITY.md](SECURITY.md). Never report a vulnerability in a public issue.

## Versions and releases

Every change bumps the version. We use [semantic versioning](https://semver.org); before 1.0, MINOR is a new capability or a breaking change, PATCH is fixes, tweaks and docs.

In each change, at propose time:

1. Bump `VERSION` to the next version above `main`'s.
2. Add a `## [X.Y.Z] - YYYY-MM-DD` section at the top of `CHANGELOG.md` ([Keep a Changelog](https://keepachangelog.com) format). Refine the text during apply.

Sections stay unique and newest first. `scripts/check-release.sh --base origin/main` (and CI on every PR) fails if they are not.

On merge into `main`, CI tags `vX.Y.Z` and publishes a GitHub Release with that changelog section. The tag is also the Go module version (`go install github.com/dafrick/dirmcp@vX.Y.Z`).
