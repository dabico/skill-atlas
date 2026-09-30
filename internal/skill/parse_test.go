package skill

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseErrors(t *testing.T) {
	long := func(n int) string { return strings.Repeat("é", n) }
	tests := []struct {
		name    string
		dir     string
		content string
		want    []string
	}{
		{
			name:    "minimal valid",
			dir:     "pdf-processing",
			content: "---\nname: pdf-processing\ndescription: Extract PDF text, fill forms, merge files. Use when handling PDFs.\n---\n",
		},
		{
			name:    "unicode name valid",
			dir:     "données",
			content: "---\nname: données\ndescription: d\n---\n",
		},
		{
			name:    "eszett is lowercase",
			dir:     "straße",
			content: "---\nname: straße\ndescription: d\n---\n",
		},
		{
			name:    "NFKC directory match",
			dir:     "café",
			content: "---\nname: café\ndescription: d\n---\n",
		},
		{
			name:    "uppercase example",
			dir:     "PDF-Tool",
			content: "---\nname: PDF-Tool\ndescription: d\n---\n",
			want:    []string{`name "PDF-Tool" contains uppercase letters`},
		},
		{
			name:    "directory mismatch example",
			dir:     "pdf-tools",
			content: "---\nname: pdf\ndescription: d\n---\n",
			want:    []string{`name "pdf" doesn't match directory "pdf-tools"`},
		},
		{
			name:    "missing frontmatter",
			dir:     "x",
			content: "# hello\n",
			want:    []string{`missing YAML frontmatter (file must start with "---")`},
		},
		{
			name:    "unclosed frontmatter",
			dir:     "x",
			content: "---\nname: x\ndescription: d\n",
			want:    []string{`frontmatter isn't closed with "---"`},
		},
		{
			name:    "empty frontmatter",
			dir:     "x",
			content: "---\n---\n",
			want:    []string{"name is missing", "description is missing"},
		},
		{
			name:    "yaml syntax error",
			dir:     "x",
			content: "---\nname: [unclosed\n---\n",
			want:    []string{"frontmatter isn't valid YAML: line 1: did not find expected ',' or ']'"},
		},
		{
			name:    "list at top level",
			dir:     "x",
			content: "---\n- a\n- b\n---\n",
			want:    []string{"frontmatter must be a YAML mapping"},
		},
		{
			name:    "duplicate keys",
			dir:     "x",
			content: "---\nname: x\nname: x\ndescription: d\n---\n",
			want:    []string{`duplicate field "name"`},
		},
		{
			name:    "unexpected fields sorted",
			dir:     "x",
			content: "---\nname: x\ndescription: d\nmodel: m\nargument-hint: h\n---\n",
			want:    []string{"unexpected fields: argument-hint, model"},
		},
		{
			name:    "name null",
			dir:     "x",
			content: "---\nname: ~\ndescription: d\n---\n",
			want:    []string{"name is empty"},
		},
		{
			name:    "name whitespace",
			dir:     "x",
			content: "---\nname: \"   \"\ndescription: d\n---\n",
			want:    []string{"name is empty"},
		},
		{
			name:    "name not a string",
			dir:     "x",
			content: "---\nname: [a]\ndescription: d\n---\n",
			want:    []string{"name must be a string"},
		},
		{
			name:    "name too long",
			dir:     strings.Repeat("a", 65),
			content: "---\nname: " + strings.Repeat("a", 65) + "\ndescription: d\n---\n",
			want:    []string{`name "` + strings.Repeat("a", 65) + `" is longer than 64 characters (65)`},
		},
		{
			name:    "name bad characters",
			dir:     "a_b",
			content: "---\nname: a_b\ndescription: d\n---\n",
			want:    []string{`name "a_b" contains characters other than letters, digits and hyphens`},
		},
		{
			name:    "name hyphens",
			dir:     "-a--b-",
			content: "---\nname: -a--b-\ndescription: d\n---\n",
			want: []string{
				`name "-a--b-" starts with a hyphen`,
				`name "-a--b-" ends with a hyphen`,
				`name "-a--b-" contains consecutive hyphens`,
			},
		},
		{
			name:    "many errors at once",
			dir:     "dir",
			content: "---\nname: Bad_Name\nextra: 1\ncompatibility: \"\"\n---\n",
			want: []string{
				"unexpected fields: extra",
				`name "Bad_Name" contains uppercase letters`,
				`name "Bad_Name" contains characters other than letters, digits and hyphens`,
				`name "Bad_Name" doesn't match directory "dir"`,
				"description is missing",
				"compatibility is empty",
			},
		},
		{
			name:    "description missing",
			dir:     "x",
			content: "---\nname: x\n---\n",
			want:    []string{"description is missing"},
		},
		{
			name:    "description empty",
			dir:     "x",
			content: "---\nname: x\ndescription: \"  \"\n---\n",
			want:    []string{"description is empty"},
		},
		{
			name:    "description null",
			dir:     "x",
			content: "---\nname: x\ndescription:\n---\n",
			want:    []string{"description is empty"},
		},
		{
			name:    "description 1024 runes ok",
			dir:     "x",
			content: "---\nname: x\ndescription: " + long(1024) + "\n---\n",
		},
		{
			name:    "description 1025 runes",
			dir:     "x",
			content: "---\nname: x\ndescription: " + long(1025) + "\n---\n",
			want:    []string{"description is longer than 1024 characters (1025)"},
		},
		{
			name:    "description not a string",
			dir:     "x",
			content: "---\nname: x\ndescription: {a: b}\n---\n",
			want:    []string{"description must be a string"},
		},
		{
			name:    "compatibility too long",
			dir:     "x",
			content: "---\nname: x\ndescription: d\ncompatibility: " + long(501) + "\n---\n",
			want:    []string{"compatibility is longer than 500 characters (501)"},
		},
		{
			name:    "compatibility not a string",
			dir:     "x",
			content: "---\nname: x\ndescription: d\ncompatibility: [a]\n---\n",
			want:    []string{"compatibility must be a string"},
		},
		{
			name:    "license and allowed-tools not strings",
			dir:     "x",
			content: "---\nname: x\ndescription: d\nlicense: [a]\nallowed-tools: {a: b}\n---\n",
			want:    []string{"license must be a string", "allowed-tools must be a string"},
		},
		{
			name:    "metadata not a map",
			dir:     "x",
			content: "---\nname: x\ndescription: d\nmetadata: [a]\n---\n",
			want:    []string{"metadata must be a map of strings"},
		},
		{
			name:    "metadata nested value",
			dir:     "x",
			content: "---\nname: x\ndescription: d\nmetadata:\n  a: 1\n  b: {c: d}\n---\n",
			want:    []string{`metadata "b" must be a string`},
		},
		{
			name:    "metadata null is absent",
			dir:     "x",
			content: "---\nname: x\ndescription: d\nmetadata:\n---\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse("p/SKILL.md", tt.dir, []byte(tt.content))
			if !reflect.DeepEqual(got.Errors, tt.want) {
				t.Errorf("errors = %q\nwant     %q", got.Errors, tt.want)
			}
			if got.Valid() != (len(tt.want) == 0) {
				t.Errorf("Valid() = %v", got.Valid())
			}
		})
	}
}

