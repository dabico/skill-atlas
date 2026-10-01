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
- No state is stored. The clone goes to a temp dir that is deleted before exit. The one exception is the `--html` report file. Superseded on 2026-10-01 by the archive cache.
  Source: "I don't plan for the tool to store any state"
- Symlinks are ignored. Submodules are skipped.
  Source: "Ignore symlinks", "Skip submodules for now"

## Archive download (2026-10-01)

These replace the cloning decisions above.

- No clone. The tool resolves the ref with go-git's ls-remote over HTTPS, then downloads `https://github.com/<owner>/<repo>/archive/<sha>.tar.gz` (GitHub redirects to codeload.github.com) and unpacks it while it streams. Ref rules stay: no ref is the default branch from the HEAD symref, `#<ref>` tries branch then tag, commit SHAs aren't supported. Annotated tags use the peeled `refs/tags/x^{}` entry, so the SHA is still the commit.
  Source: "Let's go with option A and narrow support to GitHub for now."
- Only `github.com` is supported (host compared case-insensitively). The path must be `<owner>/<repo>`. Other hosts fail in `ParseURL` with `unsupported host "gitlab.com", only github.com is supported`.
- SSH URLs are still accepted but treated as their HTTPS form: ls-remote and the download go over HTTPS with no auth. The SSH agent and `known_hosts` code and the direct `skeema/knownhosts` dependency are gone. Private repos aren't supported in any URL form. Option A was treating SSH URLs as HTTPS and option C was `GITHUB_TOKEN` support. The maintainer prefers C; it is planned for a later PR.
  Source: "Let's go with A for now, but C would be preferred."
- Unpacking writes only regular files and directories, strips the single top-level directory, skips symlinks and hard links, and fails on absolute paths or `..` escapes. A pax global `comment` (the commit SHA from git archive) that differs from the resolved SHA fails the download.

## Organization scan (2026-10-01)

- `scan https://github.com/<org>` lists the organization's repositories and scans each one at its default branch. Refs aren't supported on an org URL (usage error, exit 2). A repository URL whose owner is an org on the command line is dropped in favor of the org scan.
  Source: "Implement support for organizations in GitHub. Submitting an organization URL downloads all skills from all repositories available to the requester. Refs should not be supported for this. If CLI has an org URL specified and a specific repository from that org is specified, then it's just ignored in favor of the org scan."
- Defaults Claude picked, not asked:
  - Org URLs are HTTPS only. `git@github.com:org` and `ssh://git@github.com/org` fail with a hint to use `https://github.com/<org>`.
  - The listing uses `GET /orgs/<org>/repos` (type=all, sorted by full name), not `/users/<user>/repos`. A user account gets `github.com/<user> isn't a GitHub organization or doesn't exist`.
  - Forks and archived repositories are included ("all repositories").
  - Empty repositories (no commits) from a listing are left out silently. An empty repo given by its own URL still fails. An org with no repos, or only empty ones, fails with `no repositories found in github.com/<org>`.
  - Covered repositories are dropped silently, with or without a ref, in any argument order, owner compared case-insensitively. The same org twice is a usage error.
  - No credentials yet, so only public repositories are listed and the unauthenticated rate limit applies (60 requests per hour, 100 repos per request). "Available to the requester" needs `GITHUB_TOKEN`, the planned next PR.
  - A failed listing becomes 1 failed result named `github.com/<org>`; the other URLs continue.

## Archive cache (2026-10-01)

- A review on PR #9 said every checkout stays on disk until the whole scan ends, so an org scan with hundreds of repositories can fill a small disk or tmpfs. The maintainer agreed and asked to keep the tarballs between runs. This replaces "no state is stored".
  Source: "Regarding the review comment, it's valid. I'd personally keep archives between runs as "caches" and delete the extracted directories in between scans!"
- `scanAll` deletes each repository's checkout as soon as its download and scan finish, worked or failed. At most `--parallel` checkouts are on disk; the temp root is still deleted at exit.
- Defaults from the brief, not asked:
  - Location `os.UserCacheDir()/skill-atlas/archives`, file `<sha>.tar.gz`. The SHA is the only key: the top-level directory, the only part that names the repository, is stripped, so forks at the same commit share 1 file.
  - ls-remote always runs. A hit makes no archive request. A miss tees the body into `<sha>-*.tmp` in the cache dir and renames it once the extraction succeeded; any failure or cancellation removes the temp file.
  - A cached file that fails to unpack (gzip, tar, SHA mismatch, path escape) is deleted, the checkout cleared and the commit downloaded once more. A cancelled unpack from the cache keeps the file.
  - An unusable cache (no cache dir, mkdir, CreateTemp, write or rename fails) never fails the download. `skill-atlas: warning: archive cache unavailable: <err>` prints once per run.
  - Dirs 0700, files 0600. A hit touches the mtime. No eviction yet.

