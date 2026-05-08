---
name: documentation-editor
description: Use when applying documentation feedback to markdown, MDX, Mintlify, README, API reference, quickstart, or integration guide files.
---

# Documentation editor workflow

Use this skill whenever the user supplies docs plus feedback, gap reports, reviewer comments, or asks you to rewrite docs based on feedback.

## Checklist

1. Confirm you have docs source and actionable feedback.
2. Inspect the file tree and read all files you plan to edit.
3. Map each feedback item to an edit decision:
   - apply now,
   - needs source truth,
   - not applicable,
   - defer / needs owner decision.
4. Preserve existing docs conventions:
   - Mintlify frontmatter and components,
   - navigation structure,
   - heading hierarchy,
   - link and code sample style,
   - product terminology.
5. Make minimal, targeted edits using exact replacements where possible. For remote-editor bundle workflows, copy the extracted input files to `/workspace/updated-docs/files` and edit that output tree only.
6. For implementation docs, ensure edits cover:
   - prerequisites and install,
   - auth and env vars,
   - exact APIs/SDK/CLI commands,
   - request/response schemas,
   - complete examples,
   - deployment/configuration sequence,
   - verification/smoke tests,
   - troubleshooting and edge cases.
7. Validate with repo-provided docs commands if available; otherwise run sanity checks such as `git diff --check`, link/placeholder searches, and duplicate-heading checks.
8. Final response must map feedback items to changed files and list unresolved items.

## Editing principles

- Do not fabricate facts. If a value, endpoint, field name, request payload, response shape, status, limit, scope, or command flag is unknown, either derive it from supplied source truth or flag it as unresolved. Never make a sample appear runnable when a required schema is only guessed.
- Avoid large rewrites when surgical changes resolve the issue.
- Prefer copy-pasteable examples with expected success and failure signals.
- Keep credential handling safe: refer to env var names and secret stores, never raw secrets.
- Call out any assumptions clearly in review notes.
