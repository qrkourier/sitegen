package main

import (
	"bytes"
	"embed"
	"encoding/json"
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
	Title        string
	Content      template.HTML
	TOC          []TOCEntry
	Tree         []*TreeNode
	StaticPath   string
	RootPath     string
	WriteMode    bool
	MdPath       string // relative markdown source path (write mode only)
	Version      string
	Session      string
	Completed    string
	Updated      string
	PlanFilename string
	WorkingDirs  []string
}

type PageMeta struct {
	Title     string
	Session   string
	Completed string // ISO date from frontmatter (empty = active)
	Updated   string // ISO date from frontmatter
	Path      string
	ModTime   string
	Size      string
}

type TreeNode struct {
	Name      string
	IsDir     bool
	Path      string
	Title     string
	Session   string // optional sidebar label from frontmatter
	Completed string // ISO date — empty means active/in-progress
	Updated   string // ISO date from frontmatter
	ModTime   string
	Size      string
	Children  []*TreeNode
}

// HasFrontmatter reports whether this node had any recognized frontmatter fields.
func (n *TreeNode) HasFrontmatter() bool {
	return n.Session != "" || n.Completed != "" || n.Updated != ""
}

// SortDate returns the best available date for ordering: Updated if present,
// otherwise ModTime.
func (n *TreeNode) SortDate() string {
	if n.Updated != "" {
		return n.Updated
	}
	return n.ModTime
}

type IndexData struct {
	Title      string
	Pages      []PageMeta
	Tree       []*TreeNode
	StaticPath string
	RootPath   string
	Version    string
}

// BuildOptions controls optional behaviors of the build pipeline.
type BuildOptions struct {
	WriteMode bool // inject editor UI into generated pages
}

type parsedPage struct {
	title        string
	session      string // from YAML frontmatter "session" field
	completed    string // ISO date from frontmatter
	updated      string // ISO date from frontmatter
	planFilename string // from frontmatter
	workingDirs  []string
	toc          []TOCEntry
	html     string
	htmlRel  string
	urlPath  string
	mdPath   string
	mdRel    string // relative to source root
	srcInfo  os.FileInfo
	source   []byte
	isReadme bool
}

