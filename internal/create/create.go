// Package create writes new files only, below allowed folders of the root.
package create

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MaxSize is the largest content create accepts.
const MaxSize = 1 << 20

// File creates path (relative to root) with content. It fails if path is not
// below one of the allowed folders (relative to root), escapes the root, or
// already exists. Parent folders are created inside the allowed folder.
func File(root string, allowed []string, path string, content []byte) error {
	if len(content) > MaxSize {
		return fmt.Errorf("content is %d bytes, at most %d allowed", len(content), MaxSize)
	}
	if filepath.IsAbs(path) {
		return errors.New("path must be relative to the served directory")
	}
	rel := filepath.Clean(path)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return errors.New("path is outside the served directory")
	}
	ok := false
	for _, a := range allowed {
		a = filepath.Clean(a)
		if strings.HasPrefix(rel, a+"/") {
			ok = true
		}
	}
	if !ok {
		return fmt.Errorf("files can only be created below: %s", strings.Join(allowed, ", "))
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := r.MkdirAll(filepath.Dir(rel), 0o770); err != nil {
		return err
	}
	f, err := r.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o660)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%s already exists; create never overwrites", path)
		}
		return err
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
