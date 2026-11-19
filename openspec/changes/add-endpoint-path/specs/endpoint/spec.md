## ADDED Requirements

### Requirement: Endpoint path
The server SHALL serve MCP only at the path given with `--path` (default `/mcp`) and answer 404 on every other path. It SHALL refuse to start with a path that is not `/name[/name…]` of letters, digits and `-_.`, or that ends in `/` or contains `..`.

#### Scenario: Custom path
- **WHEN** the server runs with `--path /notes`
- **THEN** `POST /notes` with the token is served and `POST /mcp` gets 404
