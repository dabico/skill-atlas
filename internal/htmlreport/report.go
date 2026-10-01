// Package htmlreport renders scan results as one self-contained HTML page, writes it to a temp file and opens it in the browser.
package htmlreport

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"skill-atlas/internal/skill"
)

// Repo is one scanned repository.
type Repo struct {
	Name   string // repository display name, e.g. "github.com/org/repo"
	Ref    string // branch or tag name
	SHA    string // full commit SHA
	Skills []skill.Skill
	// Excluded counts SKILL.md files the scan skipped.
	Excluded int
	// Err is why the download or scan failed; it may contain remote text. Empty when the repository was scanned.
	Err string
}

// Report is everything the page shows. Built in memory after the scan.
// With 2 or more repositories the page groups skills by repository.
type Report struct {
	Repos []Repo
}

//go:embed page.tmpl
var pageSource string

//go:embed filter.js
var filterScript string

// scriptHash is the CSP source that allows the filter script and nothing else.
var scriptHash = func() string {
	sum := sha256.Sum256([]byte(filterScript))
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}()

// The hash is base64 in quotes; it goes in before parsing so attribute escaping leaves it alone.
var pageTmpl = template.Must(template.New("page").Parse(strings.ReplaceAll(pageSource, "@SCRIPTHASH@", scriptHash)))

type pageData struct {
	Repo, Ref, SHA, ShortSHA string // Repo is "N repositories" for several
	Multi                    bool   // 2 or more repositories
	Summary                  string // "N skills, M invalid[, K excluded]"
	Empty                    string // text for a scan with no skills
	Skills                   []skillData
	Groups                   []groupData // one per repository, when Multi
	Script                   template.JS // the filter and sort script, shown only when there are skills
}

// groupData is one repository on a page with several.
type groupData struct {
	Name, Ref, SHA, ShortSHA string
	Summary                  string
	Empty                    string
	Failed                   string // failure message, when the repository failed
	Skills                   []skillData
}

type skillData struct {
	ID          string // section id, target of the contents link
	Name        string
	Path        string
	Description string
	Match       string // name, description and path (and repository, when Multi), searched by the filter
	Invalid     bool
	Errors      []string
	Fields      []field
	Body        template.HTML // goldmark output without raw HTML
	Deep        bool          // the name is an h3 under a repository heading
}

// field is one frontmatter entry; Sub holds the metadata pairs or a provider's fields.
type field struct {
	Key, Value string
	Sub        []skill.MetadataEntry
}

// Render returns the HTML page for r. Every value except the rendered Markdown goes through html/template escaping.
// Repositories and the skills in each are in name order A–Z; the script reverses the page for Z–A.
func Render(r Report) ([]byte, error) {
	data := pageData{Multi: len(r.Repos) > 1}
	shift := headingShift
	if data.Multi {
		shift++
		data.Repo = fmt.Sprintf("%d repositories", len(r.Repos))
	} else if len(r.Repos) == 1 {
		data.Repo, data.Ref, data.SHA = r.Repos[0].Name, r.Repos[0].Ref, r.Repos[0].SHA
		data.ShortSHA = shortSHA(data.SHA)
	}

	repos := slices.Clone(r.Repos)
	slices.SortStableFunc(repos, func(a, b Repo) int { return skill.CompareRepos(a.Name, a.Ref, b.Name, b.Ref) })

	var total, invalidTotal, excludedTotal, failed int
	for ri, repo := range repos {
		g := groupData{Name: repo.Name, Ref: repo.Ref, SHA: repo.SHA, ShortSHA: shortSHA(repo.SHA)}
		invalid := 0
		skills := slices.Clone(repo.Skills)
		slices.SortStableFunc(skills, skill.Compare)
		for i, s := range skills {
			body, err := renderBody(s.Body, shift)
			if err != nil {
				return nil, fmt.Errorf("render %s: %w", s.Path, err)
			}
			if !s.Valid() {
				invalid++
			}
			id, match := fmt.Sprintf("skill-%d", i+1), []string{s.DisplayName(), s.Description, s.Path}
			if data.Multi {
				id = fmt.Sprintf("repo-%d-skill-%d", ri+1, i+1)
				match = append(match, repo.Name)
			}
			g.Skills = append(g.Skills, skillData{
				ID:          id,
				Name:        s.DisplayName(),
				Path:        s.Path,
				Description: s.Description,
				Match:       strings.Join(match, "\n"),
				Invalid:     !s.Valid(),
				Errors:      s.Errors,
				Fields:      fields(s),
				Body:        body,
				Deep:        data.Multi,
			})
		}
		g.Summary, g.Empty = summary(len(repo.Skills), invalid, repo.Excluded)
		if repo.Err != "" {
			failed++
			g.Summary, g.Failed = "failed", errorText(repo.Err)
			g.Empty = g.Failed
		}
		total += len(repo.Skills)
		invalidTotal += invalid
		excludedTotal += repo.Excluded
		if data.Multi {
			data.Groups = append(data.Groups, g)
		} else {
			data.Skills = g.Skills
		}
	}
	data.Summary, data.Empty = summary(total, invalidTotal, excludedTotal)
	if failed > 0 {
		data.Summary += fmt.Sprintf(", %d failed", failed)
	}
	if !data.Multi && len(repos) == 1 && repos[0].Err != "" {
		data.Empty = errorText(repos[0].Err)
	}
	if total > 0 {
		data.Script = template.JS(filterScript)
	}

	var buf bytes.Buffer
	if err := pageTmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func shortSHA(sha string) string { return sha[:min(7, len(sha))] }

// summary returns the counts line and the text for a scan with no skills.
func summary(skills, invalid, excluded int) (counts, empty string) {
	noun := "skills"
	if skills == 1 {
		noun = "skill"
	}
	counts = fmt.Sprintf("%d %s, %d invalid", skills, noun, invalid)
	empty = "No skills found"
	if excluded > 0 {
		counts += fmt.Sprintf(", %d excluded", excluded)
		empty += fmt.Sprintf(" (%d excluded)", excluded)
	}
	return counts, empty
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
	return append(out, extensionFields(s.Extensions)...)
}

// extensionFields makes one field per provider, in first-seen order, with ANSI stripped like the TUI.
func extensionFields(exts []skill.Extension) []field {
	var out []field
	for _, e := range exts {
		i := slices.IndexFunc(out, func(f field) bool { return f.Key == ansi.Strip(e.Provider) })
		if i < 0 {
			out = append(out, field{Key: ansi.Strip(e.Provider)})
			i = len(out) - 1
		}
		out[i].Sub = append(out[i].Sub, skill.MetadataEntry{Key: ansi.Strip(e.Key), Value: ansi.Strip(e.Value)})
	}
	return out
}

// errorText makes a failure message plain text on one line: no escape sequences.
func errorText(err string) string {
	return strings.Join(strings.Fields(ansi.Strip(err)), " ")
}
