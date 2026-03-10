package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// registerFoldsAPI adds GET/PUT /api/folds endpoints for persisting
// section fold state per page. State is stored in a single JSON file
// in the output directory, keyed by page path.
func registerFoldsAPI(mux *http.ServeMux, outDir string) {
	stateFile := filepath.Join(outDir, ".folds.json")
	var mu sync.RWMutex

	// allState is map[pagePath]map[headingID]bool
	type allState = map[string]map[string]bool

	load := func() allState {
		data, err := os.ReadFile(stateFile) //nolint:gosec // path is constructed from outDir
		if err != nil {
			return make(allState)
		}
		var s allState
		if json.Unmarshal(data, &s) != nil {
			return make(allState)
		}
		return s
	}

	save := func(s allState) {
		data, err := json.Marshal(s)
		if err != nil {
			return
		}
		_ = os.WriteFile(stateFile, data, 0644) //nolint:gosec // user-controlled output dir
	}

	mux.HandleFunc("GET /api/folds", func(w http.ResponseWriter, r *http.Request) {
		pagePath := normalizeFoldPath(r.URL.Query().Get("path"))
		if pagePath == "" {
			http.Error(w, "missing path", http.StatusBadRequest)
			return
		}

		mu.RLock()
		s := load()
		mu.RUnlock()

		pageState := s[pagePath]
		if pageState == nil {
			pageState = make(map[string]bool)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pageState)
	})

	mux.HandleFunc("PUT /api/folds", func(w http.ResponseWriter, r *http.Request) {
		pagePath := normalizeFoldPath(r.URL.Query().Get("path"))
		if pagePath == "" {
			http.Error(w, "missing path", http.StatusBadRequest)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}

		var pageState map[string]bool
		if json.Unmarshal(body, &pageState) != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		mu.Lock()
		s := load()
		s[pagePath] = pageState
		save(s)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
}

// normalizeFoldPath strips leading slashes and trailing index.html to
// produce a stable key regardless of how the URL is accessed.
func normalizeFoldPath(p string) string {
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimSuffix(p, "index.html")
	if p == "" {
		p = "/"
	}
	return p
}
