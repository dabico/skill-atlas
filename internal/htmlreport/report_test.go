package htmlreport

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"skill-atlas/internal/skill"
)

// Page CSP without a script, and the same policy plus the filter script's hash.
const (
	cspNoScript = `default-src 'none'; style-src 'unsafe-inline'; img-src data:`
	cspMeta     = `<meta http-equiv="Content-Security-Policy" content="` + cspNoScript + `">`
)

// cspWithScript is the policy for a page with skills; the hash is computed here from script.
func cspWithScript(script string) string {
	sum := sha256.Sum256([]byte(script))
	return `default-src 'none'; script-src 'sha256-` + base64.StdEncoding.EncodeToString(sum[:]) + `'; style-src 'unsafe-inline'; img-src data:`
}

// hostile breaks out of text, quoted attributes and markup if it isn't escaped.
const hostile = `<script>alert(1)</script>"'><img src=x onerror=alert(2)>&amp;`

func parse(t *testing.T, page []byte) *html.Node {
	t.Helper()
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// walk calls f for every node under n, n included.
func walk(n *html.Node, f func(*html.Node)) {
	f(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, f)
	}
}

func elements(n *html.Node, tag string) []*html.Node {
	var out []*html.Node
	walk(n, func(m *html.Node) {
		if m.Type == html.ElementNode && m.Data == tag {
			out = append(out, m)
		}
	})
	return out
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasClass(n *html.Node, class string) bool {
	return slices.Contains(strings.Fields(attr(n, "class")), class)
}

// text returns the concatenated text nodes under n.
func textOf(n *html.Node) string {
	var b strings.Builder
	walk(n, func(m *html.Node) {
		if m.Type == html.TextNode {
			b.WriteString(m.Data)
		}
	})
	return b.String()
}

func render(t *testing.T, r Report) ([]byte, *html.Node) {
	t.Helper()
	page, err := Render(r)
	if err != nil {
		t.Fatal(err)
	}
	return page, parse(t, page)
}

// assertInert fails when doc has anything that can run code or load a resource.
func assertInert(t *testing.T, doc *html.Node, wantStyles, wantScripts int) {
	t.Helper()
	styles, scripts := 0, 0
	walk(doc, func(n *html.Node) {
		if n.Type != html.ElementNode {
			return
		}
		switch n.Data {
		case "html", "head", "body", "title", "header", "main", "nav", "section", "h1", "h2", "h3", "h4", "h5", "h6",
			"p", "a", "ul", "ol", "li", "dl", "dt", "dd", "div", "span", "code", "pre", "em", "strong", "del",
			"blockquote", "hr", "br", "table", "thead", "tbody", "tr", "th", "td", "input", "select", "option":
		case "style":
			styles++
		case "script":
			scripts++
			if len(n.Attr) != 0 {
				t.Errorf("script has attributes %v, want none", n.Attr)
			}
		case "meta":
			if attr(n, "http-equiv") == "" && attr(n, "name") == "" && attr(n, "charset") == "" {
				t.Errorf("unexpected meta %v", n.Attr)
			}
		case "link":
			if attr(n, "rel") != "icon" || attr(n, "href") != "data:," {
				t.Errorf("unexpected link %v", n.Attr)
			}
		default:
			t.Errorf("unexpected element <%s>", n.Data)
		}
		for _, a := range n.Attr {
			key := strings.ToLower(a.Key)
			if strings.HasPrefix(key, "on") || key == "style" || key == "src" || key == "srcset" || key == "action" || key == "formaction" {
				t.Errorf("<%s> has attribute %s=%q", n.Data, a.Key, a.Val)
			}
			if key == "href" && n.Data == "a" {
				u, err := url.Parse(a.Val)
				if err != nil {
					t.Errorf("bad href %q: %v", a.Val, err)
					continue
				}
				if !slices.Contains([]string{"", "http", "https", "mailto"}, u.Scheme) {
					t.Errorf("href %q has scheme %q", a.Val, u.Scheme)
				}
			}
		}
	})
	if styles != wantStyles {
		t.Errorf("got %d <style> elements, want %d", styles, wantStyles)
	}
	if scripts != wantScripts {
		t.Errorf("got %d <script> elements, want %d", scripts, wantScripts)
	}
}

func TestRenderHostile(t *testing.T) {
	body := strings.Join([]string{
		"# Title " + hostile,
		"<script>alert(3)</script>",
		"<div onclick=alert(4)>raw</div>",
		"inline <b onmouseover=alert(5)>raw</b> and <iframe src=//evil.test></iframe>",
		"[click](javascript:alert(6))",
		"![pic](https://evil.test/track.png)",
		"`" + hostile + "`",
		"```\n" + hostile + "\n```",
	}, "\n\n")
	s := skill.Skill{
		Path:          "dir/" + hostile + "/SKILL.md",
		Dir:           "dir",
		Name:          hostile,
		Description:   hostile,
		License:       hostile,
		Compatibility: hostile,
		AllowedTools:  hostile,
		Metadata:      []skill.MetadataEntry{{Key: hostile, Value: hostile}},
		Body:          body,
		Errors:        []string{hostile, `name "` + hostile + `" contains uppercase letters`},
	}
	page, doc := render(t, Report{Repos: []Repo{{Name: hostile, Ref: hostile, SHA: hostile + "0123456789", Skills: []skill.Skill{s}}}})

	assertInert(t, doc, 1, 1)
	lower := strings.ToLower(string(page))
	for _, bad := range []string{"javascript:", "<iframe", "<img", "<div onclick", "<b onmouseover"} {
		if strings.Contains(lower, bad) {
			t.Errorf("output contains live markup %q", bad)
		}
	}
	if n := strings.Count(lower, "<script"); n != 1 {
		t.Errorf("output has %d script tags, want only the filter script", n)
	}
	if !bytes.Contains(page, []byte("&lt;script&gt;alert(1)&lt;/script&gt;")) {
		t.Error("hostile text isn't escaped in the output")
	}

	// The parsed page shows every hostile value as literal text.
	section := elements(doc, "section")[0]
	for _, tag := range []string{"h2", "code", "dd", "li"} {
		found := false
		for _, n := range elements(section, tag) {
			if strings.Contains(textOf(n), hostile) {
				found = true
			}
		}
		if !found {
			t.Errorf("no <%s> in the skill section shows the hostile text", tag)
		}
	}
	if got := textOf(elements(section, "p")[1]); got != hostile { // .desc after .path
		t.Errorf("description text = %q, want the hostile string", got)
	}
	if got := textOf(elements(doc, "h1")[0]); got != hostile {
		t.Errorf("h1 text = %q, want the hostile string", got)
	}
	body0 := elements(section, "div")
	if len(body0) == 0 {
		t.Fatal("no div in section")
	}
	// Markdown body: code spans and blocks keep the hostile text; the link and image keep their labels.
	bodyText := textOf(body0[len(body0)-1])
	for _, want := range []string{"Title", hostile, "click", "pic"} {
		if !strings.Contains(bodyText, want) {
			t.Errorf("body text lacks %q", want)
		}
	}
	// The SHA attribute holds the full hostile value and nothing else leaked into the element.
	var sha *html.Node
	walk(doc, func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "code" && attr(n, "title") != "" {
			sha = n
		}
	})
	if sha == nil {
		t.Fatal("no code element with a title attribute")
	}
	if got := attr(sha, "title"); got != hostile+"0123456789" {
		t.Errorf("SHA title = %q", got)
	}
	if len(sha.Attr) != 1 {
		t.Errorf("SHA element has attributes %v, want only title", sha.Attr)
	}
}

