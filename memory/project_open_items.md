---
name: Open items
description: Open PRs, known bugs, quirks and ideas not started
type: project
---

# Open items

Last updated 2026-09-30. `main` is at `0a2c14b`: PRs #1 to #4 merged, then the maintainer committed `README.md`, `AGENTS.md` (`CLAUDE.md` symlinks to it), `memory/`, `.gitignore`, a PR template and the `record-demo` skill.

## Open PRs

- #5 `feature/multi-repo`: several Git URLs per scan, `<url>#<ref>`, parallel clones, TUI and HTML grouped per repo. Also `DEMO_ARGS` and `assets/multi-repo.tape` for `record-demo`. Worktree `.claude/worktrees/multi-repo`. Reworked after the first review: `--ref` removed, partial results with exit 1, `--parallel N`. CI green on 5a30bab; waiting on the maintainer.
- #5 demo page: `docs/demo/index.html` in that worktree, untracked. The Artifact publish failed: this machine authenticates with `apiKeyHelper`, and Artifacts need a claude.ai login.

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
