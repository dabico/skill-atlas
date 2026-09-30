package skill

import (
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

const providerClaude = "Claude Code"

type fieldKind int

const (
	kindString fieldKind = iota
	kindStringOrList
	kindBool
	kindEnum
	kindMapping
)

// fieldRule is the type check for one provider field.
type fieldRule struct {
	key  string
	kind fieldKind
	enum []string // kindEnum only
}

// claudeFields lists the Claude Code frontmatter fields (https://code.claude.com/docs/en/skills).
var claudeFields = []fieldRule{
	{key: "when_to_use", kind: kindString},
	{key: "argument-hint", kind: kindString},
	{key: "arguments", kind: kindStringOrList},
	{key: "disable-model-invocation", kind: kindBool},
	{key: "user-invocable", kind: kindBool},
	{key: "disallowed-tools", kind: kindStringOrList},
	{key: "model", kind: kindString},
	{key: "effort", kind: kindEnum, enum: []string{"low", "medium", "high", "xhigh", "max"}},
	{key: "context", kind: kindEnum, enum: []string{"fork"}},
	{key: "agent", kind: kindString},
	{key: "background", kind: kindBool},
	{key: "hooks", kind: kindMapping},
	{key: "paths", kind: kindStringOrList},
	{key: "shell", kind: kindEnum, enum: []string{"bash", "powershell"}},
}

// claudeRule returns the rule for key, or nil when Claude Code defines no such field.
func claudeRule(key string) *fieldRule {
	for i := range claudeFields {
		if claudeFields[i].key == key {
			return &claudeFields[i]
		}
	}
	return nil
}

// checkClaude validates the Claude Code fields and records them in file order.
func (s *Skill) checkClaude(fields map[string]*yaml.Node, order []string) {
	for _, key := range order {
		r := claudeRule(key)
		if r == nil {
			continue
		}
		v, present, ok := r.read(fields[key])
		if !ok {
			s.errorf("%s %s", key, r.want())
			continue
		}
		if present {
			s.Extensions = append(s.Extensions, Extension{Provider: providerClaude, Key: key, Value: v})
		}
	}
}

// read returns the display value. present is false for a null field; ok is false on a type mismatch.
func (r fieldRule) read(n *yaml.Node) (v string, present, ok bool) {
	n = resolve(n)
	if isNull(n) {
		return "", false, true
	}
	switch r.kind {
	case kindString:
		v, ok = scalar(n)
	case kindStringOrList:
		v, ok = stringOrList(n, ", ")
	case kindBool:
		v, ok = parseBool(n)
	case kindEnum:
		v, ok = scalar(n)
		ok = ok && slices.Contains(r.enum, v)
	case kindMapping:
		v, ok = hooksSummary(n)
	}
	return v, ok, ok
}

func (r fieldRule) want() string {
	switch r.kind {
	case kindString:
		return "must be a string"
	case kindStringOrList:
		return "must be a string or a list of strings"
	case kindBool:
		return "must be a boolean"
	case kindEnum:
		return "must be one of: " + strings.Join(r.enum, ", ")
	default:
		return "must be a mapping"
	}
}

// parseBool accepts true/false and the yes/no/on/off/1/0 forms Claude Code reads, case-insensitively.
func parseBool(n *yaml.Node) (string, bool) {
	if n.Kind != yaml.ScalarNode {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(n.Value)) {
	case "true", "yes", "on", "1":
		return "true", true
	case "false", "no", "off", "0":
		return "false", true
	}
	return "", false
}

// hooksSummary lists each hook event with its handler count, e.g. "PreToolUse (2), Stop (1)".
func hooksSummary(n *yaml.Node) (string, bool) {
	if n.Kind != yaml.MappingNode {
		return "", false
	}
	var parts []string
	for i := 0; i+1 < len(n.Content); i += 2 {
		event := resolve(n.Content[i]).Value
		count := 1
		if seq := resolve(n.Content[i+1]); seq.Kind == yaml.SequenceNode {
			count = len(seq.Content)
		}
		parts = append(parts, event+" ("+strconv.Itoa(count)+")")
	}
	return strings.Join(parts, ", "), true
}
