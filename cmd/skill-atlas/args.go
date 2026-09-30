package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"skill-atlas/internal/scan"
)

const usageText = `Usage:
  skill-atlas scan [--html] [--exclude <pattern>]... <git-url>[#<ref>]...

Arguments:
  <git-url>[#<ref>]    repository to scan; #<ref> picks a branch or tag (default: remote's default branch)

Flags:
  --html              open the results as an HTML file in the web browser instead of the TUI
  --exclude <pattern>  skip SKILL.md files matching a gitignore-style pattern (repeatable)
  -h, --help           show this help
`

type action int

const (
	actionHelp action = iota
	actionScan
)

// command is the result of parsing the command line.
type command struct {
	action  action
	repos   []repoArg // in command-line order
	html    bool
	exclude []string // --exclude patterns, in order
}

// repoArg is one <git-url>[#<ref>] argument; ref is empty for the remote's default branch.
type repoArg struct {
	url string
	ref string
}

// usageError is a bad command line; msg may be empty when only usage is shown.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func parseArgs(args []string) (command, error) {
	if len(args) == 0 {
		return command{}, &usageError{}
	}
	switch args[0] {
	case "help", "-h", "--help", "-help":
		return command{action: actionHelp}, nil
	case "scan":
		return parseScan(args[1:])
	default:
		return command{}, &usageError{msg: fmt.Sprintf("unknown command %q", args[0])}
	}
}

func parseScan(args []string) (command, error) {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	html := fs.Bool("html", false, "")
	var exclude stringList
	fs.Var(&exclude, "exclude", "")

	var rawURLs []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return command{action: actionHelp}, nil
			}
			return command{}, &usageError{msg: err.Error()}
		}
		if fs.NArg() == 0 {
			break
		}
		rawURLs = append(rawURLs, fs.Arg(0))
		args = fs.Args()[1:]
	}

	for _, p := range exclude {
		if err := scan.CheckPattern(p); err != nil {
			return command{}, &usageError{msg: fmt.Sprintf("--exclude %q: %v", p, err)}
		}
	}
	if len(rawURLs) == 0 {
		return command{}, &usageError{msg: "scan needs a git url"}
	}
	repos := make([]repoArg, len(rawURLs))
	for i, a := range rawURLs {
		r := repoArg{url: a}
		if u, refPart, ok := strings.Cut(a, "#"); ok {
			if refPart == "" {
				return command{}, &usageError{msg: fmt.Sprintf("%q: missing branch or tag after #", a)}
			}
			r = repoArg{url: u, ref: refPart}
		}
		repos[i] = r
	}
	return command{action: actionScan, repos: repos, html: *html, exclude: exclude}, nil
}
