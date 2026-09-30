// Package skill parses SKILL.md files and validates them against the Agent Skills specification.
package skill

// FileName marks a directory as a skill. Matched case-sensitively.
const FileName = "SKILL.md"

// Skill is one SKILL.md found in a repository, valid or not.
type Skill struct {
	// Path is the SKILL.md path relative to the repository root, slash-separated.
	Path string
	// Dir is the name of the directory holding SKILL.md (the repository name for a root-level SKILL.md).
	Dir string

	// Frontmatter values as written. Empty when absent or unusable.
	Name          string
	Description   string
	License       string
	Compatibility string
	AllowedTools  string
	Metadata      []MetadataEntry // file order

	// Extensions holds provider-specific fields in file order, e.g. Claude Code's argument-hint.
	Extensions []Extension

	// Body is the Markdown after the frontmatter.
	Body string

	// Errors lists every specification rule the skill breaks. Empty when valid.
	Errors []string
}

// MetadataEntry is one key-value pair from the metadata field.
type MetadataEntry struct {
	Key   string
	Value string
}

// Extension is one provider-specific frontmatter field with a display-ready value.
type Extension struct {
	Provider string // e.g. "Claude Code"
	Key      string
	Value    string
}

// Valid reports whether the skill breaks no specification rules.
func (s Skill) Valid() bool { return len(s.Errors) == 0 }

// DisplayName returns Name, falling back to Dir when Name is empty.
func (s Skill) DisplayName() string {
	if s.Name != "" {
		return s.Name
	}
	return s.Dir
}