func runBuild(src, outDir string, opts ...BuildOptions) error {
	var opt BuildOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
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

	// Clean output directory contents without removing the directory itself,
	// which may be a mount point.
	var entries []os.DirEntry
	info, err = os.Stat(outDir)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("checking output dir: %w", err)
		}
	} else {
		if !info.IsDir() {
			// If the output path exists but is not a directory, remove it so we can
			// recreate it as a directory, matching the behavior of the previous
			// implementation that used os.RemoveAll(outDir).
			if err := os.Remove(outDir); err != nil {
				return fmt.Errorf("removing non-directory output path %q: %w", outDir, err)
			}
		} else {
			entries, err = os.ReadDir(outDir)
			if err != nil {
				return fmt.Errorf("reading output dir: %w", err)
			}
		}
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(outDir, entry.Name())); err != nil {
			return fmt.Errorf("cleaning output: %w", err)
		}
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("creating output: %w", err)
	}

	if err := writeStaticAssets(outDir); err != nil {
		return fmt.Errorf("writing static assets: %w", err)
	}

	// First pass: collect page metadata and parsed content for tree building
	var parsed []parsedPage
	var pages []PageMeta

	readmePath := filepath.Join(baseDir, "README.md")
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

		// Strip YAML frontmatter before parsing markdown.
		fm, body := parseFrontmatter(source)
		reader := text.NewReader(body)
		doc := md.Parser().Parse(reader)

		title, toc := extractMeta(doc, body)
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(mdPath), filepath.Ext(mdPath))
		}

		var htmlBuf bytes.Buffer
		if err := md.Renderer().Render(&htmlBuf, body, doc); err != nil {
			return fmt.Errorf("rendering %s: %w", mdPath, err)
		}

		// Detect root README.md — it becomes index.html instead of a regular page.
		isReadme := filepath.Clean(mdPath) == filepath.Clean(readmePath)

		parsed = append(parsed, parsedPage{
			title:        title,
			session:      fm.Session,
			completed:    fm.Completed,
			updated:      fm.Updated,
			planFilename: fm.PlanFilename,
			workingDirs:  fm.WorkingDirs,
			toc:          toc,
			html:     htmlBuf.String(),
			htmlRel:  htmlRel,
			urlPath:  urlPath,
			mdPath:   mdPath,
			mdRel:    filepath.ToSlash(rel),
			srcInfo:  srcInfo,
			source:   body,
			isReadme: isReadme,
		})

		// Exclude the root README from the page list and sidebar tree;
		// it will be rendered as index.html separately.
		if !isReadme {
			pages = append(pages, PageMeta{
				Title:     title,
				Session:   fm.Session,
				Completed: fm.Completed,
				Updated:   fm.Updated,
				Path:      urlPath,
				ModTime:   srcInfo.ModTime().Format(time.DateOnly),
			})
		}
	}

	tree := buildTree(pages)

	// Second pass: render regular pages (skip README — handled below)
	var readmePage *parsedPage
	pageIdx := 0
	for _, pp := range parsed {
		if pp.isReadme {
			cp := pp
			readmePage = &cp
			continue
		}

		depth := strings.Count(pp.urlPath, "/")
		rootPrefix := strings.Repeat("../", depth)

		page := PageData{
			Title:        pp.title,
			Content:      template.HTML(pp.html),
			TOC:          pp.toc,
			Tree:         prefixTreePaths(tree, rootPrefix),
			StaticPath:   rootPrefix + "static",
			RootPath:     rootPrefix,
			WriteMode:    opt.WriteMode,
			MdPath:       pp.mdRel,
			Version:      version,
			Session:      pp.session,
			Completed:    pp.completed,
			Updated:      pp.updated,
			PlanFilename: pp.planFilename,
			WorkingDirs:  pp.workingDirs,
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
		pages[pageIdx].Size = humanBytes(renderedSize)
		pageIdx++

		rel, _ := filepath.Rel(baseDir, pp.mdPath)
		fmt.Printf("  %s -> %s\n", rel, pp.htmlRel)
	}

	// Render index.html — use README.md content if present, otherwise
	// fall back to the auto-generated page list.
	indexPath := filepath.Join(outDir, "index.html")
	if readmePage != nil {
		page := PageData{
			Title:        readmePage.title,
			Content:      template.HTML(readmePage.html),
			TOC:          readmePage.toc,
			Tree:         tree,
			StaticPath:   "static",
			RootPath:     "",
			WriteMode:    opt.WriteMode,
			MdPath:       readmePage.mdRel,
			Version:      version,
			Session:      readmePage.session,
			Completed:    readmePage.completed,
			Updated:      readmePage.updated,
			PlanFilename: readmePage.planFilename,
			WorkingDirs:  readmePage.workingDirs,
		}
		f, err := os.Create(indexPath)
		if err != nil {
			return fmt.Errorf("creating index.html: %w", err)
		}
		if err := tmpl.ExecuteTemplate(f, "page.html", page); err != nil {
			_ = f.Close()
			return fmt.Errorf("rendering index: %w", err)
		}
		_ = f.Close()
	} else {
		indexData := IndexData{
			Title:      "Documents",
			Pages:      pages,
			Tree:       tree,
			StaticPath: "static",
			RootPath:   "",
			Version:    version,
		}
		f, err := os.Create(indexPath)
		if err != nil {
			return fmt.Errorf("creating index.html: %w", err)
		}
		if err := tmpl.ExecuteTemplate(f, "index.html", indexData); err != nil {
			_ = f.Close()
			return fmt.Errorf("rendering index: %w", err)
		}
		_ = f.Close()
	}

	fmt.Printf("  index.html\n")

	// Write search index for client-side filtering.
	if err := writeSearchIndex(outDir, parsed); err != nil {
		return fmt.Errorf("writing search index: %w", err)
	}

	fmt.Printf("Built %d page(s) -> %s/\n", len(parsed), outDir)
	return nil
}

// frontmatter holds the parsed YAML frontmatter fields.
type frontmatter struct {
	Session      string
	Completed    string
	Updated      string
	PlanFilename string
	WorkingDirs  []string
}

