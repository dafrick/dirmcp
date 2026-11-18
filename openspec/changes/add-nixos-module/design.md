## Recon

Checked 2026-11-18 against nixpkgs-unstable (systemd 262): `LoadCredential=` copies a root-only file into `$CREDENTIALS_DIRECTORY` (mode 0400, owned by the service user); `RestrictNamespaces=` takes a list of allowed types; `SystemCallFilter=@mount` covers `mount`, `umount2`, `pivot_root`; `CapabilityBoundingSet=` also bounds what a process gets on `execve` as uid 0 inside a user namespace; `PR_SET_SECUREBITS` and `PR_CAPBSET_DROP` need `CAP_SETPCAP`. `testers.runNixOSTest` for the VM test.

## Context

The sandbox (v0.2) needs, for each started program: creating user, mount and network namespaces; `mount` and `pivot_root` inside them; `CAP_SYS_ADMIN` and `CAP_SETPCAP` inside the new user namespace (gained on the helper's `execve` as uid 0 there, so they must stay in the bounding set); Landlock. Everything else can be closed.

## Decisions

- **One unit per instance**, `dirmcp-<name>`, as its own system user (default `dirmcp-<name>`, created by the module; or an existing user, e.g. to share vault group rights).
- **Token through `LoadCredential`**: `tokenFile` stays root-only (sops), the server reads `$CREDENTIALS_DIRECTORY/token`.
- **Hardening**: `NoNewPrivileges`, `ProtectSystem=strict` (+ `ReadWritePaths=root` only if `writable` or `createIn`), `ProtectHome` (unless the root is under `/home`), `PrivateTmp`, `PrivateDevices`, `ProtectKernel*`, `ProtectControlGroups`, `ProtectClock`, `ProtectHostname`, `ProtectProc=invisible`, `RestrictRealtime`, `RestrictSUIDSGID`, `LockPersonality`, `SystemCallArchitectures=native`, `RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6`, `IPAddressDeny=any` + `IPAddressAllow=localhost` when listening on loopback, `UMask=0007`, `MemoryMax`, `TasksMax`.
- **Kept open for the sandbox, and why**: `RestrictNamespaces=user mnt net`; `SystemCallFilter=@system-service @mount` (no other privileged groups); `CapabilityBoundingSet=CAP_SYS_ADMIN CAP_SETPCAP` — the service user holds no capabilities on the host (not root, no ambient caps, `NoNewPrivileges`); the set only bounds what the helper gets inside its own user namespace, which it drops before the program starts.
- **Not used**: `MemoryDenyWriteExecute` (inherited by tools; breaks ripgrep's PCRE2 JIT), `PrivateUsers` (we create our own user namespace).

## Threat model

Adds to v0.2's: a compromised server process (bug in dirmcp, not a tool) is still limited to the unit's view: no home directories, read-only system, no writes outside the root, no new namespaces besides user/mnt/net, no network except loopback, memory and task limits. The token never appears in the Nix store, the unit file or the environment.

## Risks / Trade-offs

- Two capabilities in the bounding set look scary in `systemd-analyze security`; they are needed and harmless on the host (see above). The VM test proves the sandbox works with exactly this set.
