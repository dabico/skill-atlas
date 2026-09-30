// Package htmlreport renders scan results as one self-contained HTML page, writes it to a temp file and opens it in the browser.
package htmlreport

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"

	"skill-atlas/internal/skill"
)

// Report is everything the page shows. Built in memory after the scan.
type Report struct {
	Repo   string // repository display name, e.g. "github.com/org/repo"
	Ref    string // branch or tag name
	SHA    string // full commit SHA
	Skills []skill.Skill
}

//go:embed page.tmpl
var pageSource string

var pageTmpl = template.Must(template.New("page").Parse(pageSource))

type pageData struct {
	Repo, Ref, SHA, ShortSHA string
	Summary                  string // "N skills, M invalid"
	Skills                   []skillData
}

type skillData struct {
	ID          string // section id, target of the contents link
	Name        string
	Path        string
	Description string
	Invalid     bool
	Errors      []string
	Fields      []field
	Body        template.HTML // goldmark output without raw HTML
}

// field is one frontmatter entry; Sub holds the metadata pairs.
type field struct {
	Key, Value string
	Sub        []skill.MetadataEntry
}

// Render returns the HTML page for r. Every value except the rendered Markdown goes through html/template escaping.
func Render(r Report) ([]byte, error) {
	data := pageData{
		Repo:     r.Repo,
		Ref:      r.Ref,
		SHA:      r.SHA,
		ShortSHA: r.SHA[:min(7, len(r.SHA))],
	}

	invalid := 0
	for i, s := range r.Skills {
		body, err := renderBody(s.Body)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", s.Path, err)
		}
		if !s.Valid() {
			invalid++
		}
		data.Skills = append(data.Skills, skillData{
			ID:          fmt.Sprintf("skill-%d", i+1),
			Name:        s.DisplayName(),
			Path:        s.Path,
			Description: s.Description,
			Invalid:     !s.Valid(),
			Errors:      s.Errors,
			Fields:      fields(s),
			Body:        body,
		})
	}
	noun := "skills"
	if len(r.Skills) == 1 {
		noun = "skill"
	}
	data.Summary = fmt.Sprintf("%d %s, %d invalid", len(r.Skills), noun, invalid)

	var buf bytes.Buffer
	if err := pageTmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// fields lists the frontmatter values other than name and description.
func fields(s skill.Skill) []field {
	var out []field
	add := func(k, v string) {
		if v != "" {
			out = append(out, field{Key: k, Value: v})
		}
	}
	add("license", s.License)
	add("compatibility", s.Compatibility)
	add("allowed-tools", s.AllowedTools)
	if len(s.Metadata) > 0 {
		out = append(out, field{Key: "metadata", Sub: s.Metadata})
	}
	return out
}
