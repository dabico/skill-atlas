---
name: Provider field research
description: Frontmatter fields defined by each agent tool, with sources, in research/provider-fields.json
type: reference
---

`research/provider-fields.json` lists skill frontmatter fields by the tools that define them: Claude Code, Codex, VS Code (GitHub Copilot), Cursor, Factory and others. Each entry has the field, value type, constraints, meaning, source URLs and a confidence rating.

Collected by a research workflow on 2026-09-30. Claude Code fields are already implemented. Use this file as the starting point before adding another provider, and check the sources again first because agent docs change often.

Codex keeps its extra fields in `agents/openai.yaml` next to `SKILL.md`, outside the frontmatter.
