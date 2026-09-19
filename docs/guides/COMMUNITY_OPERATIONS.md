# Community operations

Knowoff supports text contributions, moderated release operations and the Weekly
Nown Challenge. The player interface is Flutter; Contributor Studio is web-only
and the Admin Console remains on a separate internal listener. The
[Blueprint](../../BLUEPRINT.md) defines the five text modes. The
[Roadmap](../planning/ROADMAP.md) and
[resumption evidence](../reports/2026-09-12-text-transition-resumption.md)
record remaining production, provider, policy and human release gates. A working
contribution or release form does not authorize a real pack or deployment.

## Open the studio

1. In the game, open **Profile → Contributor Portal → Open portal**.
2. The browser displays a short-lived pairing code. Enter it back in the game
   and explicitly confirm that you initiated this connection.
3. Return to that same browser and choose **Continue**. Pairing expires after
   five minutes and establishes an eight-hour HttpOnly session.
4. Open **Roles & applications** to see your account's eligibility and apply.
   An administrator reviews the request. Curator includes Contributor access;
   Guard is an independent safety role with timeboxed, audited freeze tools.

The browser must use the server origin configured in the game. Do not paste
access tokens into URLs. Signing out revokes the browser session; the server
also checks bans, deletion and active freezes on browser requests.

## Admin setup and the text workflow

Use the existing [local admin seeding instructions](DEV_CREDENTIALS.md) for a
development account. There is no default admin login. Open `/admin/` on the
internal admin listener and sign in with password and TOTP. The public listener
does not expose these administration pages.

Before collecting real contributions, publish the actual contribution terms
through **Contribution terms**, using a new version and activation time. The
bootstrap does not manufacture owner-authored terms. Do not treat historical
placeholder text as a reviewed legal document or rewrite accepted versions. The latest
effective stored version is shown to players and checked when they consent.

