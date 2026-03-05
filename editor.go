package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// registerEditorAPI adds read/write endpoints for the markdown editor.
// These routes are only registered when --write mode is enabled.
func registerEditorAPI(mux *http.ServeMux, srcDir string) {
	absSrc, err := filepath.Abs(srcDir)
	if err != nil {
		absSrc = srcDir
	}

	// GET /api/page?path=guides/getting-started.md — returns raw markdown
	mux.HandleFunc("GET /api/page", func(w http.ResponseWriter, r *http.Request) {
		mdPath := r.URL.Query().Get("path")
		if mdPath == "" {
			http.Error(w, "missing path parameter", http.StatusBadRequest)
			return
		}

		resolved, err := safePath(absSrc, mdPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		data, err := os.ReadFile(resolved)
		if err != nil {
			if os.IsNotExist(err) {
				http.Error(w, "not found", http.StatusNotFound)
			} else {
				http.Error(w, "read error", http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Write(data)
	})

	// PUT /api/page?path=guides/getting-started.md — writes markdown content
	mux.HandleFunc("PUT /api/page", func(w http.ResponseWriter, r *http.Request) {
		mdPath := r.URL.Query().Get("path")
		if mdPath == "" {
			http.Error(w, "missing path parameter", http.StatusBadRequest)
			return
		}

		resolved, err := safePath(absSrc, mdPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Limit request body to 10 MB
		body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}

		if err := os.MkdirAll(filepath.Dir(resolved), 0755); err != nil {
			http.Error(w, "directory creation failed", http.StatusInternalServerError)
			return
		}

		if err := os.WriteFile(resolved, body, 0644); err != nil {
			http.Error(w, "write error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		fmt.Printf("  saved: %s\n", mdPath)
	})

	// GET /api/pages — list all markdown files (for creating new pages)
	mux.HandleFunc("GET /api/pages", func(w http.ResponseWriter, r *http.Request) {
		var paths []string
		filepath.WalkDir(absSrc, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
				rel, _ := filepath.Rel(absSrc, path)
				paths = append(paths, filepath.ToSlash(rel))
			}
			return nil
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(paths)
	})
}

// safePath validates and resolves a user-supplied path to ensure it stays
// within the source directory. Prevents directory traversal attacks.
func safePath(base, userPath string) (string, error) {
	if userPath == "" {
		return "", fmt.Errorf("empty path")
	}

	// Must be a markdown file
	if !strings.HasSuffix(strings.ToLower(userPath), ".md") {
		return "", fmt.Errorf("only .md files may be edited")
	}

	// Reject absolute paths outright
	if filepath.IsAbs(userPath) {
		return "", fmt.Errorf("absolute paths not allowed")
	}

	// Clean and join
	cleaned := filepath.Clean(filepath.FromSlash(userPath))
	full := filepath.Join(base, cleaned)

	// Resolve symlinks and verify containment
	absResolved, err := filepath.Abs(full)
	if err != nil {
		return "", fmt.Errorf("invalid path")
	}

	if !strings.HasPrefix(absResolved, base+string(filepath.Separator)) && absResolved != base {
		return "", fmt.Errorf("path traversal denied")
	}

	return absResolved, nil
}
