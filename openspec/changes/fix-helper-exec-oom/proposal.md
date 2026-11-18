## Why

CI on `main` failed once with `runtime: out of memory` in the exec helper (#2). Once the 1 GiB address-space rlimit is set, any Go runtime allocation before `execve` (a new thread, heap growth, `syscall.Exec`'s own work) can fail, because the runtime has already reserved more address space than the program may use. v0.2.0 was not released because of it.

## What Changes

- The helper prepares path, argv, env and the rlimit values first, then sets the rlimits and calls `execve` with raw syscalls, with no Go runtime work in between.
- Test `TestManyStarts`: 200 command lines, four in parallel.
- Bump to `0.2.1`.

## Capabilities

### New Capabilities

### Modified Capabilities

## Impact

`internal/confine/confine.go`, `internal/shell/shell_test.go`, CHANGELOG, VERSION. Behaviour is unchanged.
