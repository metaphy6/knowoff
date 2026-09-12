# Changelog

All notable changes to Knowoff are documented in this file.

## Text-only transition planning — 2026-09-12

- Adopt text-only five-mode Blueprint and seven-phase Roadmap; preserve earlier
  roadmap/ADR evidence as explicitly historical.
- Add source-backed protocol, card ownership, secrecy, migration, settlement,
  compatibility, test coverage and complete retirement contracts.
- Add business/content/liquidity/economics validation plan and align active
  guides, glossary, launch materials and documentation indexes.
- No runtime, SQL migration, tuning, pack activation, test deletion or deployment
  implemented; live release remains gated on the new proof matrix.

## [Unreleased]

### Runtime retirement and operational proofs — 2026-09-12

- Remove the association engine, specialty/dev-grant APIs, production bot
  manager and v1 transport implementations; transfer their behavioral tests to
  the five-mode engine and explicit unsupported-input checks.
- Record genuine Quick Play abandonment once after reconnect grace, using the
  match's pinned escalating cooldown policy; exclude local/prototype play and
  confirmed server interruptions, and reject queue/start during cooldown.
- Protect retained ledger, award, first-win, settlement, outbox and weekly
  history against populated TRUNCATE, including cascading and hidden-row cases;
  migration rollback requires empty retained history.
- Block obsolete client protocol routes and carry the selected four/six-seat
  size, API and normalized room code into explicit text admission.
- Load active configuration with only PostgreSQL, Redis and JWT core secrets;
  reject legacy playable storage/media/specialty/backfill keys before resolving
  secret values. Remove MinIO and playable pack/ingest wiring from the active
  Compose/proxy stack while retaining historical backup fixtures.
- Add bounded authenticated runtime drain/status, real WebSocket connection
  metrics and a compiled migration manifest with strict startup schema checks.
- Enforce exact admin session authority after domain/audit lock waits, including
  idempotent actions, notices and contribution publication.
- Replace unsafe snapshot operations with isolated capture/restore proofs that
  preserve database values, metadata and retained object history. Ordinary live
  capture remains closed pending an all-writer quiescence boundary.

### Five-mode integration and account safety — 2026-09-12

- Add native purchase/restore integration with configured store products and
  native prices, account-bound receipt retries and durable server grants before
  store completion. Actual Android/iOS store-device verification remains open.
- Add paid custom-avatar upload with bounded WebP normalization, screening,
  revision/account fences and private authorized image delivery; takedown keeps
  the paid unlock and returns the profile to its preset.
- Add explicit user terms, private block/report/help journeys, same-table player
  identity lookup and the current Weekly Challenge winner badge.
- Separate Guard freezes from admin bans and timed suspensions. Final decisions
  revoke JWT/browser/admin sessions and close live peers with durable retry;
  lifting a sanction does not revive old credentials.
- Schedule reviewed weekly topics and transfer the current title with one payout;
  ties use earliest accepted submission, then immutable entry ID. Private blocks
  hide optional authored entries without changing recorded votes or scores.
- Preserve original contributor credit when an accepted input is captured again
  after a nickname change. Fix concurrent socket replies and redact all peer
  credentials before writing network replay traces.
- Complete Google/Facebook link and account restoration with explicit account
  switching, private completion proof and cancellation-safe session storage.
- Add exact-revision text reports, audited report cases, live notice refresh and
  wallet privacy through disconnect, interruption and restart.
- Verify platform receipt signatures and transaction/account identity, durable
  acknowledgement, named entitlements and source-specific refund handling;
  provider sandbox/device evidence and subscription continuation remain open.
- Require an explicitly published contribution agreement; startup no longer
  manufactures placeholder terms.
- Verify 489 Flutter tests, two PWA cache tests, web build and targeted real
  PostgreSQL/Redis race suites. Full transition validation, remaining account,
  billing, retirement and external release evidence are still in progress.

### Five-mode implementation checkpoint — 2026-09-12

