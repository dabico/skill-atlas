package htmlreport

import (
	"bytes"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"skill-atlas/internal/skill"
)

func metaContent(doc *html.Node, httpEquiv string) string {
	for _, m := range elements(doc, "meta") {
		if attr(m, "http-equiv") == httpEquiv {
			return attr(m, "content")
		}
	}
	return ""
}

func filterSkills() []skill.Skill {
	return []skill.Skill{
		{Path: "skills/alpha/SKILL.md", Dir: "alpha", Name: "alpha", Description: "First one.\nTwo lines."},
		{Path: "skills/beta-dir/SKILL.md", Dir: "beta-dir", Errors: []string{"missing name"}},
	}
}

func TestRenderOneScriptPinnedByHash(t *testing.T) {
	_, doc := render(t, Report{Repo: "github.com/o/r", Ref: "main", SHA: "abcdef0123", Skills: filterSkills()})
	assertInert(t, doc, 1, 1)

	scripts := elements(doc, "script")
	if len(scripts) != 1 {
		t.Fatalf("got %d script elements, want 1", len(scripts))
	}
	text := textOf(scripts[0])
	if text != filterScript {
		t.Error("script text differs from the embedded filter script")
	}
	if got, want := metaContent(doc, "Content-Security-Policy"), cspWithScript(text); got != want {
		t.Errorf("CSP = %q\nwant  %q", got, want)
	}
	csp := metaContent(doc, "Content-Security-Policy")
	for _, bad := range []string{"unsafe-eval", "script-src 'unsafe-inline'", "script-src *", "http:", "https:"} {
		if strings.Contains(csp, bad) {
			t.Errorf("CSP contains %q: %s", bad, csp)
		}
	}
	if strings.Count(csp, "'unsafe-inline'") != 1 || !strings.Contains(csp, "style-src 'unsafe-inline'") {
		t.Errorf("'unsafe-inline' must appear once, for styles only: %s", csp)
	}
	if !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "img-src data:") {
		t.Errorf("CSP lost its base policy: %s", csp)
	}
}

// TestFilterScriptIsSmall guards the narrow scope the spec states for the script.
func TestFilterScriptIsSmall(t *testing.T) {
	for _, bad := range []string{"eval(", "Function(", "innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "fetch(", "XMLHttpRequest", "WebSocket", "import(", "localStorage", "location", "src"} {
		if strings.Contains(filterScript, bad) {
			t.Errorf("filter script uses %q", bad)
		}
	}
	if strings.Contains(filterScript, "</") || strings.Contains(filterScript, "<!--") {
		t.Error("filter script can end its own element early")
	}
}

func TestRenderNoScriptWithoutSkills(t *testing.T) {
	page, doc := render(t, Report{Repo: "github.com/o/r", Excluded: 2})
	assertInert(t, doc, 1, 0)
	if got := metaContent(doc, "Content-Security-Policy"); got != cspNoScript {
		t.Errorf("CSP = %q, want %q", got, cspNoScript)
	}
	if bytes.Contains(page, []byte("script-src")) {
		t.Error("empty page allows a script")
	}
}

func TestRenderFilterBoxHiddenByDefault(t *testing.T) {
	_, doc := render(t, Report{Repo: "github.com/o/r", Skills: filterSkills()})
	byID := map[string]*html.Node{}
	walk(doc, func(n *html.Node) {
		if n.Type == html.ElementNode && attr(n, "id") != "" {
			byID[attr(n, "id")] = n
		}
	})
	hasHidden := func(n *html.Node) bool {
		for _, a := range n.Attr {
			if a.Key == "hidden" {
				return true
			}
		}
		return false
	}
	for _, id := range []string{"filter", "fcount", "nomatch"} {
		n := byID[id]
		if n == nil {
			t.Fatalf("no element with id %q", id)
		}
		if !hasHidden(n) {
			t.Errorf("#%s isn't hidden by default", id)
		}
	}
	if q := byID["q"]; q == nil || q.Data != "input" || attr(q, "value") != "" {
		t.Error("#q isn't an empty input")
	}
	// Entries and sections start visible, so the page reads the same without the script.
	walk(doc, func(n *html.Node) {
		if n.Type == html.ElementNode && attr(n, "data-match") != "" && hasHidden(n) {
			t.Errorf("<%s data-match> is hidden by default", n.Data)
		}
	})
}

func TestRenderMatchText(t *testing.T) {
	bad := skill.Skill{Path: "dir/" + hostile + "/SKILL.md", Dir: "dir", Name: hostile, Description: hostile}
	skills := append(filterSkills(), bad)
	page, doc := render(t, Report{Repo: "github.com/o/r", Skills: skills})

	var want []string
	for _, s := range skills {
		want = append(want, s.DisplayName()+"\n"+s.Description+"\n"+s.Path)
	}
	for _, tag := range []string{"li", "section"} {
		var got []string
		for _, n := range elements(doc, tag) {
			if tag == "li" && len(elements(n, "a")) == 0 {
				continue
			}
			got = append(got, attr(n, "data-match"))
		}
		if len(got) != len(want) {
			t.Fatalf("%s: %d entries with match text, want %d", tag, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s %d: data-match = %q, want %q", tag, i, got[i], want[i])
			}
		}
	}
	// A directory name stands in for a missing name, like the TUI.
	if !strings.HasPrefix(want[1], "beta-dir\n") {
		t.Errorf("match text for a nameless skill = %q", want[1])
	}
	// The hostile text stays inside the attribute value.
	lower := strings.ToLower(string(page))
	if strings.Contains(lower, "<img") || strings.Count(lower, "<script") != 1 {
		t.Error("hostile match text added markup")
	}
	if !bytes.Contains(page, []byte("&lt;script&gt;alert(1)&lt;/script&gt;")) {
		t.Error("hostile match text isn't escaped")
	}
}
