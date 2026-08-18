---
name: security-by-default
description: "Security by default. Any code that touches: user input, external services, file paths, shell"
---

# Security by default

## When to use

Any code that touches: user input, external services, file paths, shell
commands, SQL, HTML, secrets, auth, network sockets.

## Procedure

Run through the OWASP Top 10 reflexes for every change:

1. **Injection.** Parameterised queries; never `format`/`+` a query string
   from user input. Same for shell commands — use `argv`, not strings.
2. **Auth & session.** Validate tokens on every request, not just at entry.
   Compare secrets with constant-time comparison.
3. **Sensitive data.** Don't log secrets, tokens, PII. Don't paste them in
   chat. Scrub before uploading.
4. **XSS / HTML injection.** Escape by default in the template engine, not
   by remembering at each call site.
5. **Access control.** Authorize on the action, not just the route.
6. **Misconfiguration.** Defaults must be safe. New flags default to the
   *more restrictive* option.
7. **Vulnerable deps.** No new dep without a quick check of its CVE history
   and maintenance status.
8. **SSRF / open redirects.** Validate URLs and IP ranges; deny by default.
9. **Logging gaps.** A failed auth attempt must produce a log line.
10. **Software supply chain.** Pin versions; verify hashes for binaries.

## Anti-patterns

- ❌ `eval(user_input)` or any moral equivalent.
- ❌ "I'll add input validation in a follow-up."
- ❌ Catching an auth failure and returning 200.
- ❌ Trusting a header you didn't put there yourself.
- ❌ Generating or guessing a URL / package name — look it up.
- ❌ Running untrusted code returned by an LLM tool without auditing it.
