---
name: Session 2026-10-09 govulncheck
description: Scheduled CI failed on 10 new Go advisories; bumped Go to 1.27.2 and x/net to v0.60.0, screenshot image with it
type: project
---

# 2026-10-09, govulncheck failure

An Air Automations run ("Fix Failed CI") picked up the failed scheduled CI run 37917748863 on `main` at `43f4e32`. Only the Vulnerability scan job failed.

- govulncheck v1.8.0 reported 10 reachable vulnerabilities: GO-2026-6617, -6611, -6610, -6603 (x/net v0.59.0 and the bundled `net/http/internal/http2`), and GO-2026-6613, -6608, -6607, -6605, -6600, -6599 (`net/http`, `mime/multipart`, `crypto/tls`, `html/template`). All are fixed in Go 1.27.2 and x/net v0.60.0. The code didn't change; the advisories are new.
- Built `fix/go-1.27.2` in `.claude/worktrees/go-1.27.2`: `go 1.27.2` and `golang.org/x/net v0.60.0` in `go.mod`, and `e2e/screenshots.sh` pinned to `golang:1.27.2-bookworm` with its digest. The golang images set `GOTOOLCHAIN=local`, so the 1.27.1 image would refuse the new `go.mod`.
- Unit tests and lint passed locally. govulncheck and the screenshot container couldn't run here: the proxy blocks `vuln.go.dev` and `deb.debian.org`. Both were checked in CI on the branch instead (workflow_dispatch for the vuln job, the PR run for screenshots).
