# create-tool Specification

## Purpose
TBD - created by archiving change add-server. Update Purpose after archive.

## Requirements

### Requirement: Additive writes
With `--create-in DIR`, the `create(path, content)` tool SHALL create a new file below one of those folders (inside the root), creating parent folders there, and SHALL fail if the file exists or the path is outside them. Content SHALL be at most 1 MiB; the mode SHALL be 0660 before umask. Without `--create-in` the tool SHALL not be offered.

#### Scenario: New note
- **WHEN** `create("Inbox/clip.md", "…")` is called and the file does not exist
- **THEN** the file exists with that content

#### Scenario: Existing file
- **WHEN** `create("Inbox/clip.md", "…")` is called again
- **THEN** it fails and the file is unchanged

#### Scenario: Outside
- **WHEN** `create("Notes/x.md", …)` or `create("Inbox/../x.md", …)` is called with `--create-in Inbox`
- **THEN** it fails
