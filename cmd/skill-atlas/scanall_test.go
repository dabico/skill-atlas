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
	clone := func(_ context.Context, tg repo.Target, _, _ string) (repo.Checkout, error) {
		n := int(tg.Name[1] - '0')
		time.Sleep(time.Duration(6-n) * 10 * time.Millisecond)
		return repo.Checkout{SHA: tg.Name}, nil
	}
	got, err := scanAll(context.Background(), srcs, t.TempDir(), scan.Options{}, clone, okScan, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	for i, g := range got {
		if g.target.Name != srcs[i].target.Name || g.checkout.SHA != g.target.Name || g.res.Skills[0].Name != g.target.Name {
			t.Errorf("result %d = %+v, want repository %s", i, g, srcs[i].target.Name)
		}
	}
}

func TestScanAllLimitsConcurrencyAndSeparatesDirs(t *testing.T) {
	root := t.TempDir()
	var cur, peak atomic.Int32
	var mu sync.Mutex
	dirs := map[string]bool{}
	clone := func(_ context.Context, _ repo.Target, _, dir string) (repo.Checkout, error) {
		n := cur.Add(1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		time.Sleep(20 * time.Millisecond)
		cur.Add(-1)
		mu.Lock()
		dirs[dir] = true
		mu.Unlock()
		return repo.Checkout{}, nil
	}
	if _, err := scanAll(context.Background(), sources(10), root, scan.Options{}, clone, okScan, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if p := peak.Load(); p != maxClones {
		t.Errorf("peak concurrency %d, want %d", p, maxClones)
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
	clone := func(context.Context, repo.Target, string, string) (repo.Checkout, error) { return repo.Checkout{}, nil }
	opts := scan.Options{Exclude: []string{"docs/"}}
	if _, err := scanAll(context.Background(), srcs, t.TempDir(), opts, clone, scanDir, &progress); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Cloning h/o/r0…\n", "Cloning h/o/r1 @ v1…\n"} {
		if !strings.Contains(progress.String(), want) {
			t.Errorf("progress %q lacks %q", progress.String(), want)
		}
	}
	if len(got) != 2 || len(got[0].Exclude) != 1 || got[0].Exclude[0] != "docs/" || len(got[1].Exclude) != 1 {
		t.Errorf("scan options = %+v, want --exclude passed to every repository", got)
	}
}

func TestScanAllFailFast(t *testing.T) {
	srcs := sources(3)
	boom := errors.New("boom")
	var canceled atomic.Int32
	var running sync.WaitGroup
	running.Add(2)
	clone := func(ctx context.Context, tg repo.Target, _, _ string) (repo.Checkout, error) {
		if tg.Name == "r1" {
			running.Wait() // fail only once both siblings are cloning
			return repo.Checkout{}, boom
		}
		running.Done()
		select {
		case <-ctx.Done():
			canceled.Add(1)
			return repo.Checkout{}, ctx.Err()
		case <-time.After(5 * time.Second):
			return repo.Checkout{}, nil
		}
	}
	start := time.Now()
	got, err := scanAll(context.Background(), srcs, t.TempDir(), scan.Options{}, clone, okScan, &bytes.Buffer{})
	if got != nil || !errors.Is(err, boom) {
		t.Fatalf("got %v, %v; want the first failure", got, err)
	}
	if !strings.HasPrefix(err.Error(), "h/o/r1: ") {
		t.Errorf("error %q doesn't name the repository", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Error("a cancelled sibling replaced the real failure")
	}
	if time.Since(start) > 2*time.Second || canceled.Load() != 2 {
		t.Errorf("others weren't cancelled (took %v, %d cancelled)", time.Since(start), canceled.Load())
	}
}

func TestScanAllQueuedReposSkippedAfterFailure(t *testing.T) {
	var started atomic.Int32
	clone := func(ctx context.Context, _ repo.Target, _, _ string) (repo.Checkout, error) {
		if started.Add(1) == 1 {
			return repo.Checkout{}, errors.New("boom")
		}
		<-ctx.Done()
		return repo.Checkout{}, ctx.Err()
	}
	if _, err := scanAll(context.Background(), sources(12), t.TempDir(), scan.Options{}, clone, okScan, &bytes.Buffer{}); err == nil {
		t.Fatal("no error")
	}
	if n := started.Load(); n > maxClones {
		t.Errorf("%d clones started, want the queue to stop after the failure", n)
	}
}

func TestScanAllScanFailureNamesRepo(t *testing.T) {
	clone := func(context.Context, repo.Target, string, string) (repo.Checkout, error) { return repo.Checkout{}, nil }
	bad := func(_, name string, _ scan.Options) (scan.Result, error) {
		if name == "r1" {
			return scan.Result{}, errors.New("walk failed")
		}
		return scan.Result{}, nil
	}
	_, err := scanAll(context.Background(), sources(2), t.TempDir(), scan.Options{}, clone, bad, &bytes.Buffer{})
	if err == nil || err.Error() != "h/o/r1: walk failed" {
		t.Errorf("err = %v", err)
	}
}

func TestScanAllErrorNaming(t *testing.T) {
	named := func(_ context.Context, tg repo.Target, _, _ string) (repo.Checkout, error) {
		return repo.Checkout{}, fmt.Errorf("repository %s not found", tg.Display)
	}
	_, err := scanAll(context.Background(), sources(2), t.TempDir(), scan.Options{}, named, okScan, &bytes.Buffer{})
	if err == nil || !strings.HasPrefix(err.Error(), "repository h/o/r") {
		t.Errorf("an error that names the repository was wrapped: %v", err)
	}
	// A single repository keeps today's message.
	plain := func(context.Context, repo.Target, string, string) (repo.Checkout, error) {
		return repo.Checkout{}, errors.New("nope")
	}
	_, err = scanAll(context.Background(), sources(1), t.TempDir(), scan.Options{}, plain, okScan, &bytes.Buffer{})
	if err == nil || err.Error() != "nope" {
		t.Errorf("single-repo error = %v, want it unchanged", err)
	}
}

func TestScanAllInterrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	clone := func(ctx context.Context, _ repo.Target, _, _ string) (repo.Checkout, error) {
		cancel()
		<-ctx.Done()
		return repo.Checkout{}, fmt.Errorf("clone x: %w", ctx.Err())
	}
	_, err := scanAll(ctx, sources(6), t.TempDir(), scan.Options{}, clone, okScan, &bytes.Buffer{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if code := failure(&bytes.Buffer{}, err); code != exitInterrupted {
		t.Errorf("exit = %d, want %d", code, exitInterrupted)
	}
}
