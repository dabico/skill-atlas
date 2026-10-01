package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"

	"skill-atlas/internal/repo"
)

// orgDeps is fakeDeps plus a fake organization listing. orgs maps a lowercase organization name
// to its repository names; an organization that isn't in orgs fails to list with "boom".
// Repositories named empty* are empty. The returned slice records the downloads in order.
func orgDeps(t *testing.T, orgs map[string][]string, bad ...string) (deps, *[]string) {
	t.Helper()
	d, _ := fakeDeps(bad...)
	var (
		mu         sync.Mutex
		downloaded []string
	)
	download := d.download
	d.download = func(ctx context.Context, tg repo.Target, ref, dir string, warn func(error)) (repo.Checkout, error) {
		mu.Lock()
		downloaded = append(downloaded, tg.Display)
		mu.Unlock()
		if strings.HasPrefix(tg.Name, "empty") {
			return repo.Checkout{}, fmt.Errorf("repository %s is %w", tg.Display, repo.ErrEmptyRepository)
		}
		return download(ctx, tg, ref, dir, warn)
	}
	d.listOrg = func(_ context.Context, org repo.Target) ([]repo.Target, error) {
		if !org.Org {
			t.Errorf("listOrg got repository %s", org.Display)
		}
		names, ok := orgs[strings.ToLower(org.Owner)]
		if !ok {
			return nil, errors.New("boom")
		}
		var out []repo.Target
		for _, n := range names {
			tg, err := repo.ParseURL("https://github.com/" + n)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, tg)
		}
		return out, nil
	}
	return d, &downloaded
}

// htmlScan runs "scan --html args..." with d and returns the exit code, stderr and the report ("" if none).
func htmlScan(t *testing.T, d deps, args ...string) (int, string, string) {
	t.Helper()
	dir := useTempDir(t)
	var errb bytes.Buffer
	code := runWith(d, append([]string{"scan", "--html"}, args...), &bytes.Buffer{}, &errb)
	files := reports(t, dir)
	if len(files) == 0 {
		return code, errb.String(), ""
	}
	page, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	return code, errb.String(), string(page)
}

// inOrder fails unless each of want appears in s after the one before.
func inOrder(t *testing.T, what, s string, want ...string) {
	t.Helper()
	at := 0
	for _, w := range want {
		i := strings.Index(s[at:], w)
		if i < 0 {
			t.Errorf("%s lacks %q after offset %d:\n%s", what, w, at, s)
			return
		}
		at += i + len(w)
	}
}

func TestOrgExpandsInPlace(t *testing.T) {
	d, downloaded := orgDeps(t, map[string][]string{"acme": {"Acme/a", "Acme/b"}})
	code, stderr, page := htmlScan(t, d, "https://github.com/x/one", "https://github.com/acme/", "https://github.com/y/two")
	if code != exitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	// Downloads run in parallel, so only the listing has a fixed place: before all of them.
	listed := strings.Index(stderr, "Listing repositories in github.com/acme…\n")
	first := strings.Index(stderr, "Downloading ")
	if listed < 0 || first < listed {
		t.Errorf("stderr doesn't list before downloading: %q", stderr)
	}
	for _, r := range []string{"github.com/x/one", "github.com/Acme/a", "github.com/Acme/b", "github.com/y/two"} {
		if !strings.Contains(stderr, "Downloading "+r+"…\n") {
			t.Errorf("stderr lacks the download of %s: %q", r, stderr)
		}
	}
	if len(*downloaded) != 4 {
		t.Errorf("downloads %q, want 4", *downloaded)
	}
	inOrder(t, "report", page, "<h1>4 repositories</h1>", "github.com/x/one", "github.com/Acme/a", "github.com/Acme/b", "github.com/y/two")
}

