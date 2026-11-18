# shell-tool Specification

## Purpose
TBD - created by archiving change add-server. Update Purpose after archive.

## Requirements

### Requirement: Only allowlisted programs
The `sh` tool SHALL interpret the command line with `mvdan/sh` in the root directory and start only programs on the allowlist, by absolute store path from the tools manifest, never through `PATH` or a system shell. Programs that start other programs or code (`sh`, `bash`, `env`, `xargs`, `timeout`, `nice`, `exec`, `eval`, `source`, `command`, `builtin`) SHALL be rejected.

#### Scenario: Unknown program
- **WHEN** the command is `curl https://example.com`
- **THEN** nothing runs and the result says `curl` is not allowed and lists the allowed programs

#### Scenario: Pipes and loops
- **WHEN** the command is `for f in $(rg -l old); do sed -i 's/old/new/g' "$f"; done` on a writable server
- **THEN** every matching file is changed

### Requirement: Only allowlisted options
Each program SHALL accept only options from its table, in any GNU form (`-abc`, `-n5`, `--opt=val`, `--opt val`, after `--` none). `sed` and `gawk` SHALL always get `--sandbox`. Options that run other programs or write outside the normal output SHALL not be in any table, including `find -exec/-execdir/-ok/-okdir/-delete/-fprint/-fprint0/-fprintf/-fls`, `rg --pre/--pre-glob/-z/--search-zip`, `sort --compress-program`.

#### Scenario: Dangerous option
- **WHEN** the command is `find . -name x -exec cat {} ;`
- **THEN** nothing runs and the result names `-exec` as not allowed

#### Scenario: Help
- **WHEN** the command is `rg --help`
- **THEN** ripgrep's help is returned

#### Scenario: Tables match the tools
- **WHEN** the tests run against the packaged tools
- **THEN** every allowlisted option appears in that program's `--help`

### Requirement: rm moves to trash
`rm` SHALL be a built-in that moves its arguments (inside the root) to `.trash/` with a timestamp suffix instead of deleting them. On a read-only server it SHALL fail.

#### Scenario: Remove a note
- **WHEN** a writable server runs `rm Inbox/a.md`
- **THEN** `Inbox/a.md` is gone and `.trash/a.md.<time>` holds its content

### Requirement: cd stays inside the root
`cd` SHALL fail for any directory outside the root.

#### Scenario: Escape
- **WHEN** the command is `cd /etc` or `cd ../..`
- **THEN** it fails and the working directory stays inside the root
