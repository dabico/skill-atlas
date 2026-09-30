---
name: How the maintainer works
description: Observed habits of the maintainer when planning, reviewing and changing scope
type: user
---

- During spec work they wanted 1 question at a time, each with a recommendation. They often answer "go with your recommendation". "Enough grilling" means stop asking and build.
- They test builds on real repos (Kotlin, MPS) and bring back concrete failures.
- They review PRs themselves and merge with squash.
- They change scope mid-run: stopped the koog/MPS e2e for ideavim, reworked PRs #1 and #2 after review, cancelled the Codex stack after 3 minutes.

**How to apply:** say the plan in 2 or 3 lines before a long run, so they can redirect early. Keep each PR small enough to drop without losing other work.
