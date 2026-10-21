---
name: Session 2026-10-21 govulncheck
description: Scheduled CI picked up GO-2026-6809 (crypto/x509, fixed in Go 1.27.3); new branch on top of PR #13's commits bumps Go to 1.27.3
type: project
---

# 2026-10-21, govulncheck failure with a new advisory

An Air Automations run ("Fix Failed CI") picked up scheduled CI run 39569680324 on `main` at `43f4e32`. Only the Vulnerability scan job failed.

- govulncheck v1.8.0 reported 11 advisories: the 10 from 2026-10-09 (see [[2026-10-09-govulncheck]]) plus GO-2026-6809, "Distrust compromised intermediate certificates in crypto/x509". It's fixed in Go 1.27.3, not 1.27.2. The trace runs through `repo.extract` (`internal/repo/download.go:362`), where reading the HTTPS body reaches `x509.Certificate.Verify`. PR #13 (Go 1.27.2) doesn't cover it anymore.
- Built `fix/go-1.27.3` in `.claude/worktrees/go-1.27.3` off `main`. It cherry-picks #13's 2 commits, then sets `go 1.27.3` and pins `e2e/screenshots.sh` to `golang:1.27.3-bookworm@sha256:21ade014…`. x/net stays at v0.60.0.
- `vuln.go.dev` still returns 403 from the sandbox. A local DB from the 11 OSV records (`golang.org/x/vulndb/cmd/indexdb`) gave: `main` (go1.27.1) has all 11, `fix/go-1.27.2` (go1.27.2) has GO-2026-6809, and this branch has none.
