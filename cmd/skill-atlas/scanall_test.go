package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"skill-atlas/internal/repo"
	"skill-atlas/internal/scan"
	"skill-atlas/internal/skill"
)

func sources(n int) []source {
	out := make([]source, n)
	for i := range out {
		out[i] = source{target: repo.Target{Display: fmt.Sprintf("h/o/r%d", i), Name: fmt.Sprintf("r%d", i)}}
	}
	return out
}

func okScan(_, name string, _ scan.Options) (scan.Result, error) {
	return scan.Result{Skills: []skill.Skill{{Name: name}}}, nil
}

func TestScanAllKeepsOrder(t *testing.T) {
	srcs := sources(6)
	// Earlier repositories finish later.
	download := func(_ context.Context, tg repo.Target, _, _ string) (repo.Checkout, error) {
		n := int(tg.Name[1] - '0')
		time.Sleep(time.Duration(6-n) * 10 * time.Millisecond)
		return repo.Checkout{SHA: tg.Name}, nil
	}
	got, err := scanAll(context.Background(), srcs, t.TempDir(), defaultParallel, scan.Options{}, download, okScan, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	for i, g := range got {
		if g.target.Name != srcs[i].target.Name || g.checkout.SHA != g.target.Name || g.res.Skills[0].Name != g.target.Name {
			t.Errorf("result %d = %+v, want repository %s", i, g, srcs[i].target.Name)
		}
	}
}

// The limit follows the parallel argument. The first N downloads wait for each other, so all N are in flight at once
// (a lower limit would never open the gate), and the peak shows whether more than N ever ran.
func TestScanAllLimitsConcurrency(t *testing.T) {
	for _, limit := range []int{1, 2, 4, 7} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			var cur, peak atomic.Int32
			gate := make(chan struct{})
			var open sync.Once
			download := func(_ context.Context, _ repo.Target, _, _ string) (repo.Checkout, error) {
				n := cur.Add(1)
				for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
				}
				if int(n) == limit {
					open.Do(func() { close(gate) })
				}
				select {
				case <-gate:
				case <-time.After(30 * time.Second): // only reached when the limit is too low
					return repo.Checkout{}, errors.New("gate never opened")
				}
				cur.Add(-1)
				return repo.Checkout{}, nil
			}
			got, err := scanAll(context.Background(), sources(limit*2+1), t.TempDir(), limit, scan.Options{}, download, okScan, &bytes.Buffer{})
			if err != nil {
				t.Fatal(err)
			}
			for _, g := range got {
				if g.err != nil {
					t.Fatalf("%s: %v", g.target.Name, g.err)
				}
			}
			if p := int(peak.Load()); p != limit {
				t.Errorf("peak concurrency %d, want %d", p, limit)
			}
		})
	}
}

func TestScanAllSeparatesDirs(t *testing.T) {
	root := t.TempDir()
	var mu sync.Mutex
	dirs := map[string]bool{}
	download := func(_ context.Context, _ repo.Target, _, dir string) (repo.Checkout, error) {
		mu.Lock()
		dirs[dir] = true
		mu.Unlock()
		return repo.Checkout{}, nil
	}
	if _, err := scanAll(context.Background(), sources(10), root, defaultParallel, scan.Options{}, download, okScan, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 10 {
		t.Errorf("%d distinct dirs, want 10", len(dirs))
	}
	for d := range dirs {
		if filepath.Dir(d) != root {
			t.Errorf("dir %q is not directly under %q", d, root)
		}
	}
}

func TestScanAllProgressAndOptions(t *testing.T) {
	srcs := sources(2)
	srcs[1].ref = "v1"
	var progress bytes.Buffer
	var mu sync.Mutex
	var got []scan.Options
	scanDir := func(_, name string, o scan.Options) (scan.Result, error) {
		mu.Lock()
		got = append(got, o)
		mu.Unlock()
		return okScan("", name, o)
	}
	download := func(context.Context, repo.Target, string, string) (repo.Checkout, error) { return repo.Checkout{}, nil }
	opts := scan.Options{Exclude: []string{"docs/"}}
	if _, err := scanAll(context.Background(), srcs, t.TempDir(), defaultParallel, opts, download, scanDir, &progress); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Downloading h/o/r0…\n", "Downloading h/o/r1 @ v1…\n"} {
		if !strings.Contains(progress.String(), want) {
			t.Errorf("progress %q lacks %q", progress.String(), want)
		}
	}
	if len(got) != 2 || len(got[0].Exclude) != 1 || got[0].Exclude[0] != "docs/" || len(got[1].Exclude) != 1 {
		t.Errorf("scan options = %+v, want --exclude passed to every repository", got)
	}
}

// One repository fails while its siblings, already running, finish and return results.
func TestScanAllPartialResults(t *testing.T) {
	srcs := sources(3)
	boom := errors.New("boom")
	failing := make(chan struct{})
	download := func(ctx context.Context, tg repo.Target, _, _ string) (repo.Checkout, error) {
		if tg.Name == "r1" {
			close(failing)
			return repo.Checkout{}, boom
		}
		<-failing // finish only after the failure started
		if err := ctx.Err(); err != nil {
			return repo.Checkout{}, err
		}
		return repo.Checkout{SHA: tg.Name}, nil
	}
	got, err := scanAll(context.Background(), srcs, t.TempDir(), defaultParallel, scan.Options{}, download, okScan, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("err = %v, want nil: a repository failure isn't a scan failure", err)
	}
	if len(got) != 3 {
		t.Fatalf("%d results, want 3", len(got))
	}
	for _, i := range []int{0, 2} {
		if got[i].err != nil || got[i].checkout.SHA != srcs[i].target.Name || len(got[i].res.Skills) != 1 {
			t.Errorf("result %d = %+v, want the finished scan", i, got[i])
		}
	}
	if !errors.Is(got[1].err, boom) || len(got[1].res.Skills) != 0 {
		t.Errorf("result 1 = %+v, want the failure", got[1])
	}
}

