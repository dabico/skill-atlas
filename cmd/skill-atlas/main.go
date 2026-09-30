package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"

	"skill-atlas/internal/htmlreport"
	"skill-atlas/internal/repo"
	"skill-atlas/internal/scan"
	"skill-atlas/internal/tui"
)

const (
	exitOK          = 0
	exitFail        = 1
	exitUsage       = 2
	exitInterrupted = 130
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	cmd, err := parseArgs(args)
	if err != nil {
		var ue *usageError
		if errors.As(err, &ue) && ue.msg != "" {
			fmt.Fprintf(stderr, "skill-atlas: %s\n", ue.msg)
		}
		fmt.Fprint(stderr, usageText)
		return exitUsage
	}
	if cmd.action == actionHelp {
		fmt.Fprint(stdout, usageText)
		return exitOK
	}
	return runScan(cmd, stderr)
}

func runScan(cmd command, stderr io.Writer) int {
	srcs := make([]source, len(cmd.repos))
	for i, r := range cmd.repos {
		target, err := repo.ParseURL(r.url)
		if err != nil {
			if len(cmd.repos) > 1 { // name the bad argument
				err = fmt.Errorf("%q: %w", r.url, err)
			}
			fmt.Fprintf(stderr, "skill-atlas: %v\n", err)
			return exitFail
		}
		srcs[i] = source{target: target, ref: r.ref}
	}
	if msg := duplicate(srcs); msg != "" {
		fmt.Fprintf(stderr, "skill-atlas: %s\n%s", msg, usageText)
		return exitUsage
	}
	if !cmd.html && (!term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd()))) {
		fmt.Fprintln(stderr, "skill-atlas: scan needs an interactive terminal")
		return exitFail
	}

	dir, err := os.MkdirTemp("", "skill-atlas-*")
	if err != nil {
		fmt.Fprintf(stderr, "skill-atlas: %v\n", err)
		return exitFail
	}
	defer os.RemoveAll(dir)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	results, err := scanAll(ctx, srcs, dir, scan.Options{Exclude: cmd.exclude}, repo.Clone, scan.Dir, stderr)
	if err != nil {
		return failure(stderr, err)
	}

	// Results are in memory; drop the clones and release Ctrl+C.
	os.RemoveAll(dir)
	stop()
	if cmd.html {
		if err := showHTML(results, htmlreport.OpenBrowser, stderr); err != nil {
			return failure(stderr, err)
		}
		return exitOK
	}

	report := tui.Report{Repos: make([]tui.Repo, len(results))}
	for i, r := range results {
		report.Repos[i] = tui.Repo{
			Name:     r.target.Display,
			Ref:      r.checkout.Ref,
			SHA:      r.checkout.SHA,
			Skills:   r.res.Skills,
			Excluded: r.res.Excluded,
		}
	}
	if err := tui.Run(report); err != nil {
		return failure(stderr, err)
	}
	return exitOK
}

// duplicate returns an error message for the first repository given twice with the same ref, or "".
func duplicate(srcs []source) string {
	seen := map[dupKey]bool{}
	for _, s := range srcs {
		k := dupKey{s.target.Display, s.ref}
		if seen[k] {
			if s.ref != "" {
				return fmt.Sprintf("%s @ %s given twice", s.target.Display, s.ref)
			}
			return fmt.Sprintf("%s given twice", s.target.Display)
		}
		seen[k] = true
	}
	return ""
}

type dupKey struct{ display, ref string }

func failure(stderr io.Writer, err error) int {
	if errors.Is(err, context.Canceled) {
		return exitInterrupted
	}
	fmt.Fprintf(stderr, "skill-atlas: %v\n", err)
	return exitFail
}
