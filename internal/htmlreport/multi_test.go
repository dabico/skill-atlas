package htmlreport

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"skill-atlas/internal/skill"
)

func sk(name string) skill.Skill {
	return skill.Skill{Path: "skills/" + name + "/SKILL.md", Dir: name, Name: name, Description: "About " + name + ".", Body: "# Title\n\n###### Deep\n"}
}

// multiRepos has alpha (2 skills, 1 invalid), beta (1 skill) and empty (none, 2 excluded).
func multiRepos() Report {
	bad := sk("x2")
	bad.Errors = []string{"broken"}
	return Report{Repos: []Repo{
		{Name: "github.com/o/alpha", Ref: "main", SHA: "aaaaaaa1111111", Skills: []skill.Skill{sk("x1"), bad}},
		{Name: "github.com/o/beta", Ref: "v2", SHA: "bbbbbbb2222222", Skills: []skill.Skill{sk("y1")}},
		{Name: "github.com/o/empty", Ref: "dev", SHA: "ccccccc3333333", Excluded: 2},
	}}
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func TestMultiHeader(t *testing.T) {
	_, doc := render(t, multiRepos())
	assertInert(t, doc, 1, 1)
	if h1 := elements(doc, "h1"); len(h1) != 1 || textOf(h1[0]) != "3 repositories" {
		t.Errorf("h1 = %v", h1)
	}
	if title := elements(doc, "title"); textOf(title[0]) != "Skills in 3 repositories" {
		t.Errorf("title = %q", textOf(title[0]))
	}
	for _, p := range elements(doc, "p") {
		if hasClass(p, "meta") {
			t.Error("multi-repo header has a ref/SHA meta line")
		}
		if hasClass(p, "counts") && textOf(p) != "3 skills, 1 invalid, 2 excluded" {
			t.Errorf("counts = %q", textOf(p))
		}
	}
}

func TestMultiGroupHeadings(t *testing.T) {
	page, doc := render(t, multiRepos())
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
		"github.com/o/alpha @ main (aaaaaaa) 2 skills, 1 invalid",
		"github.com/o/beta @ v2 (bbbbbbb) 1 skill, 0 invalid",
		"github.com/o/empty @ dev (ccccccc) 0 skills, 0 invalid, 2 excluded",
	}
	if !slices.Equal(contents, wantContents) {
		t.Errorf("contents headings = %q\nwant %q", contents, wantContents)
	}
	if !slices.Equal(sections, wantContents[:2]) {
		t.Errorf("section headings = %q\nwant %q", sections, wantContents[:2])
	}
	for _, sha := range []string{"aaaaaaa1111111", "bbbbbbb2222222", "ccccccc3333333"} {
		if !strings.Contains(string(page), `<code title="`+sha+`">`+sha[:7]+`</code>`) {
			t.Errorf("no hover SHA for %s", sha)
		}
	}
}

func TestMultiEmptyRepo(t *testing.T) {
	_, doc := render(t, multiRepos())
	var notes []string
	for _, p := range elements(doc, "p") {
		if hasClass(p, "empty") && attr(p, "id") == "" {
			notes = append(notes, textOf(p))
		}
	}
	if !slices.Equal(notes, []string{"No skills found (2 excluded)"}) {
		t.Errorf("empty notes = %q", notes)
	}
	// A repository without skills has no section group.
	if n := len(elements(doc, "section")); n != 3 {
		t.Errorf("%d sections, want 3", n)
	}

	r := multiRepos()
	r.Repos[2].Excluded = 0
	page, _ := render(t, r)
	if !strings.Contains(string(page), `<p class="empty">No skills found</p>`) {
		t.Error("empty repository without exclusions lacks its note")
	}
}

