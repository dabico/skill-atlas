package repo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	hello     = "https://github.com/githubtraining/hellogitworld.git"
	helloHead = "ef7bebf8bdb1919d947afe46ab4b2fb4278039b3"
)

func cloneNet(t *testing.T, url, ref string) (Checkout, string, error) {
	t.Helper()
	if testing.Short() {
		t.Skip("network test")
	}
	tg, err := ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dir := t.TempDir()
	co, err := Clone(ctx, tg, ref, dir)
	return co, dir, err
}

func checkSHA(t *testing.T, got string) {
	t.Helper()
	if len(got) != 40 {
		t.Errorf("SHA %q isn't 40 chars", got)
	}
}

func TestCloneDefaultBranch(t *testing.T) {
	co, dir, err := cloneNet(t, hello, "")
	if err != nil {
		t.Fatal(err)
	}
	if co.Ref != "master" || co.SHA != helloHead {
		t.Errorf("got %+v, want master @ %s", co, helloHead)
	}
	if _, err := os.Stat(filepath.Join(dir, "README.txt")); err != nil {
		t.Error(err)
	}
}

func TestCloneBranch(t *testing.T) {
	co, dir, err := cloneNet(t, hello, "feature_image")
	if err != nil {
		t.Fatal(err)
	}
	want := "7c0ffa9d88616972bb84befbec40a2212478149e"
	if co.Ref != "feature_image" || co.SHA != want {
		t.Errorf("got %+v, want feature_image @ %s", co, want)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Error(err)
	}
}

func TestCloneLightweightTag(t *testing.T) {
	co, dir, err := cloneNet(t, hello, "RELEASE_1.0")
	if err != nil {
		t.Fatal(err)
	}
	want := "2a52e96389d02209b451ae1ddf45d645b42d744c"
	if co.Ref != "RELEASE_1.0" || co.SHA != want {
		t.Errorf("got %+v, want RELEASE_1.0 @ %s", co, want)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) < 2 {
		t.Errorf("checkout looks empty: %v", entries)
	}
}

func TestCloneAnnotatedTag(t *testing.T) {
	co, dir, err := cloneNet(t, "https://github.com/sindresorhus/is-online", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	// Tag object is eab3b0f...; the commit is the peeled one.
	want := "4035842281adf73106caf50c515ed2d150687875"
	if co.Ref != "v1.0.0" || co.SHA != want {
		t.Errorf("got %+v, want v1.0.0 @ %s", co, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err != nil {
		t.Error(err)
	}
}

func TestCloneUnknownRef(t *testing.T) {
	_, _, err := cloneNet(t, hello, "no-such-ref")
	if err == nil || !strings.Contains(err.Error(), `ref "no-such-ref" not found in github.com/githubtraining/hellogitworld`) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCloneRepoNotFound(t *testing.T) {
	_, _, err := cloneNet(t, "https://github.com/githubtraining/no-such-repo-skill-atlas", "")
	if err == nil {
		t.Fatal("expected error")
	}
	t.Log(err)
	if !strings.Contains(err.Error(), "authentication failed for github.com/githubtraining/no-such-repo-skill-atlas") &&
		!strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
	if strings.Contains(err.Error(), "https://") {
		t.Errorf("error should use the display form: %v", err)
	}
}

func TestCloneCanceled(t *testing.T) {
	if testing.Short() {
		t.Skip("network test")
	}
	tg, _ := ParseURL(hello)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Clone(ctx, tg, "", t.TempDir())
	if !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want context.Canceled", err)
	}
}

func TestCloneSSH(t *testing.T) {
	if testing.Short() {
		t.Skip("network test")
	}
	if os.Getenv("SSH_AUTH_SOCK") == "" {
		t.Skip("SSH_AUTH_SOCK isn't set")
	}
	tg, _ := ParseURL("git@github.com:githubtraining/hellogitworld.git")
	co, err := Clone(context.Background(), tg, "", t.TempDir())
	if err != nil {
		t.Skipf("SSH clone isn't usable here: %v", err)
	}
	if co.Ref != "master" {
		t.Errorf("got %+v", co)
	}
	checkSHA(t, co.SHA)
}

func TestCloneSSHNoAgent(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	tg, _ := ParseURL("git@github.com:org/repo.git")
	_, err := Clone(context.Background(), tg, "", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "SSH_AUTH_SOCK") {
		t.Errorf("unexpected error: %v", err)
	}
}