- Save the unfinished five-mode text catalog, engine, lobby/protocol, Flutter
  client, development network bot and durable value/content/trust implementation.
- Add migrations 9–14 for admission, settlement, content lifecycle, terms/blocks,
  process ownership and development identities; preserve migrations 1–8.
- Record clean Flutter analysis and 407 passing tests, plus targeted real-service
  network, authentication, ownership and content/admin evidence. Final combined
  review and repository validation remain open, including the known portal
  rejection/immutable-review compatibility regression.
- This owner-authorized backup checkpoint preserves progress, with production
  availability still release-gated. Resume instructions and verification limits
  are in `docs/reports/2026-09-12-text-transition-pause.md`.

### Text transition foundations

- Added strict version-2 action, lobby, snapshot and history contracts for all
  five text modes, with shared Go/Dart fixture evidence and separate card-copy
  identities. The active game remains protocol v1; no text mode is enabled.
- Added closed mode/language availability, compatibility policy, trade/round
  timers and bounded history/request config; legacy config remains usable
  until explicit cutover, with obsolete-key preflight errors.
- Added read-only database schema/row fingerprints and disposable migration
  tests. Migration helpers now release their reserved connections without
  closing the application's database pool.
- Extended preflight artifacts with timestamps, local SQL hashes, fixed-category
  content/entitlement counts, aggregate value totals and explicit observation
  limits; ambiguous migration state is rejected. Recorded the owner's absence
  of a deployment and the concrete wallet transaction/replay design review.
- Added an isolated additive archival design fixture with bounded copy/verify
  cursors, interruption/resume, source-drift and identity-conflict refusals,
  legacy-data parity and guarded down. Applied migrations remain unchanged;
  actual production backfill and settlement implementation are later work.
- Extended validation to all retained Go/Python tools, disposable PostgreSQL
  and Redis, native-compiler container fallback and failing skip accounting.
  iOS CI now runs separately on macOS. Existing fixture skips were repaired
  with executable assertions; no test files were deleted.

### Removed

- Removed GIF and animated-image game content. Nowns, cards and challenge media
  use static images or plain text; unsupported formats are rejected rather than
  relabeled. Existing UI effects remain.
- Removed the experimental image, GIF and text batches created during content
  trials, including their source assets, prompts and preview galleries.

- Removed the Flutter frontend for a redesign: screens, widgets, theme,
  doodles, display font, and media-rendering widgets before the reconstruction
  below. Localization and service initialization were preserved.
- Retired tests and snapshots that exclusively covered the removed UI, with
  owner approval. Nonvisual action rules and their tests remain alongside
  game state, authentication, API/transport, localization, and media services.

### Added

- Documented the confirmed move to text-based Knowoff with five selectable
  modes and proposed answers for turns, card ownership, evidence, dealing,
  queues and fair rollout. Missed the Briefing is the proposed default; the
  detailed rules are now adopted in Blueprint/Roadmap, while the modes remain
  unimplemented.

- Recorded the static-image-and-text decision in ADR-011 and made the content
  skills automatically consult High/Distant/Chaos tuning, server dealing and
  client state/rendering logic before drafting or evaluating a batch.

- Documented system FFmpeg/FFprobe discovery, installation when missing, and
  verification from another directory for reuse across workspaces.
- Added shared content creation, review and pack-integration skills with current
  documentation references and an editorial record. All repository agents are
  directed to use them for Nown/card work, preserving human review and release
  evidence across handoffs.
- Added a usable Contributor Studio: account pairing from Profile, browser
  sessions, role applications, private text drafts, explicit contribution terms,
  editing, submission, withdrawal and review history.
- Added internal operations pages for applications, submission review, weekly
  challenges, terms, notices, report/feedback triage and account/economy lookup.
  Text approval requires configured automatic screening and human review.
- Added an in-app Weekly Nown Challenge with approved entries, own review status,
  consented text submissions, immutable voting and current-week results.

