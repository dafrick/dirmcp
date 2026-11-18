// Command dirmcp serves one directory to remote MCP clients.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dafrick/dirmcp/internal/confine"
	"github.com/dafrick/dirmcp/internal/manifest"
	"github.com/dafrick/dirmcp/internal/server"
	"github.com/dafrick/dirmcp/internal/shell"
)

// Set by the Nix package (-ldflags -X).
var (
	version      = "dev"
	defaultTools = ""
)

type list []string

func (l *list) String() string     { return strings.Join(*l, ",") }
func (l *list) Set(v string) error { *l = append(*l, v); return nil }

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case confine.HelperArg:
			confine.RunHelper(os.Args[2:])
		case confine.SelfTestArg:
			if len(os.Args) < 3 {
				os.Exit(2)
			}
			if err := confine.SelfTest(os.Args[2]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "dirmcp: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		root      = flag.String("root", "", "directory to serve (required)")
		listen    = flag.String("listen", "127.0.0.1:8090", "address to listen on")
		tokenFile = flag.String("token-file", "", "file with the bearer token, at least 32 bytes, mode 0600 or 0400 (required)")
		writable  = flag.Bool("writable", false, "let programs change files under the root")
		tools     = flag.String("tools", defaultTools, "tools manifest (set by the Nix package)")
		showVer   = flag.Bool("version", false, "print the version")
		createIn  list
	)
	flag.Var(&createIn, "create-in", "enable the create tool below this folder of the root (repeatable)")
	flag.Parse()
	if *showVer {
		fmt.Println(version)
		return nil
	}
	if *root == "" || *tokenFile == "" {
		flag.Usage()
		return errors.New("--root and --token-file are required")
	}
	dir, err := filepath.EvalSymlinks(*root)
	if err != nil {
		return err
	}
	if dir, err = filepath.Abs(dir); err != nil {
		return err
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return fmt.Errorf("--root %s is not a directory", *root)
	}
	token, err := readToken(*tokenFile)
	if err != nil {
		return err
	}
	for _, c := range createIn {
		if filepath.IsAbs(c) || strings.HasPrefix(filepath.Clean(c), "..") || filepath.Clean(c) == "." {
			return fmt.Errorf("--create-in %q must be a folder inside the root", c)
		}
	}
	m, err := manifest.Load(*tools)
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if err := confine.CheckKernel(self); err != nil {
		return fmt.Errorf("this kernel cannot confine programs: %w", err)
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	h := server.Handler(ctx, server.Config{
		Shell:    &shell.Config{Root: dir, Writable: *writable, Tools: m, Self: self},
		CreateIn: createIn,
		Token:    token,
		Version:  version,
		Log:      log,
	})
	srv := &http.Server{Addr: *listen, Handler: h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second, // whole request, incl. the body
		WriteTimeout:      60 * time.Second, // answers wait at most 15 s
		IdleTimeout:       120 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("serving", "root", dir, "listen", *listen, "writable", *writable, "create_in", createIn.String(), "version", version)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(sctx)
	}
}

// readToken reads the token file and refuses weak tokens or loose modes.
func readToken(path string) ([]byte, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	// systemd credentials are 0440 (root-owned) inside a directory only the
	// service user can enter; group read is fine there. Never readable by
	// others, never owned by another unprivileged user.
	loose := st.Mode().Perm() & 0o077
	if inCredentials(path) {
		loose = st.Mode().Perm() & 0o007
		// systemd owns credentials as root; the directory is the service's.
		if sys, ok := st.Sys().(*syscall.Stat_t); ok && sys.Uid != 0 && int(sys.Uid) != os.Getuid() {
			return nil, fmt.Errorf("token file %s is owned by another user", path)
		}
	}
	if loose != 0 {
		return nil, fmt.Errorf("token file %s is readable by group or others (mode %o); use 0600 or 0400", path, st.Mode().Perm())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	b = bytes.TrimSpace(b)
	if len(b) < 32 {
		return nil, fmt.Errorf("token in %s is %d bytes; at least 32 needed (e.g. openssl rand -hex 32)", path, len(b))
	}
	return b, nil
}

// inCredentials reports whether path is inside $CREDENTIALS_DIRECTORY.
func inCredentials(path string) bool {
	dir := os.Getenv("CREDENTIALS_DIRECTORY")
	if dir == "" {
		return false
	}
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}
