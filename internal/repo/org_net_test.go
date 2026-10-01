package repo

import (
	"context"
	"testing"
	"time"
)

// Network test against the real GitHub API. go test -short skips it.
func TestListOrgNet(t *testing.T) {
	if testing.Short() {
		t.Skip("network test")
	}
	org, err := ParseURL("https://github.com/agentskills")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	repos, err := ListOrg(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	want := Target{
		URL: "https://github.com/agentskills/agentskills", Remote: "https://github.com/agentskills/agentskills.git",
		Display: "github.com/agentskills/agentskills", Owner: "agentskills", Name: "agentskills",
	}
	for _, r := range repos {
		if r == want {
			return
		}
	}
	t.Errorf("%d repositories, none is %+v: %+v", len(repos), want, repos)
}
