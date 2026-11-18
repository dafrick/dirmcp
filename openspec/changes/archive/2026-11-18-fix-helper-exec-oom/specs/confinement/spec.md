## MODIFIED Requirements

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
