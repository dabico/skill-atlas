# Skill Atlas

Skill Atlas scans 1 or more remote Git repositories for agent skills and shows them in a terminal UI or an HTML page.

A skill is a directory with a `SKILL.md` file, as defined by the [Agent Skills specification](https://agentskills.io/specification).
Skills that break the spec are listed as invalid, with every rule they break.
Skill Atlas also reads the [Claude Code skill fields](https://code.claude.com/docs/en/skills),
such as `disable-model-invocation` and `argument-hint`, and shows them under a `Claude Code` heading.

## Build

```shell
go install ./cmd/skill-atlas
# or
go build -o skill-atlas ./cmd/skill-atlas
```

Needs Go 1.27. The `git` binary isn't needed.

## Usage

```shell
skill-atlas scan [--html] [--exclude <pattern>]... [--parallel <n>] <git-url>[#<ref>]...
```

- `<git-url>` is a remote URL over HTTPS or SSH. Pass several to scan them together. Local paths aren't supported.
- `#<ref>` after a URL picks a branch or tag for that URL. Without it, the scan uses the remote's default branch. Commit SHAs aren't supported. In zsh with `extendedglob`, quote a URL that has a `#`.
- `--exclude` skips `SKILL.md` files that match a [gitignore pattern](https://git-scm.com/docs/gitignore#_pattern_format). Repeat it to add patterns. It applies to every repository.
- `--html` writes the results to an HTML file in the temp directory and opens it in the browser instead of the TUI.
- `--parallel` sets how many repositories clone at once. The default is 4.

With several repositories, the TUI and the HTML page group skills by repository.
A repository that fails to clone or scan doesn't stop the others: its error goes to stderr, the results show the rest with the failed one marked, and the exit code is 1.
If every repository fails, nothing opens and the exit code is 1.

Public HTTPS repositories need no credentials. SSH uses the SSH agent (`SSH_AUTH_SOCK`) and `~/.ssh/known_hosts`.
Each clone is shallow, goes to a temporary directory, and is deleted before exit.

Examples:

```shell
skill-atlas scan https://github.com/anthropics/skills.git
skill-atlas scan 'https://github.com/obra/superpowers.git#v3.1.0'
skill-atlas scan --exclude integration-tests/ --exclude '**/fixtures' https://github.com/anthropics/skills.git
skill-atlas scan --html https://github.com/anthropics/skills.git
skill-atlas scan https://github.com/anthropics/skills.git 'https://github.com/obra/superpowers.git#v3.1.0'
```

## Keys

| Key           | Action                                                                   |
|---------------|--------------------------------------------------------------------------|
| `j` / `k`     | move in the list, or scroll the detail pane when it has focus            |
| `Tab`         | switch focus between list and detail                                     |
| `/`           | filter by name, description and path (`Enter` keeps it, `Esc` clears it) |
| `q`, `Ctrl+C` | quit                                                                     |

The HTML page has a filter box that matches the same fields.
With several repositories, both filters also match the repository name.

## Spec

The full spec is in [spec/cli.md](spec/cli.md).
