---
name: Open items
description: Open PRs, known bugs, quirks and ideas not started
type: project
---

# Open items

Last updated 2026-10-01. `main` is at `4e77a2a`: PRs #1 to #8 merged (#6 HTML screenshot tests, #7 startup script for JetBrains Air, #8 archive download instead of cloning). Between #4 and #5 the maintainer committed `README.md`, `AGENTS.md` (`CLAUDE.md` symlinks to it), `memory/`, `.gitignore`, a PR template and the `record-demo` skill.

## Open PRs

- `feature/org-scan`: scan a GitHub organization (`https://github.com/<org>`) through the REST API listing. Worktree `.claude/worktrees/org-scan`. Committed locally; the push and the PR are next. The download network tests and the e2e suite (including the new `TestHTMLOrg`) can't run on this machine, so CI is their first run.
- The PR #5 demo page is unpublished: `docs/demo/multi-repo/index.html` in the main checkout, ignored by git. The Artifact publish failed: this machine authenticates with `apiKeyHelper`, and Artifacts need a claude.ai login.

## Known bugs

From a code review run on 2026-09-30. Its fix phase never ran, so none of these are fixed.

- High: the TUI prints untrusted text (name, description and other fields) without stripping escape sequences, so a `SKILL.md` can inject OSC 8 links or SGR colors into the terminal. Claude Code field values are already stripped with `ansi.Strip`.
- Medium: YAML syntax error line numbers are relative to the frontmatter block, so they're off by 1 against the file.
- Low: `ansi.Strip` removes escape sequences and leaves bare control characters. TUI and HTML both have this.

## Quirks to keep in mind

- `--exclude '!pattern'` can bring back a file inside an excluded directory. Git itself doesn't allow that.
- Ctrl+C during the `--html` browser launch wait (up to 3s) ends the process with default signal handling, so the exit code isn't 130.
- The zcaceres/skills fixture could move or disappear. CI would fail on the SHA check.
- This machine's egress proxy blocks `codeload.github.com` (CONNECT 403), where GitHub redirects archive downloads. `github.com` itself works, so ls-remote works. Real downloads, the network tests in `internal/repo` and the e2e suite fail here with `can't reach github.com: Get "https://codeload.github.com/...": Forbidden`. Run them in CI. Don't try to bypass the proxy.
- The org listing sends no credentials. GitHub allows 60 unauthenticated API requests per hour per IP, and CI runners share IPs, so `TestListOrgNet` and `TestHTMLOrg` could hit the limit in CI. Each uses 1 request.
- `TestHTMLOrg` scans `github.com/agentskills` unpinned. It only checks for the `agentskills/agentskills` group and no failures.
- A repository failure printed with its prefix during the downloads isn't printed again, even when empty repositories from an org shrink the results to 1.
- `skeema/knownhosts` stays in `go.mod` as an indirect dependency of go-git's SSH transport, even though our code no longer imports it.

## Ideas not started

- `GITHUB_TOKEN` support for private repos. The maintainer prefers it, next PR. It would also let an org scan list private repos ("available to the requester") and raise the API rate limit to 5,000 requests per hour. `apiRequest` in `internal/repo/org.go` is the place to add the header; `nextPage` already refuses next-page links off the API host.
- Scanning a user's repositories (`/users/<user>/repos`). Org URLs for user accounts fail with "isn't a GitHub organization or doesn't exist".
- A filter for forks and archived repositories in org scans. Both are included now.
- A fail-fast flag for multi-repo scans. The maintainer wants it "later"; partial results are the default.
- Codex fields from `agents/openai.yaml`. Research is in `research/provider-fields.json`.
- Fields from other tools (VS Code, Cursor and others). Same research file.
- A README testing section and a `.gitignore` for `.idea/` and `bin/`.
