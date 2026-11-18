# Agent instructions

Read `README.md` and `CONTRIBUTING.md` first. They apply to you too.

## Style

- Keep things lean and simple. No fluff.
- Use plain, short language in code, docs and commit messages.

## Workflow

- Every change is an OpenSpec change. No changes outside one.
- Follow the change workflow in CONTRIBUTING: recon and draft PR at propose, small commits, PR description updated after apply and archive, reviews posted as PR comments.
- Name branches `<type>/<change-name>` and write Conventional Commits.
- Every change sets its version (CONTRIBUTING, "Versions") and updates `README.md` / `CONTRIBUTING.md` if they are affected.
- Structural decisions get an ADR in `docs/adr/` (Y-statement, a few lines).
- Work inside `nix develop` so the git hooks run. Run the checks locally before pushing (see README). Do not rely on CI to find problems.
- Never commit secrets.

## Security

dirmcp gives remote clients access to a directory. Treat every change to tools, argument checks, path handling, auth or confinement as security-relevant: write a test that tries to break it, and say in the PR what an attacker with a valid token could do before and after.
