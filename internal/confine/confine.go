// Package confine starts one allowlisted program, isolated from the rest of
// the machine.
//
// The server never execs a tool directly. It starts its own binary as
//
//	dirmcp __exec <spec-json> -- <program path> <args...>
//
// in new user, mount and network namespaces (see Command). That helper
//
//  1. builds a new root that holds only the served directory, the Nix store,
//     the job's scratch directory and /dev/null, /dev/zero, /dev/urandom, and
//     pivots into it: nothing else (no /etc, /home, /proc) exists by path,
//     not even for stat or readlink;
//  2. applies Landlock: read the root and the tools' closure, write only if
//     allowed, execute only the closure, no TCP, scoped IPC;
//  3. sets rlimits;
//  4. replaces itself with the program (execve). Namespaces, Landlock and
//     rlimits are inherited and cannot be lifted; capabilities in the user
//     namespace are dropped by execve (the uid is not 0).
package confine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"syscall"
	"unsafe"

	"github.com/landlock-lsm/go-landlock/landlock"
	ll "github.com/landlock-lsm/go-landlock/landlock/syscall"
	"golang.org/x/sys/unix"
)

// HelperArg is the first argument that turns the binary into the helper.
const HelperArg = "__exec"

// SelfTestArg makes the binary check that the kernel supports what we need.
const SelfTestArg = "__selftest"

// Spec describes what the started program may access.
type Spec struct {
	Root     string   `json:"root"`           // served directory
	Writable bool     `json:"writable"`       // may the program change files under Root
	Tmp      string   `json:"tmp"`            // private scratch dir (read-write), may be empty
	Stage    string   `json:"stage"`          // empty dir, becomes the new root
	Exec     []string `json:"exec"`           // store paths: readable and executable
	Bind     []string `json:"bind,omitempty"` // tests only: extra paths, readable (executable only if also in Exec)
}

// Limits for every started program. Wall time is enforced by the caller.
var limits = []struct {
	res int
	val uint64
}{
	{unix.RLIMIT_CPU, 60},          // seconds
	{unix.RLIMIT_FSIZE, 100 << 20}, // 100 MiB per written file
	{unix.RLIMIT_NOFILE, 256},      // open files
	{unix.RLIMIT_CORE, 0},          // no core dumps
	{unix.RLIMIT_MEMLOCK, 0},       // no locked memory
	{unix.RLIMIT_MSGQUEUE, 0},      // no POSIX message queues
	{unix.RLIMIT_AS, 1 << 30},      // 1 GiB address space; last, see limitAndExec
}

const (
	rootRead  = ll.AccessFSReadFile | ll.AccessFSReadDir
	rootWrite = ll.AccessFSWriteFile | ll.AccessFSTruncate | ll.AccessFSRemoveFile |
		ll.AccessFSRemoveDir | ll.AccessFSMakeReg | ll.AccessFSMakeDir | ll.AccessFSRefer
	storeAccess = ll.AccessFSReadFile | ll.AccessFSReadDir | ll.AccessFSExecute
	devAccess   = ll.AccessFSReadFile | ll.AccessFSWriteFile
)

// abi is the Landlock version we require: filesystem incl. truncate and
// refer, TCP rules (4), ioctl on devices (5), IPC scoping (6).
var abi = landlock.V6

var devices = []string{"/dev/null", "/dev/zero", "/dev/urandom"}

// Command returns the command that runs program with args under spec.
// self is the dirmcp binary.
func Command(self string, s Spec, program string, args ...string) (*exec.Cmd, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(self, append([]string{HelperArg, string(b), "--", program}, args...)...)
	cmd.Env = []string{}
	cmd.SysProcAttr = attr()
	return cmd, nil
}

// attr: new user, mount and network namespaces, own process group, killed
// if the server dies. Our uid and gid become 0 inside, so the helper keeps
// the capabilities it needs to build the new root across its execve; it
// drops all of them before starting the program (dropCaps).
func attr() *syscall.SysProcAttr {
	uid, gid := os.Getuid(), os.Getgid()
	return &syscall.SysProcAttr{
		Cloneflags:  syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET,
		UidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: uid, Size: 1}},
		GidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: gid, Size: 1}},
		Setpgid:     true,
		Pdeathsig:   syscall.SIGKILL,
	}
}

// rules returns the Landlock rules for spec.
func rules(s Spec) []landlock.Rule {
	root := landlock.AccessFSSet(rootRead)
	if s.Writable {
		root |= rootWrite
	}
	r := []landlock.Rule{
		landlock.PathAccess(root, s.Root),
		landlock.PathAccess(storeAccess, s.Exec...),
		landlock.PathAccess(devAccess, "/dev/null"),
		landlock.PathAccess(ll.AccessFSReadFile, "/dev/zero", "/dev/urandom"),
	}
	if s.Tmp != "" {
		r = append(r, landlock.PathAccess(rootRead|rootWrite, s.Tmp))
	}
	if len(s.Bind) > 0 {
		r = append(r, landlock.PathAccess(rootRead, s.Bind...)) // visible, not executable
	}
	// No NetRule: all TCP bind and connect is denied. Restrict also scopes
	// abstract UNIX sockets and signals to the sandbox.
	return r
}

