package htmlreport

import (
	"slices"
	"strings"
	"testing"

	"skill-atlas/internal/skill"
)

// failedRepos has alpha (2 skills), broken (failed before the download finished) and walk (failed after it).
func failedRepos() Report {
	return Report{Repos: []Repo{
		{Name: "github.com/o/alpha", Ref: "main", SHA: "aaaaaaa1111111", Skills: []skill.Skill{sk("x1"), sk("x2")}},
		{Name: "github.com/o/broken", Ref: "v9", Err: "authentication failed for github.com/o/broken"},
		{Name: "github.com/o/walk", Ref: "dev", SHA: "ccccccc3333333", Err: "walk failed"},
	}}
}

func TestFailedRepoSummaryAndHeadings(t *testing.T) {
	page, doc := render(t, failedRepos())
	assertInert(t, doc, 1, 1)
	for _, p := range elements(doc, "p") {
		if hasClass(p, "counts") && textOf(p) != "2 skills, 0 invalid, 2 failed" {
			t.Errorf("counts = %q", textOf(p))
		}
	}
	var contents, sections []string
	for _, n := range append(elements(doc, "h2"), elements(doc, "h3")...) {
		if !hasClass(n, "repohead") {
			continue
		}
		if n.Data == "h3" {
			contents = append(contents, textOf(n))
		} else {
			sections = append(sections, textOf(n))
		}
	}
	wantContents := []string{
		"github.com/o/alpha @ main (aaaaaaa) 2 skills, 0 invalid",
		"github.com/o/broken @ v9 failed",
		"github.com/o/walk @ dev (ccccccc) failed",
	}
	if !slices.Equal(contents, wantContents) {
		t.Errorf("contents headings = %q\nwant %q", contents, wantContents)
	}
	if !slices.Equal(sections, wantContents) {
		t.Errorf("section headings = %q\nwant %q", sections, wantContents)
	}
	if strings.Contains(string(page), "broken @ v9 (") {
		t.Error("a repository that failed to download shows a SHA")
	}
}

func TestFailedRepoNoteInContentsAndSection(t *testing.T) {
	_, doc := render(t, failedRepos())
	var notes []string
	for _, p := range elements(doc, "p") {
		if hasClass(p, "fail") {
			notes = append(notes, textOf(p))
		}
	}
	want := []string{
		"authentication failed for github.com/o/broken", "walk failed", // contents
		"authentication failed for github.com/o/broken", "walk failed", // sections
	}
	if !slices.Equal(notes, want) {
		t.Errorf("failure notes = %q, want %q", notes, want)
	}
	// Failed repositories add no skill sections, and their groups have nothing for the filter to match.
	if n := len(elements(doc, "section")); n != 2 {
		t.Errorf("%d sections, want 2", n)
	}
}

func TestFailedRepoErrorIsEscaped(t *testing.T) {
	r := failedRepos()
	r.Repos[1].Err = `<script>alert(1)</script> "quoted" &` + "\x1b]0;evil\x07\x1b[31mred\x1b[0m"
	page, doc := render(t, r)
	if strings.Contains(string(page), "<script>alert") {
		t.Error("error text reached the page unescaped")
	}
	if strings.Contains(string(page), "\x1b") {
		t.Error("escape sequence reached the page")
	}
	assertInert(t, doc, 1, 1)
	if !strings.Contains(string(page), "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Error("escaped error text missing")
	}
}

func TestFailedRepoKeepsCSPPinned(t *testing.T) {
	page, _ := render(t, failedRepos())
	checkCSP := cspWithScript(filterScript)
	if !strings.Contains(string(page), checkCSP) {
		t.Errorf("CSP lacks the hash of the embedded filter.js:\n%s", page)
	}
	if strings.Contains(string(page), "script-src 'unsafe-inline'") {
		t.Error("CSP allows inline scripts")
	}
}

func TestOnlyFailedReposHaveNoScript(t *testing.T) {
	page, doc := render(t, Report{Repos: []Repo{{Name: "x/one", Err: "e1"}, {Name: "x/two", Err: "e2"}}})
	assertInert(t, doc, 1, 0)
	if strings.Contains(string(page), "script-src") {
		t.Error("CSP has script-src on a page without skills")
	}
	if !strings.Contains(string(page), "0 skills, 0 invalid, 2 failed") {
		t.Error("summary lacks the failed count")
	}
}

func TestNoFailuresNoFailedCount(t *testing.T) {
	page, _ := render(t, multiRepos())
	if strings.Contains(string(page), "failed") {
		t.Error("report without failures mentions them")
	}
}

// A failed organization listing has no ref and no SHA.
func TestFailedEntryWithoutRef(t *testing.T) {
	page, doc := render(t, Report{Repos: []Repo{
		{Name: "github.com/acme", Err: "github.com/acme isn't a GitHub organization or doesn't exist"},
		{Name: "github.com/o/alpha", Ref: "main", SHA: "aaaaaaa1111111", Skills: []skill.Skill{sk("x1")}},
	}})
	var heads []string
	for _, n := range elements(doc, "h3") {
		if hasClass(n, "repohead") {
			heads = append(heads, textOf(n))
		}
	}
	if want := []string{"github.com/acme failed", "github.com/o/alpha @ main (aaaaaaa) 1 skill, 0 invalid"}; !slices.Equal(heads, want) {
		t.Errorf("contents headings = %q, want %q", heads, want)
	}
	if strings.Contains(string(page), "github.com/acme @") {
		t.Error("the failed entry shows a ref")
	}
}
