## ADDED Requirements

### Requirement: Endpoint path
The server SHALL serve MCP only at the path given with `--path` (default `/dirmcp`) and answer 404 on every other path. It SHALL refuse to start with a path that is not `/name[/name…]` of letters, digits and `-_.`, or that ends in `/` or contains `..`.

#### Scenario: Custom path
- **WHEN** the server runs with `--path /notes`
- **THEN** `POST /notes` with the token is served and `POST /dirmcp` gets 404

## MODIFIED Requirements

### Requirement: Bearer token
The server SHALL serve MCP over Streamable HTTP at its endpoint path (`--path`, default `/dirmcp`) and reject every request without the exact bearer token from `--token-file`, compared in constant time, before any MCP processing. It SHALL refuse to start if the token is shorter than 32 bytes or the file is readable by group or others.

#### Scenario: No or wrong token
- **WHEN** a request has no `Authorization` header or a wrong token
- **THEN** the server answers 401 and runs nothing

#### Scenario: Weak token file
- **WHEN** the token file holds 16 bytes or has mode 0644
- **THEN** the server exits with an error at startup
