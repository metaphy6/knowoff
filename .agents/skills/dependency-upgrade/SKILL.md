---
name: dependency-upgrade
description: "Dependency Upgrade. A dependency has a known CVE or security advisory."
---

# Dependency Upgrade

## When to use

- A dependency has a known CVE or security advisory.
- A minor or patch version bump is routine maintenance.
- A major version bump is required for a feature or compatibility reason.
- Running `make doctor` or a dependency audit tool reports outdated packages.

## Procedure

1. **Audit first** — know what you're changing before you change it.
   ```bash
   # Python
   pip list --outdated
   pip-audit      # or: safety check

   # Node
   npm outdated
   npm audit

   # Go
   go list -u -m all

   # Rust
   cargo outdated
   cargo audit
   ```

2. **Upgrade one dependency at a time** (or one related group) to keep
   diffs reviewable and bisect-friendly.

3. **Read the changelog / migration guide** for the version range you're
   crossing. Never skip this step for major bumps.

4. **Pin the new version** in the manifest (`requirements.txt`, `package.json`,
   `go.mod`, `Cargo.toml`). Avoid range specifiers like `>=` for production
   deps — prefer exact pins or `~=` (Python compatible release).

5. **Run the full test suite** (not just the smoke test). A dependency upgrade
   is a behaviour-changing commit and requires green tests.

6. **Commit the lock file** alongside the manifest change in the same commit.
   Split separate upgrades into separate commits.

7. **Add a tracking row:**
   ```bash
   make track.add ACTION=commit STATUS=completed \
     SUMMARY="chore(deps): upgrade <pkg> from <old> to <new>"
   ```

## Vendor-tag overlays

claude: |
  Claude: after upgrading, run `make test` and paste the full output before
  declaring success. Do not guess that tests pass.

gpt: |
  GPT: do not batch-upgrade all outdated deps in a single commit — upgrade
  one or one related group at a time.

## Anti-patterns

- ❌ Upgrading all dependencies at once — one change, one diff, one bisect target.
- ❌ Skipping the test run after upgrade — "it's just a patch bump" is how
  regressions slip in.
- ❌ Committing without updating the lock file.
- ❌ Ignoring breaking changes in CHANGELOG — especially for zero-major versions
  (0.x → 0.y can be breaking).
- ❌ Applying a security upgrade in the same commit as feature work.
