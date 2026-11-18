## Recon

Checked 2026-11-18: failure log of the `main` CI run after #2 (`TestWritable`, `fatal error: runtime: out of memory` in the helper). Go's `syscall.Exec` calls into the runtime before `execve`; `golang.org/x/sys/unix` has `RawSyscall`, `SYS_PRLIMIT64`, `SYS_EXECVE`, `BytePtrFromString`; `syscall.SlicePtrFromStrings` builds argv/envp.

## Decisions

- Raw `prlimit64(0, …)` and `execve` on a locked OS thread after all allocations are done. If a `prlimit` fails, exit 125 without printing (printing could allocate).
- Keep the address-space limit: dropping it would leave memory unbounded outside systemd.

## Risks / Trade-offs

- The runtime could still run (GC, preemption) between the raw syscalls; that window is a few instructions, and nothing there allocates. The stress test guards it.
