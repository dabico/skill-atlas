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

// sortRepos lists repositories and their skills out of name order: zeta (2 skills), beta (failed), alpha (2 skills), empty.
func sortRepos() Report {
	bad := sk("b-skill")
	bad.Errors = []string{"broken"}
	return Report{Repos: []Repo{
		{Name: "github.com/o/zeta", Ref: "main", SHA: "ddddddd4444444", Skills: []skill.Skill{sk("z2"), sk("z1")}},
		{Name: "github.com/o/beta", Ref: "v9", Err: "auth failed"},
		{Name: "github.com/O/alpha", Ref: "main", SHA: "aaaaaaa1111111", Skills: []skill.Skill{bad, sk("A-skill")}},
		{Name: "github.com/o/empty", Ref: "dev", SHA: "ccccccc3333333", Excluded: 2},
	}}
}

// heading returns the first h2 or h3 under n.
func heading(n *html.Node) *html.Node {
	var h *html.Node
	walk(n, func(m *html.Node) {
		if h == nil && m.Type == html.ElementNode && (m.Data == "h2" || m.Data == "h3") {
			h = m
		}
	})
	return h
}

// sectionOrder returns "<id> <name>" for each skill section, in page order.
func sectionOrder(doc *html.Node) []string {
	var out []string
	for _, s := range elements(doc, "section") {
		out = append(out, attr(s, "id")+" "+strings.Fields(textOf(heading(s)))[0])
	}
	return out
}

func TestRenderSortsSkillsByName(t *testing.T) {
	skills := []skill.Skill{sk("pdf-processing"), sk("code-review"), {Path: "skills/PDF-Tool/SKILL.md", Dir: "PDF-Tool"}, sk("data-analysis")}
	_, doc := render(t, Report{Repos: []Repo{{Name: "github.com/o/r", Skills: skills}}})
	want := []string{"skill-1 code-review", "skill-2 data-analysis", "skill-3 pdf-processing", "skill-4 PDF-Tool"}
	if got := sectionOrder(doc); !slices.Equal(got, want) {
		t.Errorf("sections = %q\nwant %q", got, want)
	}
	var contents []string
	for _, a := range elements(elements(doc, "nav")[0], "a") {
		contents = append(contents, strings.TrimPrefix(attr(a, "href"), "#")+" "+textOf(a))
	}
	if !slices.Equal(contents, want) {
		t.Errorf("contents = %q\nwant %q", contents, want)
	}
}

func TestMultiRenderSortsRepositoriesThenSkills(t *testing.T) {
	_, doc := render(t, sortRepos())
	var contents, sections []string
	for _, n := range append(elements(doc, "h3"), elements(doc, "h2")...) {
		if hasClass(n, "repohead") && n.Data == "h3" {
			contents = append(contents, strings.Fields(textOf(n))[0])
		} else if hasClass(n, "repohead") {
			sections = append(sections, strings.Fields(textOf(n))[0])
		}
	}
	if want := []string{"github.com/O/alpha", "github.com/o/beta", "github.com/o/empty", "github.com/o/zeta"}; !slices.Equal(contents, want) {
		t.Errorf("contents groups = %q\nwant %q", contents, want)
	}
	if want := []string{"github.com/O/alpha", "github.com/o/beta", "github.com/o/zeta"}; !slices.Equal(sections, want) {
		t.Errorf("section groups = %q\nwant %q", sections, want)
	}
	// Repository numbers in the ids count in name order, so zeta is 4 even though it came first.
	want := []string{"repo-1-skill-1 A-skill", "repo-1-skill-2 b-skill", "repo-4-skill-1 z1", "repo-4-skill-2 z2"}
	if got := sectionOrder(doc); !slices.Equal(got, want) {
		t.Errorf("sections = %q\nwant %q", got, want)
	}
}

func TestMultiRenderSameRepositoryByRef(t *testing.T) {
	_, doc := render(t, Report{Repos: []Repo{
		{Name: "github.com/o/r", Ref: "v2", Skills: []skill.Skill{sk("two")}},
		{Name: "github.com/o/r", Ref: "v1", Skills: []skill.Skill{sk("one")}},
	}})
	if got, want := sectionOrder(doc), []string{"repo-1-skill-1 one", "repo-2-skill-1 two"}; !slices.Equal(got, want) {
		t.Errorf("sections = %q, want %q", got, want)
	}
}

