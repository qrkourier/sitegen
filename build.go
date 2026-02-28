package main

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

//go:embed templates/*
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

type TOCEntry struct {
	Level int
	Title string
	ID    string
}

type PageData struct {
	Title      string
	Content    template.HTML
	TOC        []TOCEntry
	Tree       []*TreeNode
	StaticPath string
}

type PageMeta struct {
	Title   string
	Path    string
	ModTime string
	Size    string
}

type TreeNode struct {
	Name     string
	IsDir    bool
	Path     string
	Title    string
	ModTime  string
	Size     string
	Children []*TreeNode
}

type IndexData struct {
	Title      string
	Pages      []PageMeta
	Tree       []*TreeNode
	StaticPath string
}

func runBuild(src, outDir string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("source not found: %w", err)
	}

	var mdFiles []string
	var baseDir string

	if info.IsDir() {
		baseDir = src
		err = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
				mdFiles = append(mdFiles, path)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("scanning source: %w", err)
		}
	} else {
		if !strings.HasSuffix(strings.ToLower(src), ".md") {
			return fmt.Errorf("%s is not a markdown file", src)
		}
		baseDir = filepath.Dir(src)
		mdFiles = []string{src}
	}

	if len(mdFiles) == 0 {
		return fmt.Errorf("no markdown files found in %s", src)
	}

	// Safety: refuse to wipe dangerous paths
	absOut, _ := filepath.Abs(outDir)
	if absOut == "/" || absOut == os.Getenv("HOME") {
		return fmt.Errorf("refusing to clean %s", absOut)
	}

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)

	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return fmt.Errorf("parsing templates: %w", err)
	}

	if err := os.RemoveAll(outDir); err != nil {
		return fmt.Errorf("cleaning output: %w", err)
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("creating output: %w", err)
	}

	if err := writeStaticAssets(outDir); err != nil {
		return fmt.Errorf("writing static assets: %w", err)
	}

	// First pass: collect page metadata and parsed content for tree building
	type parsedPage struct {
		title     string
		toc       []TOCEntry
		html      string
		htmlRel   string
		urlPath   string
		mdPath    string
		srcInfo   os.FileInfo
	}

	var parsed []parsedPage
	var pages []PageMeta

	for _, mdPath := range mdFiles {
		rel, _ := filepath.Rel(baseDir, mdPath)
		htmlRel := strings.TrimSuffix(rel, filepath.Ext(rel)) + ".html"
		urlPath := filepath.ToSlash(htmlRel)

		srcInfo, err := os.Stat(mdPath)
		if err != nil {
			return fmt.Errorf("stat %s: %w", mdPath, err)
		}

		source, err := os.ReadFile(mdPath)
		if err != nil {
			return fmt.Errorf("reading %s: %w", mdPath, err)
		}

		reader := text.NewReader(source)
		doc := md.Parser().Parse(reader)

		title, toc := extractMeta(doc, source)
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(mdPath), filepath.Ext(mdPath))
		}

		var htmlBuf bytes.Buffer
		if err := md.Renderer().Render(&htmlBuf, source, doc); err != nil {
			return fmt.Errorf("rendering %s: %w", mdPath, err)
		}

		parsed = append(parsed, parsedPage{
			title:   title,
			toc:     toc,
			html:    htmlBuf.String(),
			htmlRel: htmlRel,
			urlPath: urlPath,
			mdPath:  mdPath,
			srcInfo: srcInfo,
		})

		pages = append(pages, PageMeta{
			Title:   title,
			Path:    urlPath,
			ModTime: srcInfo.ModTime().Format(time.DateOnly),
		})
	}

	tree := buildTree(pages)

	// Second pass: render pages with nav tree
	for i, pp := range parsed {
		depth := strings.Count(pp.urlPath, "/")
		staticPrefix := strings.Repeat("../", depth) + "static"

		page := PageData{
			Title:      pp.title,
			Content:    template.HTML(pp.html),
			TOC:        pp.toc,
			Tree:       tree,
			StaticPath: staticPrefix,
		}

		outPath := filepath.Join(outDir, pp.htmlRel)
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return fmt.Errorf("creating dir for %s: %w", outPath, err)
		}
		f, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("creating %s: %w", outPath, err)
		}
		if err := tmpl.ExecuteTemplate(f, "page.html", page); err != nil {
			_ = f.Close()
			return fmt.Errorf("rendering template for %s: %w", pp.mdPath, err)
		}
		_ = f.Close()

		outInfo, _ := os.Stat(outPath)
		var renderedSize int64
		if outInfo != nil {
			renderedSize = outInfo.Size()
		}
		pages[i].Size = humanBytes(renderedSize)

		rel, _ := filepath.Rel(baseDir, pp.mdPath)
		fmt.Printf("  %s -> %s\n", rel, pp.htmlRel)
	}

	indexData := IndexData{
		Title:      "Documents",
		Pages:      pages,
		Tree:       tree,
		StaticPath: "static",
	}
	indexPath := filepath.Join(outDir, "index.html")
	f, err := os.Create(indexPath)
	if err != nil {
		return fmt.Errorf("creating index.html: %w", err)
	}
	if err := tmpl.ExecuteTemplate(f, "index.html", indexData); err != nil {
		_ = f.Close()
		return fmt.Errorf("rendering index: %w", err)
	}
	_ = f.Close()

	fmt.Printf("  index.html\n")
	fmt.Printf("Built %d page(s) -> %s/\n", len(pages), outDir)
	return nil
}

