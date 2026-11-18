# dirmcp

An [MCP](https://modelcontextprotocol.io) server that gives a remote AI client (for example a claude.ai custom connector) shell-like access to **one directory**, using the real Unix tools from an allowlist.

Status: usable. Server (`sh`, `more`, `create`) and NixOS module with hardened instances.

> **Warning.** dirmcp gives whoever holds its token remote access to the directory it serves, with everything the allowed tools can do there. Read [Deploying securely](#deploying-securely) before you expose it.

## What the client gets

| Tool | What it does |
|---|---|
| `sh` | Runs a command line in the directory: pipes, `for` loops, `$( )`, redirections. Only these programs run: `awk basename cat cmp comm cp cut date df diff dirname du file find fmt fold gawk grep head join ls md5sum mkdir mv nl paste readlink realpath rg sed sha256sum sort stat tac tail touch tr tree uniq wc`, plus a built-in `rm` that moves to `.trash/`. Most options work; options that run other programs or write elsewhere are refused (`find -exec`, `rg --pre`, `sort --compress-program`, …), and `sed`/`gawk` always run with `--sandbox`. No shells, `xargs` or `env`. |
| `more` | Next part of an answer that was cut at 30 KB or still running after 15 s. Output is kept in RAM for 10 minutes. |
| `create` | Only with `--create-in DIR`: makes a new file below `DIR`, never overwrites. |

Every started program gets its own root in new user, mount and network namespaces: only the directory, the Nix store and three devices exist there (no `/etc`, `/home` or `/proc`), with no network and no capabilities. On top, [Landlock](https://landlock.io) lets it read only the directory and the tools' own files, write only when the server is started with `--writable`, and start only the allowed programs. It also gets CPU, memory and file-size limits and is killed after 5 minutes.

Needs Linux 6.12 or newer (Landlock ABI 6) with unprivileged user namespaces; the server checks at startup and refuses to run otherwise. NixOS allows them by default; Ubuntu blocks them through AppArmor (`kernel.apparmor_restrict_unprivileged_userns`).

## Usage

```sh
nix build github:dafrick/dirmcp        # or: nix run github:dafrick/dirmcp -- …
openssl rand -hex 32 > token && chmod 600 token
./result/bin/dirmcp --root ~/notes --token-file token --create-in Inbox
```

| Flag | Default | |
|---|---|---|
| `--root DIR` | required | the directory to serve |
| `--token-file FILE` | required | bearer token, ≥ 32 bytes, mode 0600 or 0400 |
| `--listen ADDR` | `127.0.0.1:8090` | MCP endpoint is `http://ADDR/mcp` (Streamable HTTP) |
| `--writable` | off | programs may change files |
| `--create-in DIR` | none | enable `create` below `DIR` (repeatable) |
| `--tools FILE` | set by the package | tools manifest: program → store path |

Clients send `Authorization: Bearer <token>`. For a claude.ai custom connector, put the URL of a public HTTPS proxy in front of `/mcp` and the header under "Request headers". The binary must come from the Nix package: it pins every tool to a store path and never searches `PATH`.

## NixOS

```nix
# flake.nix
inputs.dirmcp.url = "github:dafrick/dirmcp/v0.3.0";
# in the host's modules:
imports = [ inputs.dirmcp.nixosModules.default ];
services.dirmcp.instances.notes = {
  root = "/srv/notes";
  listen = "127.0.0.1:8090";
  tokenFile = "/run/secrets/dirmcp_notes_token"; # root-only, e.g. sops-nix
  createIn = [ "Inbox" ];                         # read-only, plus new files in Inbox/
  # writable = true;                              # let programs change files
  # user = "…"; extraGroups = [ "…" ];            # to share access to the root
};
```

Each instance is `dirmcp-<name>.service`, running as its own system user (created unless you name an existing one). The unit reads the token as a systemd credential and is hardened: read-only system, no home directories, private `/tmp` and `/dev`, loopback-only networking when it listens on loopback, `MemoryMax` and `TasksMax` (options `memoryMax`, `tasksMax`). It keeps open only what the sandbox needs: user, mount and network namespaces, the `@mount` and `@sandbox` system calls, and `CAP_SYS_ADMIN` and `CAP_SETPCAP` in the bounding set. The service user has no capabilities on the host; those two only exist inside each program's own user namespace and are dropped before the program starts.

## Deploying securely

**What you are giving away.** Whoever holds the token can read every file under the root, with all the power of grep, rg, find, awk and sed. With `--writable` they can also change, move and delete (to `.trash/`, but `mv`, `sed -i` and `>` overwrite for good). Treat the token like an SSH key to that directory.

- **Serve as little as possible.** Point `--root` at the folder you mean, not its parent. Prefer read-only with `--create-in` for additions; use `--writable` only if you need edits, and keep backups or version history (e.g. Obsidian Sync) of the folder.
- **Run as a user that can only reach that folder.** File permissions are the last line: a read-only instance should run as a user without write access to the root, so a bug in dirmcp cannot write either. The module creates its own user; give it access to the root through a group, not by running it as you.
- **Keep it on loopback and put HTTPS in front.** dirmcp speaks plain HTTP. Expose only `/mcp` through a TLS reverse proxy or a tunnel (Tailscale Funnel, Cloudflare Tunnel). Never bind it to a public interface directly.
- **Token.** 32 random bytes or more (`openssl rand -hex 32`), stored root-only (sops, agenix) and passed as a credential. Put it only into the client's connector settings (claude.ai: "Request headers", `Authorization: Bearer …`). Rotate it if it may have leaked: change the secret, restart the service, update the connector.
- **Watch it.** Every call is logged (tool, first 200 characters of the command, exit code, size; never file contents): `journalctl -u dirmcp-<name>`. Failed tokens are logged with the remote address.
- **Switch it off** at once with `systemctl stop dirmcp-<name>` (or by removing the tunnel), then rotate the token.
- **Check the host.** The server refuses to start without Landlock ABI 6 and unprivileged user namespaces; do not work around that.

## Layout

```
AGENTS.md, CLAUDE.md   agent instructions (CLAUDE.md imports AGENTS.md)
SECURITY.md            how to report a vulnerability
CONTRIBUTING.md        how to make changes
VERSION, CHANGELOG.md  version and changelog
flake.nix              dev shell, checks, package (with the tools manifest)
cmd/dirmcp/            main: flags, startup checks, HTTP server
nix/                   NixOS module and its VM test
internal/              argcheck (option allowlists), confine (Landlock helper),
                       shell (interpreter), jobs (paging), create, server, manifest
docs/adr/              architecture decision records
openspec/              OpenSpec specs and changes
scripts/               small helper scripts (release check)
.github/               CI, Dependabot
```

## Getting started

1. Install Nix ([nixos.org/download](https://nixos.org/download)) and enable flakes.
2. Enter the dev shell: `nix develop`. It provides the tools (Go, `openspec`, `gh`) and installs the git hooks.
3. Run all checks:
   ```sh
   nix flake check                                     # pre-commit hooks
   pre-commit run --all-files --hook-stage pre-push    # pre-push hooks
   ```

Then read [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT, see [LICENSE](LICENSE).