func TestRenderSortSelect(t *testing.T) {
	for name, r := range map[string]Report{
		"single": {Repos: []Repo{{Name: "github.com/o/r", Skills: filterSkills()}}},
		"multi":  sortRepos(),
	} {
		t.Run(name, func(t *testing.T) {
			_, doc := render(t, r)
			selects := elements(doc, "select")
			if len(selects) != 1 {
				t.Fatalf("%d select elements, want 1", len(selects))
			}
			sel := selects[0]
			if attr(sel, "id") != "sort" || attr(sel, "aria-label") != "Sort skills" {
				t.Errorf("select attributes = %v", sel.Attr)
			}
			// It sits in the filter box, which is hidden until the script runs.
			if p := sel.Parent; p == nil || attr(p, "id") != "filter" || !hasAttr(p, "hidden") {
				t.Errorf("select isn't in the hidden filter box")
			}
			var opts []string
			for _, o := range elements(sel, "option") {
				opt := attr(o, "value") + "=" + textOf(o)
				if hasAttr(o, "selected") {
					opt += " (selected)"
				}
				opts = append(opts, opt)
			}
			if want := []string{"asc=Name A–Z (selected)", "desc=Name Z–A"}; !slices.Equal(opts, want) {
				t.Errorf("options = %q, want %q", opts, want)
			}
		})
	}
}

func TestRenderNoSortSelectWithoutSkills(t *testing.T) {
	for name, r := range map[string]Report{
		"single": {Repos: []Repo{{Name: "github.com/o/r", Excluded: 2}}},
		"multi":  {Repos: []Repo{{Name: "a/b"}, {Name: "c/d", Err: "boom"}}},
	} {
		t.Run(name, func(t *testing.T) {
			page, doc := render(t, r)
			if len(elements(doc, "select")) != 0 || bytes.Contains(page, []byte(`id="sort"`)) {
				t.Error("page without skills has a sort select")
			}
			assertInert(t, doc, 1, 0)
		})
	}
}

func TestRenderCSPHasOneScriptHash(t *testing.T) {
	_, doc := render(t, sortRepos())
	csp := metaContent(doc, "Content-Security-Policy")
	if n := strings.Count(csp, "'sha256-"); n != 1 {
		t.Errorf("CSP has %d script hashes, want 1: %s", n, csp)
	}
	if csp != cspWithScript(filterScript) {
		t.Errorf("CSP = %q, want the hash of the embedded script", csp)
	}
}

