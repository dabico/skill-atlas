---
name: Open items
description: Open PRs, known bugs, quirks and ideas not started
type: project
---

# Open items

Last updated 2026-10-01. `main` is at `afc22c7`: PRs #1 to #8 and #10 merged (#6 HTML screenshot tests, #7 startup script for JetBrains Air, #8 archive download, #10 sort by name). Between #4 and #5 the maintainer committed `README.md`, `AGENTS.md` (`CLAUDE.md` symlinks to it), `memory/`, `.gitignore`, a PR template and the `record-demo` skill.

## Open PRs

- `feature/org-scan`: PR #9, scan a GitHub organization (`https://github.com/<org>`) through the REST API listing. Worktree `.claude/worktrees/org-scan`. `origin/main` was merged in after #10 (the maintainer asked for a rebase; AGENTS.md forbids force-pushes, so it was a merge). The Air review asked to delete each checkout after its scan; the maintainer agreed and asked to keep the archives as a cache between runs. Both are done in a follow-up commit on the branch, not pushed yet: `scanAll` deletes each checkout after its scan, and `repo.Download` keeps tarballs in `<user cache dir>/skill-atlas/archives/<sha>.tar.gz` (it gained a `warn func(error)` argument).
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
- `TestHTMLOrg` scans `github.com/agentskills` unpinned. It only checks for the `agentskills/agentskills` group and no failures.
- A repository failure printed with its prefix during the downloads isn't printed again, even when empty repositories from an org shrink the results to 1.
- The same proxy blocks `deb.debian.org` and `cdn.playwright.dev`, so `e2e/screenshots.sh` can't install Chromium here, in Docker or on the host. GitHub Actions artifacts (`productionresultssa*.blob.core.windows.net`) are blocked as well, so `gh run download` fails with Forbidden.
- The e2e harness sets `XDG_CACHE_HOME` to the scratch root, so binary runs after the first download of a fixture hit the cache. `TestTUIInterrupt` gets its own empty cache so Ctrl+C still lands mid-download. `os.UserCacheDir` ignores `XDG_CACHE_HOME` on macOS, so e2e runs there would use `~/Library/Caches`; CI runs e2e on Ubuntu only.
- `README.md` still describes shallow clones deleted before exit (line 37). It is only updated when the maintainer asks.
- `skeema/knownhosts` stays in `go.mod` as an indirect dependency of go-git's SSH transport, even though our code no longer imports it.

## Ideas not started

- Archive cache eviction: a size cap or a maximum age. A cache hit already touches the file's mtime, so last use is known. The cache grows without limit until then.
- A way to bypass the cache for 1 run, e.g. a `--no-cache` flag or an environment variable.
- Scanning a user's repositories (`/users/<user>/repos`). Org URLs for user accounts fail with "isn't a GitHub organization or doesn't exist".
- A filter for forks and archived repositories in org scans. Both are included now.
- A fail-fast flag for multi-repo scans. The maintainer wants it "later"; partial results are the default.
- Codex fields from `agents/openai.yaml`. Research is in `research/provider-fields.json`.
- Fields from other tools (VS Code, Cursor and others). Same research file.
- A README testing section and a `.gitignore` for `.idea/` and `bin/`.
