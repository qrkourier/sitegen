package main

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/sirupsen/logrus"
)

func runServe(srcDir, outDir, addr string, noAddr, verbose bool) error {
	if verbose {
		pfxlog.GlobalInit(logrus.DebugLevel, pfxlog.DefaultOptions().SetTrimPrefix("github.com/openziti/"))
	} else {
		logrus.SetLevel(logrus.PanicLevel)
	}

	// Initial build
	fmt.Println("Building site...")
	if err := runBuild(srcDir, outDir); err != nil {
		return fmt.Errorf("initial build: %w", err)
	}

	// Start watcher
	go watchAndRebuild(srcDir, outDir)

	fileServer := http.FileServer(http.Dir(outDir))
	handler := securityHeaders(fileServer)

	// Obtain TLS config if ACME env vars are set
	tlsConfig, err := obtainTLSConfig()
	if err != nil {
		return fmt.Errorf("TLS setup: %w", err)
	}

	// Start TCP listener unless -no-addr is set
	if !noAddr {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("TCP listen: %w", err)
		}
		scheme := "http"
		if tlsConfig != nil {
			ln = tls.NewListener(ln, tlsConfig)
			scheme = "https"
		}
		fmt.Printf("Serving %s at %s://localhost%s\n", outDir, scheme, addr)
		go func() {
			if err := http.Serve(ln, handler); err != nil && !isClosedErr(err) {
				fmt.Fprintf(os.Stderr, "TCP listener error: %v\n", err)
			}
		}()
	}
	fmt.Printf("Watching %s for changes\n", srcDir)

	// Start Ziti listener if configured
	zitiCleanup, err := startZiti(handler, tlsConfig)
	if err != nil {
		if noAddr {
			return fmt.Errorf("ziti: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Ziti: %v\n", err)
	}

	if noAddr && zitiCleanup == nil {
		return fmt.Errorf("-no-addr requires ZITI_IDENTITY and ZITI_SERVICE to be set")
	}

	// Block until shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	fmt.Println("\nShutting down...")

	if zitiCleanup != nil {
		zitiCleanup()
	}

	return nil
}

// startZiti reads ZITI_IDENTITY and ZITI_SERVICE from the environment and, if
// both are set, starts serving the handler on the Ziti overlay network. It
// returns a cleanup function that closes the listener and Ziti context.
// Returns (nil, nil) when neither env var is set.
func startZiti(handler http.Handler, tlsConfig *tls.Config) (func(), error) {
	identityB64 := os.Getenv("ZITI_IDENTITY")
	serviceName := os.Getenv("ZITI_SERVICE")
	if identityB64 == "" || serviceName == "" {
		return nil, nil
	}

	identityJSON, err := base64.StdEncoding.DecodeString(identityB64)
	if err != nil {
		return nil, fmt.Errorf("failed to base64-decode ZITI_IDENTITY: %w", err)
	}

	cfg := &ziti.Config{}
	if err := json.Unmarshal(identityJSON, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse identity JSON: %w", err)
	}

	ctx, err := ziti.NewContext(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create context: %w", err)
	}

	listener, err := ctx.Listen(serviceName)
	if err != nil {
		ctx.Close()
		return nil, fmt.Errorf("failed to listen on service %q: %w", serviceName, err)
	}

	var ln net.Listener = listener
	if tlsConfig != nil {
		ln = tls.NewListener(listener, tlsConfig)
	}

	fmt.Printf("Serving on Ziti service %q\n", serviceName)
	go func() {
		if err := http.Serve(ln, handler); err != nil && !isClosedErr(err) {
			fmt.Fprintf(os.Stderr, "Ziti listener error: %v\n", err)
		}
	}()

	return func() {
		listener.Close()
		ctx.Close()
	}, nil
}

// isClosedErr reports whether the error is a "use of closed network connection"
// error, which is expected during shutdown.
func isClosedErr(err error) bool {
	if opErr, ok := err.(*net.OpError); ok {
		return strings.Contains(opErr.Err.Error(), "use of closed network connection")
	}
	return strings.Contains(err.Error(), "use of closed network connection")
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// watchAndRebuild polls the source directory for changes and rebuilds when
// any markdown file is added, removed, or modified.
func watchAndRebuild(srcDir, outDir string) {
	var (
		mu        sync.Mutex
		building  bool
		lastState = snapshotDir(srcDir)
	)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		current := snapshotDir(srcDir)
		if mapsEqual(lastState, current) {
			continue
		}
		lastState = current

		mu.Lock()
		if building {
			mu.Unlock()
			continue
		}
		building = true
		mu.Unlock()

		fmt.Println("Change detected, rebuilding...")
		if err := runBuild(srcDir, outDir); err != nil {
			fmt.Fprintf(os.Stderr, "rebuild error: %v\n", err)
		} else {
			fmt.Println("Rebuild complete")
		}

		mu.Lock()
		building = false
		mu.Unlock()
	}
}

// fileEntry holds the mod time and size of a file for change detection.
type fileEntry struct {
	modTime time.Time
	size    int64
}

// snapshotDir returns a map of relative path -> fileEntry for all markdown
// files in the directory tree.
func snapshotDir(dir string) map[string]fileEntry {
	snap := make(map[string]fileEntry)
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		info, err := d.Info()
		if err != nil {
			return nil
		}
		snap[rel] = fileEntry{modTime: info.ModTime(), size: info.Size()}
		return nil
	})
	return snap
}

func mapsEqual(a, b map[string]fileEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok || va != vb {
			return false
		}
	}
	return true
}