// domHarness runs filter.js against a small DOM built from the parsed page: element nodes with
// parentNode, childNodes, nextSibling and insertBefore, and the few selectors the script uses.
const domHarness = `
const vm = require("vm");
const spec = JSON.parse(require("fs").readFileSync(0, "utf8"));
function build(n, parent) {
  const el = {
    tagName: n.tag.toUpperCase(), key: n.key, attrs: n.attrs, parentNode: parent, childNodes: [],
    hidden: "hidden" in n.attrs, dataset: { match: n.attrs["data-match"] }, value: "", textContent: "",
    listeners: {}, addEventListener(ev, fn) { this.listeners[ev] = fn; }, focus() {}, blur() {},
    get nextSibling() { const c = this.parentNode.childNodes; return c[c.indexOf(this) + 1] || null; },
    insertBefore(node, ref) {
      const from = node.parentNode.childNodes;
      from.splice(from.indexOf(node), 1);
      const at = ref === null ? this.childNodes.length : this.childNodes.indexOf(ref);
      if (at < 0) throw new Error("insertBefore: ref isn't a child");
      this.childNodes.splice(at, 0, node);
      node.parentNode = this;
      return node;
    },
    querySelector(sel) { return find(this, sel)[0] || null; },
  };
  el.childNodes = n.children.map((c) => build(c, el));
  return el;
}
function walk(el, f) { for (const c of el.childNodes) { f(c); walk(c, f); } }
const selectors = {
  "[data-match]": (e) => "data-match" in e.attrs,
  "section[data-match]": (e) => e.tagName === "SECTION" && "data-match" in e.attrs,
  "[data-group]": (e) => "data-group" in e.attrs,
  "[data-match]:not([hidden])": (e) => "data-match" in e.attrs && !e.hidden,
};
function find(root, sel) {
  if (!selectors[sel]) throw new Error("unsupported selector " + sel);
  const out = [];
  walk(root, (e) => { if (selectors[sel](e)) out.push(e); });
  return out;
}
const top = { childNodes: [] };
top.childNodes.push(build(spec.doc, top));
const byId = (id) => { let hit = null; walk(top, (e) => { if (hit === null && e.attrs.id === id) hit = e; }); return hit; };
const document = { activeElement: null, addEventListener() {}, getElementById: byId, querySelectorAll: (sel) => find(top, sel) };
const q = byId("q"), sort = byId("sort"), count = byId("fcount");
sort.value = spec.initial;
vm.runInNewContext(spec.script, { document });
const snap = () => {
  const keys = [];
  walk(top, (e) => { if (e.key) keys.push(e.key + (e.hidden ? " (hidden)" : "")); });
  return { keys, count: count.hidden ? "" : count.textContent, box: byId("filter").hidden };
};
const out = [snap()];
for (const step of spec.steps) {
  if (step.filter !== undefined) { q.value = step.filter; q.listeners.input(); }
  if (step.sort !== undefined) { sort.value = step.sort; sort.listeners.change(); }
  out.push(snap());
}
console.log(JSON.stringify(out));
`

type domNode struct {
	Tag      string            `json:"tag"`
	Attrs    map[string]string `json:"attrs"`
	Key      string            `json:"key,omitempty"` // what the harness reports for contents entries, sections and groups
	Children []domNode         `json:"children"`
}

func toDOM(n *html.Node) domNode {
	d := domNode{Tag: n.Data, Attrs: map[string]string{}, Children: []domNode{}}
	for _, a := range n.Attr {
		d.Attrs[a.Key] = a.Val
	}
	switch {
	case n.Data == "li" && hasAttr(n, "data-match"):
		d.Key = "li " + textOf(elements(n, "a")[0])
	case n.Data == "section":
		d.Key = "sec " + attr(n, "id")
	case hasAttr(n, "data-group"):
		d.Key = "grp " + strings.Fields(textOf(heading(n)))[0]
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			d.Children = append(d.Children, toDOM(c))
		}
	}
	return d
}

type domStep struct {
	Filter *string `json:"filter,omitempty"`
	Sort   string  `json:"sort,omitempty"`
}

type domSnap struct {
	Keys  []string `json:"keys"`
	Count string   `json:"count"`
	Box   bool     `json:"box"`
}

// runSortScript runs filter.js on the page of r with the select at initial, then applies steps; it returns a snapshot before and after each step.
func runSortScript(t *testing.T, r Report, initial string, steps []domStep) []domSnap {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found")
	}
	_, doc := render(t, r)
	root := elements(doc, "html")[0]
	spec, err := json.Marshal(map[string]any{"script": filterScript, "doc": toDOM(root), "initial": initial, "steps": steps})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, "-e", domHarness)
	cmd.Stdin = bytes.NewReader(spec)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, stderr.String())
	}
	var got []domSnap
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func str(s string) *string { return &s }

func checkKeys(t *testing.T, step string, got domSnap, want []string) {
	t.Helper()
	if !slices.Equal(got.Keys, want) {
		t.Errorf("%s:\n got %q\nwant %q", step, got.Keys, want)
	}
}

