package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skill-atlas/internal/repo"
	"skill-atlas/internal/scan"
	"skill-atlas/internal/skill"
)

// fakeDeps downloads every repository except those whose name is in bad.
func fakeDeps(bad ...string) (deps, *[]string) {
	var opened []string
	d := deps{
		download: func(_ context.Context, tg repo.Target, _, _ string, _ func(error)) (repo.Checkout, error) {
			for _, b := range bad {
				if tg.Name == b {
					return repo.Checkout{}, errors.New("authentication failed for " + tg.Display)
				}
			}
			return repo.Checkout{Ref: "main", SHA: "0123456789abcdef"}, nil
		},
		scanDir: func(_, name string, _ scan.Options) (scan.Result, error) {
			return scan.Result{Skills: []skill.Skill{{Path: name + "/SKILL.md", Dir: name, Name: name, Description: "d"}}}, nil
		},
		open: func(u string) error { opened = append(opened, u); return nil },
	}
	return d, &opened
}

func reports(t *testing.T, dir string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(dir, "skill-atlas-report-*.html"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestPartialFailureHTMLWritesReportAndExits1(t *testing.T) {
	dir := useTempDir(t)
	d, opened := fakeDeps("bad")
	var out, errb bytes.Buffer
	code := runWith(d, []string{"scan", "--html", "https://github.com/o/good.git", "https://github.com/o/bad.git#v1"}, &out, &errb)
	if code != exitFail {
		t.Fatalf("exit = %d, want %d; stderr %q", code, exitFail, errb.String())
	}
	if want := "skill-atlas: github.com/o/bad @ v1: authentication failed for github.com/o/bad\n"; !strings.Contains(errb.String(), want) {
		t.Errorf("stderr %q lacks %q", errb.String(), want)
	}
	files := reports(t, dir)
	if len(files) != 1 {
		t.Fatalf("%d report files, want 1", len(files))
	}
	page, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"github.com/o/good", "github.com/o/bad @ v1", "1 skill, 0 invalid, 1 failed", "authentication failed for github.com/o/bad"} {
		if !strings.Contains(string(page), want) {
			t.Errorf("report lacks %q", want)
		}
	}
	// The page lists repositories by name, not in command-line order.
	if strings.Index(string(page), "github.com/o/bad") > strings.Index(string(page), "github.com/o/good") {
		t.Error("repositories are not in name order")
	}
	if len(*opened) != 1 {
		t.Errorf("browser opened %d times, want 1", len(*opened))
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("temp dir holds %d entries, want only the report", len(entries))
	}
}

func TestAllFailedExits1WithoutReport(t *testing.T) {
	dir := useTempDir(t)
	d, opened := fakeDeps("a", "b")
	var errb bytes.Buffer
	code := runWith(d, []string{"scan", "--html", "https://github.com/o/a.git", "https://github.com/o/b.git"}, &bytes.Buffer{}, &errb)
	if code != exitFail {
		t.Fatalf("exit = %d, want %d", code, exitFail)
	}
	for _, want := range []string{"skill-atlas: github.com/o/a: ", "skill-atlas: github.com/o/b: "} {
		if !strings.Contains(errb.String(), want) {
			t.Errorf("stderr %q lacks %q", errb.String(), want)
		}
	}
	if strings.Contains(errb.String(), "Report:") || len(*opened) != 0 {
		t.Errorf("a report was written or opened (stderr %q)", errb.String())
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("temp dir holds %d entries, want none", len(entries))
	}
}

func TestSingleRepoFailureMessageUnchanged(t *testing.T) {
	useTempDir(t)
	d, _ := fakeDeps("only")
	var errb bytes.Buffer
	code := runWith(d, []string{"scan", "--html", "https://github.com/o/only.git#v1"}, &bytes.Buffer{}, &errb)
	if want := "skill-atlas: authentication failed for github.com/o/only\n"; code != exitFail || strings.Contains(errb.String(), "@ v1:") || !strings.Contains(errb.String(), want) {
		t.Errorf("exit %d, stderr %q; want %q with no repository prefix", code, errb.String(), want)
	}
}

// An unusable archive cache is reported once per run, however many repositories warn.
func TestCacheWarningOnce(t *testing.T) {
	useTempDir(t)
	d, _ := fakeDeps()
	download := d.download
	d.download = func(ctx context.Context, tg repo.Target, ref, dir string, warn func(error)) (repo.Checkout, error) {
		warn(errors.New("mkdir /nope: read-only file system"))
		return download(ctx, tg, ref, dir, warn)
	}
	var errb bytes.Buffer
	code := runWith(d, []string{"scan", "--html", "https://github.com/o/a", "https://github.com/o/b", "https://github.com/o/c"}, &bytes.Buffer{}, &errb)
	if code != exitOK {
		t.Fatalf("exit = %d, want %d; stderr %q", code, exitOK, errb.String())
	}
	const want = "skill-atlas: warning: archive cache unavailable: mkdir /nope: read-only file system\n"
	if n := strings.Count(errb.String(), want); n != 1 {
		t.Errorf("stderr has the warning %d times, want 1:\n%s", n, errb.String())
	}
}
