package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadToken(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string, mode os.FileMode) string {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(content), mode)
		os.Chmod(p, mode)
		return p
	}
	good := write("good", "0123456789abcdef0123456789abcdef\n", 0o600)
	if _, err := readToken(good); err != nil {
		t.Errorf("good token: %v", err)
	}
	if _, err := readToken(write("ro", "0123456789abcdef0123456789abcdef", 0o400)); err != nil {
		t.Errorf("0400 token: %v", err)
	}
	for name, p := range map[string]string{
		"short":   write("short", "0123456789abcdef", 0o600),
		"group":   write("group", "0123456789abcdef0123456789abcdef", 0o640),
		"world":   write("world", "0123456789abcdef0123456789abcdef", 0o644),
		"blank":   write("blank", "   \n", 0o600),
		"missing": filepath.Join(dir, "missing"),
	} {
		if _, err := readToken(p); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
