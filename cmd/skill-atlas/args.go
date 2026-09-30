package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"skill-atlas/internal/scan"
)

const usageText = `Usage:
  skill-atlas scan [--ref <branch|tag>] [--exclude <pattern>]... <git-url>

Flags:
  --ref <branch|tag>   branch or tag to scan (default: remote's default branch)
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
	url     string
	ref     string
	exclude []string // --exclude patterns, in order
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
	ref := fs.String("ref", "", "")
	var exclude stringList
	fs.Var(&exclude, "exclude", "")

	var urls []string
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
		urls = append(urls, fs.Arg(0))
		args = fs.Args()[1:]
	}

	refSet := false
	fs.Visit(func(f *flag.Flag) { refSet = refSet || f.Name == "ref" })
	if refSet && *ref == "" {
		return command{}, &usageError{msg: "--ref needs a branch or tag name"}
	}
	for _, p := range exclude {
		if err := scan.CheckPattern(p); err != nil {
			return command{}, &usageError{msg: fmt.Sprintf("--exclude %q: %v", p, err)}
		}
	}
	switch {
	case len(urls) == 0:
		return command{}, &usageError{msg: "scan needs a git url"}
	case len(urls) > 1:
		return command{}, &usageError{msg: "scan takes exactly one git url"}
	}
	return command{action: actionScan, url: urls[0], ref: *ref, exclude: exclude}, nil
}