func TestOrgCoversItsRepos(t *testing.T) {
	for _, args := range [][]string{
		{"https://github.com/acme/r#v1", "https://github.com/Acme", "git@github.com:ACME/other.git"},
		{"https://github.com/ACME", "https://github.com/acme/r", "https://github.com/acme/r#v1"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			d, downloaded := orgDeps(t, map[string][]string{"acme": {"Acme/a"}})
			code, stderr, page := htmlScan(t, d, args...)
			if code != exitOK {
				t.Fatalf("exit %d, stderr %q", code, stderr)
			}
			if strings.Join(*downloaded, " ") != "github.com/Acme/a" {
				t.Errorf("downloads %q, want only the listed repository", *downloaded)
			}
			if strings.Contains(page, "acme/r") || strings.Contains(page, "other") || strings.Contains(stderr, "given twice") {
				t.Errorf("a covered repository shows up; stderr %q", stderr)
			}
		})
	}
}

func TestOrgUsageErrors(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"https://github.com/JetBrains#v1"}, "skill-atlas: github.com/JetBrains is an organization, #v1 isn't supported\n"},
		{[]string{"https://github.com/o/r", "https://github.com/JetBrains/#main"}, "skill-atlas: github.com/JetBrains is an organization, #main isn't supported\n"},
		{[]string{"https://github.com/acme", "https://github.com/o/r", "https://github.com/ACME/"}, "skill-atlas: github.com/ACME given twice\n"},
		// A covered repository given twice isn't a duplicate: it is dropped first. The org itself still counts.
		{[]string{"https://github.com/acme", "https://github.com/acme"}, "github.com/acme given twice"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			d, downloaded := orgDeps(t, map[string][]string{"acme": {"acme/a"}, "jetbrains": {"JetBrains/a"}})
			var out, errb bytes.Buffer
			code := runWith(d, append([]string{"scan", "--html"}, tt.args...), &out, &errb)
			if code != exitUsage || !strings.Contains(errb.String(), "Usage:") {
				t.Errorf("exit %d, stderr %q; want usage error", code, errb.String())
			}
			if !strings.Contains(errb.String(), tt.want) {
				t.Errorf("stderr %q lacks %q", errb.String(), tt.want)
			}
			if strings.Contains(errb.String(), "Listing") || len(*downloaded) != 0 || out.Len() != 0 {
				t.Errorf("the scan started: stderr %q", errb.String())
			}
		})
	}
}

func TestOrgListingFailureIsAnEntry(t *testing.T) {
	d, downloaded := orgDeps(t, map[string][]string{"acme": {"Acme/a"}})
	code, stderr, page := htmlScan(t, d, "https://github.com/Bad", "https://github.com/o/good", "https://github.com/acme")
	if code != exitFail {
		t.Fatalf("exit %d, want %d; stderr %q", code, exitFail, stderr)
	}
	inOrder(t, "stderr", stderr,
		"Listing repositories in github.com/Bad…\n",
		"skill-atlas: github.com/Bad: boom\n",
		"Listing repositories in github.com/acme…\n",
		"Downloading ")
	if strings.Contains(stderr, "Downloading github.com/Bad") || strings.Count(stderr, "boom") != 1 {
		t.Errorf("stderr %q", stderr)
	}
	if strings.Join(*downloaded, " ") != "github.com/o/good github.com/Acme/a" && strings.Join(*downloaded, " ") != "github.com/Acme/a github.com/o/good" {
		t.Errorf("downloads %q", *downloaded)
	}
	inOrder(t, "report", page, "<h1>3 repositories</h1>", "2 skills, 0 invalid, 1 failed", "github.com/Bad", "boom", "github.com/o/good", "github.com/Acme/a")
	if regexp.MustCompile(`github\.com/Bad @`).MatchString(page) {
		t.Error("the failed organization shows a ref")
	}
}

func TestSingleOrgListingFailure(t *testing.T) {
	d, _ := orgDeps(t, nil)
	code, stderr, page := htmlScan(t, d, "https://github.com/Bad")
	if want := "Listing repositories in github.com/Bad…\nskill-atlas: boom\n"; code != exitFail || stderr != want {
		t.Errorf("exit %d, stderr %q; want %d, %q", code, stderr, exitFail, want)
	}
	if page != "" {
		t.Error("a report was written")
	}
}

