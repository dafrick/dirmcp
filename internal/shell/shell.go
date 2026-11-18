// Package shell runs one command line in the served directory.
//
// The command line is interpreted by mvdan.cc/sh, never by a system shell.
// Every program it wants to start goes through exec below: only programs in
// the tools manifest, only with allowed options (argcheck), and always
// through the confinement helper (Landlock, rlimits). Shell built-ins that
// would load or run other code, signal processes, or leave the root are
// blocked; file access by the interpreter itself (redirections, globbing,
// tests) goes through os.Root.
package shell

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dafrick/dirmcp/internal/argcheck"
	"github.com/dafrick/dirmcp/internal/confine"
	"github.com/dafrick/dirmcp/internal/manifest"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// Config is fixed for the lifetime of the server.
type Config struct {
	Root     string // absolute, symlinks resolved
	Writable bool
	Tools    *manifest.Manifest
	Self     string // path of the dirmcp binary (the helper)
}

// TrashDir is where the rm built-in moves files, relative to the root.
const TrashDir = ".trash"

// blocked built-ins: they run code from strings or files, start programs
// past the exec allowlist, signal processes, or change process state.
var blocked = map[string]string{
	".": "runs a file", "source": "runs a file", "eval": "runs a string",
	"exec": "replaces the shell", "command": "bypasses checks", "builtin": "bypasses checks",
	"kill": "signals processes", "newgrp": "changes groups", "enable": "changes built-ins",
	"ulimit": "changes limits", "umask": "changes file modes", "trap": "runs code on signals",
	"bg": "job control", "fg": "job control", "disown": "job control", "suspend": "job control",
	"fc": "history", "history": "history", "bind": "line editing", "hash": "path lookup",
	"logout": "not a login shell",
}

// Run interprets command in the root and writes stdout and stderr to w. It
// returns the exit status. scratch is an empty private directory for this
// command line; the caller removes it afterwards.
func Run(ctx context.Context, c *Config, command, scratch string, w io.Writer) (int, error) {
	// Both sides of a pipeline write stderr here from different goroutines.
	out := &lockedWriter{w: w}
	file, err := syntax.NewParser().Parse(strings.NewReader(command), "")
	if err != nil {
		fmt.Fprintf(out, "syntax error: %v\n", err)
		return 2, nil
	}
	root, err := os.OpenRoot(c.Root)
	if err != nil {
		return 0, err
	}
	defer root.Close()
	f := &fsys{c: c, root: root, tmp: filepath.Join(scratch, "tmp"), stage: filepath.Join(scratch, "stage")}
	for _, d := range []string{f.tmp, f.stage} {
		if err := os.Mkdir(d, 0o700); err != nil {
			return 0, err
		}
	}
	r, err := interp.New(
		interp.Dir(c.Root),
		interp.Env(expand.ListEnviron("HOME="+c.Root, "LC_ALL=C.UTF-8")),
		interp.StdIO(nil, out, out),
		interp.CallHandler(f.call),
		interp.ExecHandlers(func(interp.ExecHandlerFunc) interp.ExecHandlerFunc { return f.exec }),
		interp.OpenHandler(f.open),
		interp.ReadDirHandler2(f.readDir),
		interp.StatHandler(f.stat),
		interp.AccessHandler(f.access),
	)
	if err != nil {
		return 0, err
	}
	err = r.Run(ctx, file)
	if status, ok := interp.IsExitStatus(err); ok {
		return int(status), nil
	}
	if err != nil {
		if ctx.Err() != nil {
			fmt.Fprintf(out, "dirmcp: stopped: %v\n", context.Cause(ctx))
			return 124, nil
		}
		fmt.Fprintf(out, "dirmcp: %v\n", err)
		return 1, nil
	}
	return 0, nil
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

type fsys struct {
	c     *Config
	root  *os.Root
	tmp   string // scratch for the programs (TMPDIR)
	stage string // mount point for the programs' new root
}

// rel maps p (absolute or relative to dir) to a path relative to the root.
// ok is false if it lies outside. Symlinks are resolved by os.Root on use.
func (f *fsys) rel(dir, p string) (string, bool) {
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	r, err := filepath.Rel(f.c.Root, filepath.Clean(p))
	if err != nil || r == ".." || strings.HasPrefix(r, "../") {
		return "", false
	}
	return r, true
}

func (f *fsys) call(ctx context.Context, args []string) ([]string, error) {
	if len(args) == 0 {
		return args, nil
	}
	if why, ok := blocked[args[0]]; ok {
		return nil, fmt.Errorf("%s: not allowed (%s)", args[0], why)
	}
	switch args[0] {
	case "cd", "pushd":
		hc := interp.HandlerCtx(ctx)
		for _, a := range args[1:] {
			if strings.HasPrefix(a, "-") && a != "-" {
				continue
			}
			if a == "-" {
				return nil, errors.New("cd -: not allowed")
			}
			if err := f.inside(hc.Dir, a); err != nil {
				return nil, fmt.Errorf("%s: %s: %w", args[0], a, err)
			}
		}
	}
	return args, nil
}

// inside checks that p is a directory inside the root, after symlinks.
func (f *fsys) inside(dir, p string) error {
	r, ok := f.rel(dir, p)
	if !ok {
		return errors.New("outside the served directory")
	}
	// os.Root refuses paths that escape through symlinks.
	st, err := f.root.Stat(r)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return errors.New("not a directory")
	}
	abs, err := filepath.EvalSymlinks(filepath.Join(f.c.Root, r))
	if err != nil {
		return err
	}
	if _, ok := f.rel("/", abs); !ok {
		return errors.New("outside the served directory")
	}
	return nil
}

