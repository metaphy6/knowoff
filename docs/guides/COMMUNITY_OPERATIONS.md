# Community operations

This first working slice supports text contributions and the Weekly Nown
Challenge. The player interface is Flutter; Contributor Studio is web-only and
the Admin Console remains on the separate internal listener.

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
media pack is a separate pipeline; the old status-only publish action is
disabled instead of pretending a pack was deployed.

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
image/GIF processing, pack calls and Nown/deck authoring, avatar approval and
activation are still open deliverables in the [roadmap](../planning/ROADMAP.md).
The legacy non-production workbench is a separate development surface; this
slice does not certify its authentication or production readiness.

## API and verification

`POST /api/portal/connect` accepts an authenticated player's `{code}` and
returns 204 on successful pairing. Browser GET `/portal/login` and POST
`/portal/session` manage the other side; browser mutations require CSRF tokens.

The existing challenge endpoints are `GET /api/challenge/active`,
`POST /api/challenge/entry` and `POST /api/challenge/vote`. Their response shape,
consent fields and stable error codes are documented in the roadmap's
**Challenge API handoff**. No current-week topic returns 204.

Use a dedicated disposable database for integration tests: several suites
truncate fixtures. Set `KNOWOFF_TEST_DSN` to that database before running
`python3 xops/test/tests-lints.py`; otherwise database tests may report skips.
Screening tests use a local fake provider and do not make paid or live API calls.
