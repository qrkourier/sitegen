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

func runServe(srcDir, outDir, addr string, addrSet, noAddr, writeMode, verbose bool) error {
	if verbose {
		pfxlog.GlobalInit(logrus.DebugLevel, pfxlog.DefaultOptions().SetTrimPrefix("github.com/openziti/"))
	} else {
		logrus.SetLevel(logrus.PanicLevel)
	}

	buildOpts := BuildOptions{WriteMode: writeMode}

	// Initial build
	fmt.Println("Building site...")
	if err := runBuild(srcDir, outDir, buildOpts); err != nil {
		return fmt.Errorf("initial build: %w", err)
	}

	// SSE hub for live-change notifications
	hub := newSSEHub()

	// Start watcher
	go watchAndRebuild(srcDir, outDir, hub, buildOpts)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/events", hub.handler)
	registerFoldsAPI(mux, outDir)

	// Register editor API routes only in write mode
	if writeMode {
		fmt.Println("Write mode ENABLED — editor API active")
		registerEditorAPI(mux, srcDir)
	}

	// File server for the built output
	fileServer := http.FileServer(http.Dir(outDir))
	mux.Handle("/", fileServer)

	handler := securityHeaders(mux, writeMode)

	// Obtain TLS config if ACME env vars are set
	tlsConfig, err := obtainTLSConfig()
	if err != nil {
		return fmt.Errorf("TLS setup: %w", err)
	}

	// Start overlay listeners first to determine whether TCP is needed.
	zitiCleanup, err := startZiti(handler, tlsConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ziti: %v\n", err)
	}

	zrokCleanup, err := startZrok(handler)
	if err != nil {
		fmt.Fprintf(os.Stderr, "zrok: %v\n", err)
	}

	// When an overlay is active, skip the TCP listener unless -addr was
	// explicitly provided. When no overlay is configured, always listen on
	// the default (or given) address.
	hasOverlay := zitiCleanup != nil || zrokCleanup != nil
	listenTCP := !noAddr && (addrSet || !hasOverlay)

	if listenTCP {
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

	if !listenTCP && !hasOverlay {
		return fmt.Errorf("no listeners active: set ZITI or ZROK env vars, or pass --addr")
	}

	fmt.Printf("Watching %s for changes\n", srcDir)

	// Block until shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	fmt.Println("\nShutting down...")

	if zitiCleanup != nil {
		zitiCleanup()
	}
	if zrokCleanup != nil {
		zrokCleanup()
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
		_ = listener.Close()
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

func securityHeaders(next http.Handler, writeMode bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")

		if writeMode {
			w.Header().Set("Content-Security-Policy",
				"default-src 'self'; "+
					"style-src 'self' 'unsafe-inline'; "+
					"img-src 'self' data: https:")
		} else {
			w.Header().Set("Content-Security-Policy", "default-src 'self'")
		}

		next.ServeHTTP(w, r)
	})
}

// watchAndRebuild polls the source directory for changes and rebuilds when
// any markdown file is added, removed, or modified.
func watchAndRebuild(srcDir, outDir string, hub *sseHub, opts ...BuildOptions) {
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
		building = true //nolint:ineffassign // read on next iteration under mu.Lock
		mu.Unlock()

		fmt.Println("Change detected, rebuilding...")
		if err := runBuild(srcDir, outDir, opts...); err != nil {
			fmt.Fprintf(os.Stderr, "rebuild error: %v\n", err)
		} else {
			fmt.Println("Rebuild complete")
			hub.broadcast("changed")
		}

		mu.Lock()
		building = false
		mu.Unlock()
	}
}

// sseHub manages Server-Sent Events connections for live-change notifications.
type sseHub struct {
	mu      sync.Mutex
	clients map[chan string]struct{}
}

func newSSEHub() *sseHub {
	return &sseHub{clients: make(map[chan string]struct{})}
}

func (h *sseHub) broadcast(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (h *sseHub) handler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan string, 4)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, ch)
		h.mu.Unlock()
	}()

	// Send initial keepalive so the client knows the connection is live
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
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
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
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