// isolate builds the new root in s.Stage and pivots into it. It must run in
// the helper, inside the namespaces from attr.
func isolate(s Spec) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	// Nothing we mount here may reach the host's mount table.
	if err := unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("make mounts private: %w", err)
	}
	if err := unix.Mount("tmpfs", s.Stage, "tmpfs", unix.MS_NOSUID|unix.MS_NODEV, "mode=0755,size=1m"); err != nil {
		return fmt.Errorf("mount new root: %w", err)
	}
	bind := func(src string) error {
		st, err := os.Stat(src)
		if err != nil {
			return err
		}
		dst := filepath.Join(s.Stage, src)
		if st.IsDir() {
			err = os.MkdirAll(dst, 0o755)
		} else if err = os.MkdirAll(filepath.Dir(dst), 0o755); err == nil {
			err = os.WriteFile(dst, nil, 0o644)
		}
		if err != nil {
			return err
		}
		if err := unix.Mount(src, dst, "", unix.MS_BIND|unix.MS_REC, ""); err != nil {
			return fmt.Errorf("bind %s: %w", src, err)
		}
		return nil
	}
	paths := append([]string{s.Root, "/nix/store"}, devices...)
	if s.Tmp != "" {
		paths = append(paths, s.Tmp)
	}
	for _, p := range append(paths, s.Bind...) {
		if err := bind(p); err != nil {
			return err
		}
	}
	old := filepath.Join(s.Stage, ".old")
	if err := os.Mkdir(old, 0o700); err != nil {
		return err
	}
	if err := unix.PivotRoot(s.Stage, old); err != nil {
		return fmt.Errorf("pivot_root: %w", err)
	}
	if err := unix.Chdir("/"); err != nil {
		return err
	}
	if err := unix.Unmount("/.old", unix.MNT_DETACH); err != nil {
		return fmt.Errorf("detach old root: %w", err)
	}
	if err := os.Remove("/.old"); err != nil {
		return err
	}
	// The new root itself (our tmpfs) becomes read-only.
	if err := unix.Mount("", "/", "", unix.MS_REMOUNT|unix.MS_BIND|unix.MS_RDONLY|unix.MS_NOSUID|unix.MS_NODEV, ""); err != nil {
		return fmt.Errorf("remount new root read-only: %w", err)
	}
	if err := unix.Chdir(cwd); err != nil {
		return fmt.Errorf("working directory %s is not inside the new root: %w", cwd, err)
	}
	return nil
}

// dropCaps removes every capability for good: uid 0 in the namespace gets
// none back on execve (securebits, locked), the bounding and ambient sets
// are emptied, and the current sets are cleared.
func dropCaps() error {
	const (
		noRoot, noRootLocked     = 1 << 0, 1 << 1
		noFixup, noFixupLocked   = 1 << 2, 1 << 3
		keepCapsLocked           = 1 << 5
		noAmbient, noAmbientLock = 1 << 6, 1 << 7
	)
	bits := noRoot | noRootLocked | noFixup | noFixupLocked | keepCapsLocked | noAmbient | noAmbientLock
	if err := unix.Prctl(unix.PR_SET_SECUREBITS, uintptr(bits), 0, 0, 0); err != nil {
		return fmt.Errorf("securebits: %w", err)
	}
	if err := unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_CLEAR_ALL, 0, 0, 0); err != nil {
		return fmt.Errorf("ambient caps: %w", err)
	}
	for c := 0; c <= unix.CAP_LAST_CAP; c++ {
		if err := unix.Prctl(unix.PR_CAPBSET_DROP, uintptr(c), 0, 0, 0); err != nil && !errors.Is(err, unix.EINVAL) {
			return fmt.Errorf("drop cap %d: %w", c, err)
		}
	}
	hdr := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	var data [2]unix.CapUserData
	if err := unix.Capset(&hdr, &data[0]); err != nil {
		return fmt.Errorf("capset: %w", err)
	}
	return nil
}

// RunHelper is the helper's main. It never returns on success.
// args are the arguments after HelperArg.
func RunHelper(args []string) {
	if err := helper(args); err != nil {
		fmt.Fprintf(os.Stderr, "dirmcp: %v\n", err)
		os.Exit(126)
	}
}

