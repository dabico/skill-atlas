# Skill Atlas

Skill Atlas scans a remote Git repository for agent skills and shows them in a terminal UI or an HTML page.

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
skill-atlas scan [--html] [--ref <branch|tag>] [--exclude <pattern>]... <git-url>
```

- `<git-url>` is 1 remote URL over HTTPS or SSH. Local paths aren't supported.
- `--ref` picks a branch or tag. Without it, the scan uses the remote's default branch. Commit SHAs aren't supported.
- `--exclude` skips `SKILL.md` files that match a [gitignore pattern](https://git-scm.com/docs/gitignore#_pattern_format). Repeat it to add patterns.
- `--html` writes the results to an HTML file in the temp directory and opens it in the browser instead of the TUI.

Public HTTPS repositories need no credentials. SSH uses the SSH agent (`SSH_AUTH_SOCK`) and `~/.ssh/known_hosts`.
The clone is shallow, goes to a temporary directory, and is deleted before exit.

Examples:

```shell
skill-atlas scan https://github.com/anthropics/skills.git
skill-atlas scan --ref v3.1.0 https://github.com/obra/superpowers.git
skill-atlas scan --exclude integration-tests/ --exclude '**/fixtures' https://github.com/anthropics/skills.git
skill-atlas scan --html https://github.com/anthropics/skills.git
```

## Keys

| Key           | Action                                                                   |
|---------------|--------------------------------------------------------------------------|
| `j` / `k`     | move in the list, or scroll the detail pane when it has focus            |
| `Tab`         | switch focus between list and detail                                     |
| `/`           | filter by name, description and path (`Enter` keeps it, `Esc` clears it) |
| `q`, `Ctrl+C` | quit                                                                     |

The HTML page has a filter box that matches the same fields.

## Spec

The full spec is in [spec/cli.md](spec/cli.md).
