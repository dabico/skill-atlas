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
	target, err := repo.ParseURL(cmd.url)
	if err != nil {
		fmt.Fprintf(stderr, "skill-atlas: %v\n", err)
		return exitFail
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

	if cmd.ref != "" {
		fmt.Fprintf(stderr, "Cloning %s @ %s…\n", target.Display, cmd.ref)
	} else {
		fmt.Fprintf(stderr, "Cloning %s…\n", target.Display)
	}

	checkout, err := repo.Clone(ctx, target, cmd.ref, dir)
	if err != nil {
		return failure(stderr, err)
	}
	res, err := scan.Dir(dir, target.Name, scan.Options{Exclude: cmd.exclude})
	if err != nil {
		return failure(stderr, err)
	}

	// Results are in memory; drop the clone and release Ctrl+C.
	os.RemoveAll(dir)
	stop()
	if cmd.html {
		if err := showHTML(target, checkout, res.Skills, htmlreport.OpenBrowser, stderr); err != nil {
			return failure(stderr, err)
		}
		return exitOK
	}

	err = tui.Run(tui.Report{
		Repo:     target.Display,
		Ref:      checkout.Ref,
		SHA:      checkout.SHA,
		Skills:   res.Skills,
		Excluded: res.Excluded,
	})
	if err != nil {
		return failure(stderr, err)
	}
	return exitOK
}

func failure(stderr io.Writer, err error) int {
	if errors.Is(err, context.Canceled) {
		return exitInterrupted
	}
	fmt.Fprintf(stderr, "skill-atlas: %v\n", err)
	return exitFail
}
