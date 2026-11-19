// Package server wires the tools into an MCP server behind a bearer token.
package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dafrick/dirmcp/internal/argcheck"
	"github.com/dafrick/dirmcp/internal/create"
	"github.com/dafrick/dirmcp/internal/jobs"
	"github.com/dafrick/dirmcp/internal/shell"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MaxBody is the largest request body accepted.
const MaxBody = 2 << 20

// Config of one server.
type Config struct {
	Shell    *shell.Config
	CreateIn []string
	Token    []byte
	Version  string
	Log      *slog.Logger
	// Path of the MCP endpoint, e.g. "/mcp" (the default) or "/notes".
	Path string
	// AllowedHosts are public names (Host header) a proxy in front may use,
	// e.g. "notes.example.ts.net". Localhost names are always allowed; any
	// other Host is refused (DNS rebinding protection).
	AllowedHosts []string
}

// ShIn is the input of the sh tool.
type ShIn struct {
	Command string `json:"command" jsonschema:"the command line to run in the served directory"`
}

// MoreIn is the input of the more tool.
type MoreIn struct {
	ID string `json:"id" jsonschema:"the id from a truncated or still-running answer"`
}

// CreateIn is the input of the create tool.
type CreateIn struct {
	Path    string `json:"path" jsonschema:"new file, relative to the served directory"`
	Content string `json:"content" jsonschema:"the file content"`
}

// Handler returns the HTTP handler: the endpoint (Path) behind the bearer
// token. Every other path is 404.
func Handler(ctx context.Context, c Config) http.Handler {
	m := jobs.New(ctx)
	s := mcp.NewServer(&mcp.Implementation{Name: "dirmcp", Version: c.Version}, &mcp.ServerOptions{
		Instructions: instructions(c),
	})
	mcp.AddTool(s, &mcp.Tool{Name: "sh", Description: shDescription(c)},
		func(ctx context.Context, _ *mcp.CallToolRequest, in ShIn) (*mcp.CallToolResult, any, error) {
			start := time.Now()
			ans, err := m.Start(func(ctx context.Context, w *jobs.Writer) (int, error) {
				tmp, err := os.MkdirTemp("", "dirmcp-job-")
				if err != nil {
					return 1, err
				}
				defer os.RemoveAll(tmp)
				return shell.Run(ctx, c.Shell, in.Command, tmp, w)
			})
			c.Log.Info("sh", "command", clip(in.Command, 200), "answer_bytes", len(ans), "took", time.Since(start).Round(time.Millisecond), "err", err)
			return result(ans, err)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "more", Description: "Next part of a long or still-running `sh` answer. Waits up to 15 s for new output."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in MoreIn) (*mcp.CallToolResult, any, error) {
			ans, err := m.More(in.ID)
			c.Log.Info("more", "id", clip(in.ID, 40), "answer_bytes", len(ans), "err", err)
			return result(ans, err)
		})
	if len(c.CreateIn) > 0 {
		mcp.AddTool(s, &mcp.Tool{Name: "create", Description: fmt.Sprintf(
			"Create a new file below %s (relative to the served directory). Never overwrites: fails if the file exists. Parent folders are created. Content up to 1 MiB.",
			strings.Join(c.CreateIn, ", "))},
			func(ctx context.Context, _ *mcp.CallToolRequest, in CreateIn) (*mcp.CallToolResult, any, error) {
				err := create.File(c.Shell.Root, c.CreateIn, in.Path, []byte(in.Content))
				c.Log.Info("create", "path", clip(in.Path, 200), "bytes", len(in.Content), "err", err)
				return result("created "+in.Path, err)
			})
	}
	// The SDK refuses non-localhost Host headers on a loopback listener,
	// which also refuses every request through a proxy. We do the same check
	// ourselves, with the proxy's public names added.
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s },
		&mcp.StreamableHTTPOptions{Stateless: true, DisableLocalhostProtection: true})
	authed := auth.RequireBearerToken(verifier(c), &auth.RequireBearerTokenOptions{AllowMissingExpiration: true})(h)
	path := c.Path
	if path == "" {
		path = "/mcp"
	}
	mux := http.NewServeMux()
	mux.Handle(path, checkHost(c, limitBody(authed)))
	return mux
}