// Repositories still queued when another fails are downloaded anyway.
func TestScanAllQueuedReposStillRunAfterFailure(t *testing.T) {
	var started atomic.Int32
	download := func(context.Context, repo.Target, string, string) (repo.Checkout, error) {
		if started.Add(1) == 1 {
			return repo.Checkout{}, errors.New("boom")
		}
		return repo.Checkout{}, nil
	}
	got, err := scanAll(context.Background(), sources(12), t.TempDir(), defaultParallel, scan.Options{}, download, okScan, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if n := started.Load(); n != 12 {
		t.Errorf("%d downloads started, want all 12", n)
	}
	failed := 0
	for _, g := range got {
		if g.err != nil {
			failed++
		}
	}
	if failed != 1 {
		t.Errorf("%d failed, want 1", failed)
	}
}

// Each error stays on its own repository, in command-line order.
func TestScanAllErrorsStayWithTheirRepo(t *testing.T) {
	download := func(_ context.Context, tg repo.Target, _, _ string) (repo.Checkout, error) {
		if tg.Name == "r0" || tg.Name == "r3" {
			return repo.Checkout{}, fmt.Errorf("download %s failed", tg.Name)
		}
		return repo.Checkout{}, nil
	}
	scanDir := func(_, name string, o scan.Options) (scan.Result, error) {
		if name == "r4" {
			return scan.Result{}, errors.New("walk r4 failed")
		}
		return okScan("", name, o)
	}
	got, err := scanAll(context.Background(), sources(6), t.TempDir(), defaultParallel, scan.Options{}, download, scanDir, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"download r0 failed", "", "", "download r3 failed", "walk r4 failed", ""}
	for i, g := range got {
		if g.target.Name != fmt.Sprintf("r%d", i) {
			t.Errorf("result %d is %s, want r%d", i, g.target.Name, i)
		}
		switch {
		case want[i] == "" && g.err != nil:
			t.Errorf("r%d: unexpected error %v", i, g.err)
		case want[i] != "" && (g.err == nil || g.err.Error() != want[i]):
			t.Errorf("r%d: err = %v, want %q", i, g.err, want[i])
		}
	}
}

func TestScanAllPrintsFailures(t *testing.T) {
	srcs := sources(3)
	srcs[1].ref = "v1"
	download := func(_ context.Context, tg repo.Target, _, _ string) (repo.Checkout, error) {
		if tg.Name == "r1" {
			return repo.Checkout{}, errors.New("ref \"v1\" not found\x1b[31m")
		}
		return repo.Checkout{}, nil
	}
	var progress bytes.Buffer
	if _, err := scanAll(context.Background(), srcs, t.TempDir(), defaultParallel, scan.Options{}, download, okScan, &progress); err != nil {
		t.Fatal(err)
	}
	if want := "skill-atlas: h/o/r1 @ v1: ref \"v1\" not found\n"; !strings.Contains(progress.String(), want) {
		t.Errorf("progress %q lacks %q (escape sequences must be stripped)", progress.String(), want)
	}
	if n := strings.Count(progress.String(), "skill-atlas: "); n != 1 {
		t.Errorf("%d failure lines, want 1", n)
	}
}

// With 1 repository scanAll prints nothing itself; the caller reports the error.
func TestScanAllSingleRepoPrintsNoFailure(t *testing.T) {
	download := func(context.Context, repo.Target, string, string) (repo.Checkout, error) {
		return repo.Checkout{}, errors.New("nope")
	}
	var progress bytes.Buffer
	got, err := scanAll(context.Background(), sources(1), t.TempDir(), defaultParallel, scan.Options{}, download, okScan, &progress)
	if err != nil || len(got) != 1 || got[0].err == nil || got[0].err.Error() != "nope" {
		t.Fatalf("got %+v, %v; want the error unchanged on the result", got, err)
	}
	if strings.Contains(progress.String(), "skill-atlas:") {
		t.Errorf("progress %q has a failure line", progress.String())
	}
}

func TestScanAllInterrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var started atomic.Int32
	var progress bytes.Buffer
	download := func(ctx context.Context, _ repo.Target, _, _ string) (repo.Checkout, error) {
		started.Add(1)
		cancel()
		<-ctx.Done()
		return repo.Checkout{}, fmt.Errorf("download x: %w", ctx.Err())
	}
	got, err := scanAll(ctx, sources(6), t.TempDir(), defaultParallel, scan.Options{}, download, okScan, &progress)
	if !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("got %v, %v; want context.Canceled and no results", got, err)
	}
	if code := failure(&bytes.Buffer{}, err); code != exitInterrupted {
		t.Errorf("exit = %d, want %d", code, exitInterrupted)
	}
	if n := started.Load(); n > defaultParallel {
		t.Errorf("%d downloads started, want at most %d", n, defaultParallel)
	}
	if strings.Contains(progress.String(), "skill-atlas:") {
		t.Errorf("progress %q reports cancelled downloads as failures", progress.String())
	}
}
