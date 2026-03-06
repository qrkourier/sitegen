package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafePath(t *testing.T) {
	base := t.TempDir()

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid file", "hello.md", false},
		{"nested file", "guides/getting-started.md", false},
		{"traversal", "../etc/passwd.md", true},
		{"absolute path", "/etc/passwd.md", true},
		{"non-markdown", "hello.txt", true},
		{"empty path", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := safePath(base, tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("safePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestEditorAPIGetPage(t *testing.T) {
	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "test.md"), []byte("# Hello\nWorld"), 0644)

	mux := http.NewServeMux()
	registerEditorAPI(mux, srcDir)

	req := httptest.NewRequest("GET", "/api/page?path=test.md", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("GET /api/page status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if body != "# Hello\nWorld" {
		t.Errorf("body = %q, want %q", body, "# Hello\nWorld")
	}
}

func TestEditorAPIGetPageNotFound(t *testing.T) {
	srcDir := t.TempDir()

	mux := http.NewServeMux()
	registerEditorAPI(mux, srcDir)

	req := httptest.NewRequest("GET", "/api/page?path=missing.md", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Fatalf("GET /api/page status = %d, want 404", rec.Code)
	}
}

func TestEditorAPIPutPage(t *testing.T) {
	srcDir := t.TempDir()

	mux := http.NewServeMux()
	registerEditorAPI(mux, srcDir)

	body := strings.NewReader("# Updated\nNew content")
	req := httptest.NewRequest("PUT", "/api/page?path=new.md", body)
	req.Header.Set("Content-Type", "text/markdown")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("PUT /api/page status = %d, want 200", rec.Code)
	}

	data, err := os.ReadFile(filepath.Join(srcDir, "new.md"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(data) != "# Updated\nNew content" {
		t.Errorf("written content = %q, want %q", string(data), "# Updated\nNew content")
	}
}

func TestEditorAPIPutPageTraversal(t *testing.T) {
	srcDir := t.TempDir()

	mux := http.NewServeMux()
	registerEditorAPI(mux, srcDir)

	body := strings.NewReader("malicious")
	req := httptest.NewRequest("PUT", "/api/page?path=../escape.md", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != 400 {
		t.Fatalf("PUT with traversal status = %d, want 400", rec.Code)
	}
}

func TestEditorAPIListPages(t *testing.T) {
	srcDir := t.TempDir()
	os.MkdirAll(filepath.Join(srcDir, "guides"), 0755)
	os.WriteFile(filepath.Join(srcDir, "a.md"), []byte("# A"), 0644)
	os.WriteFile(filepath.Join(srcDir, "guides", "b.md"), []byte("# B"), 0644)
	os.WriteFile(filepath.Join(srcDir, "image.png"), []byte("not md"), 0644)

	mux := http.NewServeMux()
	registerEditorAPI(mux, srcDir)

	req := httptest.NewRequest("GET", "/api/pages", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("GET /api/pages status = %d, want 200", rec.Code)
	}

	body, _ := io.ReadAll(rec.Body)
	s := string(body)
	if !strings.Contains(s, "a.md") || !strings.Contains(s, "guides/b.md") {
		t.Errorf("expected both .md files in response, got %s", s)
	}
	if strings.Contains(s, "image.png") {
		t.Error("non-markdown file should not appear in listing")
	}
}

func TestSecurityHeadersWriteMode(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := securityHeaders(inner, true)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "'self'") {
		t.Errorf("write mode CSP should contain 'self', got %q", csp)
	}
	if !strings.Contains(csp, "'unsafe-inline'") {
		t.Errorf("write mode CSP should allow unsafe-inline for editor styles, got %q", csp)
	}
}
