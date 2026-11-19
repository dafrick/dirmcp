# nixos-module Specification

## Purpose
TBD - created by archiving change add-nixos-module. Update Purpose after archive.

## Requirements

### Requirement: Instances
The module SHALL offer `services.dirmcp.instances.<name>` with `root`, `listen` (default `127.0.0.1:8090`), `tokenFile`, `writable` (default false), `createIn` (default none), `user` and `group` (default `dirmcp-<name>`, created by the module), `memoryMax` and `tasksMax`, and SHALL run each as service `dirmcp-<name>`.

#### Scenario: Read-only instance
- **WHEN** an instance serves `/srv/vault` with `createIn = [ "Inbox" ]`
- **THEN** a client with the token can read files and create new ones in `Inbox/`, but cannot change existing files

### Requirement: Token through credentials
The token SHALL reach the server only through `LoadCredential`, never through the Nix store, the unit file or the environment.

#### Scenario: Token file root-only
- **WHEN** `tokenFile` is mode 0400, owned by root
- **THEN** the service starts and accepts that token

### Requirement: Hardened unit
The unit SHALL apply the hardening listed in the design and keep open only what the sandbox needs: user, mount and network namespaces, `@mount` and `@sandbox` (Landlock) system calls, `CAP_SYS_ADMIN` and `CAP_SETPCAP` in the bounding set.

#### Scenario: Sandbox works under the unit
- **WHEN** a client runs `stat /etc/passwd` and `cat a.md` through the service
- **THEN** `/etc/passwd` does not exist for the program and `a.md` is printed

### Requirement: Path option
Each instance SHALL have an option `path` (default `/dirmcp`) passed to the server as `--path`.

#### Scenario: Instance on /notes
- **WHEN** an instance sets `path = "/notes"`
- **THEN** it answers on `/notes` only