func result(text string, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}

// failures slows down repeated wrong tokens per remote address.
var failures = struct {
	sync.Mutex
	m map[string][]time.Time
}{m: map[string][]time.Time{}}

func verifier(c Config) auth.TokenVerifier {
	return func(ctx context.Context, token string, r *http.Request) (*auth.TokenInfo, error) {
		// Compare hashes: equal length, so the token's length does not leak.
		got, want := sha256.Sum256([]byte(token)), sha256.Sum256(c.Token)
		if subtle.ConstantTimeCompare(got[:], want[:]) == 1 {
			return &auth.TokenInfo{}, nil
		}
		host := r.RemoteAddr
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		failures.Lock()
		now := time.Now()
		var recent []time.Time
		for _, t := range failures.m[host] {
			if now.Sub(t) < time.Minute {
				recent = append(recent, t)
			}
		}
		failures.m[host] = append(recent, now)
		n := len(failures.m[host])
		failures.Unlock()
		c.Log.Warn("auth failed", "remote", host, "recent_failures", n)
		if n > 5 {
			time.Sleep(time.Second)
		}
		return nil, auth.ErrInvalidToken
	}
}

// checkHost refuses requests whose Host is neither a localhost name nor one
// of the allowed public names (case and port do not matter).
func checkHost(c Config, h http.Handler) http.Handler {
	norm := func(h string) string { return strings.TrimSuffix(strings.ToLower(h), ".") }
	allowed := map[string]bool{"localhost": true}
	for _, a := range c.AllowedHosts {
		if a = norm(strings.TrimSpace(a)); a != "" {
			allowed[a] = true
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if hh, _, err := net.SplitHostPort(host); err == nil {
			host = hh
		}
		host = norm(strings.Trim(host, "[]"))
		if ip := net.ParseIP(host); (ip != nil && ip.IsLoopback()) || (host != "" && allowed[host]) {
			h.ServeHTTP(w, r)
			return
		}
		c.Log.Warn("refused host", "host", clip(r.Host, 100))
		http.Error(w, "forbidden host", http.StatusForbidden)
	})
}

func limitBody(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, MaxBody)
		h.ServeHTTP(w, r)
	})
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func instructions(c Config) string {
	mode := "read-only: programs cannot change files"
	if c.Shell.Writable {
		mode = "writable: programs can change files; rm moves to .trash/"
	}
	s := "This server gives you a shell-like view of one directory (" + mode + "). " +
		"Use the sh tool with normal shell syntax (pipes, loops, $( ), redirections) and the real Unix tools listed in its description, with their usual options; run `<program> --help` for details. " +
		"Paths are relative to the directory. Long output is cut; continue with the more tool. Nothing outside the directory and no network is reachable."
	if len(c.CreateIn) > 0 {
		s += " To add a new file, use the create tool (below " + strings.Join(c.CreateIn, ", ") + ")."
	}
	return s
}

func shDescription(c Config) string {
	w := "Programs cannot change files here (read-only)."
	if c.Shell.Writable {
		w = "Programs can change files; `rm` moves to .trash/ instead of deleting."
	}
	return "Run a command line in the served directory (bash-like syntax, interpreted safely). " +
		"Allowed programs: " + strings.Join(argcheck.Programs(), " ") + ", rm. " +
		"Most of their options work; options that run other programs or write elsewhere are refused (e.g. find -exec, rg --pre). " +
		"Shells, xargs and env are not available: use for loops and $( ) instead. " + w + " " +
		"Answers wait up to 15 s and show up to 30 KB; then use `more` with the given id."
}
