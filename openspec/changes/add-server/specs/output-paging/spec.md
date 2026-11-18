## ADDED Requirements

### Requirement: Truncate and page
A call SHALL wait at most 15 s. If the output exceeds 30 KB or the command is still running, the result SHALL hold the first 30 KB cut at a line boundary and a note with the total size, whether it still runs, and an id for `more`. `more(id)` SHALL return the next chunk of up to 30 KB, waiting up to 15 s for new output of a running command.

#### Scenario: Long output
- **WHEN** a command prints 100 KB
- **THEN** the result has about 30 KB and a note; three more `more` calls return the rest, the last one says it is complete

#### Scenario: Slow command
- **WHEN** a command runs for 40 s
- **THEN** the first call returns after 15 s with output so far, and `more` returns the rest when it finishes

### Requirement: Bounded buffers
Buffers SHALL live in RAM only, at most 20 MB per call and 64 MB in total (oldest dropped first), and be dropped 10 minutes after their last read or once fully read and finished. Ids SHALL be 128-bit random.

#### Scenario: Expired
- **WHEN** `more` is called with an id older than 10 minutes since its last read
- **THEN** it returns "unknown or expired id"
