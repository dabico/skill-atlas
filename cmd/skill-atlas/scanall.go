package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"

	"skill-atlas/internal/repo"
	"skill-atlas/internal/scan"
)

// maxClones is how many repositories are cloned at once.
const maxClones = 4

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
}

type (
	cloneFunc func(ctx context.Context, t repo.Target, ref, dir string) (repo.Checkout, error)
	scanFunc  func(root, rootName string, opts scan.Options) (scan.Result, error)
)

// scanAll clones and scans srcs, at most maxClones at a time, each in its own subdirectory of root.
// The first failure cancels the rest and is returned. Results keep the order of srcs.
func scanAll(ctx context.Context, srcs []source, root string, opts scan.Options, clone cloneFunc, scanDir scanFunc, progress io.Writer) ([]scanned, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	out := make([]scanned, len(srcs))
	sem := make(chan struct{}, maxClones)
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)
	fail := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
	}

	for i, s := range srcs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				fail(ctx.Err())
				return
			}

			if ctx.Err() != nil { // select may pick the semaphore over a closed Done
				fail(ctx.Err())
				return
			}

			mu.Lock()
			if s.ref != "" {
				fmt.Fprintf(progress, "Cloning %s @ %s…\n", s.target.Display, s.ref)
			} else {
				fmt.Fprintf(progress, "Cloning %s…\n", s.target.Display)
			}
			mu.Unlock()

			dir := filepath.Join(root, fmt.Sprintf("repo-%d", i))
			checkout, err := clone(ctx, s.target, s.ref, dir)
			var res scan.Result
			if err == nil {
				res, err = scanDir(dir, s.target.Name, opts)
			}
			if err != nil {
				// Name the repository unless the error already does.
				if len(srcs) > 1 && ctx.Err() == nil && !strings.Contains(err.Error(), s.target.Display) {
					err = fmt.Errorf("%s: %w", s.target.Display, err)
				}
				fail(err)
				return
			}
			out[i] = scanned{source: s, checkout: checkout, res: res}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}
