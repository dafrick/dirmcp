## ADDED Requirements

### Requirement: Path option
Each instance SHALL have an option `path` (default `/dirmcp`) passed to the server as `--path`.

#### Scenario: Instance on /notes
- **WHEN** an instance sets `path = "/notes"`
- **THEN** it answers on `/notes` only
