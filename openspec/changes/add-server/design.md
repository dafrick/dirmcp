## Recon

Checked 2026-11-18.

- **Go 1.27.3** (nixpkgs-unstable). `os.Root` (Go 1.24+) confines file access to a directory tree, symlinks included.
- **MCP go-sdk v1.8.0** (2026-09-14, Context7 `/modelcontextprotocol/go-sdk/v1.8.0`): `mcp.NewStreamableHTTPHandler` with `Stateless: true` (required for protocol `2026-07-28`); `auth.RequireBearerToken(verifier)` middleware.
- **mvdan/sh v3.14.1** (2026-09-06): `interp` runs POSIX/bash command lines in Go; `ExecHandlers` decide how each command runs, `OpenHandler` handles redirections, `Dir` sets the start directory.
- **go-landlock v0.11.2** (2026-11-10): `landlock.V<n>.RestrictPaths(RODirs, RWDirs, ROFiles…)`, network rules (ABI 4+), IPC scoping (ABI 6+). Restricts the calling process and its children, unprivileged.
- **Target kernel**: agentbox runs Linux 6.18.63 with `landlock` in `/sys/kernel/security/lsm`.
- **Tools**: GNU sed and gawk have `--sandbox` (no `e`/`r`/`w` commands; gawk: no `system()`, no redirection, no extra input files). ripgrep `--pre`/`-z` run other programs; `find -exec/-ok/-delete/-fprint*`; `sort --compress-program`.
- **claude.ai**: custom connectors send fixed request headers (bearer) and call from Anthropic's cloud; a public HTTPS endpoint is needed (deployment concern, next change).

## Context

The first user is an Obsidian vault on agentbox, served to claude.ai. The tool is generic: one directory, any MCP client that sends a bearer token.

## Goals / Non-Goals

**Goals:** the model uses real tools with their real flags; nothing outside the allowlist can run; a started program cannot leave the directory or reach the network; long output and long runs are usable.

**Non-Goals:** OAuth, several directories per process, binary download, Windows/macOS (Landlock is Linux; the server refuses to start without it).

## Decisions

### Interface

```
dirmcp --root DIR --listen 127.0.0.1:8090 --token-file FILE
       [--writable] [--create-in SUBDIR]... [--tools MANIFEST]
```

- `--writable` off: started programs get the directory **read-only**. On: read-write.
- `--create-in Inbox` (repeatable) enables the `create` tool for those subfolders.
- `--tools MANIFEST`: JSON from the Nix package: program name → store path, plus the store paths of their closure. The package wrapper passes it, so the binary only knows absolute, pinned paths and never uses `PATH`.

### Tools

| Tool | Input | Output |
|---|---|---|
| `sh` | `command` (string) | stdout+stderr, exit code; truncated with a `more` hint |
| `more` | `id` | next chunk; waits up to 15 s for a running command |
| `create` | `path`, `content` | error if the file exists |

One `sh` tool, not one tool per program: models know shell syntax, and pipes and loops give bulk edits (`for f in $(rg -l '#old'); do sed -i 's/#old/#new/g' "$f"; done`). The tool description lists the programs; `<prog> --help` works.

### Running a command line

```
command ─► mvdan/sh interp (Dir = root, no PATH lookup)
             ├─ shell built-ins (echo, printf, test, read, cd*, for, if, …)
             ├─ rm   → built in: move to .trash/<name>.<time>
             └─ prog → allowlist? flags allowed? ─► dirmcp exec-helper ─► Landlock + rlimits ─► execve(store path)
* cd only inside root
```

- The interpreter never runs anything itself: our `ExecHandler` rejects unknown programs with "not allowed; allowed: …".
- `OpenHandler` handles `>`/`<` with `os.Root` and respects `--writable`.
- Not available: `sh`, `bash`, `env`, `xargs`, `timeout`, `nice`, `exec`, `eval`, `source`, `command`, `builtin` (they start other programs or code). Loops replace `xargs`.

### Program and flag allowlist

Programs (first set): coreutils `ls cat head tail wc sort uniq cut tr paste comm join nl fold fmt tac rev stat du df basename dirname realpath readlink mkdir mv cp touch date md5sum sha256sum`, `grep`, `rg`, `sed`, `gawk` (as `awk` too), `find`, `diff`, `cmp`, `tree`, `file`.

