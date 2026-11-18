package argcheck

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/dafrick/dirmcp/internal/manifest"
)

func TestCheckForms(t *testing.T) {
	ok := []struct {
		prog string
		args []string
	}{
		{"ls", []string{"-la", "Inbox"}},
		{"ls", []string{"-l", "--color=never", "--sort=time", "."}},
		{"head", []string{"-n", "5", "a.md"}},
		{"head", []string{"-n5", "a.md"}},
		{"head", []string{"-5", "a.md"}},
		{"head", []string{"--lines=5", "a.md"}},
		{"grep", []string{"-rin", "-C", "2", "todo", "."}},
		{"grep", []string{"-e", "-starts-with-dash", "a.md"}},
		{"rg", []string{"-i", "-g", "*.md", "--count", "x"}},
		{"sed", []string{"-n", "1,10p", "a.md"}},
		{"sed", []string{"-i", "s/a/b/g", "a.md"}},
		{"awk", []string{"-F", ",", "{print $1}", "a.csv"}},
		{"sort", []string{"-k", "2", "-t", ",", "-n", "a.csv"}},
		{"cat", []string{"--", "-not-an-option"}},
		{"wc", []string{"-l", "-"}},
		{"find", []string{".", "-name", "*.md", "-mtime", "-7", "-print"}},
		{"find", []string{"-L", ".", "(", "-name", "a", "-o", "-name", "b", ")", "-newermt", "2026-01-01"}},
		{"diff", []string{"-u", "a.md", "b.md"}},
	}
	for _, c := range ok {
		if _, err := Check(c.prog, c.args); err != nil {
			t.Errorf("%s %v: %v", c.prog, c.args, err)
		}
	}
}

func TestForced(t *testing.T) {
	for prog, want := range map[string]string{"sed": "--sandbox", "awk": "--sandbox", "gawk": "--sandbox", "rg": "--no-config"} {
		got, err := Check(prog, []string{"x"})
		if err != nil || len(got) == 0 || got[0] != want {
			t.Errorf("%s: got %v, %v; want %s first", prog, got, err, want)
		}
	}
}

// Every option that can run programs, load code or write elsewhere.
func TestDenied(t *testing.T) {
	bad := []struct {
		prog string
		args []string
	}{
		{"find", []string{".", "-exec", "cat", "{}", ";"}},
		{"find", []string{".", "-execdir", "cat", "{}", "+"}},
		{"find", []string{".", "-ok", "rm", "{}", ";"}},
		{"find", []string{".", "-okdir", "rm", "{}", ";"}},
		{"find", []string{".", "-delete"}},
		{"find", []string{".", "-fprint", "/tmp/x"}},
		{"find", []string{".", "-fprint0", "x"}},
		{"find", []string{".", "-fprintf", "x", "%p"}},
		{"find", []string{".", "-fls", "x"}},
		{"find", []string{"-D", "exec", "."}},
		{"rg", []string{"--pre", "cat", "x"}},
		{"rg", []string{"--pre=cat", "x"}},
		{"rg", []string{"--pre-glob", "*", "x"}},
		{"rg", []string{"-z", "x"}},
		{"rg", []string{"--search-zip", "x"}},
		{"rg", []string{"--hostname-bin", "/bin/sh", "x"}},
		{"sort", []string{"--compress-program", "sh", "x"}},
		{"sort", []string{"--compress-program=sh", "x"}},
		{"sort", []string{"--compress-prog=sh", "x"}}, // abbreviation is not accepted either
		{"gawk", []string{"-l", "filefuncs", "x"}},
		{"gawk", []string{"--load=filefuncs", "x"}},
		{"awk", []string{"-E", "prog.awk"}},
		{"file", []string{"-z", "a.gz"}},
		{"date", []string{"-s", "2020-01-01"}},
		{"ls", []string{"--not-an-option"}},
		{"ls", []string{"-l", "-%"}},
		{"curl", []string{"https://example.com"}},
		{"sh", []string{"-c", "id"}},
		{"bash", []string{"-c", "id"}},
		{"xargs", []string{"cat"}},
		{"env", []string{"cat"}},
	}
	for _, c := range bad {
		if _, err := Check(c.prog, c.args); err == nil {
			t.Errorf("%s %v: allowed, want rejected", c.prog, c.args)
		}
	}
}

func FuzzCheck(f *testing.F) {
	f.Add("find", ".\x00-exec\x00sh")
	f.Add("rg", "--pre=x\x00a")
	f.Add("ls", "-la\x00--\x00-x")
	f.Fuzz(func(t *testing.T, prog, joined string) {
		args := strings.Split(joined, "\x00")
		got, err := Check(prog, args)
		if err != nil {
			return
		}
		// Whatever passes must never contain a denied option before "--".
		for _, a := range got {
			if a == "--" {
				break
			}
			for p, ds := range denied {
				if prog != p && !(prog == "awk" && p == "gawk") {
					continue
				}
				for _, d := range ds {
					if a == d || strings.HasPrefix(a, d+"=") {
						t.Fatalf("%s %q passed with denied %s", prog, args, d)
					}
				}
			}
		}
	})
}

var escapes = regexp.MustCompile("\x1b\\]8;;[^\x1b]*\x1b\\\\|\x1b\\[[0-9;]*m")

// The tables must match the packaged tools: every allowed long option
// appears in the program's --help. Catches drift after a tool update.
func TestTablesMatchHelp(t *testing.T) {
	m, err := manifest.Load(os.Getenv("DIRMCP_TOOLS"))
	if err != nil {
		t.Skip("no tools manifest:", err)
	}
	for name, path := range m.Programs {
		p := Table(name)
		if p == nil {
			continue
		}
		c := exec.Command(path, "--help")
		c.Env = []string{"LC_ALL=C.UTF-8", "TERM=dumb"}
		out, _ := c.CombinedOutput()
		help := escapes.ReplaceAllString(string(out), "")
		for long := range p.Long {
			if !strings.Contains(help, "--"+long) {
				t.Errorf("%s: --%s is allowed but not in its --help", name, long)
			}
		}
		if _, ok := p.Long["help"]; !ok && name != "gawk" {
			t.Errorf("%s: --help is not allowed", name)
		}
	}
}