func (f *fsys) exec(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	name := args[0]
	if strings.ContainsRune(name, '/') {
		fmt.Fprintf(hc.Stderr, "%s: only program names are allowed, not paths\n", name)
		return interp.ExitStatus(127)
	}
	if name == "rm" {
		return f.rm(hc, args[1:])
	}
	prog := name
	if prog == "awk" {
		prog = "gawk"
	}
	path, ok := f.c.Tools.Programs[prog]
	if !ok {
		fmt.Fprintf(hc.Stderr, "%s: not allowed; allowed programs: %s, rm\n", name, strings.Join(argcheck.Programs(), " "))
		return interp.ExitStatus(127)
	}
	full, err := argcheck.Check(name, args[1:])
	if err != nil {
		fmt.Fprintf(hc.Stderr, "%v\n", err)
		return interp.ExitStatus(2)
	}
	cmd, err := confine.Command(f.c.Self, confine.Spec{Root: f.c.Root, Writable: f.c.Writable,
		Tmp: f.tmp, Stage: f.stage, Exec: f.c.Tools.Paths}, path, full...)
	if err != nil {
		return err
	}
	cmd.Dir = hc.Dir
	if file, ok := hc.Stdin.(*os.File); ok && file != nil {
		cmd.Stdin = file
	}
	cmd.Stdout, cmd.Stderr = hc.Stdout, hc.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(hc.Stderr, "%s: %v\n", name, err)
		return interp.ExitStatus(126)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		err = <-done
		return ctx.Err()
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code := ee.ExitCode()
		if code < 0 {
			code = 128 + int(ee.Sys().(syscall.WaitStatus).Signal())
		}
		return interp.ExitStatus(uint8(code))
	}
	return err
}

// rm moves its operands to the trash instead of deleting them.
func (f *fsys) rm(hc interp.HandlerContext, args []string) error {
	if !f.c.Writable {
		fmt.Fprintln(hc.Stderr, "rm: read-only")
		return interp.ExitStatus(1)
	}
	status := uint8(0)
	opts := true
	for _, a := range args {
		if opts && a == "--" {
			opts = false
			continue
		}
		if opts && strings.HasPrefix(a, "-") && len(a) > 1 {
			continue // -r, -f, -v…: the whole operand always goes to the trash
		}
		r, ok := f.rel(hc.Dir, a)
		if !ok || r == "." {
			fmt.Fprintf(hc.Stderr, "rm: %s: outside the served directory\n", a)
			status = 1
			continue
		}
		if r == TrashDir || strings.HasPrefix(r, TrashDir+"/") {
			fmt.Fprintf(hc.Stderr, "rm: %s: already in the trash\n", a)
			status = 1
			continue
		}
		if _, err := f.root.Lstat(r); err != nil {
			fmt.Fprintf(hc.Stderr, "rm: %s: %v\n", a, err)
			status = 1
			continue
		}
		if err := f.root.MkdirAll(TrashDir, 0o770); err != nil {
			fmt.Fprintf(hc.Stderr, "rm: %v\n", err)
			return interp.ExitStatus(1)
		}
		dst := filepath.Join(TrashDir, filepath.Base(r)+"."+time.Now().UTC().Format("20060102T150405.000000000"))
		if err := f.root.Rename(r, dst); err != nil {
			fmt.Fprintf(hc.Stderr, "rm: %s: %v\n", a, err)
			status = 1
		}
	}
	if status != 0 {
		return interp.ExitStatus(status)
	}
	return nil
}

const writeFlags = os.O_WRONLY | os.O_RDWR | os.O_CREATE | os.O_APPEND | os.O_TRUNC

func (f *fsys) open(ctx context.Context, path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
	if path == "/dev/null" {
		return os.OpenFile(path, flag, perm)
	}
	hc := interp.HandlerCtx(ctx)
	r, ok := f.rel(hc.Dir, path)
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrPermission}
	}
	if flag&writeFlags != 0 && !f.c.Writable {
		return nil, &fs.PathError{Op: "open", Path: path, Err: syscall.EROFS}
	}
	return f.root.OpenFile(r, flag, perm&0o666)
}

func (f *fsys) readDir(ctx context.Context, path string) ([]fs.DirEntry, error) {
	r, ok := f.rel(interp.HandlerCtx(ctx).Dir, path)
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: path, Err: fs.ErrPermission}
	}
	return fs.ReadDir(f.root.FS(), filepath.ToSlash(r))
}

func (f *fsys) stat(ctx context.Context, name string, follow bool) (fs.FileInfo, error) {
	r, ok := f.rel(interp.HandlerCtx(ctx).Dir, name)
	if !ok {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	if follow {
		return f.root.Stat(r)
	}
	return f.root.Lstat(r)
}

func (f *fsys) access(ctx context.Context, path string, mode interp.AccessMode) error {
	r, ok := f.rel(interp.HandlerCtx(ctx).Dir, path)
	if !ok {
		return &fs.PathError{Op: "access", Path: path, Err: fs.ErrNotExist}
	}
	if _, err := f.root.Stat(r); err != nil {
		return err
	}
	if mode&interp.AccessWrite != 0 && !f.c.Writable {
		return &fs.PathError{Op: "access", Path: path, Err: syscall.EROFS}
	}
	return nil
}