func extractMeta(doc ast.Node, source []byte) (string, []TOCEntry) {
	var title string
	var toc []TOCEntry

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		heading, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}

		headingText := nodeText(n, source)

		var id string
		if idAttr, exists := heading.AttributeString("id"); exists {
			switch v := idAttr.(type) {
			case []byte:
				id = string(v)
			case string:
				id = v
			}
		}
		if id == "" {
			id = slugify(headingText)
		}

		if heading.Level == 1 && title == "" {
			title = headingText
		}

		toc = append(toc, TOCEntry{
			Level: heading.Level,
			Title: headingText,
			ID:    id,
		})

		return ast.WalkContinue, nil
	})

	return title, toc
}

func nodeText(n ast.Node, source []byte) string {
	var buf bytes.Buffer
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			buf.Write(t.Segment.Value(source))
		} else {
			buf.WriteString(nodeText(c, source))
		}
	}
	return buf.String()
}

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' {
			return unicode.ToLower(r)
		}
		return ' '
	}, s)
	s = strings.TrimSpace(s)
	s = nonAlphanumeric.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	suffix := []string{"KB", "MB", "GB"}
	return fmt.Sprintf("%.1f %s", float64(b)/float64(div), suffix[exp])
}

func buildTree(pages []PageMeta) []*TreeNode {
	root := &TreeNode{IsDir: true}

	for _, p := range pages {
		parts := strings.Split(p.Path, "/")
		node := root
		for i, part := range parts {
			if i == len(parts)-1 {
				node.Children = append(node.Children, &TreeNode{
					Name:    part,
					Title:   p.Title,
					Path:    p.Path,
					ModTime: p.ModTime,
					Size:    p.Size,
				})
			} else {
				var dir *TreeNode
				for _, c := range node.Children {
					if c.IsDir && c.Name == part {
						dir = c
						break
					}
				}
				if dir == nil {
					dir = &TreeNode{Name: part, IsDir: true}
					node.Children = append(node.Children, dir)
				}
				node = dir
			}
		}
	}

	sortTree(root.Children)
	return root.Children
}

func sortTree(nodes []*TreeNode) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].IsDir != nodes[j].IsDir {
			return nodes[i].IsDir
		}
		// newest first by source file modtime
		return nodes[i].ModTime > nodes[j].ModTime
	})
	for _, n := range nodes {
		if n.IsDir {
			sortTree(n.Children)
		}
	}
}

func writeStaticAssets(outDir string) error {
	return fs.WalkDir(staticFS, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dest := filepath.Join(outDir, path)
		if d.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		data, err := staticFS.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0644)
	})
}