func helper(args []string) error {
	// Capabilities and securebits are per thread: everything from dropping
	// them to execve must happen on one OS thread. Never unlocked.
	runtime.LockOSThread()
	if len(args) < 3 || args[1] != "--" {
		return errors.New("usage: __exec <spec> -- <path> [args...]")
	}
	var s Spec
	if err := json.Unmarshal([]byte(args[0]), &s); err != nil {
		return fmt.Errorf("bad spec: %w", err)
	}
	if s.Root == "" || s.Stage == "" || len(s.Exec) == 0 {
		return errors.New("bad spec: root, stage and exec are required")
	}
	argv := args[2:]
	if err := isolate(s); err != nil {
		return fmt.Errorf("isolate: %w", err)
	}
	if err := dropCaps(); err != nil {
		return err
	}
	// Not BestEffort: refuse to run if the kernel cannot enforce all of it.
	if err := abi.Restrict(rules(s)...); err != nil {
		return fmt.Errorf("landlock: %w", err)
	}
	env := []string{
		"LC_ALL=C.UTF-8",
		"HOME=" + s.Root,
		"PATH=", // tools never find other programs by name
	}
	if s.Tmp != "" {
		env = append(env, "TMPDIR="+s.Tmp)
	}
	return limitAndExec(argv, env)
}

// limitAndExec sets the rlimits and execs, with raw syscalls only. Once the
// address-space limit is set, any allocation by the Go runtime (a new thread,
// heap growth, syscall.Exec's own work) can fail with "out of memory",
// because the runtime has already reserved more than the program may use.
// So everything is prepared first, and nothing between the first prlimit and
// execve touches the runtime.
func limitAndExec(argv, env []string) error {
	path, err := unix.BytePtrFromString(argv[0])
	if err != nil {
		return err
	}
	argvp, err := syscall.SlicePtrFromStrings(argv)
	if err != nil {
		return err
	}
	envp, err := syscall.SlicePtrFromStrings(env)
	if err != nil {
		return err
	}
	rl := make([]unix.Rlimit, len(limits))
	for i, l := range limits {
		rl[i] = unix.Rlimit{Cur: l.val, Max: l.val}
	}
	limitErr := []byte("dirmcp: cannot set resource limits\n")
	execErr := []byte("dirmcp: cannot start " + argv[0] + "\n")
	// No collection (and no GC worker allocations) from here on.
	runtime.GC()
	debug.SetGCPercent(-1)
	for i, l := range limits {
		if _, _, e := unix.RawSyscall6(unix.SYS_PRLIMIT64, 0, uintptr(l.res), uintptr(unsafe.Pointer(&rl[i])), 0, 0, 0); e != 0 {
			rawFail(limitErr, 125) // never exec with only some limits
		}
	}
	unix.RawSyscall(unix.SYS_EXECVE, uintptr(unsafe.Pointer(path)),
		uintptr(unsafe.Pointer(&argvp[0])), uintptr(unsafe.Pointer(&envp[0])))
	rawFail(execErr, 126) // execve returned: it failed
	return nil
}

// rawFail writes msg to stderr and exits, with raw syscalls only (anything
// else could need memory the address-space limit no longer allows).
func rawFail(msg []byte, code int) {
	unix.RawSyscall(unix.SYS_WRITE, 2, uintptr(unsafe.Pointer(&msg[0])), uintptr(len(msg)))
	unix.RawSyscall(unix.SYS_EXIT_GROUP, uintptr(code), 0, 0)
}

// CheckKernel runs self with SelfTestArg in the same namespaces as a real
// program and reports whether isolation and Landlock work. Never call
// SelfTest in the server process: none of it can be undone.
func CheckKernel(self string) error {
	base, err := os.MkdirTemp("", "dirmcp-selftest-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(base)
	for _, d := range []string{"root", "tmp", "stage"} {
		if err := os.Mkdir(filepath.Join(base, d), 0o700); err != nil {
			return err
		}
	}
	cmd := exec.Command(self, SelfTestArg, base)
	cmd.Dir = filepath.Join(base, "root") // programs always start inside the root
	cmd.Env = []string{}
	cmd.SysProcAttr = attr()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("user namespaces and Landlock ABI 6 needed: %s (%w)", bytes.TrimSpace(out), err)
	}
	return nil
}

// SelfTest isolates and restricts the calling process like the helper does.
// Only for the SelfTestArg child; base comes from CheckKernel.
func SelfTest(base string) error {
	s := Spec{Root: filepath.Join(base, "root"), Writable: true, Tmp: filepath.Join(base, "tmp"),
		Stage: filepath.Join(base, "stage"), Exec: []string{"/nix/store"}}
	runtime.LockOSThread() // see helper
	if err := isolate(s); err != nil {
		return err
	}
	if _, err := os.Stat("/etc"); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("/etc is still visible after isolation (%v)", err)
	}
	if err := dropCaps(); err != nil {
		return err
	}
	return abi.Restrict(rules(s)...)
}
