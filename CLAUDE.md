# Conventions for AI-assisted work in this repo

## Language
- Commit messages: English.
- Pull request titles and bodies: English.
- Conversation with the user can stay in Japanese.

## Pull request bodies
- Write a concise prose summary of what changed and why.
- Do NOT use templated headings such as `## Summary`, `## Test plan`, `## Changes`, etc. The user dislikes that format.
- A few lines or short bullets are fine when they help; no boilerplate.

## Commits and branches
- Split unrelated work into separate PRs (e.g., a rename and a new feature go in different PRs even if touched in the same session).
- Never commit backup output directories (`<channelID>-<name>/`, `admin/`, etc. produced by the `backup` command).
