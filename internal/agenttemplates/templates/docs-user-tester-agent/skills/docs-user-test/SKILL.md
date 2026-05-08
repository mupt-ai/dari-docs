---
name: docs-user-test
description: Use when simulating a developer trying to complete a task from supplied docs and reporting brief user-style feedback.
---

# Docs user test

1. Extract/read the docs supplied by the user.
2. Search for the exact task terms and likely setup/auth/API pages.
3. Try to perform the task in `/workspace/attempt`.
4. Use runtime credentials only if provided through environment variables or `DARI_DOCS_RUNTIME_SECRETS_JSON`; never print values.
5. Stop at the first real blocker if continuing would require guessing product behavior.
6. Final feedback should be short:
   - Tried
   - Result
   - Got stuck on
   - Docs feedback
   - Artifacts

Avoid scoring, matrices, and broad audits.
