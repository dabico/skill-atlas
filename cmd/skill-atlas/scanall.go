package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sync"

	"github.com/charmbracelet/x/ansi"

	"skill-atlas/internal/repo"
	"skill-atlas/internal/scan"
)

// defaultParallel is how many repositories are cloned at once unless --parallel says otherwise.
const defaultParallel = 4

// source is one repository to scan; ref is empty for the remote's default branch.
type source struct {
	target repo.Target
	ref    string
}

// scanned is the result for one source.
type scanned struct {
	source
	checkout repo.Checkout
	res      scan.Result
	err      error // why the clone or scan failed; checkout and res may be empty
}

type (
	cloneFunc func(ctx context.Context, t repo.Target, ref, dir string) (repo.Checkout, error)
	scanFunc  func(root, rootName string, opts scan.Options) (scan.Result, error)
)

// shownRef is the resolved ref, or the requested one when the clone failed.
func (s scanned) shownRef() string {
	if s.checkout.Ref != "" {
		return s.checkout.Ref
	}
	return s.ref
}

// label is "display[ @ ref]", the name of a repository in messages.
func (s source) label() string {
	if s.ref != "" {
		return s.target.Display + " @ " + s.ref
	}
	return s.target.Display
}

// scanAll clones and scans srcs, at most parallel at a time, each in its own subdirectory of root.
// A failing repository is recorded in its result and doesn't stop the others. With several
// repositories each failure is printed to progress as it happens. Results keep the order of srcs.
// The error is non-nil only when ctx is cancelled.
func scanAll(ctx context.Context, srcs []source, root string, parallel int, opts scan.Options, clone cloneFunc, scanDir scanFunc, progress io.Writer) ([]scanned, error) {
	out := make([]scanned, len(srcs))
	sem := make(chan struct{}, parallel)
	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

	for i, s := range srcs {
		out[i].source = s
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			if ctx.Err() != nil { // select may pick the semaphore over a closed Done
				return
			}

			mu.Lock()
			fmt.Fprintf(progress, "Cloning %s…\n", s.label())
			mu.Unlock()

			dir := filepath.Join(root, fmt.Sprintf("repo-%d", i))
			checkout, err := clone(ctx, s.target, s.ref, dir)
			if err == nil {
				out[i].checkout = checkout
				out[i].res, err = scanDir(dir, s.target.Name, opts)
			}
			if err != nil {
				out[i].err = err
				if len(srcs) > 1 && ctx.Err() == nil {
					mu.Lock()
					fmt.Fprintf(progress, "skill-atlas: %s: %s\n", s.label(), ansi.Strip(err.Error()))
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