- Added distinct device experiences: phones open on Hand/Table/People workspaces
  with a pinned turn/Ready bar and bottom navigation; large phones use a wider
  hand grid; tablets pair evidence with the active task; desktop shows evidence,
  hand/ballot, and people/chat together. Home and account pages use phone bottom
  navigation, a tablet rail and a persistent desktop menu with split task views.
- Restored all five historical debug controls in a scrollable Dev tools panel:
  Freeze/Resume, Restart, Echo pokes, Grant specialty, and Next-match role.
  Debug controls remain absent from release/profile builds. Specialty grants
  never autoplay, and poke feedback settles with a static reduced-motion option.

- Recreated the entire Flutter UI around the retained state/API/localization:
  a theatrical main menu, local rooms, private roles and cards, specialty flows,
  discussion/live runoff ballots, result posters, rematches and account pages.
  Locked brand tokens and the bundled Baloo 2 font now drive a consistent set
  of sharp, colorful surfaces and accessible controls.
- Added fresh interaction, privacy, responsive/pseudo-locale, reduced-motion and
  semantics tests. Effects settle after one event, timers update locally, and
  media is isolated from animation repainting.
- Restored real profile statistics, reporting, notices and avatar upload with a
  native file selector; file selection is validated before an explicit upload.


- Using the Free Card specialty now fires a loud "FREE CARD!" announcement for the whole table while the draw pile celebrates: the card count swells big for a beat and the price chip flips from −5 to a bouncing FREE until the free draw is spent.
- Debug builds now have a dev-only specialty picker in the bottom-right dev tools: pick any of the five specialty cards and the server drops it into your hand as if it had been dealt (disabled entirely in prod), then the normal use flow runs unchanged — handy for testing specialty behavior without waiting for the deal.
- Verdict screen now opens a Play Again window once a match finishes: every seat picks "same table" (rematch with this exact table once everyone agrees) or "new table" (leaves for a fresh Quick Play match); a seat that left or never reconnected in time opens up for backfill, so a new player who simply clicks Quick Play can land straight into the reopened table instead of a brand new one.

### Fixed

- Image packs now persist their validated, content-hashed assets when bundled,
  so supported static-image Nowns and cards survive a write/load round trip.
- Community consent controls now have the Material ancestor required by Flutter,
  preserving their existing behavior and the supported Flutter 3.24 baseline.

- Retained the deliberately low-quality meme aesthetic in content briefs and
  review: rough everyday framing, compression and recognizable situations,
  usually below the 720 px image ceiling. The former GIF-first guidance is
  replaced by static images and text; tone, humor and cultural-fit rules remain.
- Made the content skills' tracking handoff explicit: completed repository
  changes require a pending commit row, staging and a verified `make git.dry`
  preview. Corrected the Codex tracking-agent enum in the setup guide.
- Completed localization resource descriptions and required them during code
  generation, resolving `arb(missing_metadata_for_key)` diagnostics.
- Fixed ordinary browser admin navigation after login; mutations retain CSRF
  protection. Contributor sessions use a short-lived, browser-bound pairing code.
- Enforce the challenge limit at intake, hide pending entries from other players,
  include all of Sunday, reject votes after closure and serialize winner payouts.
- Keep contributor and challenge rewards separate from the daily gameplay cap,
  so winning or contributing neither loses rewards nor consumes play earnings.
- Bind approval to the exact text shown to the reviewer, keep private drafts
  out of staff queues, and audit notice creation/withdrawal atomically. Invalid
  scheduling dates are rejected instead of accidentally publishing immediately.
- Show the server-selected challenge winner, including tied vote counts, and
  reject challenge access from deleted or actively frozen accounts.

- Developer Random now clears the previous role across restart and queueing.
  Pre-join choices remain local until a valid join envelope; local rooms apply
  the selected role before the final seat can start the first match. Production
  server guards remain enforced.