func TestMultiIDsAreUniqueAndLinked(t *testing.T) {
	_, doc := render(t, multiRepos())
	ids := map[string]bool{}
	var sectionIDs []string
	walk(doc, func(n *html.Node) {
		if n.Type != html.ElementNode || attr(n, "id") == "" {
			return
		}
		if ids[attr(n, "id")] {
			t.Errorf("duplicate id %q", attr(n, "id"))
		}
		ids[attr(n, "id")] = true
		if n.Data == "section" {
			sectionIDs = append(sectionIDs, attr(n, "id"))
		}
	})
	if want := []string{"repo-1-skill-1", "repo-1-skill-2", "repo-2-skill-1"}; !slices.Equal(sectionIDs, want) {
		t.Errorf("section ids = %v, want %v", sectionIDs, want)
	}
	links := 0
	for _, a := range elements(doc, "a") {
		if href := attr(a, "href"); strings.HasPrefix(href, "#") && href != "#toc" {
			links++
			if !ids[href[1:]] {
				t.Errorf("link %s has no target", href)
			}
		}
	}
	if links != 3 {
		t.Errorf("%d contents links, want 3", links)
	}
}

func TestMultiHeadingLevels(t *testing.T) {
	_, doc := render(t, multiRepos())
	var levels []string
	walk(doc, func(n *html.Node) {
		if n.Type == html.ElementNode && len(n.Data) == 2 && n.Data[0] == 'h' && n.Data[1] >= '1' && n.Data[1] <= '6' {
			levels = append(levels, n.Data)
		}
	})
	// title; contents; 3 repos in contents (h3); then per repo an h2 with h3 skills and h4/h6 body headings.
	want := []string{
		"h1", "h2", "h3", "h3", "h3",
		"h2", "h3", "h4", "h6", "h3", "h4", "h6",
		"h2", "h3", "h4", "h6",
	}
	if !slices.Equal(levels, want) {
		t.Errorf("heading levels = %v\nwant %v", levels, want)
	}
}

func TestMultiMatchIncludesRepository(t *testing.T) {
	_, doc := render(t, multiRepos())
	var got []string
	for _, tag := range []string{"li", "section"} {
		for _, n := range elements(doc, tag) {
			if m := attr(n, "data-match"); m != "" {
				got = append(got, m)
			}
		}
	}
	if len(got) != 6 {
		t.Fatalf("%d match texts, want 6", len(got))
	}
	for i, m := range got {
		if !strings.HasSuffix(m, "\ngithub.com/o/alpha") && !strings.HasSuffix(m, "\ngithub.com/o/beta") {
			t.Errorf("match text %d = %q lacks the repository", i, m)
		}
	}
}

func TestMultiAllEmptyHasNoScript(t *testing.T) {
	page, doc := render(t, Report{Repos: []Repo{{Name: "a/b"}, {Name: "c/d", Excluded: 1}}})
	assertInert(t, doc, 1, 0)
	if got := metaContent(doc, "Content-Security-Policy"); got != cspNoScript {
		t.Errorf("CSP = %q", got)
	}
	if bytes.Contains(page, []byte("id=\"filter\"")) {
		t.Error("empty page has a filter box")
	}
	if !strings.Contains(string(page), "No skills found (1 excluded)") || !strings.Contains(string(page), "0 skills, 0 invalid, 1 excluded") {
		t.Error("empty multi-repo page lacks its notes")
	}
}

func TestSingleRepoHasNoGroups(t *testing.T) {
	page, _ := render(t, Report{Repos: []Repo{{Name: "github.com/o/r", Skills: filterSkills()}}})
	markup, _, _ := strings.Cut(string(page), "<script>") // filter.js itself mentions data-group
	for _, bad := range []string{"data-group", "repohead", "repo-1-", "repositories"} {
		if strings.Contains(markup, bad) {
			t.Errorf("single-repo page contains %q", bad)
		}
	}
	if !strings.Contains(string(page), `id="skill-1"`) || !strings.Contains(string(page), "<h2>alpha</h2>") {
		t.Error("single-repo ids or heading level changed")
	}
}

