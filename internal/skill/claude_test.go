package skill

import (
	"reflect"
	"testing"
)

func parseFM(front string) Skill {
	return Parse("x/SKILL.md", "x", []byte("---\nname: x\ndescription: d\n"+front+"---\n"))
}

func claudeExt(key, value string) Extension {
	return Extension{Provider: "Claude Code", Key: key, Value: value}
}

func TestClaudeFieldsValid(t *testing.T) {
	tests := []struct {
		name  string
		front string
		want  Extension
	}{
		{"when_to_use", "when_to_use: Use for PDFs\n", claudeExt("when_to_use", "Use for PDFs")},
		{"argument-hint", "argument-hint: \"[issue-number]\"\n", claudeExt("argument-hint", "[issue-number]")},
		{"arguments string", "arguments: issue branch\n", claudeExt("arguments", "issue branch")},
		{"arguments list", "arguments: [issue, branch]\n", claudeExt("arguments", "issue, branch")},
		{"disable-model-invocation", "disable-model-invocation: true\n", claudeExt("disable-model-invocation", "true")},
		{"user-invocable", "user-invocable: false\n", claudeExt("user-invocable", "false")},
		{"disallowed-tools string", "disallowed-tools: Bash Write\n", claudeExt("disallowed-tools", "Bash Write")},
		{"disallowed-tools list", "disallowed-tools:\n  - Bash\n  - Write\n", claudeExt("disallowed-tools", "Bash, Write")},
		{"model", "model: inherit\n", claudeExt("model", "inherit")},
		{"effort", "effort: xhigh\n", claudeExt("effort", "xhigh")},
		{"context", "context: fork\n", claudeExt("context", "fork")},
		{"agent", "agent: Explore\n", claudeExt("agent", "Explore")},
		{"background", "background: true\n", claudeExt("background", "true")},
		{"hooks", "hooks:\n  PreToolUse:\n    - matcher: Bash\n      type: command\n      command: ./run.sh\n  Stop:\n    - type: command\n      command: ./stop.sh\n    - type: command\n      command: ./stop2.sh\n",
			claudeExt("hooks", "PreToolUse (1), Stop (2)")},
		{"paths string", "paths: \"src/**/*.go\"\n", claudeExt("paths", "src/**/*.go")},
		{"paths list", "paths: [\"a/**\", \"b/**\"]\n", claudeExt("paths", "a/**, b/**")},
		{"shell", "shell: powershell\n", claudeExt("shell", "powershell")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFM(tt.front)
			if !got.Valid() {
				t.Fatalf("errors: %q", got.Errors)
			}
			if want := []Extension{tt.want}; !reflect.DeepEqual(got.Extensions, want) {
				t.Errorf("extensions = %+v\nwant        %+v", got.Extensions, want)
			}
		})
	}
}

func TestClaudeFieldsWrongType(t *testing.T) {
	str := "must be a string"
	list := "must be a string or a list of strings"
	boolean := "must be a boolean"
	tests := []struct {
		name  string
		front string
		want  string
	}{
		{"when_to_use", "when_to_use: [a]\n", "when_to_use " + str},
		{"argument-hint", "argument-hint: {a: b}\n", "argument-hint " + str},
		{"arguments map", "arguments: {a: b}\n", "arguments " + list},
		{"arguments nested list", "arguments: [[a]]\n", "arguments " + list},
		{"disable-model-invocation", "disable-model-invocation: maybe\n", "disable-model-invocation " + boolean},
		{"user-invocable list", "user-invocable: [true]\n", "user-invocable " + boolean},
		{"disallowed-tools", "disallowed-tools: {a: b}\n", "disallowed-tools " + list},
		{"model", "model: [a]\n", "model " + str},
		{"agent", "agent: [a]\n", "agent " + str},
		{"background", "background: 2\n", "background " + boolean},
		{"hooks string", "hooks: nope\n", "hooks must be a mapping"},
		{"hooks list", "hooks: [a]\n", "hooks must be a mapping"},
		{"paths", "paths: {a: b}\n", "paths " + list},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFM(tt.front)
			if want := []string{tt.want}; !reflect.DeepEqual(got.Errors, want) {
				t.Errorf("errors = %q\nwant     %q", got.Errors, want)
			}
			if len(got.Extensions) != 0 {
				t.Errorf("extensions = %+v, want none", got.Extensions)
			}
		})
	}
}

