package shell

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dafrick/dirmcp/internal/confine"
	"github.com/dafrick/dirmcp/internal/manifest"
)

// The test binary doubles as the confinement helper, like dirmcp itself.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == confine.HelperArg {
		confine.RunHelper(os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] == confine.SelfTestArg {
		if err := confine.SelfTest(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "__dial" { // see TestNoNetwork
		_, err := net.Dial("tcp", os.Args[2])
		if err != nil {
			os.Exit(3)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type env struct {
	t    *testing.T
	root string
	cfg  *Config
}

func setup(t *testing.T, writable bool) *env {
	t.Helper()
	m, err := manifest.Load(os.Getenv("DIRMCP_TOOLS"))
	if err != nil {
		t.Skip("no tools manifest:", err)
	}
	self, err := os.Executable()
	must(t, err)
	if err := confine.CheckKernel(self); err != nil {
		if os.Getenv("DIRMCP_REQUIRE_SANDBOX") != "" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
	base := t.TempDir()
	root := filepath.Join(base, "root")
	must(t, os.MkdirAll(filepath.Join(root, "Inbox"), 0o755))
	must(t, os.WriteFile(filepath.Join(root, "a.md"), []byte("alpha\n#old tag\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(root, "Inbox", "b.md"), []byte("beta #old\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(base, "secret"), []byte("TOP SECRET\n"), 0o644))
	must(t, os.Symlink(filepath.Join(base, "secret"), filepath.Join(root, "link")))
	return &env{t, root, &Config{Root: root, Writable: writable, Tools: m, Self: self}}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func (e *env) run(cmd string) (string, int) {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var out bytes.Buffer
	code, err := Run(ctx, e.cfg, cmd, e.t.TempDir(), &out)
	if err != nil {
		e.t.Fatalf("%s: %v", cmd, err)
	}
	return out.String(), code
}

func (e *env) ok(cmd, want string) {
	e.t.Helper()
	out, code := e.run(cmd)
	if code != 0 || !strings.Contains(out, want) {
		e.t.Errorf("%s: exit %d, output %q; want exit 0 and %q", cmd, code, out, want)
	}
}

// denied is either outcome of touching something outside the root: the
// path does not exist in the program's root, or Landlock refuses it.
const denied = "(denied)"

func (e *env) fails(cmd, want string) {
	e.t.Helper()
	out, code := e.run(cmd)
	ok := strings.Contains(out, want)
	if want == denied {
		ok = strings.Contains(out, "ermission denied") || strings.Contains(out, "No such file")
	}
	if code == 0 || !ok {
		e.t.Errorf("%s: exit %d, output %q; want failure mentioning %q", cmd, code, out, want)
	}
}

func TestToolsWork(t *testing.T) {
	e := setup(t, false)
	e.ok("ls", "a.md")
	e.ok("cat a.md | wc -l", "2")
	e.ok("rg -l old | sort", "Inbox/b.md")
	e.ok("grep -c alpha a.md", "1")
	e.ok("find . -name '*.md' | sort | head -1", "./Inbox/b.md")
	e.ok("for f in *.md; do echo \"file $f\"; done", "file a.md")
	e.ok("echo $(cat Inbox/b.md)", "beta")
	e.ok("rg --help | head -1", "ripgrep")
	e.ok("awk '{print NR}' a.md | tail -1", "2")
	e.ok("cd Inbox && pwd", "/Inbox")
}

func TestOnlyAllowedPrograms(t *testing.T) {
	e := setup(t, false)
	e.fails("curl https://example.com", "not allowed")
	e.fails("/bin/sh -c id", "only program names")
	e.fails("./a.md", "only program names")
	e.fails("bash -c id", "not allowed")
	e.fails("xargs cat < a.md", "not allowed")
	e.fails("find . -exec cat {} ';'", "-exec")
	e.fails("rg --pre cat x", "--pre")
	e.fails("eval 'cat a.md'", "not allowed")
	e.fails(". ./a.md", "not allowed")
	e.fails("kill -9 1", "not allowed")
	e.fails("exec cat a.md", "not allowed")
	e.fails("command cat a.md", "not allowed")
	e.fails("awk 'BEGIN{system(\"id\")}'", "sandbox")
	e.fails("sed -n '1e id' a.md", "")
}

func TestReadOutsideRoot(t *testing.T) {
	e := setup(t, false)
	e.fails("cat /etc/passwd", denied)
	e.fails("cat ../secret", denied)
	e.fails("cat link", denied) // symlink out of the root
	e.fails("cat < ../secret", "")
	e.fails("cat < link", "")
	e.fails("ls /", denied)
	e.fails("cd /etc", "outside")
	e.fails("cd ..", "outside")
	e.fails("cd link", "")
	out, _ := e.run("echo /etc/*")
	if strings.Contains(out, "passwd") {
		t.Errorf("glob outside root listed files: %q", out)
	}
	for _, cmd := range []string{"cat link", "cat ../secret", "cat < link"} {
		if out, _ := e.run(cmd); strings.Contains(out, "TOP SECRET") {
			t.Errorf("%s leaked the secret", cmd)
		}
	}
}

func TestReadOnly(t *testing.T) {
	e := setup(t, false)
	e.fails("echo x > a.md", "read-only")
	e.fails("echo x >> new.md", "read-only")
	e.fails("sed -i s/alpha/omega/ a.md", "")
	e.fails("touch new.md", "")
	e.fails("mkdir d", "")
	e.fails("mv a.md c.md", "")
	e.fails("rm a.md", "read-only")
	b, _ := os.ReadFile(filepath.Join(e.root, "a.md"))
	if string(b) != "alpha\n#old tag\n" {
		t.Errorf("a.md changed: %q", b)
	}
}

func TestWritable(t *testing.T) {
	e := setup(t, true)
	e.ok("for f in $(rg -l '#old'); do sed -i 's/#old/#new/g' \"$f\"; done; rg -c '#new' | sort", "Inbox/b.md:1")
	e.ok("echo hi > Inbox/c.md && cat Inbox/c.md", "hi")
	e.ok("mkdir -p x/y && mv Inbox/c.md x/y/ && ls x/y", "c.md")
	e.ok("rm -rf x && ls -A .trash", "x.")
	if _, err := os.Stat(filepath.Join(e.root, "x")); !os.IsNotExist(err) {
		t.Errorf("x still there after rm")
	}
	// RLIMIT_FSIZE (100 MiB): the write stops at the limit.
	e.run("head -c 200000000 /dev/zero > big")
	if st, err := os.Stat(filepath.Join(e.root, "big")); err != nil || st.Size() > 100<<20 {
		t.Errorf("file size limit not applied: %v %v", st.Size(), err)
	}
	e.fails("rm .", "outside")
	e.fails("rm ../secret", "outside")
	e.fails("echo x > ../escape", "")
	e.fails("cp a.md ../escape", "")
	if _, err := os.Stat(filepath.Join(filepath.Dir(e.root), "escape")); err == nil {
		t.Errorf("wrote outside the root")
	}
}

func TestStopped(t *testing.T) {
	e := setup(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var out bytes.Buffer
	start := time.Now()
	code, err := Run(ctx, e.cfg, "tail -f a.md", t.TempDir(), &out)
	if err != nil || code != 124 || time.Since(start) > 10*time.Second {
		t.Errorf("tail -f: code %d, err %v, took %v, out %q", code, err, time.Since(start), out.String())
	}
}

var raceBuild bool

// A program under the helper cannot open TCP connections or run binaries
// outside the tools' closure. The test binary plays the program; Bind makes
// it visible inside the new root.
func TestNoNetworkNoForeignExec(t *testing.T) {
	if raceBuild {
		t.Skip("race build")
	}
	e := setup(t, false)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	defer ln.Close()
	self := e.cfg.Self
	run := func(execPaths, bind []string) int {
		base := t.TempDir()
		os.Mkdir(filepath.Join(base, "stage"), 0o700)
		c, err := confine.Command(self, confine.Spec{Root: e.root, Stage: filepath.Join(base, "stage"), Exec: execPaths, Bind: bind},
			self, "__dial", ln.Addr().String())
		must(t, err)
		c.Dir = e.root
		var stderr bytes.Buffer
		c.Stderr = &stderr
		defer func() { t.Log(stderr.String()) }()
		if err := c.Run(); err != nil {
			return c.ProcessState.ExitCode()
		}
		return 0
	}
	dir := []string{filepath.Dir(self)}
	// Allowed to exec itself, but dialing must fail (exit 3).
	if code := run(append(e.cfg.Tools.Paths, dir...), dir); code != 3 {
		t.Errorf("dial in the sandbox: exit %d, want 3", code)
	}
	// Visible but not in the allowed executables: execve fails (exit 126).
	if code := run(e.cfg.Tools.Paths, dir); code != 126 {
		t.Errorf("exec outside closure: exit %d, want 126", code)
	}
}

// Paths outside the root do not exist for programs, not even for stat.
func TestNoMetadataOutside(t *testing.T) {
	e := setup(t, false)
	for _, cmd := range []string{"stat /etc/passwd", "stat -c %s /etc/shadow", "readlink /proc/self", "stat /usr", "du -s /etc", "realpath -e /etc/hosts", "file /etc/passwd"} {
		out, code := e.run(cmd)
		if code == 0 && !strings.Contains(out, "No such file") && !strings.Contains(out, "cannot open") {
			t.Errorf("%s: exit %d, output %q", cmd, code, out)
		}
		if strings.Contains(out, "root") && strings.Contains(cmd, "passwd") && strings.Contains(out, ":") && !strings.Contains(out, "No such") {
			t.Errorf("%s leaked: %q", cmd, out)
		}
	}
	e.ok("stat -f -c %T /", "tmpfs") // the new root, not the host's
}

// Many starts, as many at a time as the server allows (jobs.Running = 4):
// the helper must never fail on its own (it once ran out of address space
// between setting rlimits and execve). A pass makes a regression unlikely,
// it does not prove its absence.
func TestManyStarts(t *testing.T) {
	e := setup(t, true)
	errs := make(chan string, 400)
	var mu sync.Mutex
	var slowest time.Duration
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Go(func() {
			for i := 0; i < 100; i++ {
				start := time.Now()
				out, code := e.run("echo hi > Inbox/w.md && cat a.md | wc -l")
				d := time.Since(start)
				mu.Lock()
				slowest = max(slowest, d)
				mu.Unlock()
				if code != 0 || strings.TrimSpace(out) != "2" {
					errs <- fmt.Sprintf("after %v: exit %d: %.300q", d, code, out)
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	t.Logf("400 command lines, slowest %v", slowest)
	for msg := range errs {
		t.Error(msg)
	}
}
