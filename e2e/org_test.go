//go:build e2e

package e2e

import (
	"os"
	"strings"
	"testing"
)

// orgTokenEnv holds a GitHub token that TestHTMLOrg hands to the binary as GITHUB_TOKEN.
// Without it the listing gets the anonymous limit of 60 API requests per hour per IP address,
// which other jobs on a shared CI runner IP can use up.
const orgTokenEnv = "SKILL_ATLAS_E2E_GITHUB_TOKEN"

// TestHTMLOrg scans the agentskills organization. Its repositories aren't pinned, so the test
// checks only that the listing ran and that the report has a group for agentskills/agentskills.
func TestHTMLOrg(t *testing.T) {
	t.Parallel()
	const repoDisplay = "github.com/agentskills/agentskills"
	var env []string
	if tok := os.Getenv(orgTokenEnv); tok != "" {
		env = []string{"GITHUB_TOKEN=" + tok}
	}
	r := runHTML(t, requireTool(t, "true"), env, "https://github.com/agentskills")
	if r.code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", r.code, r.stderr)
	}
	for _, w := range []string{"Listing repositories in github.com/agentskills…\n", "Downloading " + repoDisplay + "…\n"} {
		if !strings.Contains(r.stderr, w) {
			t.Errorf("stderr lacks %q:\n%s", w, r.stderr)
		}
	}
	if i, j := strings.Index(r.stderr, "Listing"), strings.Index(r.stderr, "Downloading"); i < 0 || j < i {
		t.Errorf("stderr doesn't list before downloading:\n%s", r.stderr)
	}
	path := r.reportPath(t)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	page := string(b)
	// The group heading reads "<repo> @ <branch> (<short sha>)".
	if !strings.Contains(page, repoDisplay+" @ ") {
		t.Errorf("report has no group for %s", repoDisplay)
	}
	if strings.Contains(page, ">failed<") {
		t.Error("report has a failed repository")
	}
	assertOnlyReport(t, r.tmpDir, path)
}
