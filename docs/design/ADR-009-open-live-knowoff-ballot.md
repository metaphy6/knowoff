# 📐 ADR-009: Open, live Knowoff ballot (replaces the blind-simultaneous ballot)

- **Status**: accepted
- **Date**: 2026-09-01
- **Deciders**: @product-owner (via chat request)

## Context

The Knowoff ballot (Rules §4) was blind-simultaneous: every seat's `cast_vote`
was collected privately in `m.ballots` and only revealed to the table once
`resolveBallot` ran, at which point the vote could no longer change. The
product owner asked for a live "X votes Y" feed on the voting screen —
watching accusations land in real time is more dramatic and more social than
a silent countdown — and to be able to change a cast vote for as long as the
window stays open.

## Decision

The Knowoff/runoff ballot is now **open and live**: `handleCastVote` accepts a
change of target at any time before the ballot resolves, and every accepted
cast broadcasts a new `vote_cast` event `{seat, target_seat}` to the whole
table immediately (`transport.EventVoteCast`). The client folds this into the
ballot itself rather than a separate feed: each candidate's own row pops an
animated, named chip for every voter as their vote lands (and briefly flashes),
in addition to the existing post-resolution tally/ballots reveal.

Casting a vote no longer counts as being done with the ballot. The ballot and
any runoff now behave exactly like Discussion and the Result window: casting
never auto-resolves it, only every active connected seat marking **Ready**
does (or the full window timer). A bot readies itself immediately after
casting — it never wants to reconsider — but a human's cast leaves the ballot
open for the whole window (or until they and everyone else Ready) so a change
of mind has somewhere to land.

## Consequences

- ➕ Matches the requested UX: votes (and vote changes) are visible the
  instant they land, with real room to reconsider before the ballot resolves.
- ➕ Minimal server surface: one new event, no new intent — `cast_vote` just
  drops its old "already voted" rejection.
- ➖ Strategic blind-voting is gone: a player can now react to (or copy,
  or pile onto) votes already cast this round, which changes match dynamics
  from the original blind-simultaneous design.
- ➖ A cast vote no longer auto-completes the ballot the moment everyone has
  voted once (that used to leave the *last* voter zero chance to reconsider
  their own cast — the bug this follow-up fixes). Bots compensate by
  Ready-ing themselves right after casting, so an all-bot table still moves
  at the old pace; a table with a human who doesn't Ready now waits out the
  full ballot timer, same as an undecided Discussion or Result window.
- 🔁 Reversible: reinstate the "already voted" rejection and the
  auto-resolve-on-complete call in `handleCastVote`, stop broadcasting
  `vote_cast`, and drop the in-row chip / live ballots map client-side to
  fully revert to the blind-simultaneous ballot.

## Considered options

- **Open, live ballot with revote, gated on Ready** (chosen) — matches the
  requested real-time "who's voting for whom" feature and an actually usable
  revote window, smallest server surface.
- **Blind casting, but reveal partial progress (e.g. a bare vote count)** —
  keeps fairness but doesn't satisfy "who votes for whom"; still a spec
  change (the ballot is meant to stay silent until close) for a
  half-measure.
- **Keep the ballot fully blind, only animate the reveal after close** —
  zero fairness impact, but does not satisfy the explicit ask for a *live*,
  *during-casting* feed with the ability to change a vote mid-window.
- **Open/live but keep auto-resolving on vote completeness** — what shipped
  first; rejected once live testing showed it gave the ballot's last voter
  (frequently the only human at a bot table) zero chance to ever use the
  revote feature they'd just been given.
