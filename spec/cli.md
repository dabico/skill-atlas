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

- The command takes 1 or more GitHub repository or organization URLs. Local paths aren't supported. No URL is a usage error (`scan needs a git url`). Organizations are under [Organizations](#organizations).
- Only `github.com` is supported. The host check ignores letter case. A URL for any other host fails with an error, e.g. `unsupported host "gitlab.com", only github.com is supported`.
- The repository path must be `<owner>/<repo>`, with or without `.git`. A path of 1 name is an organization. Any other path is an error, e.g. `https://github.com/org/repo/tree/main`.
- HTTPS URLs and SSH URLs (`git@github.com:org/repo.git`, `ssh://git@github.com/org/repo`) are both accepted for a repository. The tool treats an SSH URL as its HTTPS form: it doesn't use SSH, the SSH agent or `~/.ssh/known_hosts`.
- Flags can come before, between or after the URLs.
- A URL can end in `#<ref>` to pick the branch or tag for that URL. The tool splits at the first `#`. An empty ref (`<url>#`) is a usage error.
- Without a ref, the scan uses the remote's default branch (`HEAD`).
- Commit SHAs aren't supported. An unknown ref fails the scan with an error.
- The same repository can appear more than once with different refs. The same repository with the same ref twice is a usage error (exit 2): `github.com/org/repo given twice`, or `github.com/org/repo @ v1 given twice` when a ref is set. The check compares the short form shown in the results, so `https://github.com/org/repo` and `git@github.com:org/repo.git` count as the same repository. It runs after the URLs are parsed. A URL that doesn't parse is an exit 1 error. With several URLs, that error starts with the bad URL.
- A result is 1 repository, or 1 organization that failed to list. An organization that lists 3 repositories gives 3 results.
- The tool downloads up to `--parallel` repositories at the same time and prints `Downloading <repo>[ @ <ref>]…` to stderr for each one. The TUI and the report list the repositories by name, not in command-line order. See [Order](#order).
- With several results, a repository that fails to download or scan doesn't stop the others. The tool records the failure for that repository and prints `skill-atlas: <repo>[ @ <ref>]: <error>` to stderr as it happens, before the TUI or report starts. Escape sequences in the error text are removed.
- When some repositories fail, the TUI or the report shows all of them, with the failed ones marked. Failed repositories sort by name with the rest. The tool then exits 1, after the TUI quits or the report opens, so scripts can tell that the result is incomplete.
- When every repository fails, the tool exits 1 and shows no TUI and writes no report file.
- With 1 result, a failure prints `skill-atlas: <error>` without the repository prefix and exits 1.
- In zsh with `extendedglob`, `#` starts a pattern, so quote a URL that has a ref: `'https://github.com/org/repo#v1'`.
- `--html` writes the results to an HTML file and opens it in the web browser instead of showing the TUI. See [HTML report](#html-report).
- `--exclude` skips `SKILL.md` files by path. The flag is repeatable and applies to every repository.
- Details are under [Excluded paths](#excluded-paths).
- `--parallel <n>` sets how many repositories download at the same time. The default is 4. `<n>` is an integer of 1 or more, with no upper limit. Both `--parallel <n>` and `--parallel=<n>` work. Any other value (0, a negative number, text or nothing) is a usage error and exits 2.
- The tool doesn't clone. It lists the remote refs over HTTPS with [go-git](https://github.com/go-git/go-git) (`ls-remote`) to find the commit of the ref. Then it downloads the tarball of that commit from `https://github.com/<owner>/<repo>/archive/<sha>.tar.gz`. The `git` binary isn't required.
- The scan doesn't need history, so the tarball of 1 commit is enough.
- An annotated tag resolves to the commit it points to.
- If the tarball names a different commit than the one resolved, the download fails with an error.
- A commit that is already in the [archive cache](#state) isn't downloaded again. The ref lookup with ls-remote still runs, so a branch that moved gets its new commit. The tool prints `Downloading <repo>…` in both cases.
- The results show the commit SHA that was scanned, per repository.
- Both steps go over HTTPS. Public repositories need no credentials. A [GitHub token](#github-token) lets the tool read private repositories and raises the API rate limit.
- Without a token that can read the repository, a private repository fails the same way as one that doesn't exist, whatever the URL form: `authentication failed for github.com/org/repo: the repository may be private or may not exist`.
- The download never prompts. Pressing <kbd>Ctrl+C</kbd> stops it.

### Organizations

`skill-atlas scan https://github.com/<org>` scans every repository of a GitHub organization.

- The URL is `https://github.com/<org>`, with or without a trailing `/`. The host check ignores letter case. The name follows the rules for an owner in a repository URL.
- An organization URL must use HTTPS. An SSH URL that names only an owner (`git@github.com:org`, `ssh://git@github.com/org`) is an error: `SSH URLs can't name an organization, use https://github.com/org for an organization`.
- An organization takes no ref. `https://github.com/org#v1` is a usage error (exit 2): `github.com/org is an organization, #v1 isn't supported`.
- The same organization twice is a usage error (exit 2): `github.com/org given twice`. The check ignores letter case.
- A repository URL whose owner is an organization on the command line is dropped without a message. This holds with or without a ref and in any argument order, and the owner check ignores letter case. The organization scan covers the repository at its default branch. The [duplicate check](#scan-command) runs after the drop.
- The ref, twice and drop rules run after the URLs are parsed, together with the duplicate check.
- Before the downloads start, the tool lists each organization, one at a time in command-line order. It prints `Listing repositories in github.com/<org>…` to stderr for each one.
- The list comes from the GitHub REST API, `GET https://api.github.com/orgs/<org>/repos`, sorted by full name. It includes forks and archived repositories.
- The request goes over HTTPS. Without a [GitHub token](#github-token) the list has only public repositories, and GitHub allows 60 requests per hour per IP address. With a token the list also has the private repositories the token can see, and the limit is 5,000 requests per hour. 1 request returns up to 100 repositories.
- Over the rate limit, the listing fails with `GitHub API rate limit exceeded, resets at 14:05`, in local time. When no token was sent, the message ends with `; set GITHUB_TOKEN to raise the limit`.
- The repositories download with the others, up to `--parallel` at a time. The TUI and the report show them as ordinary repositories in [name order](#order), with the names GitHub returns, e.g. `github.com/JetBrains/ideavim` for `https://github.com/jetbrains`.
- A listed repository without commits is left out of the results without a message. A repository without commits given by its own URL still fails with `repository github.com/org/repo is empty`.
- An organization that fails to list becomes 1 failed result, named `github.com/<org>` with no ref or SHA. The other URLs continue. Examples: `github.com/org isn't a GitHub organization or doesn't exist`, the rate limit error, or the HTTP status. A user account gives the first error too, because the API path is for organizations only.
- An organization with no repositories, or with only empty ones, fails with `no repositories found in github.com/<org>`.
- Pressing <kbd>Ctrl+C</kbd> during the listing stops the tool with exit 130.

### GitHub token

- The tool reads a token from the environment variable `GITHUB_TOKEN`. If that is unset or empty, it reads `GH_TOKEN`, which the `gh` CLI also uses. With neither, it sends no credentials.
- There is no flag for the token, because a flag shows up in `ps` and in shell history.
- The token goes to `github.com` and `api.github.com` only, and never to another host. It is sent on the ref lookup, the tarball download and the organization listing: as `Authorization: Bearer <token>` on the HTTP requests, and as HTTP basic auth (user `x-access-token`) on the ref lookup. The download redirects to `codeload.github.com`, a subdomain of `github.com`, which keeps the header. A redirect to any other host drops it.
- The tool never prints the token in an error, a warning or a log.

### Scan scope

The tarball holds the tracked files of 1 commit. It has no `.git/` directory.
The scan walks every file in it.
It has no ignore list of its own, since the tarball only contains tracked files.
Only the [excluded paths](#excluded-paths) are skipped.

Submodules aren't in the tarball, so the scan doesn't see them.
Files that the repository marks `export-ignore` in `.gitattributes` aren't in the tarball either.
The tool skips symlinks and hard links in the tarball. It writes only regular files and directories.
A tarball entry with an absolute path or a path that leaves the download directory fails the download.

A `SKILL.md` nested inside another skill's directory is a separate skill.

Each skill appears once, at its real path, and the scan never reads outside the download.

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
- An empty or malformed pattern is a usage error (exit 2), reported before the download starts.

Excluded files are counted and never parsed.
Symlinks are skipped and aren't counted.
The TUI and the HTML report show the count (see [TUI](#tui) and [HTML report](#html-report)).

Example: `skill-atlas scan --exclude integration-tests/ --exclude '**/fixtures' <git-url>`.

### State

The tool leaves 2 things behind: the archive cache and the HTML report file (see [Delivery](#delivery)).

The archive cache keeps the tarball of every commit the tool downloads, so the next scan of that commit doesn't download it again.

- The cache is a directory in the user cache directory:
  - Linux and other Unix systems: `$XDG_CACHE_HOME/skill-atlas/archives`, or `~/.cache/skill-atlas/archives` when `XDG_CACHE_HOME` isn't set.
  - macOS: `~/Library/Caches/skill-atlas/archives`.
  - Windows: `%LocalAppData%\skill-atlas\archives`.
- Each tarball is 1 file, `<sha>.tar.gz`, named by the full commit SHA. The SHA is the only key. The tarball's top-level directory is the only part that names the repository, and the tool strips it, so a fork at the same commit uses the same file.
- The tool unpacks the tarball while it arrives and writes it to a temporary file in the cache directory at the same time. Only after the tarball unpacked in full does the tool rename the file to `<sha>.tar.gz`. A failed or interrupted download leaves no file. 2 runs at the same time never read a partial file.
- A cached tarball unpacks with the same checks as a download, see [Scan scope](#scan-scope). If it fails them, for example because the file is truncated or names another commit, the tool deletes the file and the partial checkout and downloads the commit again.
- The tool updates a file's modification time each time it uses the file.
- The cache has no size or age limit. The tool deletes a file only when it fails the checks. To clear the cache, delete the directory.
- Only the owner can read the cache. The directory has mode 0700 and the files 0600.
- If the cache can't be used, for example because the directory can't be created or written, the scan still works and downloads every tarball. The tool prints `skill-atlas: warning: archive cache unavailable: <error>` to stderr, once per run.

The checkouts go into 1 temporary directory, with a subdirectory per repository.
The tool deletes each subdirectory as soon as its repository is scanned or has failed, so at most `--parallel` checkouts are on disk at once.
It deletes the temporary directory before it exits.

Results are discarded after the scan. `--html` keeps the report file in the OS temp directory.

### Order

The TUI and the HTML report list skills by name, A–Z by default.

- The sort key is the skill `name`, or the directory name when the skill has no usable `name`. The comparison ignores letter case. Skills with the same key sort by the exact name, then by path.
- With several repositories, the repositories sort by name first, then the skills within each repository. The name is the one in the headings, e.g. `github.com/org/a`, compared without letter case. The same repository with 2 refs sorts by ref. Failed repositories and repositories with no skills sort with the rest.
- Z–A is the A–Z order reversed, for the repositories and for the skills in each.
- The TUI switches the order with <kbd>s</kbd>, see [TUI](#tui). The report has a select, see [Sort](#sort).
- The order only affects the TUI and the report. The scan finds the files in path order, and stderr lines print as the downloads run.

## TUI

Split view: skill list on the left, details of the selected skill on the right.
This is the layout for 1 repository. See [Several repositories](#several-repositories-in-the-tui) for more.

```text
 github.com/org/repo @ main (a1b2c3d)        12 skills, 2 invalid
┌ Skills A–Z ──────────────┬ pdf-processing ───────────────────────┐
│   code-review            │ skills/pdf-processing/SKILL.md        │
│   data-analysis          │                                       │
│ > pdf-processing         │ Extract PDF text, fill forms, merge   │
│ ! PDF-Tool   [invalid]   │ files. Use when handling PDFs.        │
│                          │                                       │
│                          │ license: Apache-2.0                   │
└──────────────────────────┴───────────────────────────────────────┘
 j/k move · tab focus · / filter · s sort · q quit
```

- Header: repository URL, ref, short commit SHA, skill count, invalid count. When the scan excluded any `SKILL.md`, the counts also show the excluded count, e.g. `2 skills, 0 invalid, 2 excluded`. Without exclusions the header has no excluded count.
- List pane: skill `name`, with an `[invalid]` badge on invalid skills. The skills are in [name order](#order), A–Z at the start. The top border shows the order: `Skills A–Z` or `Skills Z–A`.
- Detail pane: path of the `SKILL.md` relative to the repository root, full `description`, other frontmatter fields (Claude Code fields under their own heading), and validation errors for invalid skills. Below that, the full Markdown body, rendered.
- <kbd>Tab</kbd> switches focus between the panes. j/k scroll the detail pane while it has focus.
- <kbd>/</kbd> filters the list by name, description and path. <kbd>Esc</kbd> clears the filter. While a filter is active, the top border reads `Skills N/M A–Z` (or `Z–A`), with N matching and M total.
- <kbd>s</kbd> switches the list between name A–Z and Z–A. It works with either pane focused, but not while typing a filter, where it is a letter. The selected skill stays selected, and the list scrolls to keep it in view. The filter stays active. The detail pane doesn't change.
- <kbd>q</kbd> or <kbd>Ctrl+C</kbd> quits.
- A scan with no skills shows `No skills found` in the list pane. When exclusions removed every `SKILL.md`, it shows `No skills found (2 excluded)`.
- The list shows the directory name when a skill has no usable `name`.
- Footer: key hints. While a filter is active they leave out `/ filter`, so they fit in 60 columns.

### Several repositories in the TUI

```text
 2 repositories                              15 skills, 2 invalid
┌ Skills A–Z ──────────────┬ pdf-processing ───────────────────────┐
│ github.com/org/a @ main… │ github.com/org/a @ main (a1b2c3d)     │
│   code-review            │ skills/pdf-processing/SKILL.md        │
│ > pdf-processing         │                                       │
│ github.com/org/b @ v1 (… │ Extract PDF text, fill forms, merge   │
│ ! PDF-Tool   [invalid]   │ files. Use when handling PDFs.        │
└──────────────────────────┴───────────────────────────────────────┘
```

- Header: `N repositories` on the left, bold. On the right, the totals over all repositories: `N skills, M invalid`, plus `, K excluded` when any repository excluded files and `, Z failed` when Z repositories failed, e.g. `15 skills, 2 invalid, 1 failed`.
- The list is grouped by repository, with the repositories in [name order](#order). Each group starts with a heading row, `<repo> @ <ref> (<short sha>)`. Headings can't be selected. The cursor skips them.
- <kbd>s</kbd> reverses the order of the groups and of the skills in each group. Empty and failed repositories move with their headings.
- When the cursor is on the first skill of a group, the list scrolls to show the heading too, if it fits.
- A repository with no skills shows its heading and a dim `No skills found` row. With exclusions the row reads `No skills found (2 excluded)`. The row isn't shown while a filter is active.
- A repository that failed shows its heading and, under it, a row with the error message in the warning style, e.g. `authentication failed for github.com/org/b`. The heading has no `(<short sha>)` when the download did not finish. An organization that failed to list shows the same way, with the heading `github.com/<org>`. The error row can't be selected and the cursor skips it. Like the `No skills found` row it is not shown while a filter is active. A message longer than the list pane is cut with `…`. The full message is on stderr. Escape sequences in it are removed.
- The filter also matches the repository name. A heading shows only while a skill in its group matches. `Skills N/M` counts skills only.
- The first line of the detail pane is the dim repository label, then the skill path.
- The minimum terminal size stays 60x12.

## HTML report

`skill-atlas scan --html <git-url>` writes the results to an HTML file and opens it in the browser instead of showing the TUI.
It needs no terminal. It combines with `--exclude`: the page lists the skills that remain and counts the excluded files.

The page has the same information as the TUI:

- Header: repository URL, ref, short commit SHA, skill count, invalid count. With several repositories, see [Several repositories](#several-repositories-in-the-report). The full commit SHA shows as hover text on the short one. When `--exclude` skipped files, the counts end with `, K excluded`: `5 skills, 1 invalid, 1 excluded`. Without exclusions the page omits it.
- Contents: a list that links to each skill, in [name order](#order) A–Z. Invalid skills have an `invalid` badge.
- Skill sections: `name`, path of the `SKILL.md` relative to the repository root, full `description`, other frontmatter fields (Claude Code fields under their own `Claude Code` heading, like the TUI detail pane), and validation errors for invalid skills. Below that, the full Markdown body, rendered.
- A scan with no skills shows `No skills found`. When exclusions removed every `SKILL.md`, it shows `No skills found (2 excluded)`.
- The list shows the directory name when a skill has no usable `name`.
- A filter box narrows the page to matching skills. See [Filter](#filter).
- A select next to the filter box sets the order. See [Sort](#sort).

### Several repositories in the report

- The `<h1>` reads `N repositories`, followed by the totals line. There is no single ref or SHA line.
- Contents are grouped by repository, with the repositories in [name order](#order). Each group has a heading with the repository name, ref, short SHA (the full SHA is the hover text) and its own counts, e.g. `2 skills, 1 invalid, 1 excluded`.
- A repository with no skills shows `No skills found` (or `No skills found (2 excluded)`) in the contents. It has no skill sections.
- A repository that failed shows in the contents and as a section group, each with the heading (`failed` in place of the counts) and the error message as a note. The heading has no short SHA when the download did not finish. An organization that failed to list shows the same way, with the heading `github.com/<org>`. Like the empty-repository note, the note hides while a filter is active. The message is escaped and has no escape sequences.
- The totals line ends with `, Z failed` when Z repositories failed.
- Skill sections sit under a heading per repository, in the same order as the contents. Section ids are `repo-<R>-skill-<N>`, counted from 1 in A–Z order. With 1 repository the ids stay `skill-<N>`.
- Heading levels nest: the page title is `h1`, repository headings are `h2` and skill names `h3`. Headings in a body keep the same offset below the skill name as with 1 repository. Levels stop at `h6`.
- The filter also matches the repository name. A repository group hides when none of its skills match. `Skills N/M` counts the skills in all repositories.
- The page still has 1 script, for the filter and the sort.

### Filter

The box matches the TUI <kbd>/</kbd> filter.

- It keeps the skills whose name (or directory name), description or path contains the typed text. The match ignores case.
- The contents list and the skill sections both narrow.
- While the box has text, a line shows `Skills N/M`, with N matching and M total. At 0 matches the page shows `No matching skills`.
- <kbd>/</kbd> focuses the box unless focus is already in a text field or the sort select. <kbd>Esc</kbd> clears the box and restores every skill.
- The filter is for reading. It doesn't change the report file. The [sort](#sort) sets the order.
- Without JavaScript the box is hidden and the page shows every skill.

A text box can't be filtered with CSS alone. CSS selectors see the `value` attribute, which doesn't change while the user types, and `:placeholder-shown` only tells empty from non-empty. A script is the smallest way to read the typed text.

### Sort

A select next to the filter box sets the order: `Name A–Z` (the default) or `Name Z–A`. Its accessible name is `Sort skills`.

- The tool writes the page in A–Z order, so section ids count in that order. The links in the contents don't change with the order.
- `Name Z–A` reverses the contents entries and the skill sections. With several repositories it also reverses the repository groups, in the contents and in the sections. `Name A–Z` puts the page back in the written order.
- The sort and the filter combine. Changing the order keeps the filter, and `Skills N/M` stays the same.
- The sort is for reading. It doesn't change the report file.
- Without JavaScript the select is hidden with the filter box, and the page shows A–Z.

### Page

- The page is 1 HTML document with inline CSS. It makes no external requests: no remote fonts, scripts, styles or images.
- The page has 1 inline script, which runs the [filter](#filter) and the [sort](#sort). It reads the typed text, the selected order and the `data-match` attribute of each skill. It toggles the `hidden` attribute, sets text with `textContent` and moves existing elements to reverse their order. It makes no network calls and uses no `eval` or `innerHTML`. It never writes skill content into the page. A page with no skills has no script and no select.
- A Content Security Policy enforces this. With skills: `default-src 'none'; script-src 'sha256-<hash>'; style-src 'unsafe-inline'; img-src data:`. The tool computes the hash from the exact script text. The policy has no `'unsafe-inline'` for scripts, so the browser blocks any other script. Without skills the policy has no `script-src`.
- Colors follow the browser's light or dark setting. The layout fits a phone screen.
- The Markdown body uses GitHub Flavored Markdown. Headings in the body sit 2 levels below the skill `name`.
- Raw HTML in a body is dropped.
- A link with a scheme other than `http`, `https` or `mailto` shows as plain text. This covers `javascript:`, `vbscript:`, `data:` and `file:`.
- Images aren't loaded. The page shows their alt text.

### Delivery

After the scan, and after the tool deletes the checkouts, it writes the page to a new file in the OS temp directory. The file is named `skill-atlas-report-<random>.html` and is readable by its owner only.

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
- 1: a repository failed to download or scan, an organization failed to list, or the tool couldn't write the report file. With several repositories, the tool still shows the ones that worked (TUI or report) and exits 1 afterwards. If every repository failed, there is no TUI and no report file.
- 2: usage error, including the same repository and ref given twice, an organization with a ref and the same organization twice.
- 130: interrupted with <kbd>Ctrl+C</kbd> or `SIGTERM` during the organization listing or the downloads.
