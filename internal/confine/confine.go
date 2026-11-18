// Package confine starts one allowlisted program under Landlock and rlimits.
//
// The server never execs a tool directly. It re-executes its own binary as
//
//	dirmcp __exec <spec-json> -- <program path> <args...>
//
// and that helper process sets rlimits, applies Landlock to itself, then
// replaces itself with the program (execve). Landlock rules are inherited
// across execve and cannot be lifted.
package confine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

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
	Root     string   `json:"root"`     // served directory
	Writable bool     `json:"writable"` // may the program change files under Root
	Tmp      string   `json:"tmp"`      // private scratch dir (read-write), may be empty
	Exec     []string `json:"exec"`     // store paths: readable and executable
}

// Limits for every started program. Wall time is enforced by the caller.
var limits = []struct {
	res int
	val uint64
}{
	{unix.RLIMIT_CPU, 60},          // seconds
	{unix.RLIMIT_AS, 1 << 30},      // 1 GiB address space
	{unix.RLIMIT_FSIZE, 100 << 20}, // 100 MiB per written file
	{unix.RLIMIT_NOFILE, 256},      // open files
	{unix.RLIMIT_CORE, 0},          // no core dumps
	{unix.RLIMIT_MEMLOCK, 0},       // no locked memory
	{unix.RLIMIT_MSGQUEUE, 0},      // no POSIX message queues
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

// rules returns the Landlock rules for spec.
func rules(s Spec) []landlock.Rule {
	root := landlock.AccessFSSet(rootRead)
	if s.Writable {
		root |= rootWrite
	}
	r := []landlock.Rule{
		landlock.PathAccess(root, s.Root),
		landlock.PathAccess(storeAccess, s.Exec...),
		landlock.PathAccess(devAccess, "/dev/null").IgnoreIfMissing(),
		landlock.PathAccess(ll.AccessFSReadFile, "/dev/zero", "/dev/urandom").IgnoreIfMissing(),
	}
	if s.Tmp != "" {
		r = append(r, landlock.PathAccess(rootRead|rootWrite, s.Tmp))
	}
	// No NetRule: all TCP bind and connect is denied. Restrict also scopes
	// abstract UNIX sockets and signals to the sandbox.
	return r
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
	if len(args) < 3 || args[1] != "--" {
		return errors.New("usage: __exec <spec> -- <path> [args...]")
	}
	var s Spec
	if err := json.Unmarshal([]byte(args[0]), &s); err != nil {
		return fmt.Errorf("bad spec: %w", err)
	}
	if s.Root == "" || len(s.Exec) == 0 {
		return errors.New("bad spec: root and exec are required")
	}
	argv := args[2:]
	// Not BestEffort: refuse to run if the kernel cannot enforce all of it.
	if err := abi.Restrict(rules(s)...); err != nil {
		return fmt.Errorf("landlock: %w", err)
	}
	// Limits last: the Go runtime of this helper needs more address space
	// than the program gets.
	for _, l := range limits {
		if err := unix.Setrlimit(l.res, &unix.Rlimit{Cur: l.val, Max: l.val}); err != nil {
			return fmt.Errorf("rlimit %d: %w", l.res, err)
		}
	}
	env := []string{
		"LC_ALL=C.UTF-8",
		"HOME=" + s.Root,
		"PATH=", // tools never find other programs by name
	}
	if s.Tmp != "" {
		env = append(env, "TMPDIR="+s.Tmp)
	}
	return syscall.Exec(argv[0], argv, env)
}

// CheckKernel runs self with SelfTestArg in a child process and reports
// whether the kernel accepts our rules. Never call SelfTest in the server
// process: Landlock cannot be undone.
func CheckKernel(self string) error {
	out, err := exec.Command(self, SelfTestArg).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Landlock ABI 6 needed: %s (%w)", bytes.TrimSpace(out), err)
	}
	return nil
}

// SelfTest applies the strictest rules we use to the calling process and
// reports whether the kernel accepted them. Only for the SelfTestArg child.
func SelfTest() error {
	dir, err := os.MkdirTemp("", "dirmcp-selftest-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	return abi.Restrict(rules(Spec{Root: dir, Writable: true, Tmp: dir, Exec: []string{dir}})...)
}
