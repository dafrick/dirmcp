package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dafrick/dirmcp/internal/confine"
	"github.com/dafrick/dirmcp/internal/manifest"
	"github.com/dafrick/dirmcp/internal/shell"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
	os.Exit(m.Run())
}

const token = "0123456789abcdef0123456789abcdef-test" // gitleaks:allow (test value)

func newServer(t *testing.T, createIn []string, allowedHosts ...string) (*httptest.Server, string) {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "Inbox"), 0o755)
	os.WriteFile(filepath.Join(root, "a.md"), []byte("alpha\n"), 0o644)
	tools, _ := manifest.Load(os.Getenv("DIRMCP_TOOLS"))
	self, _ := os.Executable()
	h := Handler(t.Context(), Config{
		Shell:        &shell.Config{Root: root, Tools: tools, Self: self},
		CreateIn:     createIn,
		AllowedHosts: allowedHosts,
		Token:        []byte(token),
		Version:      "test",
		Log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	return s, root
}

const initBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`

func post(t *testing.T, url, auth, body string, host ...string) int {
	t.Helper()
	req, _ := http.NewRequest("POST", url+"/dirmcp", strings.NewReader(body))
	if len(host) > 0 {
		req.Host = host[0]
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode
}

func TestAuth(t *testing.T) {
	s, _ := newServer(t, nil)
	for name, auth := range map[string]string{
		"none": "", "wrong": "Bearer nope", "prefix": "Bearer " + token[:31], "longer": "Bearer " + token + "x",
		"basic": "Basic " + token,
	} {
		if code := post(t, s.URL, auth, initBody); code != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", name, code)
		}
	}
	if code := post(t, s.URL, "Bearer "+token, initBody); code != http.StatusOK {
		t.Errorf("valid token: status %d, want 200", code)
	}
	if code := post(t, s.URL, "Bearer "+token, `{"x":"`+strings.Repeat("a", MaxBody+10)+`"}`); code < 400 {
		t.Errorf("oversized body: status %d, want an error", code)
	}
}

// Behind a proxy the Host is the public name: allowed only if configured.
func TestHosts(t *testing.T) {
	s, _ := newServer(t, nil, "Notes.Example.ts.net", "")
	for host, want := range map[string]int{
		"notes.example.ts.net":      http.StatusOK,
		"NOTES.example.ts.net:443":  http.StatusOK,
		"localhost:8090":            http.StatusOK,
		"127.0.0.1":                 http.StatusOK,
		"[::1]:8090":                http.StatusOK,
		"evil.example":              http.StatusForbidden,
		"notes.example.ts.net.evil": http.StatusForbidden,
		"notes.example.ts.net.":     http.StatusOK, // same DNS name
	} {
		if code := post(t, s.URL, "Bearer "+token, initBody, host); code != want {
			t.Errorf("Host %s: status %d, want %d", host, code, want)
		}
	}
	// Without allowed hosts, only localhost names.
	s2, _ := newServer(t, nil)
	if code := post(t, s2.URL, "Bearer "+token, initBody, "notes.example.ts.net"); code != http.StatusForbidden {
		t.Errorf("no allowed hosts: status %d, want 403", code)
	}
	// Refused before auth: no token needed to be told no.
	if code := post(t, s.URL, "", initBody, "evil.example"); code != http.StatusForbidden {
		t.Errorf("bad host without token: status %d, want 403", code)
	}
}

// An empty Host (HTTP/1.0 without Host) is refused even if an empty name
// was configured. Tested on the handler: the client would fill in Host.
func TestEmptyHost(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	h := checkHost(Config{AllowedHosts: []string{"", " "}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}, ok)
	r := httptest.NewRequest("POST", "/dirmcp", nil)
	r.Host = ""
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("empty Host: status %d, want 403", w.Code)
	}
}

// A custom path replaces the default; nothing else answers.
func TestPath(t *testing.T) {
	root := t.TempDir()
	h := Handler(t.Context(), Config{Shell: &shell.Config{Root: root}, Token: []byte(token), Path: "/obsidian",
		Version: "test", Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	s := httptest.NewServer(h)
	defer s.Close()
	for path, want := range map[string]int{"/obsidian": http.StatusOK, "/dirmcp": http.StatusNotFound, "/mcp": http.StatusNotFound, "/obsidian/x": http.StatusNotFound} {
		req, _ := http.NewRequest("POST", s.URL+path, strings.NewReader(initBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("%s: status %d, want %d", path, resp.StatusCode, want)
		}
	}
}

type bearer struct{ rt http.RoundTripper }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+token)
	return b.rt.RoundTrip(r)
}

func TestTools(t *testing.T) {
	if _, err := manifest.Load(os.Getenv("DIRMCP_TOOLS")); err != nil {
		t.Skip("no tools manifest")
	}
	self, _ := os.Executable()
	if err := confine.CheckKernel(self); err != nil {
		if os.Getenv("DIRMCP_REQUIRE_SANDBOX") != "" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
	s, root := newServer(t, []string{"Inbox"})
	ctx := context.Background()
	c := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "1"}, nil)
	sess, err := c.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: s.URL + "/dirmcp", HTTPClient: &http.Client{Transport: bearer{http.DefaultTransport}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	tl, err := sess.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tl.Tools {
		names = append(names, tool.Name)
	}
	if strings.Join(names, ",") != "create,more,sh" {
		t.Errorf("tools %v", names)
	}
	call := func(name string, args map[string]any) (string, bool) {
		r, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		return r.Content[0].(*mcp.TextContent).Text, r.IsError
	}
	if out, isErr := call("sh", map[string]any{"command": "cat a.md"}); isErr || out != "alpha\n[exit status 0]" {
		t.Errorf("sh: %q %v", out, isErr)
	}
	if out, _ := call("sh", map[string]any{"command": "echo x > a.md"}); !strings.Contains(out, "read-only") {
		t.Errorf("write on read-only server: %q", out)
	}
	if out, isErr := call("create", map[string]any{"path": "Inbox/n.md", "content": "new"}); isErr {
		t.Errorf("create: %q", out)
	}
	if _, isErr := call("create", map[string]any{"path": "Inbox/n.md", "content": "again"}); !isErr {
		t.Errorf("create twice: no error")
	}
	if b, _ := os.ReadFile(filepath.Join(root, "Inbox/n.md")); string(b) != "new" {
		t.Errorf("Inbox/n.md = %q", b)
	}
	if _, isErr := call("more", map[string]any{"id": "nope"}); !isErr {
		t.Errorf("more with a bad id: no error")
	}
}
