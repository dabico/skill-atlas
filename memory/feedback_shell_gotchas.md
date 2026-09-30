---
name: Shell gotchas on this machine
description: cat is aliased to a highlighter and injects ANSI codes; sed is GNU sed
type: feedback
---

`cat` is aliased to a syntax highlighter. A heredoc through `cat > file` writes ANSI escape codes into the file.

**Why:** it happened 4 times on 2026-09-30. It broke `internal/repo/url_test.go` (`illegal character U+001B`) and `internal/skill/claude.go`, and left PR #4's description and `README.md` unreadable. The em-dash check on the PR body didn't catch it.

**How to apply:**
- Write files with the Write or Edit tools. Tell subagents the same.
- After writing a PR body or doc, run `grep -c $'\x1b' <file>` and expect 0.
- `sed` is GNU sed: use `sed -i`, not `sed -i ''`.
- Shell commands go through the `rtk` proxy, which trims output. Use `rtk proxy <cmd>` for raw output.
- gh 2.101.0 has `--attach` on `gh pr create/edit/comment`: it uploads images and videos and rewrites local references in the body. Claude wrongly told the maintainer gh couldn't attach files; the maintainer pointed to the docs. Check `gh <cmd> --help` before claiming gh can't do something.
