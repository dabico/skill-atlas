---
name: Session 2026-10-26 govulncheck
description: Scheduled CI failed in govulncheck on new advisories; raised Go to 1.27.3, x/net to v0.60.0 and the screenshot image
type: project
---

# 2026-10-26, govulncheck failure

- Run by the "Fix Failed CI" automation, not a live conversation. Scheduled CI run 40294838699 failed only in the Vulnerability scan job.
- 11 standard library advisories against go1.27.1 (fixed in 1.27.2, GO-2026-6809 in 1.27.3) and 4 against golang.org/x/net v0.59.0 (fixed in v0.60.0). No code change caused it.
- Fix: `go 1.27.3` in go.mod, golang.org/x/net v0.60.0, and the `e2e/screenshots.sh` image moved to `golang:1.27.3-bookworm` with its index digest.
- The sandbox blocks vuln.go.dev and deb.debian.org. The PR run (40295852511) passed, screenshots included. `workflow_dispatch` returned 403 for the automation token, so govulncheck (skipped on PRs) only runs again on main after merge or at the next schedule.
