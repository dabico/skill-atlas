package htmlreport

import (
	"bytes"
	"html/template"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// headingShift moves body headings below the skill name (h2) and the page title (h1).
// A page with several repositories adds 1 more for the repository heading.
const headingShift = 2

// renderBody converts a skill's Markdown body to HTML. Raw HTML is dropped (no html.WithUnsafe).
func renderBody(body string, shift int) (template.HTML, error) {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithASTTransformers(util.Prioritized(sanitizer{shift: shift}, 100))),
	)
	var buf bytes.Buffer
	if err := md.Convert([]byte(body), &buf); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil // #nosec G203 -- goldmark output without raw HTML
}

// sanitizer rewrites the AST before rendering: unsafe links become text, images become alt text, headings shift down.
type sanitizer struct{ shift int }

func (s sanitizer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	src := reader.Source()
	var images, links, autoLinks []ast.Node
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Heading:
			n.Level = min(n.Level+s.shift, 6)
		case *ast.Image:
			images = append(images, n)
		case *ast.Link:
			if !safeURL(n.Destination) {
				links = append(links, n)
			}
		case *ast.AutoLink:
			if !safeURL(n.URL(src)) {
				autoLinks = append(autoLinks, n)
			}
		}
		return ast.WalkContinue, nil
	})

	for _, n := range images {
		img := n.(*ast.Image)
		if img.FirstChild() == nil {
			img.AppendChild(img, ast.NewString(img.Destination))
		}
		unwrap(n)
	}
	for _, n := range links {
		unwrap(n)
	}
	for _, n := range autoLinks {
		a := n.(*ast.AutoLink)
		n.Parent().InsertBefore(n.Parent(), n, ast.NewString(a.Label(src)))
		n.Parent().RemoveChild(n.Parent(), n)
	}
}

// unwrap replaces n with its children.
func unwrap(n ast.Node) {
	parent := n.Parent()
	var kids []ast.Node
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		kids = append(kids, c)
	}
	for _, c := range kids {
		n.RemoveChild(n, c)
		parent.InsertBefore(parent, n, c)
	}
	parent.RemoveChild(parent, n)
}

// safeURL allows http, https, mailto and scheme-less (relative or fragment) destinations.
func safeURL(dest []byte) bool {
	// Same value goldmark writes to href. Browsers ignore whitespace and control characters inside a scheme.
	s := strings.Map(func(r rune) rune {
		if r <= ' ' || r == 0x7f {
			return -1
		}
		return r
	}, string(util.URLEscape(dest, true)))
	i := strings.IndexAny(s, ":/?#")
	if i < 0 || s[i] != ':' {
		return true
	}
	switch strings.ToLower(s[:i]) {
	case "http", "https", "mailto":
		return true
	}
	return false
}