- Keep profile drafts when changing destinations, resizing, refreshing or
  retrying a failed refresh. Hidden destinations relinquish keyboard focus;
  account pages expose Refresh for cached balances and statistics.
- Preserve hand positions through bot activity and discussion; make workspace
  tabs screen-reader actionable, bring timed ballots/results into view, and
  allow small-phone task dialogs and narrow tablet evidence to fit.

- Render complete hand cards in a responsive grid and keep their positions
  stable across bot plays, turn changes, selection and discussion. Reserve
  table/status/hint space, retain card identity and keep selection controls
  below the cards instead of inserting them above the hand.
- Place the attributed played-card table directly below Nown. A bounded,
  scrollable table keeps full rooms and earlier evidence from burying hand
  controls; desktop hand and draw actions remain beside the clue. Live tables
  show the latest round first and reset their scroll when the round changes.
- Aligned retained store client routes with the server and accept successful
  empty responses; refreshed wallet balances after conversion-dialog dismissal.
- Carry server-provided runoff candidates into the replacement ballot UI.
- Present server round zero as Round 1 and retain its attributed card evidence.
- Local-room QR codes and clipboard links open the app join form, with
  validated room codes and matching native scheme registrations.
- Avoid duplicate screen-reader button labels, keep keyboard focus visible,
  isolate private overlays, and keep desktop turn actions beside the clue.


- A match no longer freezes for good when the server rejects the stored access token (a restart with a new signing key, or a revoked session): the client now reissues its device credentials and drops the dead room instead of re-sending the same refused token on every rejoin, which used to leave the screen stuck on a stale phase where every tap silently queued.
- Bots now recast their votes when a Nower uses Revote during an open ballot, so the reopened vote can finish instead of leaving the match stalled.
- The exposed-hand panel no longer triggers a Flutter web layout assertion that could leave the game stuck on a Knowoff result screen after focus changed.
- The full “View [player]’s exposed hand” panel is tappable again; previously only its small avatar control received taps, so clicking the visible central button did nothing.
- Reveal a Hand no longer opens a burn-card picker or silently consumes a hand card; after choosing the player to expose, the owner uses the normal hand interaction to place their card on the table.
- Play Again → same table no longer boots the fresh match into the previous one's leftovers: the client now drops the finished match's winner, revealed Nowns, points, table plays, ballots, chat, announcements and the old role the moment the new match's countdown opens, so round 0 starts clean with the players who stayed.
- The Free Card specialty now actually makes your next pile draw free instead of pulling a random out-of-nowhere card: it fires the moment it's tapped (no discard toll, no picker), banks a round-scoped token that zeroes the −5 draw cost for that turn's first pile draw, an unused token expires with the round, and the turn's card can't be played while the free draw is still pending.
- The Verdict screen no longer reveals Nowns from rounds that were never played: the schedule is sized to the full vote budget, so a match that ended early (for example the Donower caught on the first ballot) used to display every scheduled Nown as if each had been played — now exactly the rounds that began are revealed.
- Reveal a Hand no longer depends on a separate button below the hand, publicly broadcasts hidden cards before anyone asks to view them, or remains usable during the final five seconds of a turn.
- Knowoff result posters no longer show the stale ballot countdown or remaining-votes wheels; the result owns the full stage during its four-second display.
- Knowoff result window is 8 seconds again (was cut to 4), restoring visible time for the Nower/Donower role-reveal poster — the four-second falling reveal previously consumed the entire window, so the poster and Revote button never rendered before the round advanced.
- Discussion's Ready flag no longer carries over into the next round — it now resets when a fresh discussion phase opens, instead of leaving the button permanently locked from a Ready tapped last round.
- Ready can now be taken back any time before the window finalizes (discussion, Knowoff ballot, and the post-ballot result window) — tapping Ready again un-readies instead of being ignored; the button only locks once every active seat has agreed or the window's timer runs out.
- "Back to menu" on the Verdict screen now clears the finished match's session state (room code, seat, phase) before popping back — previously the stale state made the next Quick Play jump straight back into the same finished match instead of queuing.

