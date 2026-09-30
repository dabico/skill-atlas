---
name: Decision log
description: What the maintainer decided, when and why, with their quotes, plus decisions that were reversed
type: project
---

# Decision log

All decisions below were made on 2026-09-30 unless noted. Quotes are the maintainer's words. `CLAUDE.md` has the rules that follow from these; this file keeps the history.

## Skills and validation

- **A skill follows the Agent Skills spec.** A skill is a directory with a `SKILL.md` file, as defined at agentskills.io. Validation copies the skills-ref reference validator: unknown fields give 1 combined error, scalars are strings, `name` is NFKC-normalized and must match the directory.
  Source: "Let's follow the general standard and definition of what a skill is."
- Invalid skills are listed and marked invalid. We scan other people's repos, so hiding broken skills would give an incomplete picture.
  Source: "list them anyway, mark them as invalid"
- An invalid skill lists every rule it breaks.
  Source: "Yeah let's show the reason as well"
- Skill Atlas recognizes 14 Claude Code frontmatter fields and type-checks them, e.g. `disable-model-invocation` must be a boolean. The TUI and HTML report show them under a `Claude Code` heading. `allowed-tools` also accepts a YAML list. Merged in PR #3.
  Why: real repos (Kotlin, MPS) showed many skills as invalid because of `argument-hint` and `disable-model-invocation`.
- Fields no tool documents, such as `type`, still make a skill invalid. A warning mode was offered in PR #3 and the maintainer merged the strict version.
- Codex fields are out of scope for now. Codex keeps its extra fields in `agents/openai.yaml`, outside the frontmatter.
  Source: "let's just keep it as is and leave it at Claude-only skill fields"

## Input and cloning

- Input is 1 remote Git URL over HTTPS or SSH. Local paths aren't supported.
  Source: "For the first version, let's only support a valid Git URL"
- Cloning uses go-git in-process. The `git` binary isn't needed.
  Source: "Yeah let's use the go-git client and ditch the whole delegation idea"
- Auth: public HTTPS needs no credentials. SSH uses the SSH agent (`SSH_AUTH_SOCK`) and checks `~/.ssh/known_hosts`. Private HTTPS isn't supported. The maintainer never answered this question directly; this was Claude's recommendation and went in as the default.
- Cloning never prompts. It fails with an error such as `authentication failed for <url>`.
  Source: "Let's not prompt, just fail with an error"
- Clones are shallow (depth 1, single branch, no tags) so big repos clone fast.
- `--ref` takes a branch or tag. Without it, the scan uses the remote's default branch. Commit SHAs aren't supported.
  Source: "Let's go with your recommendation"
- The scanned commit SHA is always shown.
  Source: "let's show the SHA by default why not"
- No state is stored. The clone goes to a temp dir that is deleted before exit. The one exception is the `--html` report file.
  Source: "I don't plan for the tool to store any state"
- Symlinks are ignored. Submodules are skipped.
  Source: "Ignore symlinks", "Skip submodules for now"

## Multi-repo scan (PR #5)

- `scan` takes several Git URLs. The ref only goes on the URL as `<url>#<ref>`. The `--ref` flag is gone.
  Source: "Get rid of `--ref` flag entirely, we will just use references in the URL itself"
- By default the scan returns partial results. The repos that fail are marked in the TUI and the report, and their errors go to stderr. The exit code is 1 if any repo failed, and 1 with no UI if all failed. A fail-fast flag may come later and isn't built yet.
  Source: "Fail-fast should not be the default, partial results should. That should be a new flag that we can add later". The maintainer picked exit 1 over exit 0.
- `--parallel N` sets how many clones run at once. The default is 4 and the minimum is 1.
  Source: "Pralellism should also be flag-configurable". The maintainer picked the name `--parallel`.

## Stack

- Go 1.27.1, module `skill-atlas`. Charm v2 (bubbletea, lipgloss, glamour), go-git v5, go.yaml.in/yaml/v3.
  Source: "I was planning on using Go anyway"

## TUI

- Split view: skill list on the left, detail pane on the right.
  Source: "Your design is fine"
