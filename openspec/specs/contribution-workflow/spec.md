# contribution-workflow Specification

## Purpose
TBD - created by archiving change add-repo-foundations. Update Purpose after archive.

## Requirements

### Requirement: OpenSpec per change
Every change SHALL be an OpenSpec change on its own branch with a draft PR into `main` opened at propose time, tests written first, an independent review posted as a PR comment, and a squash merge.

#### Scenario: New change
- **WHEN** a contributor starts a change
- **THEN** it has a proposal, design (with recon), specs and tasks, and a draft PR exists before apply starts

### Requirement: Local checks first
The dev shell SHALL install git hooks: pre-commit formatting and secret scan, pre-push full-history secret scan and release check. CI SHALL run the same checks.

#### Scenario: Secret in a commit
- **WHEN** a staged change contains a secret
- **THEN** the pre-commit hook blocks the commit
