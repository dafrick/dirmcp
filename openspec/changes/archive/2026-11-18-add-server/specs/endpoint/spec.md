## ADDED Requirements

### Requirement: Bearer token
The server SHALL serve MCP over Streamable HTTP at `/mcp` and reject every request without the exact bearer token from `--token-file`, compared in constant time, before any MCP processing. It SHALL refuse to start if the token is shorter than 32 bytes or the file is readable by group or others.

#### Scenario: No or wrong token
- **WHEN** a request has no `Authorization` header or a wrong token
- **THEN** the server answers 401 and runs nothing

#### Scenario: Weak token file
- **WHEN** the token file holds 16 bytes or has mode 0644
- **THEN** the server exits with an error at startup

### Requirement: Request limits and logging
The server SHALL limit request bodies to 2 MiB, header reads to 10 s and running commands to 4 at once, and SHALL log each tool call (tool, first 200 characters of the command, exit code, duration, output size) without file contents or the token.

#### Scenario: Busy
- **WHEN** a fifth command is started while four run
- **THEN** the call returns "busy, retry" and starts nothing
