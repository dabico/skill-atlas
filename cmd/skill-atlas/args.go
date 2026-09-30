package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

const usageText = `Usage:
  skill-atlas scan [--html] [--ref <branch|tag>] <git-url>

Flags:
  --html              open the results as an HTML file in the web browser instead of the TUI
  --ref <branch|tag>  branch or tag to scan (default: remote's default branch)
  -h, --help          show this help
`

type action int

const (
	actionHelp action = iota
	actionScan
)

// command is the result of parsing the command line.
type command struct {
	action action
	url    string
	ref    string
	html   bool
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
	html := fs.Bool("html", false, "")

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
	switch {
	case len(urls) == 0:
		return command{}, &usageError{msg: "scan needs a git url"}
	case len(urls) > 1:
		return command{}, &usageError{msg: "scan takes exactly one git url"}
	}
	return command{action: actionScan, url: urls[0], ref: *ref, html: *html}, nil
}
