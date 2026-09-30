//go:build e2e

package e2e

import (
	"os"
	"slices"
	"strings"
	"testing"

	"skill-atlas/internal/scan"
	"skill-atlas/internal/skill"
)

// zcaceres/skills zoom@1.0.1 has 61 skills, many with Claude Code frontmatter fields.
var claudeSkills = fixture{
	url: "https://github.com/zcaceres/skills.git", tag: "zoom@1.0.1",
	sha: "1d5af9474d0f3f729b1dd3ccdec5ed269f94d1ea",
}

func ext(key, value string) skill.Extension {
	return skill.Extension{Provider: "Claude Code", Key: key, Value: value}
}

func TestClaudeCodeFields(t *testing.T) {
	t.Parallel()
	f := claudeSkills
	res := cloneScan(t, f.url, f.tag, scan.Options{})
	if res.checkout.Ref != f.tag {
		t.Errorf("Ref = %q, want %q", res.checkout.Ref, f.tag)
	}
	if res.checkout.SHA != f.sha {
		t.Errorf("SHA = %q, want %q", res.checkout.SHA, f.sha)
	}
	if len(res.skills) != 61 {
		t.Errorf("skills = %d, want 61", len(res.skills))
	}

	// Only these two skills break a rule, and neither breaks it over a Claude Code field.
	var invalid []string
	for _, s := range res.skills {
		if !s.Valid() {
			invalid = append(invalid, s.Path)
		}
		for _, e := range s.Errors {
			if strings.HasPrefix(e, "unexpected fields") {
				t.Errorf("%s: %s", s.Path, e)
			}
		}
	}
	if want := []string{"_template/SKILL.md", "skills/pr/SKILL.md"}; !slices.Equal(invalid, want) {
		t.Errorf("invalid = %v, want %v", invalid, want)
	}

	want := map[string][]skill.Extension{
		"skills/bro/SKILL.md":         {ext("disable-model-invocation", "true")},
		"skills/copywriting/SKILL.md": nil,
		"skills/code-tour/SKILL.md":   {ext("argument-hint", "[path-to-tour]"), ext("disable-model-invocation", "true")},
		"skills/investigate-repo/SKILL.md": {
			ext("argument-hint", "[repo-url-or-path]"), ext("disable-model-invocation", "true"),
		},
		"skills/laconic/SKILL.md": {
			ext("argument-hint", "[on|off|status|mode|cadence|uninstall] [--project|--user] [prose-only|prose+code|laconic-code] [N]"),
			ext("disable-model-invocation", "true"),
			ext("hooks", "SessionStart (1), UserPromptSubmit (1)"),
		},
		"skills/loose-ends/SKILL.md":         {ext("argument-hint", "")},
		"skills/safety-rm-rf-guard/SKILL.md": {ext("hooks", "PreToolUse (1)")},
		"skills/pr/SKILL.md": {
			ext("argument-hint", "[commit | setup | update | log | walk | merge | checkpoint | submit | sync] [--draft] [args]"),
		},
	}
	byPath := map[string]skill.Skill{}
	for _, s := range res.skills {
		byPath[s.Path] = s
	}
	for p, exts := range want {
		s, ok := byPath[p]
		if !ok {
			t.Errorf("missing skill %s", p)
			continue
		}
		if !slices.Equal(s.Extensions, exts) {
			t.Errorf("%s: extensions = %+v, want %+v", p, s.Extensions, exts)
		}
	}
	if got := byPath["skills/investigate-repo/SKILL.md"].AllowedTools; got != "Read, Grep, Glob, Bash, WebFetch" {
		t.Errorf("investigate-repo allowed-tools = %q", got)
	}
}

// TestHTMLClaudeFields runs --html on the Claude fixture and checks the page shows the Claude Code fields.
func TestHTMLClaudeFields(t *testing.T) {
	t.Parallel()
	f := claudeSkills
	r := runHTML(t, requireTool(t, "true"), nil, "--ref", f.tag, f.url)
	if r.code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", r.code, r.stderr)
	}
	b, err := os.ReadFile(r.reportPath(t))
	if err != nil {
		t.Fatal(err)
	}
	page := string(b)

	if n := strings.Count(page, "<section "); n != 61 {
		t.Errorf("report has %d skill sections, want 61", n)
	}
	for _, w := range []string{
		"<dt>Claude Code</dt>",
		"<dt>argument-hint</dt><dd>[path-to-tour]</dd>",
		"<dt>disable-model-invocation</dt><dd>true</dd>",
		"<dt>hooks</dt><dd>PreToolUse (1)</dd>",
		"<dt>hooks</dt><dd>SessionStart (1), UserPromptSubmit (1)</dd>",
	} {
		if !strings.Contains(page, w) {
			t.Errorf("report lacks %q", w)
		}
	}
	// A skill without Claude Code fields has no group.
	found := false
	for _, sec := range strings.Split(page, "<section ")[1:] {
		if strings.Contains(sec, "<code>skills/copywriting/SKILL.md</code>") {
			found = true
			if strings.Contains(sec, "Claude Code") {
				t.Error("copywriting section shows a Claude Code group")
			}
		}
	}
	if !found {
		t.Error("report lacks the copywriting section")
	}
	checkScriptPinned(t, page)
}
