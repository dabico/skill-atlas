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

// deps are the parts of a scan that tests replace.
type deps struct {
	download downloadFunc
	listOrg  listFunc
	scanDir  scanFunc
	open     func(url string) error
	showTUI  func(tui.Report) error
}

var realDeps = deps{download: repo.Download, listOrg: repo.ListOrg, scanDir: scan.Dir, open: htmlreport.OpenBrowser, showTUI: tui.Run}

func run(args []string, stdout, stderr io.Writer) int {
	return runWith(realDeps, args, stdout, stderr)
}

func runWith(d deps, args []string, stdout, stderr io.Writer) int {
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
	return runScan(d, cmd, stderr)
}

func runScan(d deps, cmd command, stderr io.Writer) int {
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
	srcs, msg := checkOrgs(srcs)
	if msg == "" {
		msg = duplicate(srcs)
	}
	if msg != "" {
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

	entries, err := expandOrgs(ctx, srcs, d.listOrg, stderr)
	if err != nil {
		return failure(stderr, err)
	}
	// Failed organizations aren't downloaded; the rest go through scanAll and back to their place.
	var todo []source
	var at []int
	for i, e := range entries {
		if e.err == nil {
			todo = append(todo, e.source)
			at = append(at, i)
		}
	}
	done, err := scanAll(ctx, todo, dir, cmd.parallel, scan.Options{Exclude: cmd.exclude}, d.download, d.scanDir, stderr)
	if err != nil {
		return failure(stderr, err)
	}
	for j, r := range done {
		entries[at[j]] = r
	}
	results := dropEmpty(entries)

	failed := 0
	for _, r := range results {
		if r.err == nil {
			continue
		}
		failed++
		if len(results) > 1 && !r.printed {
			printFailure(stderr, r)
		}
	}
	if failed == len(results) {
		// With several results each failure is printed above or as it happened.
		if len(results) == 1 && !results[0].printed {
			return failure(stderr, results[0].err)
		}
		return exitFail
	}

	// Results are in memory; drop the downloads and release Ctrl+C.
	os.RemoveAll(dir)
	stop()
	// Some repositories failed: show the rest, then exit 1.
	code := exitOK
	if failed > 0 {
		code = exitFail
	}
	if cmd.html {
		if err := showHTML(results, d.open, stderr); err != nil {
			return failure(stderr, err)
		}
		return code
	}

	report := tui.Report{Repos: make([]tui.Repo, len(results))}
	for i, r := range results {
		report.Repos[i] = tui.Repo{
			Name:     r.target.Display,
			Ref:      r.shownRef(),
			SHA:      r.checkout.SHA,
			Skills:   r.res.Skills,
			Excluded: r.res.Excluded,
		}
		if r.err != nil {
			report.Repos[i].Err = r.err.Error()
		}
	}
	if err := d.showTUI(report); err != nil {
		return failure(stderr, err)
	}
	return code
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