- The detail pane shows the full `SKILL.md` body rendered as Markdown, scrollable after `Tab`.
  Source: "Let's show a preview why not", "let's render markdown"
- `/` filters by name, description and path (case-insensitive substring).

## Exclusions

- `--exclude <pattern>` takes gitignore patterns and is repeatable. The last matching pattern wins. Excluded files are counted in the header (`N skills, M invalid, K excluded`) and never parsed. Nothing is excluded by default. Merged in PR #1.
  Source: "don't implicitly exclude tests. I want to just support the multi-gitignore exlusion flag"

## HTML report

- `--html` writes 1 self-contained page to `$TMPDIR/skill-atlas-report-*.html` (mode 0600), prints `Report: <path>` and opens the `file://` URL in the browser. It exits 0 even if the browser fails to open. Merged in PR #2.
  Source: "I just need you to generate an HTML file that is opened as a file in the browser. It's not interactive anyway."
- The page has a filter that matches the TUI's `/` filter. It needs 1 inline script, because CSS can't read typed text. The CSP allows only that script, by sha256 hash, with no `unsafe-inline`. Without JS the page still works and the filter box stays hidden.
  Source: "If possible no JS, but if unavoidable let's roll back the decision to ban JS."
- The page shows the Claude Code fields like the TUI. The filter ignores them. PR #4, open.
  Source: "Only add existing supported fields to the HTML."

## Testing and CI

- CI runs on GitHub Actions in `dabico/skill-atlas` (private). Jobs: Lint, Vulnerability scan (skipped on PRs), Test on ubuntu and macOS, Network clone tests, E2E (tmux). Actions are pinned to commit SHAs. Dependabot opens weekly grouped updates.
- The E2E baseline is JetBrains/ideavim at tag `2.47.1` (commit `c1ae565`), checked against a golden file.
  Source: "Let's instead use JetBrains/ideavim as a baseline and not these other repositories."
- Other pinned fixtures, each added for 1 feature:
  - JetBrains/koog `1.3.0` (`3acc88cf`) for `--exclude`. It has 2 skills in test data under `integration-tests/`.
  - zcaceres/skills `zoom@1.0.1` (`1d5af94`, 61 skills) for Claude Code fields. It's a personal repo, so the tag could move. The test checks the SHA, so CI would fail loudly.
- Every feature ships as a PR gated on green CI. The maintainer reviews and merges (squash).

## Superseded

| Was                                                      | Replaced by                                       | Why                                                                    |
|----------------------------------------------------------|---------------------------------------------------|------------------------------------------------------------------------|
| Clone with the `git` binary                              | Any Git client in the app's language, then go-git | "we don't need to use the Git binary per-se"                           |
| Delegate credentials to the user's `git` setup           | go-git with public HTTPS and SSH agent            | go-git can't run credential helpers; the maintainer dropped delegation |
| E2E against JetBrains/koog and JetBrains/MPS             | ideavim baseline                                  | Maintainer's call mid-run                                              |
| Skip test directories by default, with `--include-tests` | `--exclude` only, nothing implicit                | Maintainer's review of PR #1                                           |
| `--html` serves the page once over loopback              | Write a file and open it                          | Maintainer's review of PR #2: no server needed                         |
| No JavaScript in the HTML page                           | 1 inline filter script pinned by CSP hash         | Filter needs JS                                                        |
| Codex fields plus HTML as a stacked PR                   | Claude Code fields only, single PR #4             | Maintainer cancelled mid-run                                           |
| `--ref` as the default ref for URLs without `#<ref>`     | `#<ref>` on the URL only                          | Maintainer's review of PR #5                                           |
| Fail-fast: 1 bad repo fails the whole multi-repo scan    | Partial results, exit 1                           | Maintainer's review of PR #5; fail-fast may return as a flag           |
| Parallel clone limit fixed at 4 (`maxClones`)            | `--parallel N`, default 4                         | Maintainer's review of PR #5                                           |
| `README.md` and `memory/` left out of feature PRs        | Both committed to PR #5                           | Maintainer's call on PR #5                                             |