func TestSortScriptSingleRepo(t *testing.T) {
	skills := []skill.Skill{sk("c"), sk("a"), sk("b")}
	got := runSortScript(t, Report{Repos: []Repo{{Name: "github.com/o/r", Skills: skills}}}, "asc",
		[]domStep{{Sort: "desc"}, {Sort: "asc"}})
	az := []string{"li a", "li b", "li c", "sec skill-1", "sec skill-2", "sec skill-3"}
	za := []string{"li c", "li b", "li a", "sec skill-3", "sec skill-2", "sec skill-1"}
	if got[0].Box {
		t.Error("the script didn't show the filter box")
	}
	checkKeys(t, "load", got[0], az)
	checkKeys(t, "Z–A", got[1], za)
	checkKeys(t, "A–Z again", got[2], az)
}

func TestSortScriptMultiRepo(t *testing.T) {
	got := runSortScript(t, sortRepos(), "asc", []domStep{
		{Sort: "desc"},
		{Sort: "asc"},
		{Filter: str("z")},
		{Sort: "desc"},
		{Filter: str("")},
		{Sort: "asc"},
	})
	// Contents groups hold their entries; section groups hold their sections. Empty has no section group.
	az := []string{
		"grp github.com/O/alpha", "li A-skill", "li b-skill", "grp github.com/o/beta", "grp github.com/o/empty",
		"grp github.com/o/zeta", "li z1", "li z2",
		"grp github.com/O/alpha", "sec repo-1-skill-1", "sec repo-1-skill-2", "grp github.com/o/beta",
		"grp github.com/o/zeta", "sec repo-4-skill-1", "sec repo-4-skill-2",
	}
	za := []string{
		"grp github.com/o/zeta", "li z2", "li z1", "grp github.com/o/empty", "grp github.com/o/beta",
		"grp github.com/O/alpha", "li b-skill", "li A-skill",
		"grp github.com/o/zeta", "sec repo-4-skill-2", "sec repo-4-skill-1", "grp github.com/o/beta",
		"grp github.com/O/alpha", "sec repo-1-skill-2", "sec repo-1-skill-1",
	}
	checkKeys(t, "load", got[0], az)
	checkKeys(t, "Z–A", got[1], za)
	checkKeys(t, "A–Z again", got[2], az)

	// Sorting keeps the filter: only zeta matches "z".
	filteredAZ := []string{
		"grp github.com/O/alpha (hidden)", "li A-skill (hidden)", "li b-skill (hidden)", "grp github.com/o/beta (hidden)",
		"grp github.com/o/empty (hidden)", "grp github.com/o/zeta", "li z1", "li z2",
		"grp github.com/O/alpha (hidden)", "sec repo-1-skill-1 (hidden)", "sec repo-1-skill-2 (hidden)",
		"grp github.com/o/beta (hidden)", "grp github.com/o/zeta", "sec repo-4-skill-1", "sec repo-4-skill-2",
	}
	filteredZA := []string{
		"grp github.com/o/zeta", "li z2", "li z1", "grp github.com/o/empty (hidden)", "grp github.com/o/beta (hidden)",
		"grp github.com/O/alpha (hidden)", "li b-skill (hidden)", "li A-skill (hidden)",
		"grp github.com/o/zeta", "sec repo-4-skill-2", "sec repo-4-skill-1", "grp github.com/o/beta (hidden)",
		"grp github.com/O/alpha (hidden)", "sec repo-1-skill-2 (hidden)", "sec repo-1-skill-1 (hidden)",
	}
	checkKeys(t, "filter z", got[3], filteredAZ)
	checkKeys(t, "filter z, Z–A", got[4], filteredZA)
	for i := 3; i <= 4; i++ {
		if got[i].Count != "Skills 2/4" {
			t.Errorf("step %d: count = %q, want Skills 2/4", i, got[i].Count)
		}
	}
	checkKeys(t, "filter cleared, Z–A", got[5], za)
	checkKeys(t, "A–Z at the end", got[6], az)
}

// TestSortScriptRestoredSelect covers a browser that restores the select to Z–A on reload: the script reorders on load.
func TestSortScriptRestoredSelect(t *testing.T) {
	skills := []skill.Skill{sk("b"), sk("a")}
	got := runSortScript(t, Report{Repos: []Repo{{Name: "github.com/o/r", Skills: skills}}}, "desc", []domStep{})
	checkKeys(t, "load with Z–A", got[0], []string{"li b", "li a", "sec skill-2", "sec skill-1"})
}