func TestRenderStructure(t *testing.T) {
	// In name order A–Z, so the sections line up with the slice.
	skills := []skill.Skill{
		{Path: "skills/nameless/SKILL.md", Dir: "nameless", Errors: []string{"missing name", "missing description"}},
		{
			Path: "skills/pdf-processing/SKILL.md", Dir: "pdf-processing", Name: "pdf-processing",
			Description: "Extract PDF text.\nUse when handling PDFs.", License: "Apache-2.0", AllowedTools: "Bash Read",
			Metadata: []skill.MetadataEntry{{Key: "version", Value: "1.0"}, {Key: "author", Value: "me"}},
			Body:     "# Usage\n\nRun `pdf`.\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n- [x] done\n- ~~old~~\n\nSee https://example.com and [docs](docs/a.md).\n",
		},
		{
			Path: "skills/PDF-Tool/SKILL.md", Dir: "PDF-Tool", Name: "PDF-Tool", Description: "Broken.",
			Errors: []string{`name "PDF-Tool" contains uppercase letters`, `name "PDF-Tool" doesn't match directory "pdf-tools"`},
		},
	}
	sha := "c1ae565cfb98be30ea75e4b351e823846c69c3c8"
	page, doc := render(t, Report{Repos: []Repo{{Name: "github.com/org/repo", Ref: "v1.2.0", SHA: sha, Skills: skills}}})
	assertInert(t, doc, 1, 1)

	if !bytes.Contains(page, []byte(`<meta http-equiv="Content-Security-Policy" content="`+cspWithScript(filterScript)+`">`)) {
		t.Error("CSP meta tag is missing")
	}
	if !bytes.Contains(page, []byte(`<link rel="icon" href="data:,">`)) {
		t.Error("icon link is missing")
	}
	if !bytes.Contains(page, []byte(`name="viewport"`)) || !bytes.Contains(page, []byte("prefers-color-scheme: dark")) {
		t.Error("viewport meta or dark mode styles are missing")
	}

	// Header.
	header := elements(doc, "header")[0]
	head := textOf(header)
	for _, want := range []string{"github.com/org/repo", "@ v1.2.0", "(c1ae565)", "3 skills, 2 invalid"} {
		if !strings.Contains(head, want) {
			t.Errorf("header lacks %q: %q", want, head)
		}
	}
	if code := elements(header, "code")[0]; attr(code, "title") != sha {
		t.Errorf("SHA title = %q, want %q", attr(code, "title"), sha)
	}

	// Contents links match section ids, in order, and the invalid badge sits on the right entries.
	nav := elements(doc, "nav")[0]
	var ids []string
	var badged []bool
	for _, li := range elements(nav, "li") {
		a := elements(li, "a")[0]
		ids = append(ids, strings.TrimPrefix(attr(a, "href"), "#"))
		badged = append(badged, len(elements(li, "span")) == 1 && hasClass(elements(li, "span")[0], "badge"))
	}
	sections := elements(doc, "section")
	if len(sections) != len(skills) || len(ids) != len(skills) {
		t.Fatalf("got %d sections and %d contents links, want %d each", len(sections), len(ids), len(skills))
	}
	seen := map[string]bool{}
	for i, sec := range sections {
		id := attr(sec, "id")
		if id == "" || seen[id] {
			t.Errorf("section %d has empty or duplicate id %q", i, id)
		}
		seen[id] = true
		if ids[i] != id {
			t.Errorf("contents link %d targets #%s, section id is %q", i, ids[i], id)
		}
		wantBadge := !skills[i].Valid()
		if badged[i] != wantBadge {
			t.Errorf("contents entry %d badge = %v, want %v", i, badged[i], wantBadge)
		}
		heading := elements(sec, "h2")[0]
		hasBadge := len(elements(heading, "span")) == 1
		if hasBadge != wantBadge {
			t.Errorf("section %d heading badge = %v, want %v", i, hasBadge, wantBadge)
		}
		if got := strings.TrimSpace(textOf(heading)); !strings.HasPrefix(got, skills[i].DisplayName()) {
			t.Errorf("section %d heading = %q, want prefix %q", i, got, skills[i].DisplayName())
		}
		if got := textOf(elements(sec, "code")[0]); got != skills[i].Path {
			t.Errorf("section %d path = %q, want %q", i, got, skills[i].Path)
		}
		var errs []*html.Node
		for _, d := range elements(sec, "div") {
			if hasClass(d, "errors") {
				errs = append(errs, elements(d, "li")...)
			}
		}
		if len(errs) != len(skills[i].Errors) {
			t.Errorf("section %d lists %d errors, want %d", i, len(errs), len(skills[i].Errors))
		}
		for j, li := range errs {
			if j < len(skills[i].Errors) && textOf(li) != skills[i].Errors[j] {
				t.Errorf("section %d error %d = %q, want %q", i, j, textOf(li), skills[i].Errors[j])
			}
		}
	}

	// The pdf-processing section: description, fields, metadata order and rendered body.
	first := textOf(sections[1])
	for _, want := range []string{"Extract PDF text.\nUse when handling PDFs.", "license", "Apache-2.0", "allowed-tools", "Bash Read", "metadata", "version", "1.0", "author"} {
		if !strings.Contains(first, want) {
			t.Errorf("first section lacks %q", want)
		}
	}
	if strings.Contains(first, "compatibility") {
		t.Error("pdf-processing section shows an empty compatibility field")
	}
	if strings.Index(first, "version") > strings.Index(first, "author") {
		t.Error("metadata isn't in file order")
	}
	if got := textOf(elements(sections[1], "h3")[0]); got != "Usage" { // "# Usage" shifted below the skill name
		t.Errorf("body heading = %q", got)
	}
	for _, tag := range []string{"table", "del", "input"} {
		if len(elements(sections[1], tag)) == 0 {
			t.Errorf("GFM output lacks <%s>", tag)
		}
	}
	var hrefs []string
	for _, a := range elements(sections[1], "a") {
		hrefs = append(hrefs, attr(a, "href"))
	}
	for _, want := range []string{"https://example.com", "docs/a.md"} {
		if !slices.Contains(hrefs, want) {
			t.Errorf("pdf-processing section hrefs %v lack %q", hrefs, want)
		}
	}

	// A skill without a name shows its directory name.
	if !strings.Contains(textOf(sections[0]), "nameless") {
		t.Error("nameless skill doesn't show its directory name")
	}
	// A skill without a body has no body block.
	for _, d := range elements(sections[2], "div") {
		if hasClass(d, "body") {
			t.Error("skill without a body has a body block")
		}
	}
	// Valid skills have no errors block.
	for _, d := range elements(sections[1], "div") {
		if hasClass(d, "errors") {
			t.Error("valid skill has an errors block")
		}
	}
}

