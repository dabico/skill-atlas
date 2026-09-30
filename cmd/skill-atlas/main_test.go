package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"golang.org/x/term"
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
		{name: "html before url", args: []string{"scan", "--html", u}, want: command{action: actionScan, url: u, html: true}},
		{name: "html after url", args: []string{"scan", u, "--html"}, want: command{action: actionScan, url: u, html: true}},
		{name: "html with ref", args: []string{"scan", "--html", "--ref", "v1", u}, want: command{action: actionScan, url: u, ref: "v1", html: true}},
		{name: "ref then html", args: []string{"scan", "--ref=v1", u, "--html"}, want: command{action: actionScan, url: u, ref: "v1", html: true}},
		{name: "html false", args: []string{"scan", "--html=false", u}, want: command{action: actionScan, url: u}},
		{name: "html needs url", args: []string{"scan", "--html"}, wantErr: "needs a git url", usage: true},
		{name: "html takes no value", args: []string{"scan", "--html=maybe", u}, wantErr: "html", usage: true},
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
			if got != tt.want {
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
		{name: "help", args: []string{"help"}, code: exitOK, wantOut: "skill-atlas scan [--html] [--ref <branch|tag>] <git-url>"},
		{name: "scan -h", args: []string{"scan", "-h"}, code: exitOK, wantOut: "--ref"},
		{name: "help lists html", args: []string{"help"}, code: exitOK, wantOut: "--html"},
		{name: "html bad url", args: []string{"scan", "--html", "http://example.com/a/b.git"}, code: exitFail, wantErrHas: "skill-atlas: "},
		{name: "unknown", args: []string{"x"}, code: exitUsage, wantErrHas: `skill-atlas: unknown command "x"`},
		{name: "no url", args: []string{"scan"}, code: exitUsage, wantErrHas: "Usage:"},
		{name: "two urls", args: []string{"scan", "a", "b"}, code: exitUsage, wantErrHas: "Usage:"},
		{name: "empty ref", args: []string{"scan", "--ref=", "https://x.test/a/b"}, code: exitUsage, wantErrHas: "--ref"},
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
