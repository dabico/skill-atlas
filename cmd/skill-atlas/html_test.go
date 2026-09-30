package main

import (
	"bytes"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skill-atlas/internal/htmlreport"
	"skill-atlas/internal/repo"
	"skill-atlas/internal/scan"
	"skill-atlas/internal/skill"
)

var (
	testTarget   = repo.Target{Display: "github.com/o/r"}
	testCheckout = repo.Checkout{Ref: "main", SHA: "0123456789abcdef"}
	testResult   = scan.Result{Skills: []skill.Skill{{Path: "a/SKILL.md", Dir: "a", Name: "a", Description: "d"}}, Excluded: 2}
)

func useTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	t.Setenv("TMP", dir)
	t.Setenv("TEMP", dir)
	return dir
}

func TestShowHTMLOpensFile(t *testing.T) {
	dir := useTempDir(t)
	var opened string
	var errb bytes.Buffer
	err := showHTML(testTarget, testCheckout, testResult, func(u string) error { opened = u; return nil }, &errb)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSuffix(errb.String(), "\n")
	path, ok := strings.CutPrefix(line, "Report: ")
	if !ok || strings.Contains(line, "\n") {
		t.Fatalf("stderr = %q, want one Report line", errb.String())
	}
	if !filepath.IsAbs(path) || !strings.HasPrefix(filepath.Base(path), htmlreport.FilePrefix) {
		t.Errorf("report path %q", path)
	}
	page, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(page), "github.com/o/r") {
		t.Errorf("report file unreadable or lacks the repo (err %v)", err)
	}
	if !strings.Contains(string(page), "1 skill, 0 invalid, 2 excluded") {
		t.Error("report lacks the excluded count from the scan result")
	}
	u, err := url.Parse(opened)
	if err != nil || u.Scheme != "file" || u.Path != filepath.ToSlash(path) {
		t.Errorf("opened %q, want the file URL of %q", opened, path)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("temp dir holds %d entries, want only the report", len(entries))
	}
}

func TestShowHTMLOpenFailureWarns(t *testing.T) {
	useTempDir(t)
	var errb bytes.Buffer
	err := showHTML(testTarget, testCheckout, testResult, func(string) error { return errors.New("boom") }, &errb)
	if err != nil {
		t.Fatalf("a failed launch returned %v, want nil", err)
	}
	out := errb.String()
	path := strings.TrimPrefix(strings.SplitN(out, "\n", 2)[0], "Report: ")
	for _, want := range []string{"couldn't open a browser: boom", "Open the report yourself: " + path} {
		if !strings.Contains(out, want) {
			t.Errorf("stderr %q lacks %q", out, want)
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("report file missing: %v", err)
	}
}

func TestShowHTMLWriteFailure(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "missing")
	t.Setenv("TMPDIR", bad)
	t.Setenv("TMP", bad)
	t.Setenv("TEMP", bad)
	called := false
	var errb bytes.Buffer
	err := showHTML(testTarget, testCheckout, testResult, func(string) error { called = true; return nil }, &errb)
	if err == nil {
		t.Fatal("showHTML succeeded without a writable temp dir")
	}
	if code := failure(&errb, err); code != exitFail {
		t.Errorf("exit = %d, want %d", code, exitFail)
	}
	if called {
		t.Error("the browser was launched without a report file")
	}
}
