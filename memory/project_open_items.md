---
name: Open items
description: Open PRs, known bugs, quirks and ideas not started
type: project
---

# Open items

Last updated 2026-09-30. `main` is at `cb297dc`: PRs #1 to #5 merged. Between #4 and #5 the maintainer committed `README.md`, `AGENTS.md` (`CLAUDE.md` symlinks to it), `memory/`, `.gitignore`, a PR template and the `record-demo` skill.

## Open PRs

- PR #6 `feature/html-screenshots`: Playwright screenshot tests for the HTML report. Worktree `.claude/worktrees/html-screenshots`. Waiting for CI and review. CI's native amd64 run is the first outside emulation.
- The PR #5 demo page is unpublished: `docs/demo/multi-repo/index.html` in the main checkout, ignored by git. The Artifact publish failed: this machine authenticates with `apiKeyHelper`, and Artifacts need a claude.ai login.

## Known bugs

From a code review run on 2026-09-30. Its fix phase never ran, so none of these are fixed.

- High: the TUI prints untrusted text (name, description and other fields) without stripping escape sequences, so a `SKILL.md` can inject OSC 8 links or SGR colors into the terminal. Claude Code field values are already stripped with `ansi.Strip`.
- Medium: a `known_hosts` entry with a different key type for the same host gives a false "host key doesn't match" error.
- Medium: YAML syntax error line numbers are relative to the frontmatter block, so they're off by 1 against the file.
- Low: `ansi.Strip` removes escape sequences and leaves bare control characters. TUI and HTML both have this.

## Quirks to keep in mind

- `--exclude '!pattern'` can bring back a file inside an excluded directory. Git itself doesn't allow that.
- Ctrl+C during the `--html` browser launch wait (up to 3s) ends the process with default signal handling, so the exit code isn't 130.
- The zcaceres/skills fixture could move or disappear. CI would fail on the SHA check.

## Ideas not started

- A fail-fast flag for multi-repo scans. The maintainer wants it "later"; partial results are the default.
- Codex fields from `agents/openai.yaml`. Research is in `research/provider-fields.json`.
- Fields from other tools (VS Code, Cursor and others). Same research file.
- A README testing section and a `.gitignore` for `.idea/` and `bin/`.