func TestRenderCounts(t *testing.T) {
	one := skill.Skill{Path: "a/SKILL.md", Dir: "a", Name: "a", Description: "d"}
	tests := []struct {
		name   string
		skills []skill.Skill
		want   string
	}{
		{"none", nil, "0 skills, 0 invalid"},
		{"one", []skill.Skill{one}, "1 skill, 0 invalid"},
		{"one invalid", []skill.Skill{{Path: "a/SKILL.md", Dir: "a", Errors: []string{"x"}}}, "1 skill, 1 invalid"},
		{"two", []skill.Skill{one, one}, "2 skills, 0 invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, _ := render(t, Report{Repos: []Repo{{Name: "github.com/o/r", Ref: "main", SHA: "abcdef0123", Skills: tt.skills}}})
			if !bytes.Contains(page, []byte(">"+tt.want+"<")) {
				t.Errorf("page lacks %q", tt.want)
			}
		})
	}
}

// TestRenderExcluded checks the excluded count reads like the TUI header and empty state.
func TestRenderExcluded(t *testing.T) {
	one := skill.Skill{Path: "a/SKILL.md", Dir: "a", Name: "a", Description: "d"}
	tests := []struct {
		name     string
		skills   []skill.Skill
		excluded int
		want     string
		notWant  string
	}{
		{"summary", []skill.Skill{one, one}, 3, ">2 skills, 0 invalid, 3 excluded<", ""},
		{"singular", []skill.Skill{one}, 1, ">1 skill, 0 invalid, 1 excluded<", ""},
		{"zero is hidden", []skill.Skill{one}, 0, ">1 skill, 0 invalid<", "excluded"},
		{"empty state", nil, 2, ">No skills found (2 excluded)<", ""},
		{"empty summary", nil, 2, ">0 skills, 0 invalid, 2 excluded<", ""},
		{"empty without exclusions", nil, 0, ">No skills found<", "excluded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, _ := render(t, Report{Repos: []Repo{{Name: "github.com/o/r", Skills: tt.skills, Excluded: tt.excluded}}})
			if !bytes.Contains(page, []byte(tt.want)) {
				t.Errorf("page lacks %q", tt.want)
			}
			if tt.notWant != "" && bytes.Contains(page, []byte(tt.notWant)) {
				t.Errorf("page contains %q", tt.notWant)
			}
		})
	}
}

