# Community operations

This first working slice supports text contributions and the Weekly Nown
Challenge. The player interface is Flutter; Contributor Studio is web-only and
the Admin Console remains on the separate internal listener.

The [Blueprint](../../BLUEPRINT.md) now adopts five text-only gameplay modes.
The operations below describe the existing working text slice; mode/language
catalog authoring, certification and activation remain planned in the
[transition design](../design/DESIGN-text-transition.md) and
[Roadmap](../planning/ROADMAP.md). A working contribution form does not mean the
new gameplay or complete release pipeline exists.

## Open the studio

1. In the game, open **Profile → Contributor Portal → Open portal**.
2. The browser displays a short-lived pairing code. Enter it back in the game
   and explicitly confirm that you initiated this connection.
3. Return to that same browser and choose **Continue**. Pairing expires after
   five minutes and establishes an eight-hour HttpOnly session.
4. Open **Roles & applications** to see your account's eligibility and apply.
   An administrator reviews the request. Curator includes Contributor access;
   Guard is an independent safety role whose enforcement tools remain pending.

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
bootstrap terms body is a placeholder; do not treat it as a reviewed legal
document or rewrite versions already accepted by contributors. The latest
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

Approval means accepted for curation. Building, certifying and deploying a real
text bundle is a separate pipeline; the old status-only publish action is
disabled instead of pretending a pack was deployed.

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
for themselves, and cannot vote after closure. Admin closes the week explicitly
in this slice. Closing more than once credits the recorded winner only once;
the full configured reward is separate from the daily gameplay cap.

The current week's result stays readable after closure. Automated weekly
rollover and transfer of the current-winner title remain roadmap work.

## Other available operations

The console links to role/application review, versioned terms, text review,
challenge review, notices, report and feedback triage, and account wallet and
entitlement lookup. Triage records supported status changes with an audit; it
does not claim that closing a report removes media or bans an account.

Guard enforcement, arbitrary grants/refunds, complete leaderboard controls,
reviewed text catalog authoring/certification/activation, and avatar approval
and activation remain open work in the [roadmap](../planning/ROADMAP.md).
The text target supersedes gameplay-image processing and
[ADR-011](../design/ADR-011-static-image-and-text-content.md) through
[ADR-012](../design/ADR-012-text-only-selectable-modes.md); image/GIF/video
submissions are not future playable formats. Preserve separate avatar support.
The legacy non-production workbench is a separate development surface; this
slice does not certify its authentication or production readiness.

## Preserve contributions during transition

Keep accepted terms/version/timestamps, original text, creator and decision
history, contributor credits, rewards, report targets and challenge references.
Current tables can contain historical image records: do not relabel bytes,
filenames or captions as approved text, delete them in place or edit applied
SQL migrations. Use the reviewed archive/backfill plan with verified reference
mapping before enforcing text-only active-content constraints.

New catalog records need stable content IDs/revisions, response/item or prompt
kind, mode suitability, canonical language and rights/review evidence. Those
fields are a target contract, not fields already available in Contributor
Studio. Content publication must preserve approval reward idempotency; importing
or republishing previously rewarded work never pays it again. Closing a report
still does not deactivate a catalog: a takedown needs a separately verified
replacement release, with existing matches pinned to their recorded contract
or explicitly ended under the operational policy.

## API and verification

`POST /api/portal/connect` accepts an authenticated player's `{code}` and
returns 204 on successful pairing. Browser GET `/portal/login` and POST
`/portal/session` manage the other side; browser mutations require CSRF tokens.

The existing challenge endpoints are `GET /api/challenge/active`,
`POST /api/challenge/entry` and `POST /api/challenge/vote`. Their response shape,
consent fields and stable error codes are retained in the
[historical roadmap's community slice](../planning/ROADMAP-pre-text-20260912.md#community-and-operations-source-audit-and-first-working-slice--2026-09-10),
under **Challenge API handoff**. It documents the current slice, not protocol
v2 mode actions. No current-week topic returns 204.

Use a dedicated disposable database for integration tests: several suites
truncate fixtures. Set `KNOWOFF_TEST_DSN` to that database before running
`python3 xops/test/tests-lints.py`; otherwise database tests may report skips.
Screening tests use a local fake provider and do not make paid or live API calls.
