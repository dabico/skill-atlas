package skill

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
	"golang.org/x/text/unicode/norm"
)

const (
	maxName          = 64
	maxDescription   = 1024
	maxCompatibility = 500
)

var specFields = []string{"name", "description", "license", "compatibility", "metadata", "allowed-tools"}

// Parse reads a SKILL.md and validates it against the Agent Skills specification.
// path is relative to the repository root; dir is the name of the directory holding the file.
func Parse(path, dir string, content []byte) Skill {
	s := Skill{Path: path, Dir: dir}

	front, body, errMsg := splitFrontmatter(content)
	s.Body = body
	if errMsg != "" {
		s.Errors = append(s.Errors, errMsg)
		return s
	}

	fields, order, errs := parseFields(front)
	s.Errors = append(s.Errors, errs...)
	if fields == nil {
		return s
	}
	s.validate(fields, order)
	return s
}

// parseFields decodes the frontmatter into its top-level fields.
// order lists the keys as written. A nil map means field checks must stop.
func parseFields(front string) (fields map[string]*yaml.Node, order, errs []string) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(front), &doc); err != nil {
		msg := strings.TrimPrefix(err.Error(), "yaml: ")
		return nil, nil, []string{"frontmatter isn't valid YAML: " + msg}
	}
	if doc.Kind == 0 { // empty or comment-only
		return map[string]*yaml.Node{}, nil, nil
	}
	root := resolve(doc.Content[0])
	if root.Kind != yaml.MappingNode {
		return nil, nil, []string{"frontmatter must be a YAML mapping"}
	}

	fields = make(map[string]*yaml.Node)
	var unexpected []string
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := resolve(root.Content[i]).Value
		if _, dup := fields[key]; dup {
			errs = append(errs, fmt.Sprintf("duplicate field %q", key))
			continue
		}
		fields[key] = root.Content[i+1]
		order = append(order, key)
		if !slices.Contains(specFields, key) && claudeRule(key) == nil {
			unexpected = append(unexpected, key)
		}
	}
	if len(unexpected) > 0 {
		slices.Sort(unexpected)
		errs = append(errs, "unexpected fields: "+strings.Join(unexpected, ", "))
	}
	return fields, order, errs
}

func (s *Skill) validate(fields map[string]*yaml.Node, order []string) {
	s.checkName(fields["name"])
	s.checkDescription(fields["description"])
	s.checkCompatibility(fields["compatibility"])
	s.License = s.optionalString("license", fields["license"])
	s.checkAllowedTools(fields["allowed-tools"])
	s.checkMetadata(fields["metadata"])
	s.checkClaude(fields, order)
}

func (s *Skill) errorf(format string, args ...any) {
	s.Errors = append(s.Errors, fmt.Sprintf(format, args...))
}

func (s *Skill) checkName(n *yaml.Node) {
	if n == nil {
		s.errorf("name is missing")
		return
	}
	raw, ok := scalar(n)
	if !ok {
		s.errorf("name must be a string")
		return
	}
	name := norm.NFKC.String(strings.TrimSpace(raw))
	s.Name = name
	if name == "" {
		s.errorf("name is empty")
		return
	}

	if c := utf8.RuneCountInString(name); c > maxName {
		s.errorf("name %q is longer than %d characters (%d)", name, maxName, c)
	}
	if name != strings.ToLower(name) {
		s.errorf("name %q contains uppercase letters", name)
	}
	if strings.ContainsFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '-'
	}) {
		s.errorf("name %q contains characters other than letters, digits and hyphens", name)
	}
	if strings.HasPrefix(name, "-") {
		s.errorf("name %q starts with a hyphen", name)
	}
	if strings.HasSuffix(name, "-") {
		s.errorf("name %q ends with a hyphen", name)
	}
	if strings.Contains(name, "--") {
		s.errorf("name %q contains consecutive hyphens", name)
	}
	if norm.NFKC.String(s.Dir) != name {
		s.errorf("name %q doesn't match directory %q", name, s.Dir)
	}
}

func (s *Skill) checkDescription(n *yaml.Node) {
	if n == nil {
		s.errorf("description is missing")
		return
	}
	raw, ok := scalar(n)
	if !ok {
		s.errorf("description must be a string")
		return
	}
	s.Description = strings.TrimSpace(raw)
	if s.Description == "" {
		s.errorf("description is empty")
		return
	}
	if c := utf8.RuneCountInString(raw); c > maxDescription {
		s.errorf("description is longer than %d characters (%d)", maxDescription, c)
	}
}

func (s *Skill) checkCompatibility(n *yaml.Node) {
	if n == nil {
		return
	}
	raw, ok := scalar(n)
	if !ok {
		s.errorf("compatibility must be a string")
		return
	}
	s.Compatibility = raw
	if strings.TrimSpace(raw) == "" {
		s.errorf("compatibility is empty")
		return
	}
	if c := utf8.RuneCountInString(raw); c > maxCompatibility {
		s.errorf("compatibility is longer than %d characters (%d)", maxCompatibility, c)
	}
}

func (s *Skill) optionalString(field string, n *yaml.Node) string {
	if n == nil {
		return ""
	}
	v, ok := scalar(n)
	if !ok {
		s.errorf("%s must be a string", field)
	}
	return v
}

// checkAllowedTools accepts a string or a list of strings; a list joins with spaces.
func (s *Skill) checkAllowedTools(n *yaml.Node) {
	if n == nil {
		return
	}
	v, ok := stringOrList(n, " ")
	if !ok {
		s.errorf("allowed-tools must be a string or a list of strings")
	}
	s.AllowedTools = v
}

func (s *Skill) checkMetadata(n *yaml.Node) {
	if n == nil {
		return
	}
	n = resolve(n)
	if isNull(n) {
		return
	}
	if n.Kind != yaml.MappingNode {
		s.errorf("metadata must be a map of strings")
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key := resolve(n.Content[i]).Value
		v, ok := scalar(n.Content[i+1])
		if !ok {
			s.errorf("metadata %q must be a string", key)
			continue
		}
		s.Metadata = append(s.Metadata, MetadataEntry{Key: key, Value: v})
	}
}

// resolve follows alias nodes to their target.
func resolve(n *yaml.Node) *yaml.Node {
	for n.Kind == yaml.AliasNode && n.Alias != nil {
		n = n.Alias
	}
	return n
}

func isNull(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.Tag == "!!null"
}

// scalar returns the node's value as a string; null reads as "". ok is false for non-scalars.
func scalar(n *yaml.Node) (string, bool) {
	n = resolve(n)
	if n.Kind != yaml.ScalarNode {
		return "", false
	}
	if isNull(n) {
		return "", true
	}
	return n.Value, true
}

// stringOrList reads a scalar, or a sequence of scalars joined with sep. Null reads as "".
func stringOrList(n *yaml.Node, sep string) (string, bool) {
	n = resolve(n)
	if n.Kind != yaml.SequenceNode {
		return scalar(n)
	}
	items := make([]string, 0, len(n.Content))
	for _, c := range n.Content {
		v, ok := scalar(c)
		if !ok {
			return "", false
		}
		items = append(items, v)
	}
	return strings.Join(items, sep), true
}
