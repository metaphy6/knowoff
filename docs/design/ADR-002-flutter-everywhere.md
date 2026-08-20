# ADR-002: Flutter everywhere

## Status

Accepted

## Context

The client must ship on:

- Android native (launch target)
- iOS native (fast-follow once retention is proven)
- Web PWA (app-less guest fallback; zero-install join path for Local Rooms via QR code)

We evaluated Flutter, separate native codebases per platform, and React Native.

## Decision

We will build one Flutter codebase for all three surfaces.

## Consequences

- **One UI/UX implementation** covers Android, iOS, and Web.
- **Native haptics** are available for Poke on mobile.
- **The Web PWA target surfaces browser limitations early** (networking, asset caching, vibration API differences) instead of bolting web support onto a native-only codebase later.
- **Build and CI complexity is centralized** around one toolchain and one set of lint/format rules.
- **Performance guardrails are required** because the same widgets must run on low-end Android devices and mid-range browsers.
