# Skill Atlas

CLI tool used for mining and visualising skills in a Git repository directory tree.

## Context

### Problem

Users want to scan Git repositories hosted on VCS for the skills they contain.
The status-quo is that they have to manually clone, run some Bash commands and manually inspect each one.

### Outcome

With this tool, a user would be able to simply run:

```shell
skill-atlas scan https://github.com/githubtraining/hellogitworld.git
```

And get an overview through a TUI of each skill found in the repository, along with a short description preview.

## Skill definition

Skill Atlas follows the [Agent Skills specification](https://agentskills.io/specification).

A skill is a directory that contains a `SKILL.md` file.
The file starts with YAML frontmatter, followed by a Markdown body with the skill's instructions.

```text
skill-name/
├── SKILL.md          # required
├── scripts/          # optional
├── references/       # optional
└── assets/           # optional
```

### Frontmatter

| Field           | Required | Constraints                                                                                                                                                          |
|-----------------|----------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `name`          | Yes      | 1-64 characters. Lowercase letters (`a-z`), digits (`0-9`) and hyphens only. Can't start or end with a hyphen or contain `--`. Must match the parent directory name. |
| `description`   | Yes      | 1-1024 characters. Says what the skill does and when to use it.                                                                                                      |
| `license`       | No       | License name, or the path to a bundled license file.                                                                                                                 |
| `compatibility` | No       | 1-500 characters. Environment requirements, e.g. target product, system packages, network access.                                                                    |
| `metadata`      | No       | Map of string keys to string values.                                                                                                                                 |
| `allowed-tools` | No       | Space-separated list of pre-approved tools, or a YAML list of strings. Experimental.                                                                                 |

Minimal valid `SKILL.md`:

```markdown
---
name: pdf-processing
description: Extract PDF text, fill forms, merge files. Use when handling PDFs.
---
```

The TUI preview uses the `description` field.

### Claude Code fields

Skill Atlas also accepts the frontmatter fields from the [Claude Code skills documentation](https://code.claude.com/docs/en/skills).
A skill that uses them stays valid when the values have the right type. A wrong type is an error, e.g. `effort must be one of: low, medium, high, xhigh, max`.

| Field                      | Type                                                          |
|----------------------------|---------------------------------------------------------------|
| `when_to_use`              | String                                                        |
| `argument-hint`            | String                                                        |
| `arguments`                | String or list of strings                                     |
| `disable-model-invocation` | Boolean                                                       |
| `user-invocable`           | Boolean                                                       |
| `disallowed-tools`         | String or list of strings                                     |
| `model`                    | String                                                        |
| `effort`                   | One of `low`, `medium`, `high`, `xhigh`, `max`                |
| `context`                  | `fork`                                                        |
| `agent`                    | String                                                        |
| `background`               | Boolean                                                       |
| `hooks`                    | Mapping. The contents aren't checked.                         |
| `paths`                    | String or list of strings                                     |
| `shell`                    | One of `bash`, `powershell`                                   |

- Booleans also accept `yes`, `no`, `on`, `off`, `1` and `0`, in any letter case, like Claude Code does. The result shows `true` or `false`.
- A field with an empty (null) value counts as absent.
- The TUI detail pane lists these fields under a `Claude Code` heading, in file order, after the specification fields. A list shows as comma-separated values, and `hooks` shows each event with its handler count, e.g. `PreToolUse (1)`.
- Fields that no tool documents, e.g. `type`, stay unexpected fields (see [Validation details](#validation-details)).

### Invalid skills

A `SKILL.md` that breaks the specification still appears in the results, marked as invalid.
Examples: YAML that doesn't parse, a missing `description`, or a `name` that doesn't match its directory.

Each invalid skill lists every rule it breaks, e.g.:

- `name "PDF-Tool" contains uppercase letters`
- `name "pdf" doesn't match directory "pdf-tools"`

### Validation details

Where the specification page is silent, Skill Atlas matches the [skills-ref](https://github.com/agentskills/agentskills/tree/main/skills-ref) reference validator:

- Top-level fields outside the 6 above and the [Claude Code fields](#claude-code-fields) are errors, e.g. `unexpected fields: type, version`.
- YAML scalars are read as strings, so `version: 1.0` in `metadata` is the string `"1.0"`.
- `name` is NFKC-normalized before the checks, and "lowercase letters and digits" includes non-ASCII ones.
- For a `SKILL.md` at the repository root, the parent directory name is the repository name from the URL.

## Scan command

```shell
skill-atlas scan [--html] [--exclude <pattern>]... [--parallel <n>] <git-url>[#<ref>]...
```

- The command takes 1 or more remote Git URLs, over HTTPS or SSH. Local paths aren't supported. No URL is a usage error (`scan needs a git url`).
- Flags can come before, between or after the URLs.
- A URL can end in `#<ref>` to pick the branch or tag for that URL. The tool splits at the first `#`. An empty ref (`<url>#`) is a usage error.
- Without a ref, the scan uses the remote's default branch (`HEAD`).
- Commit SHAs aren't supported. An unknown ref fails the scan with an error.
- The same repository can appear more than once with different refs. The same repository with the same ref twice is a usage error (exit 2): `github.com/org/repo given twice`, or `github.com/org/repo @ v1 given twice` when a ref is set. The check compares the short form shown in the results, so `https://github.com/org/repo` and `git@github.com:org/repo.git` count as the same repository. It runs after the URLs are parsed. A URL that doesn't parse is an exit 1 error. With several URLs, that error starts with the bad URL.
- With several URLs the tool clones up to `--parallel` repositories at the same time and prints `Cloning <repo>[ @ <ref>]…` to stderr for each one. Results keep the command-line order.
- With several URLs, a repository that fails to clone or scan doesn't stop the others. The tool records the failure for that repository and prints `skill-atlas: <repo>[ @ <ref>]: <error>` to stderr as it happens, before the TUI or report starts. Escape sequences in the error text are removed.
- When some repositories fail, the TUI or the report shows all of them in command-line order, with the failed ones marked. The tool then exits 1, after the TUI quits or the report opens, so scripts can tell that the result is incomplete.
- When every repository fails, the tool exits 1 and shows no TUI and writes no report file.
- With 1 URL, a failure prints `skill-atlas: <error>` without the repository prefix and exits 1.
- In zsh with `extendedglob`, `#` starts a pattern, so quote a URL that has a ref: `'https://github.com/org/repo#v1'`.
- `--html` writes the results to an HTML file and opens it in the web browser instead of showing the TUI. See [HTML report](#html-report).
- `--exclude` skips `SKILL.md` files by path. The flag is repeatable and applies to every repository.
- Details are under [Excluded paths](#excluded-paths).
- `--parallel <n>` sets how many repositories clone at the same time. The default is 4. `<n>` is an integer of 1 or more, with no upper limit. Both `--parallel <n>` and `--parallel=<n>` work. Any other value (0, a negative number, text or nothing) is a usage error and exits 2.
- Cloning uses [go-git](https://github.com/go-git/go-git). The `git` binary isn't required.
- Clones are shallow (depth 1). The scan doesn't need history.
- The results show the commit SHA that was scanned, per repository.
- Public repositories over HTTPS need no credentials. Private repositories over HTTPS aren't supported.
- SSH URLs authenticate through the SSH agent (`SSH_AUTH_SOCK`). Host keys are checked against `~/.ssh/known_hosts`.
- Cloning never prompts. If the clone needs input it can't get, the scan fails with an error, e.g. `authentication failed for <url>`.

### Scan scope

The scan walks every tracked file in the clone, except `.git/`.
It has no ignore list of its own, since a fresh clone only contains tracked files.
Only the [excluded paths](#excluded-paths) are skipped.

A `SKILL.md` nested inside another skill's directory is a separate skill.

The scan ignores symlinks, both to files and to directories.
Each skill appears once, at its real path, and the scan never reads outside the clone.

### Excluded paths

Without `--exclude`, the scan reads every `SKILL.md`.

`--exclude <pattern>` skips the `SKILL.md` files that match a [gitignore](https://git-scm.com/docs/gitignore#_pattern_format) pattern.
Pass the flag more than once to add patterns. Both `--exclude <pattern>` and `--exclude=<pattern>` work.

- Patterns match the `SKILL.md` path relative to the repository root, with `/` separators.
- A pattern without `/` matches a name at any depth: `fixtures` matches `a/fixtures/x/SKILL.md`.
- A pattern with a leading or inner `/` matches from the repository root: `/docs` and `skills/old` match only at the top.
- `**` matches any number of directories: `**/old`, `skills/**/draft`. It must fill a whole path segment, so `a**b` is an error.
- A pattern that matches a directory excludes everything below it. A trailing `/` limits the pattern to directories.
- A leading `!` re-includes paths that an earlier pattern matched. The last matching pattern wins. Unlike git, `!` can re-include a file inside an excluded directory.
- An empty or malformed pattern is a usage error (exit 2), reported before the clone starts.

Excluded files are counted and never parsed.
Symlinks and `.git/` stay ignored and aren't counted.
The TUI and the HTML report show the count (see [TUI](#tui) and [HTML report](#html-report)).

Example: `skill-atlas scan --exclude integration-tests/ --exclude '**/fixtures' <git-url>`.

### State

The tool stores no state, with 1 exception: the HTML report file (see [Delivery](#delivery)).
The clones go into one temporary directory, with a subdirectory per repository. The tool deletes it before it exits.
Results are discarded after the scan. `--html` keeps the report file in the OS temp directory.

## TUI

Split view: skill list on the left, details of the selected skill on the right.
This is the layout for 1 repository. See [Several repositories](#several-repositories-in-the-tui) for more.

```text
 github.com/org/repo @ main (a1b2c3d)        12 skills, 2 invalid
┌ Skills ──────────────────┬ pdf-processing ───────────────────────┐
│ > pdf-processing         │ skills/pdf-processing/SKILL.md        │
│   code-review            │                                       │
│ ! PDF-Tool   [invalid]   │ Extract PDF text, fill forms, merge   │
│   data-analysis          │ files. Use when handling PDFs.        │
│                          │                                       │
│                          │ license: Apache-2.0                   │
└──────────────────────────┴───────────────────────────────────────┘
 j/k move · tab focus · / filter · q quit
```

- Header: repository URL, ref, short commit SHA, skill count, invalid count. When the scan excluded any `SKILL.md`, the counts also show the excluded count, e.g. `2 skills, 0 invalid, 2 excluded`. Without exclusions the header has no excluded count.
- List pane: skill `name`, with an `[invalid]` badge on invalid skills.
- Detail pane: path of the `SKILL.md` relative to the repository root, full `description`, other frontmatter fields (Claude Code fields under their own heading), and validation errors for invalid skills. Below that, the full Markdown body, rendered.
- <kbd>Tab</kbd> switches focus between the panes. j/k scroll the detail pane while it has focus.
- <kbd>/</kbd> filters the list by name, description and path. <kbd>Esc</kbd> clears the filter.
- <kbd>q</kbd> or <kbd>Ctrl+C</kbd> quits.
- A scan with no skills shows `No skills found` in the list pane. When exclusions removed every `SKILL.md`, it shows `No skills found (2 excluded)`.
- The list shows the directory name when a skill has no usable `name`.
- Footer: key hints.

### Several repositories in the TUI

```text
 2 repositories                              15 skills, 2 invalid
┌ Skills ──────────────────┬ pdf-processing ───────────────────────┐
│ github.com/org/a @ main… │ github.com/org/a @ main (a1b2c3d)     │
│ > pdf-processing         │ skills/pdf-processing/SKILL.md        │
│   code-review            │                                       │
│ github.com/org/b @ v1 (… │ Extract PDF text, fill forms, merge   │
│ ! PDF-Tool   [invalid]   │ files. Use when handling PDFs.        │
└──────────────────────────┴───────────────────────────────────────┘
```

- Header: `N repositories` on the left, bold. On the right, the totals over all repositories: `N skills, M invalid`, plus `, K excluded` when any repository excluded files and `, Z failed` when Z repositories failed, e.g. `15 skills, 2 invalid, 1 failed`.
- The list is grouped by repository, in command-line order. Each group starts with a heading row, `<repo> @ <ref> (<short sha>)`. Headings can't be selected. The cursor skips them.
- When the cursor is on the first skill of a group, the list scrolls to show the heading too, if it fits.
- A repository with no skills shows its heading and a dim `No skills found` row. With exclusions the row reads `No skills found (2 excluded)`. The row isn't shown while a filter is active.
- A repository that failed shows its heading and, under it, a row with the error message in the warning style, e.g. `authentication failed for github.com/org/b`. The heading has no `(<short sha>)` when the clone did not finish. The error row can't be selected and the cursor skips it. Like the `No skills found` row it is not shown while a filter is active. A message longer than the list pane is cut with `…`. The full message is on stderr. Escape sequences in it are removed.
- The filter also matches the repository name. A heading shows only while a skill in its group matches. `Skills N/M` counts skills only.
- The first line of the detail pane is the dim repository label, then the skill path.
- The minimum terminal size stays 60x12.

## HTML report

`skill-atlas scan --html <git-url>` writes the results to an HTML file and opens it in the browser instead of showing the TUI.
It needs no terminal. It combines with `--exclude`: the page lists the skills that remain and counts the excluded files.

The page has the same information as the TUI:

- Header: repository URL, ref, short commit SHA, skill count, invalid count. With several repositories, see [Several repositories](#several-repositories-in-the-report). The full commit SHA shows as hover text on the short one. When `--exclude` skipped files, the counts end with `, K excluded`: `5 skills, 1 invalid, 1 excluded`. Without exclusions the page omits it.
- Contents: a list that links to each skill. Invalid skills have an `invalid` badge.
- Skill sections: `name`, path of the `SKILL.md` relative to the repository root, full `description`, other frontmatter fields (Claude Code fields under their own `Claude Code` heading, like the TUI detail pane), and validation errors for invalid skills. Below that, the full Markdown body, rendered.
- A scan with no skills shows `No skills found`. When exclusions removed every `SKILL.md`, it shows `No skills found (2 excluded)`.
- The list shows the directory name when a skill has no usable `name`.
- A filter box narrows the page to matching skills. See [Filter](#filter).

### Several repositories in the report

- The `<h1>` reads `N repositories`, followed by the totals line. There is no single ref or SHA line.
- Contents are grouped by repository. Each group has a heading with the repository name, ref, short SHA (the full SHA is the hover text) and its own counts, e.g. `2 skills, 1 invalid, 1 excluded`.
- A repository with no skills shows `No skills found` (or `No skills found (2 excluded)`) in the contents. It has no skill sections.
- A repository that failed shows in the contents and as a section group, each with the heading (`failed` in place of the counts) and the error message as a note. The heading has no short SHA when the clone did not finish. Like the empty-repository note, the note hides while a filter is active. The message is escaped and has no escape sequences.
- The totals line ends with `, Z failed` when Z repositories failed.
- Skill sections sit under a heading per repository. Section ids are `repo-<R>-skill-<N>`, counted from 1. With 1 repository the ids stay `skill-<N>`.
- Heading levels nest: the page title is `h1`, repository headings are `h2` and skill names `h3`. Headings in a body keep the same offset below the skill name as with 1 repository. Levels stop at `h6`.
- The filter also matches the repository name. A repository group hides when none of its skills match. `Skills N/M` counts the skills in all repositories.
- The page still has 1 script, the filter.

### Filter

The box matches the TUI <kbd>/</kbd> filter.

- It keeps the skills whose name (or directory name), description or path contains the typed text. The match ignores case.
- The contents list and the skill sections both narrow.
- While the box has text, a line shows `Skills N/M`, with N matching and M total. At 0 matches the page shows `No matching skills`.
- <kbd>/</kbd> focuses the box unless focus is already in a text field. <kbd>Esc</kbd> clears the box and restores every skill.
- The filter is for reading. It doesn't change the report file or the order of skills.
- Without JavaScript the box is hidden and the page shows every skill.

A text box can't be filtered with CSS alone. CSS selectors see the `value` attribute, which doesn't change while the user types, and `:placeholder-shown` only tells empty from non-empty. A script is the smallest way to read the typed text.

### Page

- The page is 1 HTML document with inline CSS. It makes no external requests: no remote fonts, scripts, styles or images.
- The page has 1 inline script, which runs the [filter](#filter). It reads the typed text and the `data-match` attribute of each skill, toggles the `hidden` attribute and sets text with `textContent`. It makes no network calls and uses no `eval` or `innerHTML`. It never writes skill content into the page. A page with no skills has no script.
- A Content Security Policy enforces this. With skills: `default-src 'none'; script-src 'sha256-<hash>'; style-src 'unsafe-inline'; img-src data:`. The tool computes the hash from the exact script text. The policy has no `'unsafe-inline'` for scripts, so the browser blocks any other script. Without skills the policy has no `script-src`.
- Colors follow the browser's light or dark setting. The layout fits a phone screen.
- The Markdown body uses GitHub Flavored Markdown. Headings in the body sit 2 levels below the skill `name`.
- Raw HTML in a body is dropped.
- A link with a scheme other than `http`, `https` or `mailto` shows as plain text. This covers `javascript:`, `vbscript:`, `data:` and `file:`.
- Images aren't loaded. The page shows their alt text.

### Delivery

After the scan, and after the tool deletes the clone, it writes the page to a new file in the OS temp directory. The file is named `skill-atlas-report-<random>.html` and is readable by its owner only.

- The tool prints `Report: <absolute path>` to stderr.
- The tool opens the file in the browser as a `file://` URL, then exits with 0.
- The file stays after the tool exits, because the browser loads it after the launch command returns. The OS temp cleanup removes it.
- Each run writes a new file.
- If the tool can't write the file, the scan fails with an error.

### Browser

- If `$BROWSER` is set, the tool runs it with the URL as its only argument. The tool uses no shell, so `$BROWSER` is 1 program name or path.
- Otherwise the tool runs `open` on macOS, `rundll32 url.dll,FileProtocolHandler` on Windows and `xdg-open` on other systems.
- The launch counts as done when the command exits with 0 or is still running after 3 seconds. A command that is the browser itself doesn't hold up the tool.
- If the command fails to start or exits with a non-zero code within those 3 seconds, the tool prints a warning with the file path and still exits with 0. The report exists, and the user can open it by hand.

### Exit codes

- 0: the tool wrote the report and launched the browser, or warned that the launch failed.
- 1: a repository failed to clone or scan, or the tool couldn't write the report file. With several repositories, the tool still shows the ones that worked (TUI or report) and exits 1 afterwards. If every repository failed, there is no TUI and no report file.
- 2: usage error, including the same repository and ref given twice.
- 130: interrupted with <kbd>Ctrl+C</kbd> or `SIGTERM` during the clones.
