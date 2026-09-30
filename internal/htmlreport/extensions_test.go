package htmlreport

import (
	"slices"
	"strings"
	"testing"

	"skill-atlas/internal/skill"
)

func claudeExt(k, v string) skill.Extension {
	return skill.Extension{Provider: "Claude Code", Key: k, Value: v}
}

func TestRenderExtensions(t *testing.T) {
	with := skill.Skill{
		Path: "a/SKILL.md", Dir: "a", Name: "a", Description: "d", License: "MIT",
		Metadata: []skill.MetadataEntry{{Key: "author", Value: "me"}},
		Extensions: []skill.Extension{
			claudeExt("argument-hint", "[path]"),
			claudeExt("disable-model-invocation", "true"),
			claudeExt("hooks", "PreToolUse (1), Stop (2)"),
		},
	}
	without := skill.Skill{Path: "b/SKILL.md", Dir: "b", Name: "b", Description: "d", License: "MIT"}
	_, doc := render(t, Report{Repo: "r", Skills: []skill.Skill{with, without}})
	sections := elements(doc, "section")

	var dts, dds []string
	for _, dt := range elements(sections[0], "dt") {
		dts = append(dts, textOf(dt))
	}
	for _, dd := range elements(sections[0], "dd") {
		if len(elements(dd, "dl")) == 0 {
			dds = append(dds, textOf(dd))
		}
	}
	wantDT := []string{"license", "metadata", "author", "Claude Code", "argument-hint", "disable-model-invocation", "hooks"}
	if !slices.Equal(dts, wantDT) {
		t.Errorf("keys = %q, want %q", dts, wantDT)
	}
	wantDD := []string{"MIT", "me", "[path]", "true", "PreToolUse (1), Stop (2)"}
	if !slices.Equal(dds, wantDD) {
		t.Errorf("values = %q, want %q", dds, wantDD)
	}

	if m := attr(sections[0], "data-match"); strings.Contains(m, "PreToolUse") || strings.Contains(m, "argument-hint") {
		t.Errorf("filter text includes extension fields: %q", m)
	}
	if got := textOf(sections[1]); strings.Contains(got, "Claude Code") {
		t.Errorf("skill without extensions shows a group: %q", got)
	}
	only := skill.Skill{Path: "c/SKILL.md", Dir: "c", Name: "c", Extensions: []skill.Extension{claudeExt("hooks", "Stop (1)")}}
	_, doc = render(t, Report{Repo: "r", Skills: []skill.Skill{only}})
	if len(elements(doc, "dl")) != 2 {
		t.Errorf("group without other fields: got %d dl elements, want 2", len(elements(doc, "dl")))
	}
}

func TestRenderExtensionsGroupByProvider(t *testing.T) {
	s := skill.Skill{Path: "a/SKILL.md", Dir: "a", Name: "a", Extensions: []skill.Extension{
		claudeExt("x", "1"), {Provider: "Other", Key: "y", Value: "2"}, claudeExt("z", "3"),
	}}
	_, doc := render(t, Report{Repo: "r", Skills: []skill.Skill{s}})
	var dts []string
	for _, dt := range elements(elements(doc, "section")[0], "dt") {
		dts = append(dts, textOf(dt))
	}
	if want := []string{"Claude Code", "x", "z", "Other", "y"}; !slices.Equal(dts, want) {
		t.Errorf("keys = %q, want %q", dts, want)
	}
}

func TestRenderExtensionsHostile(t *testing.T) {
	s := skill.Skill{Path: "a/SKILL.md", Dir: "a", Name: "a", Extensions: []skill.Extension{
		{Provider: hostile, Key: hostile, Value: hostile},
		claudeExt("k\x1b[31mred\x1b[0m", "v\x1b[31mred\x1b[0m\x1b]0;title\x07"),
	}}
	page, doc := render(t, Report{Repo: "r", Skills: []skill.Skill{s}})
	assertInert(t, doc, 1, 1)
	if n := strings.Count(strings.ToLower(string(page)), "<script"); n != 1 {
		t.Errorf("page has %d script tags, want 1", n)
	}
	if strings.Contains(string(page), "\x1b") {
		t.Error("page contains an escape character")
	}
	var texts []string
	for _, n := range append(elements(doc, "dt"), elements(doc, "dd")...) {
		texts = append(texts, textOf(n))
	}
	for _, want := range []string{"kred", "vred"} {
		if !slices.ContainsFunc(texts, func(s string) bool { return s == want }) {
			t.Errorf("no field text %q in %q", want, texts)
		}
	}
	if !slices.Contains(texts, hostile) {
		t.Errorf("hostile text isn't shown literally: %q", texts)
	}
}