- Each program has a table of allowed options: short and long forms, and whether they take a value. A GNU-style parser handles `-abc`, `-n5`, `--opt=val`, `--opt val` and `--`. Unknown option → error listing the closest allowed ones.
- The table starts from each program's `--help` at the pinned version, minus the dangerous ones (in **Threat model**). A test runs every program's `--help` and fails if an allowlisted option no longer exists, so updates cannot silently change behaviour.
- Forced options: `sed --sandbox`, `gawk --sandbox`. `find` without `-exec -execdir -ok -okdir -delete -fprint -fprint0 -fprintf -fls`. `rg` without `--pre --pre-glob -z/--search-zip`. `sort` without `--compress-program`. No `ln` in v1 (links could point outside the root).
- Non-option arguments are not checked: paths are confined by Landlock, patterns are data.

### Confinement (per started program)

The exec helper is `dirmcp` itself, re-executed as `dirmcp __exec <manifest-entry> -- args`. It sets rlimits, applies Landlock, then `execve`s the store path.

| Access | Allowed |
|---|---|
| Read | the root; the tools' store closure; `/dev/null`, `/dev/zero`, `/dev/urandom` |
| Write | the root only with `--writable` |
| Execute | the tools' store closure only |
| Network | no TCP bind/connect (ABI 4) |
| IPC | no abstract UNIX sockets or signals outside the sandbox (ABI 6) |

- Landlock is **required, not best effort**: if the kernel lacks the ABI we need (6), the server refuses to start.
- rlimits: CPU 60 s, address space 1 GiB, file size 100 MiB, open files 256, processes 64. Wall time 5 min, then the process group is killed.
- The server process itself has no Landlock rules (it needs the socket and the token). systemd hardening for it comes with the NixOS module.

### Output and long runs

- A call waits up to 15 s. Output up to 30 KB is returned whole.
- Longer output, or a command still running after 15 s: the first 30 KB, cut at a line boundary, then
  `[truncated: 1.2 MB total, still running — call more with id=<id>; kept for 10 min]`.
- Kept in RAM only: at most 20 MB per call and 64 MB in total, oldest dropped first. IDs are 128-bit random. A buffer is dropped 10 min after its last read, or once read to the end and finished.

### create

`os.Root` + `O_CREAT|O_EXCL`, only below a `--create-in` folder, parents created inside it, content ≤ 1 MiB, mode 0660 (with umask 007 the vault's sync client can read it).

### Endpoint

- Streamable HTTP, stateless, path `/mcp`. `auth.RequireBearerToken` with a verifier that compares to the token file in constant time.
- The token file must hold ≥ 32 bytes; the server refuses to start otherwise, or if the file is group/world-readable.
- After 5 failed auths from a remote address within a minute, it waits 1 s before answering (no lockout, no state on disk).
- Request body ≤ 2 MiB, `ReadHeaderTimeout` 10 s, at most 4 commands running at once (more → "busy, retry").
- Log per call to stderr (journal): tool, first 200 characters of the command, exit code, duration, output size. No file contents, no token.

## Threat model

**Assets:** files under the root (confidentiality, integrity), the rest of the machine, the network around it.

**Attacker:** anyone who can reach the endpoint; with or without the token.

| Without the token | Mitigation |
|---|---|
| Use tools | Bearer check before MCP parsing; constant time; ≥ 32-byte token |
| Brute force | 256-bit token; slow-down on failures |
| Exhaust the server | Body limit, header timeout, concurrency limit |

| With the token | Mitigation |
|---|---|
| Run arbitrary programs | ExecHandler allowlist; no `PATH`; Landlock execute rule on the tools' closure only |
| Run code through a tool's options (`find -exec`, `rg --pre`, sed `e`, awk `system()`, `sort --compress-program`) | Flag allowlist; forced `--sandbox`; Landlock execute rule as backstop |
| Read outside the root (`../`, absolute paths, symlinks) | Landlock read rule; `os.Root` for redirections and `create` |
| Write in read-only mode | Landlock write rule; `OpenHandler` checks |
| Network (exfiltration, LAN) | Landlock network rule; tools have no network use anyway |
| Exhaust the machine | rlimits, wall time, output caps, concurrency limit |
| Read other callers' output via `more` | Single token per server; IDs 128-bit random |

**What a token holder can do, by design:** read everything under the root; with `--writable`, change or delete it (`rm` goes to trash; `mv`, `sed -i` and redirections overwrite). That is the point of the tool, and the README says so.

## Risks / Trade-offs

- **Flag tables are work and can be wrong.** The `--help` test catches drift; a missing flag fails loudly with the allowed list.
- **Landlock is Linux-only** → dirmcp is Linux-only. Accepted.
- **An `mvdan/sh` bug could bypass the ExecHandler** → Landlock still limits what runs; the server never calls a system shell.
- **A tool with an unknown code-execution path** → Landlock execute and network rules contain it to the closure and the directory.
