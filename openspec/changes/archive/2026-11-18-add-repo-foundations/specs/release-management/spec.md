## ADDED Requirements

### Requirement: Release per change
Each change SHALL bump `VERSION` above `main`'s and add a matching top section to `CHANGELOG.md`. On merge into `main`, CI SHALL tag `vX.Y.Z` and publish a GitHub Release with that section, and fail if the tag exists on another commit.

#### Scenario: Version not bumped
- **WHEN** a PR keeps `main`'s VERSION
- **THEN** `scripts/check-release.sh --base origin/main` and CI fail

#### Scenario: Merge
- **WHEN** a PR with VERSION 0.1.0 is merged into `main`
- **THEN** tag `v0.1.0` and a GitHub Release with the 0.1.0 changelog section exist