## Multi-repo scan (PR #5)

- `scan` takes several Git URLs. The ref only goes on the URL as `<url>#<ref>`. The `--ref` flag is gone.
  Source: "Get rid of `--ref` flag entirely, we will just use references in the URL itself"
- By default the scan returns partial results. The repos that fail are marked in the TUI and the report, and their errors go to stderr. The exit code is 1 if any repo failed, and 1 with no UI if all failed. A fail-fast flag may come later and isn't built yet.
  Source: "Fail-fast should not be the default, partial results should. That should be a new flag that we can add later". The maintainer picked exit 1 over exit 0.
- `--parallel N` sets how many clones run at once. The default is 4 and the minimum is 1.
  Source: "Pralellism should also be flag-configurable". The maintainer picked the name `--parallel`.

## Sorting (2026-10-01)

- The TUI and the HTML report list skills by name, A–Z by default. Option A was name A–Z as the new default; option B was to keep path order and cycle path, A–Z, Z–A. The maintainer picked A.
  Source: "Let's go with A, sorting repositories first followed by individual skills after."
- With several repositories, repositories sort by name first, then the skills within each repository. The same direction applies to both levels. Failed and empty repositories sort with the rest.
- Z–A is exactly the A–Z order reversed, at both levels. The TUI and the page script then agree without a second comparator.
- In the TUI, `s` toggles A–Z and Z–A. The list title shows the order (`Skills A–Z`, `Skills N/M Z–A`). In the report it is a plain `<select>` next to the filter box, run by the same 1 script.
  Source: "In TUI, sorting ASC/DESC by name can be a hotkey one toggles. In HTML mode, this is just a plain select."
- The sort key is `DisplayName()` without letter case, then the exact name, then the path. Repositories compare by display name without letter case, then by ref. Both live in `internal/skill/order.go`. `scan.Dir` still returns path order; the order is a presentation concern.

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

- CI runs on GitHub Actions in `dabico/skill-atlas` (private). Jobs: Lint, Vulnerability scan (skipped on PRs), Test on ubuntu and macOS, Network clone tests (renamed Network download tests on 2026-10-01), E2E (tmux). Actions are pinned to commit SHAs. Dependabot opens weekly grouped updates.
- The E2E baseline is JetBrains/ideavim at tag `2.47.1` (commit `c1ae565`), checked against a golden file.
  Source: "Let's instead use JetBrains/ideavim as a baseline and not these other repositories."
- Other pinned fixtures, each added for 1 feature:
  - JetBrains/koog `1.3.0` (`3acc88cf`) for `--exclude`. It has 2 skills in test data under `integration-tests/`.
  - zcaceres/skills `zoom@1.0.1` (`1d5af94`, 61 skills) for Claude Code fields. It's a personal repo, so the tag could move. The test checks the SHA, so CI would fail loudly.
- Every feature ships as a PR gated on green CI. The maintainer reviews and merges (squash).
- The HTML report gets Playwright screenshot tests for regressions. An intentional change updates the screenshot.
  Source: "implement E2E tests involving "screenshots" with playright (test for regressions, if functionality changes intentionally then so does the "screenshot")"
  - playwright-go inside `go test -tags e2e`. `-update` rewrites the baselines, same as the JSON goldens. The maintainer picked it over Node `@playwright/test` and Python via uv.
  - 1 baseline set, rendered on Linux. The tests skip on macOS. A script runs `-update` in a pinned Linux container. Picked over per-platform baselines and Docker-only runs.
  - HTML report only. The TUI keeps its text checks. Picked over ANSI text snapshots and xterm.js screenshots.

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
| `README.md` and `memory/` left out of feature PRs        | Memory goes in the feature PR (`AGENTS.md`)       | Maintainer's call on PR #5; README still only when asked               |
| go-git shallow clone                                     | go-git ls-remote, then the GitHub tarball of the commit | "Let's go with option A and narrow support to GitHub for now." (2026-10-01) |
| SSH agent auth with `~/.ssh/known_hosts`                 | SSH URLs treated as HTTPS, no auth                | "Let's go with A for now, but C would be preferred." (2026-10-01)      |
| Any HTTPS or SSH host                                    | `github.com` only                                 | Tarball URLs are GitHub-specific (2026-10-01)                          |
| Skills in path order                                     | Name A–Z by default, Z–A with `s` or the select   | "Let's go with A, sorting repositories first followed by individual skills after." (2026-10-01) |
| Repositories in command-line order in the TUI/report     | Repositories by name, then skills in each         | Same quote (2026-10-01); stderr lines keep their order                 |
| No state stored, tarball streamed only                   | Archive cache keyed by SHA, checkouts deleted per repository | "I'd personally keep archives between runs as "caches" and delete the extracted directories in between scans!" (2026-10-01) |
