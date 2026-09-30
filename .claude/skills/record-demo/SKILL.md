---
name: record-demo
description: Record screenshots, GIFs and videos of skill-atlas, both the bubbletea TUI (with vhs) and the --html report (with Playwright and Chrome). Use this whenever the user wants a demo, screenshot, screen recording, GIF, MP4 or README/PR media of skill-atlas, its terminal UI or its HTML report, even if they don't say "demo". Examples are "show what the filter looks like", "add a GIF to the PR", "capture the dark mode page" and "record the new keybinding".
---

# Record demo

`scripts/demo.sh` builds skill-atlas from the current checkout, scans a demo repo and writes media to `docs/demo/`. Run it from the checkout you want to show. In a worktree it records that worktree's branch.

```shell
.claude/skills/record-demo/scripts/demo.sh          # TUI and HTML
.claude/skills/record-demo/scripts/demo.sh tui      # TUI only
.claude/skills/record-demo/scripts/demo.sh html     # HTML only
.claude/skills/record-demo/scripts/demo.sh tui my.tape other.tape   # custom tapes
```

A full run takes about 1 minute. Each mode clones the demo repo once.

## Output

| File                                              | What                                                    |
|---------------------------------------------------|---------------------------------------------------------|
| `tui-tour.gif`, `tui-tour.mp4`                    | Command typed, list, filter, detail scroll, quit (~17s) |
| `tui-list.png`                                    | List after load, first skill (invalid) selected         |
| `tui-filter.png`                                  | Filter kept, match's detail with the Claude Code group  |
| `tui-detail.png`                                  | Detail pane focused and scrolled                        |
| `html-light.png`, `html-dark.png`                 | Top of the report at 1280x800 in each color scheme      |
| `html-phone.png`                                  | 390x844 at 2x                                           |
| `html-filter.png`, `html-skill.png`               | Filter applied, then the first match's section          |
| `html-tour.gif`, `html-tour.mp4`                  | Scroll, filter, jump to the match, clear (~9s)          |

## Settings

Environment variables:

- `DEMO_URL`, `DEMO_REF`: repo and tag to scan. Default `https://github.com/zcaceres/skills.git` at `zoom@1.0.1`: 61 skills, 2 invalid, and several Claude Code fields.
- `DEMO_ARGS`: scan arguments, split on spaces. Replaces `<DEMO_URL>#<DEMO_REF>` in both modes, e.g. for several URLs.
- `DEMO_QUERY`: filter text, default `laconic`, a skill with 3 Claude Code fields.
- `DEMO_OUT`: output dir, default `<checkout>/docs/demo`.
- `DEMO_SRC`: checkout to build, default the current git root.
- `DEMO_BROWSER_CHANNEL`: Playwright channel, default `chrome`. Set it empty to use Playwright's bundled Chromium (`uvx playwright install chromium` first).

Pick the demo repo to fit the feature. zoom@1.0.1 covers invalid skills and Claude Code fields. JetBrains/koog `1.3.0` has test-data skills under `integration-tests/`, which suits `--exclude`. For a flag the scripts don't pass, write a custom tape (TUI) or edit `html()` in `demo.sh` (HTML).

## Prerequisites

`demo.sh` checks for these and prints the install command:

- vhs: `brew install vhs` (pulls in ttyd and ffmpeg)
- uv: runs `html.py`, which declares Playwright inline (PEP 723)
- Google Chrome
- Playwright's own ffmpeg build for video, installed by `html.py` on first run into `~/Library/Caches/ms-playwright`

## Check the result

Look at every PNG with the Read tool before reporting back. Check that:

- the header shows the expected counts, e.g. `61 skills, 2 invalid` for the default repo;
- the screenshot shows the thing the user asked about;
- nothing is cut off. The TUI needs at least 60x12 cells.

To check a video, grab a frame: `ffmpeg -v error -ss 5 -i docs/demo/tui-tour.mp4 -frames:v 1 /tmp/frame.png`, then Read it.

