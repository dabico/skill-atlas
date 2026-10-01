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

// Network tests against real GitHub. go test -short skips them.

const (
	ideavim    = "https://github.com/JetBrains/ideavim.git"
	ideavimTag = "2.47.1" // annotated tag
	ideavimSHA = "c1ae565cfb98be30ea75e4b351e823846c69c3c8"
	hello      = "https://github.com/githubtraining/hellogitworld.git"
)

func netTarget(t *testing.T, url string) (context.Context, Target) {
	t.Helper()
	if testing.Short() {
		t.Skip("network test")
	}
	tg, err := ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	return ctx, tg
}

func downloadNet(t *testing.T, url, ref string) (Checkout, string, error) {
	t.Helper()
	ctx, tg := netTarget(t, url)
	dir := filepath.Join(t.TempDir(), "repo")
	co, err := Download(ctx, tg, ref, dir)
	return co, dir, err
}

func TestDownloadAnnotatedTag(t *testing.T) {
	co, dir, err := downloadNet(t, ideavim, ideavimTag)
	if err != nil {
		t.Fatal(err)
	}
	if co.Ref != ideavimTag || co.SHA != ideavimSHA {
		t.Errorf("got %+v, want %s @ %s", co, ideavimTag, ideavimSHA)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "changelog", "SKILL.md")); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		t.Error("the download has a .git directory")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "ideavim-") {
			t.Errorf("top-level directory %s wasn't stripped", e.Name())
		}
	}
}

func TestDownloadDefaultBranch(t *testing.T) {
	co, dir, err := downloadNet(t, ideavim, "")
	if err != nil {
		t.Fatal(err)
	}
	if co.Ref != "master" || len(co.SHA) != 40 {
		t.Errorf("got %+v, want master @ a 40-char SHA", co)
	}
	if _, err := os.Stat(filepath.Join(dir, "README.md")); err != nil {
		t.Error(err)
	}
}

// The SSH form is listed and downloaded over HTTPS without an SSH agent.
func TestDownloadSSHForm(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	co, _, err := downloadNet(t, "git@github.com:JetBrains/ideavim.git", ideavimTag)
	if err != nil {
		t.Fatal(err)
	}
	if co.SHA != ideavimSHA {
		t.Errorf("got %+v, want %s", co, ideavimSHA)
	}
}

func TestDownloadUnknownRef(t *testing.T) {
	_, _, err := downloadNet(t, ideavim, "no-such-ref")
	if err == nil || err.Error() != `ref "no-such-ref" not found in github.com/JetBrains/ideavim` {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDownloadRepoNotFound(t *testing.T) {
	_, _, err := downloadNet(t, "https://github.com/JetBrains/no-such-repo-skill-atlas", "")
	want := "authentication failed for github.com/JetBrains/no-such-repo-skill-atlas: the repository may be private or may not exist"
	if err == nil || err.Error() != want {
		t.Errorf("got %v, want %q", err, want)
	}
}

func TestDownloadNetCanceled(t *testing.T) {
	_, tg := netTarget(t, ideavim)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Download(ctx, tg, "", t.TempDir())
	if !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want context.Canceled", err)
	}
}

// The resolve tests need only github.com, not the archive host.

func TestResolveNetAnnotatedTag(t *testing.T) {
	ctx, tg := netTarget(t, ideavim)
	co, err := resolve(ctx, tg, ideavimTag)
	if err != nil {
		t.Fatal(err)
	}
	if co != (Checkout{Ref: ideavimTag, SHA: ideavimSHA}) {
		t.Errorf("got %+v, want %s @ %s (the peeled commit, not the tag object)", co, ideavimTag, ideavimSHA)
	}
}

func TestResolveNet(t *testing.T) {
	tests := []struct {
		ref  string
		want Checkout
	}{
		{"", Checkout{Ref: "master", SHA: "ef7bebf8bdb1919d947afe46ab4b2fb4278039b3"}},
		{"feature_image", Checkout{Ref: "feature_image", SHA: "7c0ffa9d88616972bb84befbec40a2212478149e"}},
		{"RELEASE_1.0", Checkout{Ref: "RELEASE_1.0", SHA: "2a52e96389d02209b451ae1ddf45d645b42d744c"}}, // lightweight tag
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			ctx, tg := netTarget(t, hello)
			co, err := resolve(ctx, tg, tt.ref)
			if err != nil {
				t.Fatal(err)
			}
			if co != tt.want {
				t.Errorf("got %+v, want %+v", co, tt.want)
			}
		})
	}
}
