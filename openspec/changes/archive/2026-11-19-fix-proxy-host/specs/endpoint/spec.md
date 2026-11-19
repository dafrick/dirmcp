## ADDED Requirements

### Requirement: Host check
The server SHALL refuse with 403, before authentication, any request whose Host header is neither a localhost name or loopback IP nor one of the names given with `--allowed-host` (case and port ignored).

#### Scenario: Behind a proxy
- **WHEN** `--allowed-host notes.example.ts.net` is set and a request arrives with that Host and the token
- **THEN** it is served

#### Scenario: Rebinding
- **WHEN** a request arrives with Host `evil.example`
- **THEN** it gets 403
