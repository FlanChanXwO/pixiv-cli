---
name: pixiv-cli-commit-message
description: Write a pixiv-cli commit message grounded in the staged diff. Use when asked to name or prepare a commit, not to stage, commit, push, merge, or publish automatically.
---

# Write a pixiv-cli Commit Message

Inspect `git diff --cached --stat` and `git diff --cached`. If nothing is staged, identify the requested diff before writing; do not stage unrelated files. Describe the actual outcome in concise English, using the repository's recent commit style and a suitable type such as `fix`, `feat`, `docs`, or `refactor`.

Use an imperative subject. Add a body only for non-obvious motivation, compatibility, or consequences. Do not claim tests passed unless their results were observed, list unchanged non-features, or invent issue numbers. Mention breaking behavior only when the diff really changes a supported contract.

Return the message. A request for wording alone does not authorize creating the commit or pushing it.
