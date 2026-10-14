---
name: Session 2026-10-14 CI rate limit
description: Fix Failed CI automation, TestListOrgNet hit the anonymous API rate limit on Dependabot PR #15
type: project
---

# 2026-10-14, CI rate limit

An Air Automations run ("Fix Failed CI") was triggered by CI run 38626680021 on Dependabot PR #15 (go-deps group, 6 updates).

- Only Network download tests failed: `TestListOrgNet` (0.08s), `GitHub API rate limit exceeded, resets at 20:14; set GITHUB_TOKEN to raise the limit`. The first request was refused, so the runner IP's 60 anonymous requests per hour were already used. Not caused by the bump.
- Fix on `fix/network-tests-token`: the CI step gets `GITHUB_TOKEN: ${{ github.token }}`, and `netTarget` clears the tokens with `setTokens(t, "", "")`.
- Without the second change `TestDownloadRepoNotFound` fails with a token: `repository github.com/JetBrains/no-such-repo-skill-atlas not found` instead of the anonymous `authentication failed ...` message. Seen locally with `GITHUB_TOKEN=$(gh auth token)`.
- The nightly Vulnerability scan failures on `main` (GO-2026-6617) were noted in open items and left alone.
