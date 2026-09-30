package main

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"skill-atlas/internal/htmlreport"
	"skill-atlas/internal/repo"
	"skill-atlas/internal/skill"
)

// TestShowHTMLExitCodes checks how the two ways the wait can end map to exit codes.
func TestShowHTMLExitCodes(t *testing.T) {
	noop, err := exec.LookPath("true")
	if err != nil {
		t.Skip("no true(1) to stand in for a browser")
	}
	t.Setenv("BROWSER", noop)
	old := htmlreport.LoadTimeout
	htmlreport.LoadTimeout = 50 * time.Millisecond
	t.Cleanup(func() { htmlreport.LoadTimeout = old })

	target := repo.Target{Display: "github.com/o/r"}
	checkout := repo.Checkout{Ref: "main", SHA: "0123456789abcdef"}
	skills := []skill.Skill{{Path: "a/SKILL.md", Dir: "a", Name: "a", Description: "d"}}

	t.Run("timeout", func(t *testing.T) {
		var errb bytes.Buffer
		err := showHTML(context.Background(), target, checkout, skills, &errb)
		if code := failure(&errb, err); code != exitFail {
			t.Errorf("exit = %d, want %d", code, exitFail)
		}
		if want := "skill-atlas: the browser didn't load the report within 50ms\n"; !strings.HasSuffix(errb.String(), want) {
			t.Errorf("stderr = %q, want suffix %q", errb.String(), want)
		}
		if !strings.Contains(errb.String(), "Report: http://127.0.0.1:") {
			t.Errorf("stderr = %q lacks the report url", errb.String())
		}
	})

	t.Run("interrupt", func(t *testing.T) {
		htmlreport.LoadTimeout = time.Minute
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var errb bytes.Buffer
		err := showHTML(ctx, target, checkout, skills, &errb)
		if code := failure(&errb, err); code != exitInterrupted {
			t.Errorf("exit = %d, want %d (stderr %q)", code, exitInterrupted, errb.String())
		}
	})
}
