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
skill-atlas scan [--html] [--ref <branch|tag>] <git-url>
```

- The first version accepts 1 remote Git URL, over HTTPS or SSH. Local paths aren't supported.
- Without `--ref`, the scan uses the remote's default branch (`HEAD`).
- `--ref` takes a branch or tag name. Commit SHAs aren't supported. An unknown ref fails the scan with an error.
- `--html` shows the results as an HTML page in the web browser instead of the TUI. See [HTML report](#html-report).
- Cloning uses [go-git](https://github.com/go-git/go-git). The `git` binary isn't required.
- Clones are shallow (depth 1). The scan doesn't need history.
- The results show the commit SHA that was scanned.
- Public repositories over HTTPS need no credentials. Private repositories over HTTPS aren't supported.
- SSH URLs authenticate through the SSH agent (`SSH_AUTH_SOCK`). Host keys are checked against `~/.ssh/known_hosts`.
- Cloning never prompts. If the clone needs input it can't get, the scan fails with an error, e.g. `authentication failed for <url>`.

### Scan scope

The scan walks every tracked file in the clone, except `.git/`.
There's no ignore list, since a fresh clone only contains tracked files.

A `SKILL.md` nested inside another skill's directory is a separate skill.

The scan ignores symlinks, both to files and to directories.
Each skill appears once, at its real path, and the scan never reads outside the clone.

### State

The tool stores no state.
The clone goes into a temporary directory, which gets deleted before the tool exits.
Results are discarded after the scan.

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

- Header: repository URL, ref, short commit SHA, skill count, invalid count.
- List pane: skill `name`, with an `[invalid]` badge on invalid skills.
- Detail pane: path of the `SKILL.md` relative to the repository root, full `description`, other frontmatter fields, and validation errors for invalid skills. Below that, the full Markdown body, rendered.
- <kbd>Tab</kbd> switches focus between the panes. j/k scroll the detail pane while it has focus.
- <kbd>/</kbd> filters the list by name, description and path. <kbd>Esc</kbd> clears the filter.
- <kbd>q</kbd> or <kbd>Ctrl+C</kbd> quits.
- A scan with no skills shows `No skills found` in the list pane.
- The list shows the directory name when a skill has no usable `name`.
- Footer: key hints.

## HTML report

`skill-atlas scan --html <git-url>` shows the results as a web page instead of the TUI.
It needs no terminal.

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

The tool writes no file. After the scan, and after it deletes the clone, it serves the page from memory.

- The server listens on `127.0.0.1` on a random port. The page is at `http://127.0.0.1:<port>/<token>`, where the token is 128 random bits as 32 hex digits. Any other path returns 404.
- The tool prints `Report: <url>` to stderr, then opens the browser.
- The server sends the page once. After the browser loads it in full, the server stops and the tool exits with 0.
- If nothing loads the page within 5 minutes, the scan fails with `the browser didn't load the report within 5m0s`.
- <kbd>Ctrl+C</kbd> while the tool waits stops it.

### Browser

- If `$BROWSER` is set, the tool runs it with the URL as its only argument. The tool uses no shell, so `$BROWSER` is 1 program name or path.
- Otherwise the tool runs `open` on macOS, `rundll32 url.dll,FileProtocolHandler` on Windows and `xdg-open` on other systems.
- If the browser fails to start, the tool prints a warning with the URL and keeps waiting. The user can open the URL by hand.

### Exit codes

- 0: the browser loaded the report.
- 1: the scan failed, or the browser didn't load the report in time.
- 2: usage error.
- 130: interrupted with <kbd>Ctrl+C</kbd> or `SIGTERM`.
