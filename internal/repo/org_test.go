package repo

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeAPI replaces the GitHub API with handler and returns the server URL.
func fakeAPI(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	oldBase, oldClient := apiBase, httpClient
	t.Cleanup(func() { apiBase, httpClient = oldBase, oldClient })
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	apiBase, httpClient = srv.URL, srv.Client()
	return srv.URL
}

func testOrg(t *testing.T) Target {
	t.Helper()
	org, err := ParseURL("https://github.com/acme")
	if err != nil {
		t.Fatal(err)
	}
	return org
}

func repoJSON(names ...string) string {
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = fmt.Sprintf(`{"name":%q,"full_name":"Acme/%s","fork":%v}`, n, n, i%2 == 1)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func TestListOrgPages(t *testing.T) {
	var requests []*http.Request
	var base string
	base = fakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		switch r.URL.Query().Get("page") {
		case "":
			w.Header().Set("Link", fmt.Sprintf(`<%s/organizations/7/repos?page=2>; rel="next", <%s/organizations/7/repos?page=3>; rel="last"`, base, base))
			fmt.Fprint(w, repoJSON("a", "b"))
		case "2":
			w.Header().Set("Link", fmt.Sprintf(`<%s/organizations/7/repos?page=1>; rel="prev", <%s/organizations/7/repos?page=3>; rel="next"`, base, base))
			fmt.Fprint(w, repoJSON("c"))
		case "3":
			w.Header().Set("Link", fmt.Sprintf(`<%s/organizations/7/repos?page=2>; rel="prev", <%s/organizations/7/repos?page=1>; rel="first"`, base, base))
			fmt.Fprint(w, repoJSON("d.e"))
		default:
			http.NotFound(w, r)
		}
	})

	got, err := ListOrg(context.Background(), testOrg(t))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tg := range got {
		names = append(names, tg.Display)
	}
	if want := "github.com/Acme/a github.com/Acme/b github.com/Acme/c github.com/Acme/d.e"; strings.Join(names, " ") != want {
		t.Errorf("got %q, want %q", names, want)
	}
	want := Target{URL: "https://github.com/Acme/d.e", Remote: "https://github.com/Acme/d.e.git", Display: "github.com/Acme/d.e", Owner: "Acme", Name: "d.e"}
	if got[3] != want {
		t.Errorf("got %+v, want %+v", got[3], want)
	}

	if len(requests) != 3 {
		t.Fatalf("%d requests, want 3", len(requests))
	}
	first := requests[0]
	if first.URL.Path != "/orgs/acme/repos" || first.URL.RawQuery != "type=all&sort=full_name&per_page=100" {
		t.Errorf("first request = %s", first.URL)
	}
	for _, r := range requests {
		if r.Header.Get("Accept") != "application/vnd.github+json" || r.Header.Get("User-Agent") != "skill-atlas" {
			t.Errorf("headers %v", r.Header)
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("request sends credentials")
		}
	}
}

func TestListOrgErrors(t *testing.T) {
	reset := time.Date(2026, 10, 1, 14, 5, 0, 0, time.Local)
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{"not found", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) },
			"github.com/acme isn't a GitHub organization or doesn't exist"},
		{"rate limit 403", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", fmt.Sprint(reset.Unix()))
			w.WriteHeader(http.StatusForbidden)
		}, "GitHub API rate limit exceeded, resets at 14:05"},
		{"rate limit 429 without reset", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(http.StatusTooManyRequests)
		}, "GitHub API rate limit exceeded"},
		{"403 with requests left", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "12")
			w.WriteHeader(http.StatusForbidden)
		}, "list repositories in github.com/acme: 403 Forbidden"},
		{"server error", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
			"list repositories in github.com/acme: 500 Internal Server Error"},
		{"empty", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "[]") },
			"no repositories found in github.com/acme"},
		{"bad json", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"message":"x"}`) },
			"list repositories in github.com/acme: json: "},
		{"bad full name", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `[{"full_name":"acme/../x"}]`) },
			`unexpected repository name "acme/../x"`},
		{"next page elsewhere", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Link", `<https://evil.test/orgs/acme/repos?page=2>; rel="next"`)
			fmt.Fprint(w, repoJSON("a"))
		}, `next page "https://evil.test/orgs/acme/repos?page=2" isn't on`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeAPI(t, tt.handler)
			got, err := ListOrg(context.Background(), testOrg(t))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("got %v, %v; want error %q", got, err, tt.want)
			}
			if err != nil && tt.name != "rate limit 403" && strings.Contains(err.Error(), "resets at") {
				t.Errorf("error %q names a reset time", err)
			}
		})
	}
}

func TestListOrgCanceled(t *testing.T) {
	started := make(chan struct{})
	fakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-started; cancel() }()
	_, err := ListOrg(ctx, testOrg(t))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want context.Canceled", err)
	}
}

func TestNextPage(t *testing.T) {
	fakeAPI(t, func(http.ResponseWriter, *http.Request) {})
	tests := []struct{ link, want string }{
		{"", ""},
		{`<` + apiBase + `/x?page=2>; rel="next"`, apiBase + "/x?page=2"},
		{`<` + apiBase + `/x?page=1>; rel="prev", <` + apiBase + `/x?page=3>; rel="next"`, apiBase + "/x?page=3"},
		{`<` + apiBase + `/x?page=1>; rel="first", <` + apiBase + `/x?page=1>; rel="prev"`, ""},
	}
	for _, tt := range tests {
		got, err := nextPage(tt.link)
		if err != nil || got != tt.want {
			t.Errorf("nextPage(%q) = %q, %v; want %q", tt.link, got, err, tt.want)
		}
	}
}
