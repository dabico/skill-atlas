---
name: Session 2026-11-02 govulncheck
description: Scheduled CI run 41432716873 failed the Vulnerability scan; bumped Go to 1.27.3, x/net to v0.60.0 and the screenshot image
type: project
---

# 2026-11-02, govulncheck failure

An Air Automations run ("Fix Failed CI") picked up scheduled CI run 41432716873 on `main` at `43f4e32`. Only the Vulnerability scan job failed. The other jobs passed.

- govulncheck v1.8.0 reported 11 reachable advisories: 10 in the Go 1.27.1 standard library (net/http, net/http/internal/http2, crypto/tls, crypto/x509, net/textproto, html/template) and 4 that also list `golang.org/x/net` v0.59.0. GO-2026-6809 (crypto/x509) is fixed only in Go 1.27.3; the rest are fixed in 1.27.2 and x/net v0.60.0.
- The code didn't change. New advisories were published against the pinned versions. The job is skipped on PRs, so only the schedule caught it.
- Fix on `feature/go-1.27.3`: `go 1.27.3` in `go.mod`, `golang.org/x/net` v0.60.0, and `e2e/screenshots.sh` moved to `golang:1.27.3-bookworm` with its digest. The golang image sets `GOTOOLCHAIN=local`, so a 1.27.1 image would refuse a `go 1.27.3` module.
- This sandbox gets 403 from `vuln.go.dev` and `cdn.playwright.dev`, and the automation token can't dispatch workflows. `govulncheck -db https://storage.googleapis.com/go-vulndb ./...` works (the bucket behind vuln.go.dev): 11 advisories on `main`, 0 on the branch. The screenshot tests were left to PR CI.
