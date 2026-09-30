package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/term"
)

func one(u, ref string) []repoArg { return []repoArg{{url: u, ref: ref}} }

func TestParseArgs(t *testing.T) {
	const u = "https://github.com/org/repo.git"
	const v = "https://github.com/org/other.git"
	tests := []struct {
		name    string
		args    []string
		want    command
		wantErr string // substring; "" means no error
		usage   bool
	}{
		{name: "no args", args: nil, usage: true},
		{name: "help word", args: []string{"help"}, want: command{action: actionHelp}},
		{name: "short help", args: []string{"-h"}, want: command{action: actionHelp}},
		{name: "long help", args: []string{"--help"}, want: command{action: actionHelp}},
		{name: "scan -h", args: []string{"scan", "-h"}, want: command{action: actionHelp}},
		{name: "scan --help", args: []string{"scan", "--help"}, want: command{action: actionHelp}},
		{name: "url only", args: []string{"scan", u}, want: command{action: actionScan, repos: one(u, "")}},
		{name: "html before url", args: []string{"scan", "--html", u}, want: command{action: actionScan, repos: one(u, ""), html: true}},
		{name: "html after url", args: []string{"scan", u, "--html"}, want: command{action: actionScan, repos: one(u, ""), html: true}},
		{name: "html false", args: []string{"scan", "--html=false", u}, want: command{action: actionScan, repos: one(u, "")}},
		{name: "html needs url", args: []string{"scan", "--html"}, wantErr: "needs a git url", usage: true},
		{name: "html takes no value", args: []string{"scan", "--html=maybe", u}, wantErr: "html", usage: true},
		{name: "missing url", args: []string{"scan"}, wantErr: "needs a git url", usage: true},
		{name: "two urls", args: []string{"scan", u, v}, want: command{action: actionScan, repos: []repoArg{{url: u}, {url: v}}}},
		{name: "flags between urls", args: []string{"scan", "--html", u, "--exclude", "a", v},
			want: command{action: actionScan, repos: []repoArg{{url: u}, {url: v}}, html: true, exclude: []string{"a"}}},
		{name: "hash ref", args: []string{"scan", u + "#v1"}, want: command{action: actionScan, repos: one(u, "v1")}},
		{name: "hash ref splits on first hash", args: []string{"scan", u + "#zoom@1.0.1#x"}, want: command{action: actionScan, repos: one(u, "zoom@1.0.1#x")}},
		{name: "same repo two refs", args: []string{"scan", u + "#a", u + "#b"},
			want: command{action: actionScan, repos: []repoArg{{url: u, ref: "a"}, {url: u, ref: "b"}}}},
		{name: "empty hash ref", args: []string{"scan", u + "#"}, wantErr: "missing branch or tag after #", usage: true},
		{name: "empty hash ref among valid", args: []string{"scan", u + "#a", v + "#"}, wantErr: "missing branch or tag after #", usage: true},
		{name: "unknown command", args: []string{"x"}, wantErr: `unknown command "x"`, usage: true},
		{name: "unknown flag", args: []string{"scan", "--nope", u}, wantErr: "nope", usage: true},
		{name: "--ref is unknown", args: []string{"scan", "--ref", "v1", u}, wantErr: "ref", usage: true},
		{name: "--ref= is unknown", args: []string{"scan", "--ref=v1", u}, wantErr: "ref", usage: true},

		{name: "exclude", args: []string{"scan", "--exclude", "docs/", u},
			want: command{action: actionScan, repos: one(u, ""), exclude: []string{"docs/"}}},
		{name: "exclude equals", args: []string{"scan", "--exclude=docs/", u},
			want: command{action: actionScan, repos: one(u, ""), exclude: []string{"docs/"}}},
		{name: "exclude after url", args: []string{"scan", u, "--exclude", "a"},
			want: command{action: actionScan, repos: one(u, ""), exclude: []string{"a"}}},
		{name: "exclude repeated keeps order", args: []string{"scan", "--exclude", "a", u, "--exclude=b", "--exclude", "!c"},
			want: command{action: actionScan, repos: one(u, ""), exclude: []string{"a", "b", "!c"}}},
		{name: "exclude glob", args: []string{"scan", "--exclude", "**/fixtures/*", u},
			want: command{action: actionScan, repos: one(u, ""), exclude: []string{"**/fixtures/*"}}},
		{name: "include-tests is unknown", args: []string{"scan", "--include-tests", u}, wantErr: "include-tests", usage: true},
		{name: "empty exclude", args: []string{"scan", "--exclude", "", u}, wantErr: "--exclude", usage: true},
		{name: "empty exclude equals", args: []string{"scan", u, "--exclude="}, wantErr: "--exclude", usage: true},
		{name: "blank exclude", args: []string{"scan", "--exclude", "  ", u}, wantErr: "pattern is empty", usage: true},
		{name: "empty exclude among valid", args: []string{"scan", "--exclude", "a", "--exclude=", u}, wantErr: "--exclude", usage: true},
		{name: "exclude needs value", args: []string{"scan", u, "--exclude"}, wantErr: "exclude", usage: true},
		{name: "bad glob", args: []string{"scan", "--exclude", "[", u}, wantErr: "malformed", usage: true},
		{name: "partial doublestar", args: []string{"scan", "--exclude", "a**b", u}, wantErr: "whole path segment", usage: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArgs(tt.args)
			if tt.usage {
				var ue *usageError
				if !errors.As(err, &ue) {
					t.Fatalf("err = %v, want usageError", err)
				}
				if !strings.Contains(ue.msg, tt.wantErr) {
					t.Fatalf("msg = %q, want substring %q", ue.msg, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRunExitCodes(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		code       int
		wantOut    string
		wantNotOut string
		wantErrHas string
	}{
		{name: "no args", code: exitUsage, wantErrHas: "Usage:"},
		{name: "help", args: []string{"help"}, code: exitOK, wantOut: "skill-atlas scan [--html] [--exclude <pattern>]... <git-url>[#<ref>]..."},
		{name: "help says the default ref", args: []string{"help"}, code: exitOK, wantOut: "#<ref> picks a branch or tag"},
		{name: "help has no --ref", args: []string{"help"}, code: exitOK, wantNotOut: "--ref"},
		{name: "--ref is unknown", args: []string{"scan", "--ref", "v1", "https://x.test/a/b"}, code: exitUsage, wantErrHas: "flag provided but not defined: -ref"},
		{name: "--ref= is unknown", args: []string{"scan", "--ref=v1", "https://x.test/a/b"}, code: exitUsage, wantErrHas: "flag provided but not defined: -ref"},
		{name: "duplicate with ref", args: []string{"scan", "--html", "https://x.test/a/b#v1", "https://x.test/a/b#v1"}, code: exitUsage, wantErrHas: "x.test/a/b @ v1 given twice"},
		{name: "help lists html", args: []string{"help"}, code: exitOK, wantOut: "--html"},
		{name: "help lists exclude", args: []string{"help"}, code: exitOK, wantOut: "--exclude <pattern>"},
		{name: "html bad url", args: []string{"scan", "--html", "http://example.com/a/b.git"}, code: exitFail, wantErrHas: "skill-atlas: "},
		{name: "unknown", args: []string{"x"}, code: exitUsage, wantErrHas: `skill-atlas: unknown command "x"`},
		{name: "no url", args: []string{"scan"}, code: exitUsage, wantErrHas: "Usage:"},
		{name: "two bad urls", args: []string{"scan", "a", "b"}, code: exitFail, wantErrHas: "skill-atlas: "},
		{name: "empty hash ref", args: []string{"scan", "https://x.test/a/b#"}, code: exitUsage, wantErrHas: "missing branch or tag after #"},
		{name: "duplicate", args: []string{"scan", "--html", "https://x.test/a/b", "https://x.test/a/b.git"}, code: exitUsage, wantErrHas: "x.test/a/b given twice"},
		{name: "duplicate prints usage", args: []string{"scan", "--html", "https://x.test/a/b", "https://x.test/a/b"}, code: exitUsage, wantErrHas: "Usage:"},
		{name: "bad url among several", args: []string{"scan", "--html", "https://x.test/a/b", "http://example.com/a/b.git"}, code: exitFail, wantErrHas: `skill-atlas: "http://example.com/a/b.git": `},
		{name: "empty exclude", args: []string{"scan", "--exclude=", "https://x.test/a/b"}, code: exitUsage, wantErrHas: "--exclude"},
		{name: "bad exclude glob", args: []string{"scan", "--exclude", "[", "https://x.test/a/b"}, code: exitUsage, wantErrHas: "malformed"},
		{name: "bad url", args: []string{"scan", "http://example.com/a/b.git"}, code: exitFail, wantErrHas: "skill-atlas: "},
		{name: "local path", args: []string{"scan", "/tmp/some/repo"}, code: exitFail, wantErrHas: "skill-atlas: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			if got := run(tt.args, &out, &errb); got != tt.code {
				t.Fatalf("exit = %d, want %d (stderr %q)", got, tt.code, errb.String())
			}
			if tt.wantNotOut != "" && strings.Contains(out.String(), tt.wantNotOut) {
				t.Errorf("stdout = %q, want no %q", out.String(), tt.wantNotOut)
			}
			if !strings.Contains(out.String(), tt.wantOut) {
				t.Errorf("stdout = %q, want substring %q", out.String(), tt.wantOut)
			}
			if !strings.Contains(errb.String(), tt.wantErrHas) {
				t.Errorf("stderr = %q, want substring %q", errb.String(), tt.wantErrHas)
			}
			if tt.code == exitUsage && out.Len() != 0 {
				t.Errorf("usage error wrote to stdout: %q", out.String())
			}
		})
	}
}

// TestRunTerminalCheck checks that only the TUI needs a terminal; --html gets as far as the clone.
func TestRunTerminalCheck(t *testing.T) {
	if term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) {
		t.Skip("stdin and stdout are terminals")
	}
	const u = "https://127.0.0.1:1/org/repo.git" // refused at once, no network needed
	var errb bytes.Buffer
	if got := run([]string{"scan", u}, io.Discard, &errb); got != exitFail || !strings.Contains(errb.String(), "interactive terminal") {
		t.Errorf("TUI mode: exit %d, stderr %q, want the terminal error", got, errb.String())
	}
	errb.Reset()
	if got := run([]string{"scan", "--html", u}, io.Discard, &errb); got != exitFail || strings.Contains(errb.String(), "interactive terminal") {
		t.Errorf("--html mode: exit %d, stderr %q, want a clone failure", got, errb.String())
	}
}