func TestParseFields(t *testing.T) {
	content := "---\nname: x\ndescription: |\n  Does things.\nlicense: MIT\ncompatibility: Needs net\nallowed-tools: Bash Read\nmetadata:\n  version: 1.0\n  author: me\n  flag: true\n---\n\n\n# Title\n\nBody text.\n\n"
	got := Parse("a/x/SKILL.md", "x", []byte(content))
	if !got.Valid() {
		t.Fatalf("errors: %q", got.Errors)
	}
	if got.Path != "a/x/SKILL.md" || got.Dir != "x" || got.Name != "x" {
		t.Errorf("identity: %+v", got)
	}
	if got.Description != "Does things." {
		t.Errorf("description = %q", got.Description)
	}
	if got.License != "MIT" || got.Compatibility != "Needs net" || got.AllowedTools != "Bash Read" {
		t.Errorf("fields: %+v", got)
	}
	wantMeta := []MetadataEntry{{"version", "1.0"}, {"author", "me"}, {"flag", "true"}}
	if !reflect.DeepEqual(got.Metadata, wantMeta) {
		t.Errorf("metadata = %v", got.Metadata)
	}
	if got.Body != "# Title\n\nBody text." {
		t.Errorf("body = %q", got.Body)
	}
}

func TestParseLineEndingsAndBOM(t *testing.T) {
	tests := map[string]string{
		"crlf": "---\r\nname: x\r\ndescription: d\r\n---\r\n\r\nbody\r\n",
		"bom":  "\uFEFF---\nname: x\ndescription: d\n---\nbody\n",
		"both": "\uFEFF---\r\nname: x\r\ndescription: d\r\n---  \r\nbody",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			got := Parse("x/SKILL.md", "x", []byte(content))
			if !got.Valid() {
				t.Fatalf("errors: %q", got.Errors)
			}
			if got.Description != "d" || got.Body != "body" {
				t.Errorf("got %+v", got)
			}
		})
	}
}

func TestParseBodyOnFrontmatterErrors(t *testing.T) {
	missing := Parse("x/SKILL.md", "x", []byte("\n# Just text\n"))
	if missing.Body != "# Just text" {
		t.Errorf("missing frontmatter body = %q", missing.Body)
	}
	unclosed := Parse("x/SKILL.md", "x", []byte("---\nname: x\nbody?\n"))
	if unclosed.Body != "" {
		t.Errorf("unclosed body = %q", unclosed.Body)
	}
}

func TestParseAlias(t *testing.T) {
	got := Parse("x/SKILL.md", "x", []byte("---\nname: &n x\ndescription: *n\n---\n"))
	if !got.Valid() || got.Description != "x" {
		t.Errorf("got %+v", got)
	}
}

func TestDisplayName(t *testing.T) {
	if got := (Skill{Dir: "d"}).DisplayName(); got != "d" {
		t.Errorf("got %q", got)
	}
}
