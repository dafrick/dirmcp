# confinement Specification

## Purpose
TBD - created by archiving change add-server. Update Purpose after archive.

## Requirements

### Requirement: Own root per program
Every started program SHALL run in new user, mount and network namespaces whose root holds only the served directory, `/nix/store`, its scratch directory and `/dev/null`, `/dev/zero`, `/dev/urandom` (no `/proc`), with all capabilities dropped. The server SHALL refuse to start if this is not possible.

#### Scenario: Metadata outside the root
- **WHEN** the command is `stat /etc/shadow`, `readlink /proc/self` or `du -s /etc`
- **THEN** the path does not exist for the program

### Requirement: Landlock per program
Every started program SHALL run under Landlock rules applied before `execve`: read only the root, the tools' store closure and `/dev/null`, `/dev/zero`, `/dev/urandom`; write only the root and only with `--writable`; execute only files in the tools' store closure; no TCP bind or connect; IPC scoped. The server SHALL refuse to start if the kernel's Landlock ABI is below 6.

#### Scenario: Read outside the root
- **WHEN** the command is `cat /etc/passwd` or `cat ../secret` (also through a symlink in the root)
- **THEN** it fails: the path does not exist or access is denied

#### Scenario: Write in read-only mode
- **WHEN** a server without `--writable` runs `echo x > a.md` or `sed -i s/a/b/ a.md`
- **THEN** it fails and `a.md` is unchanged

#### Scenario: Network
- **WHEN** an allowed program tries to open a TCP connection
- **THEN** the kernel denies it

### Requirement: Resource limits
Every started program SHALL run with rlimits (CPU 60 s, address space 1 GiB, file size 100 MiB, 256 open files, no core dumps) and SHALL be killed with its process group after 5 minutes. The number of processes is limited by the service unit.

#### Scenario: Huge file
- **WHEN** a writable server runs `head -c 200000000 /dev/zero > big`
- **THEN** `big` stops at 100 MiB

#### Scenario: Endless command
- **WHEN** the command is `tail -f a.md`
- **THEN** it is killed after 5 minutes and `more` reports it as stopped

#### Scenario: Many programs
- **WHEN** 400 command lines run, eight at a time
- **THEN** every program starts; the helper never fails on its own
