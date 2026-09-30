package main

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	const u = "https://github.com/org/repo.git"
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
		{name: "url only", args: []string{"scan", u}, want: command{action: actionScan, url: u}},
		{name: "flag before url", args: []string{"scan", "--ref", "v1", u}, want: command{action: actionScan, url: u, ref: "v1"}},
		{name: "flag after url", args: []string{"scan", u, "--ref", "v1"}, want: command{action: actionScan, url: u, ref: "v1"}},
		{name: "ref equals", args: []string{"scan", "--ref=x", u}, want: command{action: actionScan, url: u, ref: "x"}},
		{name: "ref equals after url", args: []string{"scan", u, "--ref=x"}, want: command{action: actionScan, url: u, ref: "x"}},
		{name: "missing url", args: []string{"scan"}, wantErr: "needs a git url", usage: true},
		{name: "missing url with ref", args: []string{"scan", "--ref", "x"}, wantErr: "needs a git url", usage: true},
		{name: "extra args", args: []string{"scan", u, "other"}, wantErr: "exactly one", usage: true},
		{name: "unknown command", args: []string{"x"}, wantErr: `unknown command "x"`, usage: true},
		{name: "empty ref", args: []string{"scan", "--ref", "", u}, wantErr: "--ref", usage: true},
		{name: "empty ref equals", args: []string{"scan", u, "--ref="}, wantErr: "--ref", usage: true},
		{name: "unknown flag", args: []string{"scan", "--nope", u}, wantErr: "nope", usage: true},

		{name: "exclude", args: []string{"scan", "--exclude", "docs/", u},
			want: command{action: actionScan, url: u, exclude: []string{"docs/"}}},
		{name: "exclude equals", args: []string{"scan", "--exclude=docs/", u},
			want: command{action: actionScan, url: u, exclude: []string{"docs/"}}},
		{name: "exclude after url", args: []string{"scan", u, "--exclude", "a"},
			want: command{action: actionScan, url: u, exclude: []string{"a"}}},
		{name: "exclude repeated keeps order", args: []string{"scan", "--exclude", "a", u, "--exclude=b", "--exclude", "!c"},
			want: command{action: actionScan, url: u, exclude: []string{"a", "b", "!c"}}},
		{name: "exclude glob", args: []string{"scan", "--exclude", "**/fixtures/*", u},
			want: command{action: actionScan, url: u, exclude: []string{"**/fixtures/*"}}},
		{name: "include tests", args: []string{"scan", "--include-tests", u},
			want: command{action: actionScan, url: u, includeTests: true}},
		{name: "include tests after url", args: []string{"scan", u, "--include-tests"},
			want: command{action: actionScan, url: u, includeTests: true}},
		{name: "include tests false", args: []string{"scan", "--include-tests=false", u},
			want: command{action: actionScan, url: u}},
		{name: "all flags", args: []string{"scan", "--ref=v1", "--include-tests", "--exclude", "x", u},
			want: command{action: actionScan, url: u, ref: "v1", includeTests: true, exclude: []string{"x"}}},
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
		wantErrHas string
	}{
		{name: "no args", code: exitUsage, wantErrHas: "Usage:"},
		{name: "help", args: []string{"help"}, code: exitOK, wantOut: "skill-atlas scan [--ref <branch|tag>] [--exclude <pattern>]... [--include-tests] <git-url>"},
		{name: "scan -h", args: []string{"scan", "-h"}, code: exitOK, wantOut: "--ref"},
		{name: "help lists exclude", args: []string{"help"}, code: exitOK, wantOut: "--exclude <pattern>"},
		{name: "help lists include-tests", args: []string{"help"}, code: exitOK, wantOut: "--include-tests"},
		{name: "unknown", args: []string{"x"}, code: exitUsage, wantErrHas: `skill-atlas: unknown command "x"`},
		{name: "no url", args: []string{"scan"}, code: exitUsage, wantErrHas: "Usage:"},
		{name: "two urls", args: []string{"scan", "a", "b"}, code: exitUsage, wantErrHas: "Usage:"},
		{name: "empty ref", args: []string{"scan", "--ref=", "https://x.test/a/b"}, code: exitUsage, wantErrHas: "--ref"},
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