### Changed

- Adopted the humor-development guidance in the Blueprint and content guides:
  editorial dimensions, experimental freshness mix, cultural adaptation and
  human playtests. Documented current server dealing and production-content
  gaps; this documentation update does not generate or publish a pack.
- `make git` never creates empty commits anymore: all pending tracking rows in a staging window now ride one real commit (first pending summary as the subject, the rest under "Also includes:", one `[run_id]` trailer each), and with a clean tree the rows simply wait for the next real commit instead of committing empty markers.
- Developer-granted specialty cards remain current-hand test overrides. Source
  audit correction: ordinary specialties are dealt at match start and are not
  refreshed each round; held cards carry over and spent cards remain empty.
- The main menu now uses clearer doodles for Play, Local Room, Profile, Store, and Notices: a play symbol, map pin, person, market, and hailer.
- The round action window is now titled "Logs" with a larger bold heading for easier scanning.
- Round log entries are now bold as well, making the action history easier to scan.
- Round logs now keep every action in a bounded scrollable list while keeping Shuffle users anonymous.
- Revote is now a ballot-only card: it can be played while a Knowoff ballot or runoff is open, but no longer during the result window after the eliminated player's role has been exposed. Undoing a result you have already seen was overkill and left Donowers with no odds.

- Shuffle's anonymous table announcement is a proper sky-blue burst now: the deck-swirl doodle spins into a stamped medallion while "RESHUFFLE! — New hands, who dis?" double-pops onto the screen, then the whole thing slides off — replacing the plain lime banner.
- Shuffle now mulligans the whole round: every card already on the table goes back with the hands into the re-deal, the turn order restarts from the first seat, and the restarted turn gets a fresh full window plus the 10-second Shuffle bonus on top.
- Shuffle is no longer a round-start-only play: Donowers can fire it at any point in the round, in or out of turn, and the card now wears its own sky-blue identity instead of the generic violet chrome shared with buttons and timers.
- The Revote card on the Knowoff screen is no longer violet-on-violet camouflage: it now wears the specialty's tangerine identity with a mask-in-ink medallion, twinkling corner sparkles and a stamp tilt — and during the window's last five seconds it switches to rush mode, heartbeat-pulsing with a thicker border, a tangerine glow, a live seconds numeral, and "Last seconds — slam it before the result locks!" copy.
- The Verdict Play Again prompt now opens in the center of the screen and can be collapsed by tapping outside it, leaving a bright Play Again card inside the Back to menu bar so players can reopen it after reading the results.
- Reveal a Hand now opens from its specialty card, filters out the owner and eliminated targets, announces the exposed player, and adds a once-per-player avatar doodle that opens the hand for three seconds during that round.
- Donower-win verdicts now reveal each winning Donower directly on their player rectangle, matching caught-Donower results instead of listing winners in a separate declaration box.
- Bots now draw one card from their personal pile when their playable hand runs close to their reserve, then continue the same turn and play instead of stalling.
- Knowoff results now open with a four-second falling eliminated-player reveal, then show the Nower or Donower result card and Revote on the Knowoff screen without a Ready panel or duplicate result overlay.
- Specialty-card plays are now announced to the whole table with short animated alerts, including the player who used the card.
- Revote cards now work from the result screen instead of being rejected as out-of-phase.
- Bots retry a rejected Ready or vote intent instead of silently stalling the current window.
- Donower victories now name the winning Donower players on the final Verdict screen.
- Nowers can see and use Revote during an open voting ballot, resetting it without spending a vote.
- Drawing from the pile now updates the player's hand immediately without rendering the draw as a played card.
- Drawing a card now cancels any earlier preselected or queued auto-play card.
- Drawing from the pile now keeps the turn active so the player can play a card afterward.
- Every pile draw now triggers a dramatic, playful announcement naming the player and card count.
- Discussion now shows a 20-second window in four-player rooms, and every Ready tap is publicly listed by player.
