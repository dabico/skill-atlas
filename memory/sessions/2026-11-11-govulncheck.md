---
name: Session 2026-11-11 govulncheck
description: Scheduled CI run 43006702133 failed the Vulnerability scan; bumped Go to 1.27.4, x/net to v0.60.0 and the screenshot image
type: project
---

# 2026-11-11, govulncheck failure

An Air Automations run ("Fix Failed CI") picked up scheduled CI run 43006702133 on `main` at `43f4e32`. Only the Vulnerability scan job failed. The other 6 jobs passed.

- govulncheck v1.8.0 reported 12 reachable standard library advisories against Go 1.27.1. GO-2026-6859 (net/http, MaxBytesReader header race) is new and is fixed only in Go 1.27.4. GO-2026-6809 (crypto/x509) needs 1.27.3. The other 10 are fixed in 1.27.2, and 4 of them also list `golang.org/x/net` v0.59.0 (fixed in v0.60.0).
- The code didn't change. New advisories were published against the pinned versions. The job is skipped on PRs, so only the schedule caught it.
- Open PRs #17, #18 and #19 from earlier runs all stop at Go 1.27.3. On `feature/go-1.27.3` (#19) govulncheck still reports GO-2026-6859, so none of them turns the nightly run green anymore.
- Fix on `fix/go-1.27.4`: `go 1.27.4` in `go.mod`, `golang.org/x/net` v0.60.0, and `e2e/screenshots.sh` moved to `golang:1.27.4-bookworm` with its index digest. The golang image sets `GOTOOLCHAIN=local`, so the old image would refuse the module.
- `vuln.go.dev` returned EOF from this sandbox. `govulncheck -db https://storage.googleapis.com/go-vulndb ./...` worked: 12 advisories on `main`, 1 on #19's branch, 0 on `fix/go-1.27.4`. The screenshot tests were left to PR CI.
