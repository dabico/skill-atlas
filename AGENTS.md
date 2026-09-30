# Skill Atlas

Go CLI that scans a remote Git repository for agent skills (`SKILL.md`) and shows them in a TUI or an HTML report.
`spec/cli.md` defines the behavior. When behavior changes, update the spec in the same PR.

## Commands

```shell
go run ./cmd/skill-atlas scan <git-url>             # run locally
go build ./...
go test -short -race ./...                          # unit tests, no network
go test -race -count=1 ./internal/repo/...          # clone tests against GitHub
go test -tags e2e -count=1 -timeout 10m ./e2e/...   # needs tmux
```

- `-update` on the e2e run rewrites the golden files in `e2e/testdata/`.
- `SKILL_ATLAS_E2E_HEAD=1` adds tests against each fixture's HEAD.
- Lint, as in CI: `gofmt -l .`, `go vet ./...`, `go vet -tags e2e ./...`, `go mod tidy -diff`, `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12`.

## Layout

- `cmd/skill-atlas`: flags, wiring, exit codes (0 ok, 1 failure, 2 usage, 130 interrupted).
- `internal/repo`: URL parsing and the go-git clone.
- `internal/scan`: walks the checkout for `SKILL.md` files and applies `--exclude`.
- `internal/skill`: frontmatter parsing and validation. `claude.go` holds the Claude Code field rules.
- `internal/tui`: bubbletea UI.
- `internal/htmlreport`: the `--html` page. The CSP hash is computed from the embedded `filter.js`.
- `e2e/`: build tag `e2e`, fixtures pinned to a tag and commit SHA.

## Architecture rules

- Clone with go-git. Don't shell out to `git`.
- Store no state. The clone goes to a temp dir that is deleted before exit. The `--html` report file is the only thing left behind.
- Validation follows the skills-ref reference validator. Fields outside the spec and the Claude Code table make a skill invalid.
- Provider-specific fields go in a rule table like `internal/skill/claude.go` and show under the provider's heading in the TUI and the HTML report.
- The HTML page has exactly 1 script, the filter. Don't add scripts or `unsafe-inline`.
- E2E fixtures pin a tag and its commit SHA. The baseline is JetBrains/ideavim `2.47.1`.

## Writing

- Use the anti-ai-writing-style skill for the spec, PR descriptions, README and docs.

## Workflow

- Build features with subagents, each in its own worktree under `.claude/worktrees/`.
- Open a PR against `main` and wait for CI to pass. On failure, read `gh run view <id> --log-failed`, fix and push.
- The maintainer reviews PRs. Skip Claude-side review unless asked.
- Never merge without the maintainer's explicit OK.
- Never force-push. Resolve conflicts by merging `origin/main` into the branch.
- After a merge, delete the worktree and branch once the local head matches the PR's final SHA.
- Don't commit `.idea/`, `bin/` or `README.md` unless asked. `memory/` follows the Memory rules below.

## Memory

- `memory/` holds Claude's own notes: observations, decision history, open items and session logs. Read `memory/MEMORY.md` at the start of a session.
- Instructions belong in this file. Claude's notes belong in `memory/`.
- Each memory is 1 file with `name`, `description` and `type` frontmatter (`user`, `feedback`, `project` or `reference`), listed in `memory/MEMORY.md` with a one-line summary.
- Add 1 file per conversation to `memory/sessions/`, named by date.
- Memory edits made while working on a feature go in that feature's worktree and are committed with its PR. Leave the main checkout's `memory/` unchanged meanwhile, so the merge doesn't conflict.
