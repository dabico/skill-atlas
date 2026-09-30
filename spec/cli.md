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
| `allowed-tools` | No       | Space-separated list of pre-approved tools. Experimental.                                                                                                            |

Minimal valid `SKILL.md`:

```markdown
---
name: pdf-processing
description: Extract PDF text, fill forms, merge files. Use when handling PDFs.
---
```

The TUI preview uses the `description` field.

### Invalid skills

A `SKILL.md` that breaks the specification still appears in the results, marked as invalid.
Examples: YAML that doesn't parse, a missing `description`, or a `name` that doesn't match its directory.

Each invalid skill lists every rule it breaks, e.g.:

- `name "PDF-Tool" contains uppercase letters`
- `name "pdf" doesn't match directory "pdf-tools"`

### Validation details

Where the specification page is silent, Skill Atlas matches the [skills-ref](https://github.com/agentskills/agentskills/tree/main/skills-ref) reference validator:

- Top-level fields outside the 6 above are errors, e.g. `unexpected fields: argument-hint, model`.
- YAML scalars are read as strings, so `version: 1.0` in `metadata` is the string `"1.0"`.
- `name` is NFKC-normalized before the checks, and "lowercase letters and digits" includes non-ASCII ones.
- For a `SKILL.md` at the repository root, the parent directory name is the repository name from the URL.

## Scan command

```shell
skill-atlas scan [--html] [--ref <branch|tag>] [--exclude <pattern>]... <git-url>
```

- The first version accepts 1 remote Git URL, over HTTPS or SSH. Local paths aren't supported.
- Without `--ref`, the scan uses the remote's default branch (`HEAD`).
- `--ref` takes a branch or tag name. Commit SHAs aren't supported. An unknown ref fails the scan with an error.
- `--html` writes the results to an HTML file and opens it in the web browser instead of showing the TUI. See [HTML report](#html-report).
- `--exclude` skips `SKILL.md` files by path. The flag is repeatable.
- Details are under [Excluded paths](#excluded-paths).
- Cloning uses [go-git](https://github.com/go-git/go-git). The `git` binary isn't required.
- Clones are shallow (depth 1). The scan doesn't need history.
- The results show the commit SHA that was scanned.
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
The TUI shows the count (see [TUI](#tui)).

Example: `skill-atlas scan --exclude integration-tests/ --exclude '**/fixtures' <git-url>`.

### State

The tool stores no state, with 1 exception: the HTML report file (see [Delivery](#delivery)).
The clone goes into a temporary directory, which gets deleted before the tool exits.
Results are discarded after the scan. `--html` keeps the report file in the OS temp directory.

## TUI

Split view: skill list on the left, details of the selected skill on the right.

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
- Detail pane: path of the `SKILL.md` relative to the repository root, full `description`, other frontmatter fields, and validation errors for invalid skills. Below that, the full Markdown body, rendered.
- <kbd>Tab</kbd> switches focus between the panes. j/k scroll the detail pane while it has focus.
- <kbd>/</kbd> filters the list by name, description and path. <kbd>Esc</kbd> clears the filter.
- <kbd>q</kbd> or <kbd>Ctrl+C</kbd> quits.
- A scan with no skills shows `No skills found` in the list pane. When exclusions removed every `SKILL.md`, it shows `No skills found (2 excluded)`.
- The list shows the directory name when a skill has no usable `name`.
- Footer: key hints.

## HTML report

`skill-atlas scan --html <git-url>` writes the results to an HTML file and opens it in the browser instead of showing the TUI.
It needs no terminal. The page is static and has no interactive parts.

The page has the same information as the TUI:

- Header: repository URL, ref, short commit SHA, skill count, invalid count. The full commit SHA shows as hover text on the short one.
- Contents: a list that links to each skill. Invalid skills have an `invalid` badge.
- Skill sections: `name`, path of the `SKILL.md` relative to the repository root, full `description`, other frontmatter fields, and validation errors for invalid skills. Below that, the full Markdown body, rendered.
- A scan with no skills shows `No skills found`.
- The list shows the directory name when a skill has no usable `name`.

### Page

- The page is 1 HTML document with inline CSS. It has no JavaScript and makes no external requests: no remote fonts, scripts, styles or images.
- A Content Security Policy enforces this: `default-src 'none'; style-src 'unsafe-inline'; img-src data:`.
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
- 1: the scan failed, or the tool couldn't write the report file.
- 2: usage error.
- 130: interrupted with <kbd>Ctrl+C</kbd> or `SIGTERM` during the clone.