func TestClaudeBooleanForms(t *testing.T) {
	forms := map[string]string{
		"true": "true", "false": "false",
		"yes": "true", "no": "false",
		"on": "true", "off": "false",
		"1": "true", "0": "false",
		"YES": "true", "No": "false", "On": "true", "OFF": "false",
		`"true"`: "true", `"Yes"`: "true", `"0"`: "false",
	}
	for in, want := range forms {
		t.Run(in, func(t *testing.T) {
			got := parseFM("disable-model-invocation: " + in + "\n")
			if !got.Valid() {
				t.Fatalf("errors: %q", got.Errors)
			}
			if !reflect.DeepEqual(got.Extensions, []Extension{claudeExt("disable-model-invocation", want)}) {
				t.Errorf("extensions = %+v", got.Extensions)
			}
		})
	}
}

func TestClaudeEnumRejections(t *testing.T) {
	tests := []struct {
		name  string
		front string
		want  string
	}{
		{"effort unknown", "effort: extreme\n", "effort must be one of: low, medium, high, xhigh, max"},
		{"effort case", "effort: High\n", "effort must be one of: low, medium, high, xhigh, max"},
		{"effort list", "effort: [low]\n", "effort must be one of: low, medium, high, xhigh, max"},
		{"context unknown", "context: inline\n", "context must be one of: fork"},
		{"shell unknown", "shell: zsh\n", "shell must be one of: bash, powershell"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFM(tt.front)
			if want := []string{tt.want}; !reflect.DeepEqual(got.Errors, want) {
				t.Errorf("errors = %q\nwant     %q", got.Errors, want)
			}
		})
	}
}

func TestClaudeNullIsAbsent(t *testing.T) {
	got := parseFM("model:\nhooks: ~\ndisable-model-invocation:\n")
	if !got.Valid() || len(got.Extensions) != 0 {
		t.Errorf("errors %q, extensions %+v", got.Errors, got.Extensions)
	}
}

func TestAllowedToolsList(t *testing.T) {
	got := parseFM("allowed-tools:\n  - Read\n  - Bash(git *)\n")
	if !got.Valid() || got.AllowedTools != "Read Bash(git *)" {
		t.Errorf("errors %q, allowed-tools %q", got.Errors, got.AllowedTools)
	}
	str := parseFM("allowed-tools: Read Grep\n")
	if !str.Valid() || str.AllowedTools != "Read Grep" {
		t.Errorf("errors %q, allowed-tools %q", str.Errors, str.AllowedTools)
	}
	bad := parseFM("allowed-tools: [[a]]\n")
	if want := []string{"allowed-tools must be a string or a list of strings"}; !reflect.DeepEqual(bad.Errors, want) {
		t.Errorf("errors = %q", bad.Errors)
	}
}

func TestUnknownFieldsStayInvalid(t *testing.T) {
	got := parseFM("type: workflow\nmodel: opus\n")
	if want := []string{"unexpected fields: type"}; !reflect.DeepEqual(got.Errors, want) {
		t.Errorf("errors = %q", got.Errors)
	}
	if !reflect.DeepEqual(got.Extensions, []Extension{claudeExt("model", "opus")}) {
		t.Errorf("extensions = %+v", got.Extensions)
	}
}

func TestSpecAndClaudeFieldsMixed(t *testing.T) {
	content := "---\nname: x\nargument-hint: \"[file]\"\ndescription: d\nlicense: MIT\ndisable-model-invocation: yes\nallowed-tools: Read\nmetadata:\n  a: b\neffort: low\n---\nbody\n"
	got := Parse("x/SKILL.md", "x", []byte(content))
	if !got.Valid() {
		t.Fatalf("errors: %q", got.Errors)
	}
	want := []Extension{
		claudeExt("argument-hint", "[file]"),
		claudeExt("disable-model-invocation", "true"),
		claudeExt("effort", "low"),
	}
	if !reflect.DeepEqual(got.Extensions, want) {
		t.Errorf("extensions = %+v\nwant        %+v", got.Extensions, want)
	}
	if got.License != "MIT" || got.AllowedTools != "Read" || len(got.Metadata) != 1 {
		t.Errorf("spec fields: %+v", got)
	}
}

func TestClaudeErrorsWithOtherErrors(t *testing.T) {
	got := Parse("x/SKILL.md", "x", []byte("---\nname: X\ndescription: d\neffort: no\nfoo: 1\n---\n"))
	want := []string{
		"unexpected fields: foo",
		`name "X" contains uppercase letters`,
		`name "X" doesn't match directory "x"`,
		"effort must be one of: low, medium, high, xhigh, max",
	}
	if !reflect.DeepEqual(got.Errors, want) {
		t.Errorf("errors = %q\nwant     %q", got.Errors, want)
	}
}
