package shell

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
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
		if err := confine.SelfTest(); err != nil {
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

func (e *env) fails(cmd, want string) {
	e.t.Helper()
	out, code := e.run(cmd)
	if code == 0 || !strings.Contains(out, want) {
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
	e.fails("cat /etc/passwd", "ermission denied")
	e.fails("cat ../secret", "ermission denied")
	e.fails("cat link", "ermission denied") // symlink out of the root
	e.fails("cat < ../secret", "")
	e.fails("cat < link", "")
	e.fails("ls /", "ermission denied")
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

// A program under the helper cannot open TCP connections or run binaries
// outside the tools' closure. The test binary plays the program, so its own
// directory is added to the allowed executables.
var raceBuild bool

func TestNoNetworkNoForeignExec(t *testing.T) {
	if raceBuild {
		t.Skip("race build")
	}
	e := setup(t, false)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	defer ln.Close()
	self := e.cfg.Self
	spec := func(exec []string) string {
		return `{"root":"` + e.root + `","exec":["` + strings.Join(exec, `","`) + `"]}`
	}
	run := func(exec []string, args ...string) int {
		c := mustCmd(self, append([]string{confine.HelperArg, spec(exec), "--", self}, args...)...)
		err := c.Run()
		if err == nil {
			return 0
		}
		return c.ProcessState.ExitCode()
	}
	// Allowed to exec itself, but dialing must fail (exit 3).
	if code := run(append(e.cfg.Tools.Paths, filepath.Dir(self)), "__dial", ln.Addr().String()); code != 3 {
		t.Errorf("dial under Landlock: exit %d, want 3 (connection refused by Landlock)", code)
	}
	// Not in the allowed executables: execve fails in the helper (exit 126).
	if code := run(e.cfg.Tools.Paths, "__dial", ln.Addr().String()); code != 126 {
		t.Errorf("exec outside closure: exit %d, want 126", code)
	}
}