func TestRenderEmpty(t *testing.T) {
	page, doc := render(t, Report{Repos: []Repo{{Name: "github.com/o/r", Ref: "main", SHA: "abcdef0123"}}})
	assertInert(t, doc, 1, 0)
	if !bytes.Contains(page, []byte(">No skills found<")) {
		t.Error(`page lacks "No skills found"`)
	}
	if len(elements(doc, "input")) != 0 {
		t.Error("empty scan has a filter box")
	}
	if len(elements(doc, "nav")) != 0 || len(elements(doc, "section")) != 0 {
		t.Error("empty scan has a contents list or sections")
	}
	if !bytes.Contains(page, []byte(cspMeta)) {
		t.Error("CSP meta tag is missing")
	}
}

func TestRenderNoRefNoSHA(t *testing.T) {
	page, _ := render(t, Report{Repos: []Repo{{Name: "github.com/o/r"}}})
	if bytes.Contains(page, []byte("@ ")) || bytes.Contains(page, []byte("title=")) {
		t.Errorf("page shows a ref or SHA that isn't set:\n%s", page)
	}
}

// TestRenderSelfContained checks the page makes no external requests.
func TestRenderSelfContained(t *testing.T) {
	s := skill.Skill{Path: "a/SKILL.md", Dir: "a", Name: "a", Description: "d", Body: "[x](https://example.com) ![i](https://example.com/i.png)"}
	page, _ := render(t, Report{Repos: []Repo{{Name: "github.com/o/r", Ref: "main", SHA: "abcdef0123", Skills: []skill.Skill{s}}}})
	lower := strings.ToLower(string(page))
	for _, bad := range []string{"@import", "url(", "src=", "<img", "<iframe", "<object", "<embed"} {
		if strings.Contains(lower, bad) {
			t.Errorf("page contains %q", bad)
		}
	}
}