// harness runs filter.js against a fake DOM built from the page.
const harness = `
const vm = require("vm");
const spec = JSON.parse(require("fs").readFileSync(0, "utf8"));
const mk = (o) => Object.assign({ hidden: false, addEventListener() {}, focus() {}, blur() {}, textContent: "" }, o);
const items = spec.items.map((i) => mk({ tagName: i.tag, dataset: { match: i.match }, group: i.group }));
const groups = spec.groups.map((_, g) => mk({ querySelector: () => items.find((i) => i.group === g && !i.hidden) || null }));
const box = mk({ hidden: true }), q = mk({ value: "" }), count = mk({ hidden: true }), none = mk({ hidden: true });
let onInput;
q.addEventListener = (ev, fn) => { if (ev === "input") onInput = fn; };
const document = {
  activeElement: null,
  addEventListener() {},
  getElementById: (id) => ({ filter: box, q, fcount: count, nomatch: none })[id],
  querySelectorAll: (sel) => ({ "[data-match]": items, "section[data-match]": items.filter((i) => i.tagName === "SECTION"), "[data-group]": groups })[sel],
};
vm.runInNewContext(spec.script, { document });
const out = [];
for (const query of spec.queries) {
  q.value = query;
  onInput();
  out.push({ items: items.map((i) => i.hidden), groups: groups.map((g) => g.hidden), count: count.textContent, countHidden: count.hidden, noneHidden: none.hidden });
}
console.log(JSON.stringify(out));
`

type fakeItem struct {
	Tag   string `json:"tag"`
	Match string `json:"match"`
	Group int    `json:"group"`
}

type fakeResult struct {
	Items       []bool `json:"items"`
	Groups      []bool `json:"groups"`
	Count       string `json:"count"`
	CountHidden bool   `json:"countHidden"`
	NoneHidden  bool   `json:"noneHidden"`
}

func TestMultiFilterScriptBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found")
	}
	_, doc := render(t, multiRepos())

	// Groups in document order: 3 in the contents, then 2 with sections.
	var groups []*html.Node
	walk(doc, func(n *html.Node) {
		if n.Type == html.ElementNode && hasAttr(n, "data-group") {
			groups = append(groups, n)
		}
	})
	if len(groups) != 5 {
		t.Fatalf("%d groups, want 5", len(groups))
	}
	var items []fakeItem
	var names []string
	for g, gn := range groups {
		for _, n := range append(elements(gn, "li"), elements(gn, "section")...) {
			items = append(items, fakeItem{Tag: strings.ToUpper(n.Data), Match: attr(n, "data-match"), Group: g})
			names = append(names, textOf(n))
		}
	}
	queries := []string{"", "beta", "GITHUB.COM/O/ALPHA", "x2", "about y", "empty", "zzz", ""}
	spec, _ := json.Marshal(map[string]any{"script": filterScript, "items": items, "groups": make([]int, len(groups)), "queries": queries})
	cmd := exec.Command(node, "-e", harness)
	cmd.Stdin = bytes.NewReader(spec)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, stderr.String())
	}
	var got []fakeResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}

	// Skills, in order: x1, x2 (alpha), y1 (beta); groups: contents alpha/beta/empty, sections alpha/beta.
	tests := []struct {
		shown  string // skills that stay visible, per section
		groups []bool // hidden, per group
		count  string
		none   bool // "No matching skills" hidden
	}{
		{"x1 x2 y1", []bool{false, false, false, false, false}, "", true},
		{"y1", []bool{true, false, true, true, false}, "Skills 1/3", true},
		{"x1 x2", []bool{false, true, true, false, true}, "Skills 2/3", true},
		{"x2", []bool{false, true, true, false, true}, "Skills 1/3", true},
		{"y1", []bool{true, false, true, true, false}, "Skills 1/3", true},
		{"", []bool{true, true, true, true, true}, "Skills 0/3", false},
		{"", []bool{true, true, true, true, true}, "Skills 0/3", false},
		{"x1 x2 y1", []bool{false, false, false, false, false}, "", true},
	}
	for i, q := range queries {
		g := got[i]
		var shown []string
		for j, it := range items {
			if it.Tag == "SECTION" && !g.Items[j] {
				shown = append(shown, strings.Fields(names[j])[0])
			}
		}
		want := tests[i]
		if strings.Join(shown, " ") != want.shown {
			t.Errorf("query %q: sections shown %v, want %q", q, shown, want.shown)
		}
		if !slices.Equal(g.Groups, want.groups) {
			t.Errorf("query %q: groups hidden %v, want %v", q, g.Groups, want.groups)
		}
		if want.count != "" && g.Count != want.count {
			t.Errorf("query %q: count %q, want %q", q, g.Count, want.count)
		}
		if g.CountHidden != (want.count == "") || g.NoneHidden != want.none {
			t.Errorf("query %q: countHidden %v noneHidden %v", q, g.CountHidden, g.NoneHidden)
		}
	}
}
