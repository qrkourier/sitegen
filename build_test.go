package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildDirectory(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	os.WriteFile(filepath.Join(src, "hello.md"), []byte("# Hello\nWorld"), 0644)
	os.WriteFile(filepath.Join(src, "bye.md"), []byte("# Bye\nSee ya"), 0644)

	if err := runBuild(src, out); err != nil {
		t.Fatalf("runBuild: %v", err)
	}
	for _, f := range []string{"hello.html", "bye.html", "index.html"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Errorf("expected %s to exist", f)
		}
	}
}

func TestBuildSingleFile(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	mdPath := filepath.Join(src, "doc.md")
	os.WriteFile(mdPath, []byte("# Single\nContent here"), 0644)

	if err := runBuild(mdPath, out); err != nil {
		t.Fatalf("runBuild: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "doc.html")); err != nil {
		t.Error("expected doc.html to exist")
	}
}

func TestBuildNestedDirs(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	sub := filepath.Join(src, "guides", "advanced")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(src, "root.md"), []byte("# Root"), 0644)
	os.WriteFile(filepath.Join(sub, "deep.md"), []byte("# Deep"), 0644)

	if err := runBuild(src, out); err != nil {
		t.Fatalf("runBuild: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "guides", "advanced", "deep.html")); err != nil {
		t.Error("expected nested output file")
	}
}

func TestBuildEmptyMarkdown(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	os.WriteFile(filepath.Join(src, "empty.md"), []byte(""), 0644)

	if err := runBuild(src, out); err != nil {
		t.Fatalf("empty markdown should not error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "empty.html")); err != nil {
		t.Error("expected empty.html to exist")
	}
}

func TestBuildNonExistentSource(t *testing.T) {
	out := t.TempDir()
	err := runBuild("/no/such/path", out)
	if err == nil {
		t.Fatal("expected error for non-existent source")
	}
}

func TestBuildNoMarkdownFiles(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	os.WriteFile(filepath.Join(src, "readme.txt"), []byte("not markdown"), 0644)

	err := runBuild(src, out)
	if err == nil {
		t.Fatal("expected error when no markdown files found")
	}
}

func TestBuildNonMarkdownFile(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	os.WriteFile(filepath.Join(src, "doc.md"), []byte("# Doc"), 0644)
	os.WriteFile(filepath.Join(src, "image.png"), []byte("fake png"), 0644)

	if err := runBuild(src, out); err != nil {
		t.Fatalf("runBuild: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "image.html")); err == nil {
		t.Error("non-markdown file should not produce output")
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Hello World", "hello-world"},
		{"UPPER CASE", "upper-case"},
		{"special!@#chars", "special-chars"},
		{"  spaces  ", "spaces"},
		{"already-slug", "already-slug"},
		{"123 numbers", "123-numbers"},
		{"", ""},
	}
	for _, tt := range tests {
		got := slugify(tt.in)
		if got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tt := range tests {
		got := humanBytes(tt.in)
		if got != tt.want {
			t.Errorf("humanBytes(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBuildTree(t *testing.T) {
	pages := []PageMeta{
		{Title: "Old", Path: "old.html", ModTime: "2025-01-01"},
		{Title: "New", Path: "new.html", ModTime: "2026-02-28"},
		{Title: "Deep", Path: "guides/advanced/deep.html", ModTime: "2026-01-15"},
		{Title: "Getting Started", Path: "guides/getting-started.html", ModTime: "2025-06-01"},
	}
	tree := buildTree(pages)

	if len(tree) != 3 {
		t.Fatalf("expected 3 top-level nodes, got %d", len(tree))
	}
	// dirs sort before files
	if !tree[0].IsDir || tree[0].Name != "guides" {
		t.Errorf("first node should be guides dir, got %+v", tree[0])
	}
	// files sorted newest first
	if tree[1].Title != "New" {
		t.Errorf("second node should be New (newest), got %+v", tree[1])
	}
	if tree[2].Title != "Old" {
		t.Errorf("third node should be Old (oldest), got %+v", tree[2])
	}
	// nested files also sorted newest first
	guides := tree[0]
	advanced := guides.Children[0] // only subdir
	if !advanced.IsDir || advanced.Name != "advanced" {
		t.Errorf("expected advanced dir, got %+v", advanced)
	}
	gs := guides.Children[1]
	if gs.Title != "Getting Started" {
		t.Errorf("expected Getting Started file, got %+v", gs)
	}
}