GIFs come out at about 3MB. GitHub renders images up to 10MB in READMEs and PRs. To shrink a GIF, lower `fps`/`scale` in `html.py`, or lower `Width`/`Height` in the tape.

## Custom TUI recordings

Copy `assets/tui-tour.tape`, edit the copy, and pass its path to `demo.sh tui`. Write tapes with the Write tool. A heredoc through the aliased `cat` on this machine injects ANSI codes. `demo.sh` fills these placeholders:

- `{{BIN}}`: dir holding the fresh binary. The hidden setup puts it on `PATH`.
- `{{CMD}}`: `skill-atlas scan <url>#<ref>`, or `skill-atlas scan <DEMO_ARGS>`.
- `{{OUT}}`: output dir. Quote paths: `Output "{{OUT}}/x.gif"`. vhs rejects unquoted absolute paths.
- `{{QUERY}}`: `DEMO_QUERY`.

vhs basics:

```
Output "{{OUT}}/name.gif"       # also .mp4, .webm; repeat for several
Set Width 1400 / Height 800     # pixels; keep the TUI above 60x12 cells
Type "text" / Type@120ms "text" # type with a per-key delay
Enter  Tab  Escape  Down@200ms 10
Sleep 1.5s
Hide / Show                     # stop and resume capture
Wait+Screen@120s /regex/        # wait until the screen matches
Screenshot "{{OUT}}/name.png"
```

`assets/multi-repo.tape` records a scan of several repos: list, the step from one repo's group into the next, a filter across them, and the exit code after quitting. Run it with 2 URLs, plus a repo that doesn't exist to show partial results:

```shell
DEMO_ARGS='https://github.com/JetBrains/ideavim.git#2.47.1 https://github.com/JetBrains/no-such-repo.git https://github.com/zcaceres/skills.git#zoom@1.0.1' \
DEMO_QUERY=docs .claude/skills/record-demo/scripts/demo.sh tui .claude/skills/record-demo/assets/multi-repo.tape
```

It writes `tui-multi.gif`, `tui-multi.mp4`, `tui-multi-list.png`, `tui-multi-boundary.png`, `tui-multi-filter.png` and `tui-multi-exit.png`. With the 3 URLs above the header should read `67 skills, 2 invalid, 1 failed` and the exit code is 1. With several repos the filter also matches the repo name, so a query like `git` matches every skill from `github.com`. `demo.sh html` records a partial result too: exit 1 with a `Report:` line counts as success.

Keep the `Hide` / `Wait+Screen` / `Show` block after the scan command. It waits for the header (`N skills, M invalid`) and cuts the clone time out of the recording. TUI keys: j/k or arrows move, Tab switches focus, `/` filters (Enter keeps, Esc clears), q quits.

## Custom HTML shots

Edit `scripts/html.py`. The page hooks:

- `#q`: filter input. Pressing `/` focuses it and Escape clears it.
- `#fcount`: "Skills N/M", visible while filtering.
- `#nomatch`: shown when nothing matches.
- `#toc li[data-match]`, `section.skill[data-match]`: hidden with the `hidden` attribute when filtered out. Use `:visible` in locators.
- `.badge`: the "invalid" label. `dl.fields`: frontmatter fields, with the Claude Code group as a nested `dl`.

Use `color_scheme="dark"` on the context for dark mode and `full_page=True` on `screenshot` for the whole page. The whole page is very tall with 61 skills.

## After recording

`docs/demo/` stays untracked. To put media in a PR, reference the local paths in the body and pass each file to `--attach` (gh 2.101.0 or later, push access needed). gh uploads them and rewrites the references:

```shell
gh pr edit <n> --body-file body.md --attach docs/demo/tui-multi.mp4 --attach docs/demo/tui-multi-list.png
```

Put a video's `![](docs/demo/x.mp4)` alone in its paragraph so it renders as a player. `gh pr create` and `gh pr comment` take `--attach` too.