// parseFrontmatter strips YAML frontmatter (delimited by "---") from the
// beginning of source and extracts recognized fields. Returns the parsed
// frontmatter and the remaining body bytes.
func parseFrontmatter(source []byte) (frontmatter, []byte) {
	s := string(source)
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return frontmatter{}, source
	}
	end := strings.Index(s[3:], "\n---")
	if end < 0 {
		return frontmatter{}, source
	}
	// end is relative to s[3:], so frontmatter block is s[4 : 3+end]
	fm := s[4 : 3+end]
	// Skip past the closing "---" line
	rest := s[3+end+4:] // +4 for "\n---"
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if strings.HasPrefix(rest, "\r\n") {
		rest = rest[2:]
	}

	var result frontmatter
	inWorkingDirs := false
	for line := range strings.SplitSeq(fm, "\n") {
		trimmed := strings.TrimSpace(line)
		// Detect YAML list continuation for working_dirs
		if inWorkingDirs {
			if val, ok := strings.CutPrefix(trimmed, "- "); ok {
				result.WorkingDirs = append(result.WorkingDirs, unquoteYAML(val))
				continue
			}
			inWorkingDirs = false
		}
		if val, ok := strings.CutPrefix(trimmed, "session:"); ok {
			result.Session = unquoteYAML(val)
		} else if val, ok := strings.CutPrefix(trimmed, "completed:"); ok {
			result.Completed = unquoteYAML(val)
		} else if val, ok := strings.CutPrefix(trimmed, "updated:"); ok {
			result.Updated = unquoteYAML(val)
		} else if val, ok := strings.CutPrefix(trimmed, "plan_filename:"); ok {
			result.PlanFilename = unquoteYAML(val)
		} else if _, ok := strings.CutPrefix(trimmed, "working_dirs:"); ok {
			inWorkingDirs = true
		}
	}
	return result, []byte(rest)
}

// unquoteYAML trims whitespace and optional surrounding quotes from a YAML value.
func unquoteYAML(val string) string {
	val = strings.TrimSpace(val)
	if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
		val = val[1 : len(val)-1]
	}
	return val
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
					Name:      part,
					Title:     p.Title,
					Session:   p.Session,
					Completed: p.Completed,
					Updated:   p.Updated,
					Path:      p.Path,
					ModTime:   p.ModTime,
					Size:      p.Size,
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
		// dirs before files
		if nodes[i].IsDir != nodes[j].IsDir {
			return nodes[i].IsDir
		}
		// active (non-completed) before completed
		iDone := nodes[i].Completed != ""
		jDone := nodes[j].Completed != ""
		if iDone != jDone {
			return !iDone
		}
		// within each group, newest first by updated (or modtime fallback)
		return nodes[i].SortDate() > nodes[j].SortDate()
	})
	for _, n := range nodes {
		if n.IsDir {
			sortTree(n.Children)
		}
	}
}

func prefixTreePaths(nodes []*TreeNode, prefix string) []*TreeNode {
	if prefix == "" {
		return nodes
	}
	out := make([]*TreeNode, len(nodes))
	for i, n := range nodes {
		cp := *n
		if !cp.IsDir {
			cp.Path = prefix + cp.Path
		}
		if len(n.Children) > 0 {
			cp.Children = prefixTreePaths(n.Children, prefix)
		}
		out[i] = &cp
	}
	return out
}

type searchEntry struct {
	Title   string `json:"t"`
	Path    string `json:"p"`
	Content string `json:"c"`
	Updated string `json:"u,omitempty"`
}

func writeSearchIndex(outDir string, pages []parsedPage) error {
	var entries []searchEntry
	for _, pp := range pages {
		path := pp.urlPath
		if pp.isReadme {
			path = "index.html"
		}
		content := stripMarkdown(string(pp.source))
		if len(content) > 2000 {
			content = content[:2000]
		}
		// Use frontmatter updated date, fall back to file modtime
		sortDate := pp.updated
		if sortDate == "" && pp.srcInfo != nil {
			sortDate = pp.srcInfo.ModTime().Format("2006-01-02")
		}
		entries = append(entries, searchEntry{
			Title:   pp.title,
			Path:    path,
			Content: content,
			Updated: sortDate,
		})
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "search-index.json"), data, 0644)
}

var mdSyntax = regexp.MustCompile(`(?m)^#{1,6}\s+|[*_` + "`" + `~]+|\[([^\]]*)\]\([^)]*\)|^[>\-]\s+|^\d+\.\s+|!\[([^\]]*)\]\([^)]*\)`)

func stripMarkdown(s string) string {
	s = mdSyntax.ReplaceAllString(s, "$1$2")
	s = strings.Join(strings.Fields(s), " ")
	return s
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
