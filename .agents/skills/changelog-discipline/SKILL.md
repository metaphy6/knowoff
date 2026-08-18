---
name: changelog-discipline
description: "Changelog discipline. Every user-visible change. 'User' includes downstream developers consuming"
---

# Changelog discipline

## When to use

Every user-visible change. "User" includes downstream developers consuming
the public API.

## Procedure

1. **Keep a `CHANGELOG.md` at the repo root.** Format: [Keep a Changelog](https://keepachangelog.com/).
2. **Append under `## [Unreleased]`** in the same commit as the change.
   Categories: `Added`, `Changed`, `Deprecated`, `Removed`, `Fixed`,
   `Security`.
3. **Write for the consumer**, not the implementer. Example:
   - ❌ "Refactored `auth.go` to use the new context pattern."
   - ✅ "Auth tokens now expire after 1 hour (was 24h)."
4. **On release**: rename `[Unreleased]` to the version + date, open a new
   `[Unreleased]` section above it.
5. **Link to issues / PRs** where relevant: `(#42)`.

## Anti-patterns

- ❌ One vague "various improvements" line covering 20 commits.
- ❌ Auto-generated changelog from commit subjects with no curation.
- ❌ Entries describing internal refactors that don't affect the user.
- ❌ Forgetting to update `[Unreleased]` and remembering at release time (you won't).
- ❌ Security fixes mentioned only in commit messages, not in the changelog.