Configure [text screening](CHAT_MODERATION.md#contributor-and-challenge-text-screening)
before approval or challenge-topic publication. It is disabled by default, so
drafting, applications, queues and rejection work without an external key;
approval pauses if screening is unavailable or flags the text. Provider calls
are server-only. Live provider credentials are an operator setup step.

A contributor accepts the displayed terms, saves a private text draft, edits
it, then submits it. Submission uses a daily queue slot and locks the text.
Withdrawal returns it to draft; resubmission uses another slot. Staff review
excludes private drafts. Approval binds to the text actually shown on the
review form, requires human confirmation and automated screening, and records
the decision, profile credit and reward atomically. Rejection requires a reason.

Approval means accepted for curation. The text release workflow captures the
exact accepted source, builds and certifies an immutable bundle, then separately
publishes, activates or takes it down. Capture and publication preserve original
consent and credits and never pay the approval reward again. Release operations
require their own exact administrator session and audit. Missing screening or
required evidence refuses visibility. See [content/dealing](../code/MODULE-media-engine.md)
and [CLI usage](../../tools/README.md). The retired portal simulator returns 410;
use the supported text tooling for reviewed simulation input.

## Prepare the humor pilot

Follow the adopted [Humor development guide](../../content/humor-development.md)
and [Curator guide](../../content/curator-guide.md): start with bounded text
pilots for the selected modes, with at least three themes and two cultural/
language pilots to test transferability. This is not a commitment to launch
both languages. Draft situations/plans/criteria and reusable response/item
pools, then have human editors select and culturally rewrite the candidates.
Illustrative examples are drafts, not submitted, certified or deployed cards.

Keep human situation, comic mechanism, cultural reach, shelf life and
accessibility in editorial planning records alongside source links and human
decisions. These fields are not implemented in the text studio. Every eventual
Nown/card still needs exactly one of the existing four tone buckets; neither
these dimensions nor the experimental 70/20/10 freshness mix changes dealing.
The mix is a release-level hypothesis to refine using feedback and reuse data.

For topical candidates, record source, observation date, intended
regions/languages, one-sentence context, review date and expiry date. A human
editor reviews them weekly and at expiry and decides whether to retain, rewrite
or retire them. This process has no configured scheduler or automatic removal
from deployed packs.

Before release, combine automated screening with human checks for sources,
rights, originality, age suitability and local meaning, then playtest actual
4- and 6-player hands. Record recognition, laughter, alternative explanations
and references that need explaining. Deal simulation and certification must
pass separately: a funny card must also preserve ambiguity around Nown.
Each mode also needs complete 2/3-round schedules and its changing public
board: response choices, ratings, removed/added bag items, offers and comparison
chains. Evaluate draw pressure and already-public card knowledge after trades.
Prompt cosine similarity alone is not that proof; any optional text embeddings
must declare compatible model/version evidence.
Record editorial decisions in the planning record and submission decisions in
the existing review workflow; do not duplicate approval state in a separate
calendar. A successful pilot still needs a built, certified, published and
activated pack before its content can appear in matches.

## Run a text challenge

In **Weekly challenge**, choose an approved text submission as the topic and a
Monday start date. Topic publication rescreens even a legacy-approved source.
The active week spans Monday 00:00 through the start of the following Monday,
in UTC. The first configured number of entries reserves intake slots before
review (100 by default). Rejection releases capacity; a player's entry remains
immutable and cannot be replaced.

Players open **Weekly Nown Challenge** from Home to see the topic, public
approved entries, their own review status and the current terms. Only approved
entries become public and votable. Players have one final vote, cannot vote
for themselves, and cannot vote after closure. The scheduler closes expired
weeks and supports restart-safe catch-up; administrators can also close a week.
Tied vote totals choose the earliest accepted entry, then immutable ID. Closing
more than once credits the recorded winner only once;
the full configured reward is separate from the daily gameplay cap.

The current week's result stays readable after closure. Weekly rollover moves
the current-winner title and preserves its history without a duplicate payout.
A missing next approved topic remains unavailable; the scheduler does not invent
content or approve submissions.

## Other available operations

The console links to role/application review, versioned terms, text review,
challenge review, notices, report and feedback triage, and account wallet and
entitlement lookup. Triage records supported status changes with an audit; it
does not claim that closing a report removes media or bans an account.

Guards can freeze an account within their independent, timeboxed authority;
overlapping freezes remain separate and expire independently. Administrators
make final decisions. Avatar upload/activation requires the account's unlock,
automated screening and validated non-playable WebP output; provider refusal
preserves the prior avatar and entitlement. Administrative takedown remains
separate from a report's status.

The account wallet view exposes Admin-only Noin grants and full original-spend
refunds through `/admin/economy/corrections`. Each request has an immutable ID
and reason; its decision, audit, wallet credit, ledger entry and result commit
together. A refund derives its amount from one same-account spend ledger row
and can apply only once. These controls do not refund platform payments or
change entitlements, gameplay caps, points or XP. A repeated request returns
its original receipt; changed input under that ID refuses.

Administrators can sanction an account permanently or until an explicit expiry,
optionally including its known app installations. Every decision records its
reason and exact captured scope; lifting one decision leaves independent
sanctions active. A lift does not restore revoked credentials. Existing player
sessions are closed through retryable enforcement, and future protected
requests and match starts check current account and installation authority.
Installation identity is resettable app data, not physical-device attestation.

Leaderboard controls exclude or reinstate a player for an open week without
changing earned points, caps or balances. Closing a week freezes its eligibility
and results; repeated close requests resume the same durable operation.
Closed history cannot be reranked by a later exclusion. These operations require
an exact authorized Admin session, a reason, CSRF protection and an audit.
The [Roadmap](../planning/ROADMAP.md) tracks remaining production evidence.

Gameplay image/GIF/video submissions are retired under
[ADR-012](../design/ADR-012-text-only-selectable-modes.md). Historical image
records are retained; the old workbench/image candidate execution path is gone.
Avatar support is a separate non-playable consumer.

## Preserve contributions during transition

Keep accepted terms/version/timestamps, original text, creator and decision
history, contributor credits, rewards, report targets and challenge references.
Current tables can contain historical image records: do not relabel bytes,
filenames or captions as approved text, delete them in place or edit applied
SQL migrations. Use the reviewed archive/backfill plan with verified reference
mapping before enforcing text-only active-content constraints.

New catalog records need stable content IDs/revisions, response/item or prompt
kind, mode suitability, canonical language and rights/review evidence. The shared
bundle contract implements those fields; the Studio remains an accepted-text
intake surface rather than a complete mode/pool authoring editor. Content publication must preserve approval reward idempotency; importing
or republishing previously rewarded work never pays it again. Changing a report
status alone does not deactivate a catalog: use the separate audited release
takedown/activation operation. Existing matches retain their pinned contract or
are explicitly ended under the operational policy.

## API and verification

`POST /api/portal/connect` accepts an authenticated player's `{code}` and
returns 204 on successful pairing. Browser GET `/portal/login` and POST
`/portal/session` manage the other side; browser mutations require CSRF tokens.

The existing challenge endpoints are `GET /api/challenge/active`,
`POST /api/challenge/entry` and `POST /api/challenge/vote`. Their response shape,
consent fields and stable error codes are retained in the
[historical roadmap's community slice](../planning/ROADMAP-pre-text-20260912.md#community-and-operations-source-audit-and-first-working-slice--2026-09-10),
under **Challenge API handoff**. Those HTTP contribution endpoints are separate
from protocol v2 match actions; current lifecycle behavior is described above. No current-week topic returns 204.

Run `python3 xops/test/tests-lints.py` from the repository root. It provisions
guarded disposable PostgreSQL/Redis for integration fixtures, includes standalone
tools, and fails required skips. Never point these fixture suites at a retained
application database.
Screening tests use a local fake provider and do not make paid or live API calls.
