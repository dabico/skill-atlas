---
name: Open items
description: Open PRs, known bugs, quirks and ideas not started
type: project
---

# Open items

Last updated 2026-09-30. `main` is at `17002dd`: PRs #1 to #4 merged, then the maintainer committed `README.md`, `AGENTS.md` (`CLAUDE.md` symlinks to it), `memory/`, `.gitignore` and a PR template.

## Open PRs

None. Worktrees and feature branches are gone.

## Uncommitted

- `.claude/skills/record-demo/`: local skill that records TUI and HTML demos into `docs/demo/`. Neither is committed; the maintainer decides.

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

- Codex fields from `agents/openai.yaml`. Research is in `research/provider-fields.json`.
- Fields from other tools (VS Code, Cursor and others). Same research file.
- A README testing section and a `.gitignore` for `.idea/` and `bin/`.
