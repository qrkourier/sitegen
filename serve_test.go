package main

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSecurityHeaders(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := securityHeaders(inner)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	expected := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":        "DENY",
		"Content-Security-Policy": "default-src 'self'",
		"Referrer-Policy":         "no-referrer",
	}
	for header, want := range expected {
		got := rec.Header().Get(header)
		if got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestSecurityHeadersPreservesBody(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	handler := securityHeaders(inner)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Body.String() != "hello" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "hello")
	}
}

func TestIsClosedErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "closed network connection",
			err:  errors.New("use of closed network connection"),
			want: true,
		},
		{
			name: "op error wrapping closed",
			err:  &net.OpError{Op: "read", Err: errors.New("use of closed network connection")},
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("connection refused"),
			want: false,
		},
		{
			name: "op error unrelated",
			err:  &net.OpError{Op: "read", Err: errors.New("connection refused")},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isClosedErr(tt.err)
			if got != tt.want {
				t.Errorf("isClosedErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestSnapshotDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A"), 0644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("# B"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("not markdown"), 0644)

	snap := snapshotDir(dir)
	if len(snap) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(snap))
	}
	if _, ok := snap["a.md"]; !ok {
		t.Error("expected a.md in snapshot")
	}
	if _, ok := snap["b.md"]; !ok {
		t.Error("expected b.md in snapshot")
	}
	if _, ok := snap["c.txt"]; ok {
		t.Error("non-markdown file should not be in snapshot")
	}
}

func TestSnapshotDirNested(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(dir, "root.md"), []byte("# Root"), 0644)
	os.WriteFile(filepath.Join(sub, "nested.md"), []byte("# Nested"), 0644)

	snap := snapshotDir(dir)
	if len(snap) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(snap))
	}
	if _, ok := snap[filepath.Join("sub", "nested.md")]; !ok {
		t.Error("expected sub/nested.md in snapshot")
	}
}

func TestSnapshotDirEmpty(t *testing.T) {
	dir := t.TempDir()
	snap := snapshotDir(dir)
	if len(snap) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(snap))
	}
}

func TestMapsEqual(t *testing.T) {
	now := time.Now()
	a := map[string]fileEntry{
		"a.md": {modTime: now, size: 10},
		"b.md": {modTime: now, size: 20},
	}
	b := map[string]fileEntry{
		"a.md": {modTime: now, size: 10},
		"b.md": {modTime: now, size: 20},
	}
	if !mapsEqual(a, b) {
		t.Error("identical maps should be equal")
	}
}

func TestMapsEqualDifferentSize(t *testing.T) {
	now := time.Now()
	a := map[string]fileEntry{
		"a.md": {modTime: now, size: 10},
	}
	b := map[string]fileEntry{
		"a.md": {modTime: now, size: 10},
		"b.md": {modTime: now, size: 20},
	}
	if mapsEqual(a, b) {
		t.Error("maps with different lengths should not be equal")
	}
}

func TestMapsEqualDifferentContent(t *testing.T) {
	now := time.Now()
	a := map[string]fileEntry{
		"a.md": {modTime: now, size: 10},
	}
	b := map[string]fileEntry{
		"a.md": {modTime: now, size: 99},
	}
	if mapsEqual(a, b) {
		t.Error("maps with different values should not be equal")
	}
}

func TestMapsEqualDifferentKeys(t *testing.T) {
	now := time.Now()
	a := map[string]fileEntry{
		"a.md": {modTime: now, size: 10},
	}
	b := map[string]fileEntry{
		"x.md": {modTime: now, size: 10},
	}
	if mapsEqual(a, b) {
		t.Error("maps with different keys should not be equal")
	}
}

func TestMapsEqualBothEmpty(t *testing.T) {
	a := map[string]fileEntry{}
	b := map[string]fileEntry{}
	if !mapsEqual(a, b) {
		t.Error("two empty maps should be equal")
	}
}
