// Package manifest loads the tools manifest written by the Nix package:
// program name → absolute store path, plus the store paths of the tools'
// closure (read and execute access under Landlock).
package manifest

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Manifest is the parsed tools manifest.
type Manifest struct {
	Programs map[string]string `json:"programs"`
	Closure  string            `json:"closure"` // file with one store path per line
	Paths    []string          `json:"-"`       // contents of Closure
}

// Load reads and checks the manifest at path.
func Load(path string) (*Manifest, error) {
	if path == "" {
		return nil, errors.New("no tools manifest (DIRMCP_TOOLS or --tools)")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("tools manifest: %w", err)
	}
	for n, p := range m.Programs {
		if !filepath.IsAbs(p) {
			return nil, fmt.Errorf("tools manifest: %s: path %q is not absolute", n, p)
		}
	}
	f, err := os.Open(m.Closure)
	if err != nil {
		return nil, fmt.Errorf("tools manifest: closure: %w", err)
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		if p := strings.TrimSpace(s.Text()); p != "" {
			m.Paths = append(m.Paths, p)
		}
	}
	if len(m.Paths) == 0 {
		return nil, errors.New("tools manifest: empty closure")
	}
	return &m, s.Err()
}