func TestOrgEmptyReposAreLeftOut(t *testing.T) {
	d, downloaded := orgDeps(t, map[string][]string{"acme": {"Acme/a", "Acme/empty", "Acme/b"}})
	code, stderr, page := htmlScan(t, d, "https://github.com/acme")
	if code != exitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if len(*downloaded) != 3 {
		t.Errorf("downloads %q, want all 3", *downloaded)
	}
	if strings.Contains(stderr, "is empty") || strings.Contains(stderr, "skill-atlas:") {
		t.Errorf("stderr mentions the empty repository: %q", stderr)
	}
	if strings.Contains(page, "Acme/empty") || !strings.Contains(page, "<h1>2 repositories</h1>") {
		t.Error("the report shows the empty repository")
	}
}

// An empty repository given on the command line still fails.
func TestExplicitEmptyRepoFails(t *testing.T) {
	d, _ := orgDeps(t, nil)
	code, stderr, _ := htmlScan(t, d, "https://github.com/o/empty")
	if want := "skill-atlas: repository github.com/o/empty is empty\n"; code != exitFail || !strings.HasSuffix(stderr, want) {
		t.Errorf("exit %d, stderr %q; want %q", code, stderr, want)
	}
}

func TestOrgAllEmpty(t *testing.T) {
	orgs := map[string][]string{"acme": {"Acme/empty1", "Acme/empty2"}}

	d, _ := orgDeps(t, orgs)
	code, stderr, page := htmlScan(t, d, "https://github.com/acme")
	if want := "skill-atlas: no repositories found in github.com/acme\n"; code != exitFail || !strings.HasSuffix(stderr, want) || strings.Contains(stderr, "is empty") {
		t.Errorf("alone: exit %d, stderr %q; want %q", code, stderr, want)
	}
	if page != "" {
		t.Error("alone: a report was written")
	}

	d, _ = orgDeps(t, orgs)
	code, stderr, page = htmlScan(t, d, "https://github.com/acme", "https://github.com/o/good")
	if want := "skill-atlas: github.com/acme: no repositories found in github.com/acme\n"; code != exitFail || strings.Count(stderr, want) != 1 {
		t.Errorf("with a repository: exit %d, stderr %q; want %q once", code, stderr, want)
	}
	inOrder(t, "report", page, "<h1>2 repositories</h1>", "github.com/acme", "no repositories found in github.com/acme", "github.com/o/good")
}

// A failing repository next to a failed organization is printed once, with its prefix.
func TestOrgFailureNextToFailingRepo(t *testing.T) {
	d, _ := orgDeps(t, nil, "bad")
	code, stderr, _ := htmlScan(t, d, "https://github.com/Nope", "https://github.com/o/bad")
	if code != exitFail {
		t.Fatalf("exit %d", code)
	}
	for _, want := range []string{"skill-atlas: github.com/Nope: boom\n", "skill-atlas: github.com/o/bad: authentication failed for github.com/o/bad\n"} {
		if strings.Count(stderr, want) != 1 {
			t.Errorf("stderr %q has %q %d times, want once", stderr, want, strings.Count(stderr, want))
		}
	}
}

func TestOrgListingInterrupted(t *testing.T) {
	d, downloaded := orgDeps(t, nil)
	d.listOrg = func(ctx context.Context, _ repo.Target) ([]repo.Target, error) {
		p, err := os.FindProcess(os.Getpid())
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Signal(os.Interrupt); err != nil {
			t.Skipf("can't send an interrupt: %v", err)
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	code, stderr, page := htmlScan(t, d, "https://github.com/acme", "https://github.com/o/r")
	if code != exitInterrupted || len(*downloaded) != 0 || page != "" || strings.Contains(stderr, "skill-atlas:") {
		t.Errorf("exit %d, downloads %q, stderr %q; want %d and nothing else", code, *downloaded, stderr, exitInterrupted)
	}
}
