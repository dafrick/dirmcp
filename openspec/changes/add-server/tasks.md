## 1. Skeleton and checks

- [ ] 1.1 `go.mod` (go-sdk, mvdan/sh, go-landlock), `cmd/dirmcp/main.go` with flags; Go hooks (gofmt, go vet) and `go test` in the flake checks; `packages.default` with `buildGoModule` and the tools manifest wrapper

## 2. Endpoint (tests first)

- [ ] 2.1 Tests: 401 without/with wrong token, startup errors for short or readable token file, body limit, busy limit
- [ ] 2.2 Streamable HTTP handler (stateless) + bearer middleware + limits + logging

## 3. Argument allowlist (tests first)

- [ ] 3.1 Tests: GNU option parsing (`-abc`, `-n5`, `--opt=val`, `--opt val`, `--`), every denied option from the spec, fuzz test of the parser
- [ ] 3.2 Option parser and tables for every program; forced `--sandbox` for sed and gawk
- [ ] 3.3 Test: every allowlisted option appears in the program's `--help` (packaged tools)

## 4. Confinement (tests first)

- [ ] 4.1 Tests: read outside root (absolute, `..`, symlink), write in read-only mode, TCP connect, exec of a non-allowlisted binary, rlimits, wall-time kill
- [ ] 4.2 `__exec` helper: rlimits, Landlock (required ABI 6), `execve`; refuse to start below ABI 6

## 5. sh tool (tests first)

- [ ] 5.1 Tests: unknown program, rejected programs/built-ins, pipes and loops, `cd` escape, redirections in read-only and writable mode, `rm` to trash
- [ ] 5.2 Interpreter with ExecHandler, OpenHandler, `cd` guard, built-in `rm`

## 6. Output paging and create (tests first)

- [ ] 6.1 Tests: long output, slow command, buffer limits and expiry
- [ ] 6.2 `more` and buffers
- [ ] 6.3 Tests and implementation of `create`

## 7. Docs and checks

- [ ] 7.1 README: usage, tools, flags, warning; CONTRIBUTING: Go checks
- [ ] 7.2 ADR 0005: real tools with flag allowlists under Landlock (structural decision)
- [ ] 7.3 Refine CHANGELOG
- [ ] 7.4 Run `nix flake check` and `scripts/check-release.sh --base origin/main`; update the PR description

## 8. Review and close

- [ ] 8.1 Independent security review of the diff, posted on the PR; fix findings
- [ ] 8.2 Archive (sync specs), update the PR description
