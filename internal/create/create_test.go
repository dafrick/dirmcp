package create

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFile(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	os.MkdirAll(filepath.Join(root, "Inbox"), 0o755)
	os.WriteFile(filepath.Join(base, "outside"), []byte("x"), 0o644)
	os.Symlink(base, filepath.Join(root, "Inbox", "escape"))
	allowed := []string{"Inbox"}

	if err := File(root, allowed, "Inbox/clips/a.md", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, "Inbox/clips/a.md"))
	if string(b) != "hello" {
		t.Errorf("content %q", b)
	}
	if err := File(root, allowed, "Inbox/clips/a.md", []byte("again")); err == nil || !strings.Contains(err.Error(), "exists") {
		t.Errorf("overwrite: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "Inbox/clips/a.md")); string(b) != "hello" {
		t.Errorf("file changed to %q", b)
	}
	for _, p := range []string{"Notes/x.md", "Inbox/../x.md", "../x.md", "/etc/x", "Inbox", "Inboxer/x.md", "Inbox/escape/new.md", "Inbox/escape/outside"} {
		if err := File(root, allowed, p, []byte("x")); err == nil {
			t.Errorf("%s: created, want error", p)
		}
	}
	if _, err := os.Stat(filepath.Join(base, "new.md")); err == nil {
		t.Errorf("created through a symlink outside the root")
	}
	if err := File(root, allowed, "Inbox/big.md", make([]byte, MaxSize+1)); err == nil {
		t.Errorf("oversized content accepted")
	}
}
