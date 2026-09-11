<!--
docs/guides/UI_REDESIGN_PLAYBOOK.md

A record of how the Knowoff client was rebuilt on the Soft Neo-Brutalism
design matrix, written as a reusable method for the next agent. The
*prescriptive* rules live in .agents/skills/neo-brutalism-ui-design/SKILL.md
and the ROADMAP's 🎨 chapter; this file is the *execution* companion: what was
actually done, in what order, with which tools, and what went wrong.
-->

# 🎨 UI Redesign Playbook — how the Knowoff client was rebuilt

**Audience:** any agent (or human) about to do a large, cross-cutting UI change
in this repo.
**Companion docs:** [`neo-brutalism-ui-design`](../../.agents/skills/neo-brutalism-ui-design/SKILL.md)
(the normative *rules*), [🎨 Visual Identity](../../BLUEPRINT.md#-visual-identity-soft-neo-brutalism-design-matrix)
(the *spec*), [ADR-007](../design/ADR-007-display-typeface.md) and
[ADR-008](../design/ADR-008-support-accents.md) (the locked-token *decisions*).

This file is the third leg: the **method and the scar tissue**.

Its rules apply to interface chrome. For the Nown/card media inside those
surfaces, use the Blueprint's [lo-fi, GIF-first direction](../../BLUEPRINT.md#playable-media-direction)
and [content creation skill](../../.agents/skills/knowoff-content-create/SKILL.md).
Keep rough crops, compression and abrupt reaction loops; do not render pack
content as polished illustrations matching the UI palette. Calm Nown-stage
layout means a stable frame, not freezing the GIF it contains.

---

## 0. What the job actually was

The request was, roughly: *"the home page, buttons and voting screen are ugly
and non-functional, the colours are boring, be creative."*

Three separate problems hide inside that sentence, and they need different
treatment:

| Symptom the user reported | What it actually was | Fix class |
|---|---|---|
| "ugly", "boring colours" | The design grammar existed but was applied **in patches**, and the palette was doing too many jobs at once | Design-system work |
| "non-functional" | Real bugs — votes that never landed, timers that never ticked, icons that rendered as empty boxes | Engineering work |
| "be creative" | Permission to spend the intensity budget the v1 build left on the table | Judgement |

**The single most important finding of the whole job: most of the "ugly" was
bugs.** A ballot that silently refuses votes looks *broken*, and broken looks
ugly. Do not assume a visual complaint is a visual problem — run the thing.

### What shipped

Two commits (`eb466f2`, `4362f0b`) plus cleanup: every screen and widget under
`client/lib/presentation/` rebuilt, 27 widget files, 13 doodle glyphs, 181
localized strings, one bundled OFL display face, and 84 passing client tests
(up from 27). Five server/client protocol bugs fixed with regression tests.

---

## 1. Philosophy

### 1.1 Softness is the palette temperature, not the execution volume

Knowoff's design matrix says "**soft** neo-brutalism". The v1 build read that as
permission to be timid everywhere. It is not. The *palette* is soft — pastel,
rounded corners, warm cream. The *execution* — border weight, shadow depth,
type scale, motion, layout disruption — should be loud. Every place the old UI
felt generic was a place where the execution had been softened, not a place
where the palette was wrong.

Practical version: **before you reach for a new hex value, ask whether you have
spent the scale, weight, shadow-tier, motion and illustration budget.** In this
job the existing seven colours already cleared WCAG AAA against ink with room
to spare — there was no accessibility reason the UI had to look quiet.

### 1.2 The loud/calm contract

A loud style only works if most of it is calm. Concretely, in this codebase:

- **Display face** (Baloo 2) carries headlines, timers, tallies, room codes,
  Noin numerals, the verdict headline. That's it.
- **Body copy** stays on the plain platform sans, at normal weight.
- If every label shouts, nothing does. When you find yourself upgrading a
  `bodySmall` to the display face, you are diluting the thing that makes the
  headline land.

### 1.3 The macro/micro rule — where disruption is allowed

Neo-brutalism's "structured disruption" (rotation, scatter, overlap) is what
makes an interface look hand-built rather than generated. It is also the
fastest way to sabotage a game about fairness.

The boundary used throughout:

| Surface class | Examples | Treatment |
|---|---|---|
| **Macro / decorative / celebratory** | Hand fan, evidence table, empty-state stickers, wordmark, verdict banner, section stamps | Tilt, scatter, overlap, glow shadows, generous motion |
| **Micro / read-fast / fairness-critical** | Ballot rows, timers, vote budget, Nown stage, forms | Mechanically aligned. No rotation. No decorative motion. Ever. |

The rotation values come from a **closed set** (`KoTilt`), never `Random()`.
Deterministic tilt is a design decision; random tilt is noise, and it also
makes golden tests impossible.

### 1.4 Colour semantics are load-bearing; new colours must be categorical

`violet` = interact, `lime` = truth/reward, `pink` = accuse/risk, `ink` =
information. A player learns those in their first match. They are never
reassigned.

The v1 problem was that `violet` was *also* every header, *also* every timer,
*also* every progress fill — so nothing had a colour left to distinguish it.
[ADR-008](../design/ADR-008-support-accents.md) added three **categorical**
accents (`canvasDeep`, `tangerine`, `aqua`) that explicitly carry no verdict
meaning. That is the shape a palette extension should take: give the overloaded
token its job back, don't invent a fourth verdict colour.

Watch for **semantic collision**: the old store rendered a Noin balance in
`lime` — the same colour that means "a Donower was caught". Currency now has
`tangerine` and cannot be misread as a verdict.

### 1.5 Colour is never the only signal

Enforced at the type level, not by convention: `KoChip`, `VerdictChip` and
`SeatTile`'s badges all *require* an icon and a label alongside their fill.
Every state a player can be in — on the clock, ready, offline, eliminated,
revealed role, bot seat — ships icon + word + colour. Build new state widgets
the same way and colourblind-safety is free.

---

## 2. Method — the order of operations, and why

The order matters more than any individual step. Working bottom-up means each
layer is already correct when the layer above consumes it; working top-down
means rewriting screens twice.

```
 0. Read the spec + skill        ← before the first edit
 1. Tokens                       ← everything else references these
 2. Typography                   ← the highest-leverage single change
 3. Theme (stock Material)       ← so un-migrated widgets don't look foreign
 4. Primitives (Ko*)             ← every screen routes through these
 5. New shared components        ← scaffold, seat tile, meters, badges
 6. Known offenders              ← the audited list of grammar violations
 7. Screens                      ← now mostly composition
 8. Localization                 ← new strings, regenerate
 9. Tests                        ← update what changed, add what's new
10. Run the real thing           ← where the actual bugs surface
11. ADRs + spec sync             ← record locked-token changes
12. Track + stage
```

### 2.1 Step 0 is not optional

Read the ROADMAP's Game Rules chapter *and* the visual identity chapter before
touching a widget. Three things in this redesign only exist because the rules
were read first:

- The **vote budget** (`KoVoteBudget`) — Rules §1 makes it the most
  decision-relevant number on the table, and the old UI never showed it.
- The **draw penalty** surfaced on the hand (`-5 pts each`) — Rules §3 prices
  panic-drawing, so the price belongs on the control.
- The **pending result** deliberately omitting the role — Rules §5 keeps the
  eliminated seat's role hidden until the reveal lands.

You cannot invent those from a screenshot.

### 2.2 Extend tokens additively, never in place

`KoShadows.hard` stayed. `KoShadows.md` was added as an alias for it. Existing
call sites kept compiling, the brand-locked value stayed snapshot-tested, and
new code got a named scale to build hierarchy with. Same for `KoContainer` —
`shadow` and `rotation` parameters default to today's behaviour.

> An additive token change costs one line and zero migration. A replacement
> costs a codebase-wide sweep and a broken snapshot test.

### 2.3 Typography is the highest-leverage single edit

[ADR-006](../design/ADR-006-typography.md) had deferred the display face to
"heaviest weight of the platform font". That one decision was the largest
contributor to the app reading as generic Material — a brutalist skeleton in a
system font is just a bordered box.

Landing it properly meant satisfying ADR-006's own reversal criteria
(diacritics, OFL licence, size) and then **superseding, never editing**, the
old ADR. See [ADR-007](../design/ADR-007-display-typeface.md).

Two implementation notes worth keeping:

- **Bundle the TTF; don't fetch fonts at runtime.** `google_fonts` pulls from
  `fonts.gstatic.com` on first use — a network dependency, a third-party
  request, and non-deterministic offline first-launch. A committed 668 KB TTF
  plus its `OFL.txt` has none of those properties.
- **Variable fonts need the axis set explicitly.** Setting `fontWeight` alone
  gives you a synthesised weight. Set both:
  `fontWeight: FontWeight.w800` *and*
  `fontVariations: [FontVariation('wght', 800)]`.

### 2.4 Theme the stock widgets too

Not every dialog and snackbar gets migrated to `Ko*` in one pass. Push the
border grammar into `ThemeData` (card shape, dialog shape, input borders,
chips, snackbar, dividers, app bar) so the *un-migrated* surfaces still look
like they belong. This is what stops a redesign from looking half-finished
while it is half-finished.

### 2.5 Work the offender list, not your taste

The skill file ships an audited list of known grammar violations (§3). Working
that list first — a `ChoiceChip` inside the hand fan, a raw `MaterialBanner`
for notices — fixes the places a user actually feels the inconsistency, before
you spend time on screens that were already fine.

---

## 3. The architecture that resulted

| File | Owns |
|---|---|
| `theme/knowoff_tokens.dart` | Colours, radii, border widths, **named shadow tiers**, `KoTilt` closed rotation set, spacing scale, motion budget |
| `theme/knowoff_typography.dart` | `KoFonts.display`, `koDisplayStyle()`, the whole `TextTheme` |
| `theme/knowoff_theme.dart` | One `ThemeData`; pushes the grammar onto stock Material widgets |
| `theme/ko_canvas_grid.dart` | The faint canvas grid `CustomPainter` |
| `widgets/ko_container.dart` | Base surface — border, shadow tier, optional tilt |
| `widgets/ko_button.dart` | Press collapse, hover lift, size scale, disabled state, ≥48 dp target |
| `widgets/ko_chip.dart` | Pill; icon + label required |
| `widgets/ko_scaffold.dart` | Page frame: canvas + grid, banded accent header, `KoPageWidth` content column, status-bar and bottom-bar slots, `KoSectionHeader` |
| `widgets/seat_tile.dart` | Player identity: `SeatAvatar`, `SeatTile`, `seatAccent()`, `isBotSeat()`, `seatDisplayName()` |
| `widgets/ko_meters.dart` | `KoTimerBar`, `KoVoteBudget` |
| `widgets/ko_stat_tile.dart` | `KoStatTile`, `KoEmptyState`, `KoLoading` |
| `widgets/noin_badge.dart` | The currency badge |
| `widgets/ko_shake.dart` | Poke screen-shake |
| `icons/doodles.dart` | 13 single-weight `CustomPainter` glyphs |
| `widgets/guardrail_audit.dart` | The mechanical style gate |

**The standing rule this creates:** every new component is built on `Ko*`
primitives from day one. A bare `Card`/`Chip`/`Banner`/`ListTile` sitting next
to styled siblings is the failure mode that produced this redesign in the first
place, and `screens_test.dart` now asserts against it on every match screen.

---

## 4. The verification ladder

Run these in order — cheapest first, so you fail fast:

| # | Gate | Command | Catches |
|---|---|---|---|
| 1 | Format | `dart format lib test` | Diff noise |
| 2 | Analyze | `flutter analyze --no-pub` | Type errors, dead code, lints |
| 3 | Lint autofix | `dart fix --apply lib` | `prefer_const_constructors` at scale |
| 4 | Unit/widget tests | `flutter test` | Behaviour + design guardrails |
| 5 | Build | `flutter build web --release` | Asset wiring, tree-shaking, font manifest |
| 6 | **Run it** | Compose stack + browser | Everything the first five cannot see |

Gates 1–5 all passed on a build whose ballot silently refused every vote. **Gate
6 is not optional for a UI change.**

### 4.1 What the design guardrails now assert

`guardrail_audit.dart` walks the element tree and fails on blur radii, more
than one gradient, `BackdropFilter`/`ImageFiltered`, and `Opacity`. On top of
that the suite now asserts, per live match screen:

- zero guardrail violations,
- no bare `ChoiceChip` / `MaterialBanner` / `Card` / `ListTile`,
- the reveal gradient is spent exactly once, on the Knowoff result window,
- token snapshot covers colours, radii, borders, every shadow tier and the tilt set,
- `KoTilt.alternating()` never leaves the closed set,
- every shadow in the house scale has `blurRadius == 0`,
- `KoButton` clears a 48 dp minimum height.

### 4.2 The visual loop

```bash
make up                               # postgres + redis + minio + server
cd client && flutter build web --release
python3 -m http.server 8791 -d build/web --bind 127.0.0.1
# then drive it with the browser tools
make down                             # leave the machine as you found it
```

---

## 5. Fail-and-learns

The most useful section. Every one of these cost real time in this job.

### 5.1 `create_file` + `mv` leaves the editor holding the buffer

Writing `foo_v2.dart` and `mv`-ing it over `foo.dart` works on disk — and then
the still-open editor tab re-saves `foo_v2.dart` from its buffer, resurrecting
a file you thought was gone. Eleven orphaned duplicate screens were committed
this way before anyone noticed. They were unreferenced dead code, but four of
them were *stale* copies of screens that had since been fixed, which is exactly
the kind of thing that misleads a future reader.

> **Rule:** prefer editing a file in place over write-new-then-move. If you must
> stage through a temporary file, verify with `file_search` afterwards that the
> temporary name is gone.

### 5.2 `Center`/`Align` expands to fill — and can starve its siblings

`KoPageWidth` was originally `Center(child: ConstrainedBox(...))`. Used inside
`Scaffold.bottomNavigationBar` — a slot that hands down a *loose* height — the
`Align` took the entire screen height, leaving the body a `ListView` of height
`0.0`. The screen rendered its header and its bottom bar and **nothing in
between**, with no exception thrown.

The fix is `heightFactor: 1.0` wherever the column sits in a loosely
constrained slot. `KoPageWidth(hugHeight: true)` exists for exactly this, and
`ko_scaffold_test.dart` has a regression test for it.

> **Rule:** a silent empty screen in Flutter is usually a constraint bug, not a
> logic bug. Measure before you theorise.

**How it was found** (worth reusing): a throwaway widget test that pumped the
screen and printed the truth.

```dart
print('EXC: ${tester.takeException()}');
print('TEXTS: ${tester.widgetList<Text>(find.byType(Text)).map((t) => t.data)}');
final box = find.byType(ListView).evaluate().first.renderObject as RenderBox;
print('LV size: ${box.size}');   // → Size(588.0, 0.0)  ← there it is
```

Two minutes to write, deleted immediately after. Delete it — don't leave debug
scaffolding in the suite.

### 5.3 The Flutter web service worker will lie to you

`flutter build web` produces a service worker that caches aggressively. A
`?v=2` query busts `index.html` but not the cached `main.dart.js`. Twice in
this job a fix "didn't work" because the browser was running the previous
build — including one round of debugging a bug that had already been fixed.

```js
const rs = await navigator.serviceWorker.getRegistrations();
for (const r of rs) await r.unregister();
for (const k of await caches.keys()) await caches.delete(k);
```

Run that, *then* reload, before concluding anything about a web build.

### 5.4 A test can be wrong about the very thing it guards

`GuardrailAudit` counted gradients on both `Container` **and** the
`DecoratedBox` that `Container` builds internally — so the one legal reveal
gradient reported as two. The guardrail would have blocked a compliant design.
Fixed by threading the container's decoration down one level and skipping the
identical instance, plus two new tests: one that a single `Container` gradient
is legal, one that two separate gradient containers still fail.

> **Rule:** when a guardrail fires on code you believe is correct, suspect the
> guardrail. Then write the test that pins down which of you was right.

### 5.5 A green suite is not proof — the tests encoded the bug

`handleCastVote` read `payload["target"]`. Every client, every bot, and every
other seat-addressed intent in the codebase (`poke`, `reveal`) sends
`target_seat`. **No vote had ever landed on its intended seat**; ballots
silently resolved to seat 0.

The server tests passed because they were written against the handler's wrong
key. The bug survived because nobody had played a match end-to-end.

> **Rule:** protocol tests that construct the payload themselves prove only
> internal consistency. Cross-check field names against the *other* side of the
> wire, and grep for the field name repo-wide — the inconsistent one is the bug.

### 5.6 Silent visual failures don't raise

`uses-material-design: false` meant every `Icon(Icons.*)` rendered as a blank
box. No error, no warning in tests — only a build-time message nobody reads.
Turning it on costs 9 KB after tree-shaking (from 1.6 MB).

> **Rule:** read the build output, not just its exit code.

### 5.7 A timer that lies is worse than no timer

The countdown was hardcoded per screen (`_ballotSeconds = 20`) and fell back to
the full window whenever no deadline was known — so it displayed a confident,
frozen "20s left" forever. Same class of problem: the vote-budget pips rendered
"0 votes left" before the server had reported a budget, which reads as *the
Donowers have already won*.

Both fixed the same way: **get the number from the authority, and render
nothing when you don't have it.** The server now declares `window_seconds` on
every phase, and `KoVoteBudget` draws nothing until a budget arrives.

> **Rule:** the client renders state; it does not infer rules. If a number is
> missing, the honest UI is empty space, not a plausible guess.

### 5.8 A "merge" that rebuilds from scratch isn't a merge

`_mergeState` constructed a fresh `GameStateDto` field by field and simply
forgot the locally-owned ones — so every phase change silently wiped the chat
feed and the countdown deadline.

> **Rule:** when a merge function builds a new object, the fields the wire
> *doesn't* carry need an explicit `current.x` line. A `copyWith` would have
> made this impossible; a hand-rolled constructor made it invisible.

### 5.9 Optimistic local state is correct when the server deliberately won't echo

The Knowoff ballot is blind by design (Rules §4), so the server never echoes
your vote back. Without a local lock the voter got *no feedback at all* and
could tap every row in turn.

Locking `voteTarget` locally is legitimate — you already know your own vote, so
nothing leaks — with two guards: roll it back on an `error` event, and scope
that rollback to the voting phases so an unrelated error can't wipe a ballot
that did land.

### 5.10 `git stash` proves a failure is pre-existing

`internal/config TestLoadLocalCap` failed throughout the job. Rather than guess:

```bash
git stash push --keep-index --include-untracked -m wip
go test ./internal/config/... -count=1     # still fails → not mine
git stash pop
```

Thirty seconds, and the difference between "I broke something" and a tracking
note. Do this before you spend an hour on someone else's bug.

### 5.11 Note what you don't fix

Bot seats show `P0/P1/…` instead of the labelled 🤖 badge the roadmap requires,
because `game.PlayerState` has no name field and `lobby.SeatBinding.BotName`
stops at the lobby. The client side is complete and tested; the gap is
server-side plumbing that is a feature change, not a UI fix.

That went into `tracking.csv` as an `action=note` row rather than into scope.
**Scope discipline is not laziness — it is what keeps a redesign reviewable.**

---

## 6. Tooling notes

| Tool | Use it for | Gotcha |
|---|---|---|
| `flutter analyze --no-pub` | Fast type/lint pass | `--no-pub` skips a needless resolve |
| `dart fix --apply lib` | Bulk `prefer_const_constructors` after a large edit | Run it *after* the code is right, not during |
| `dart format lib test` | Keep the diff about substance | Run before every commit |
| `flutter test --plain-name '…'` | Isolate one failing test with full output | Grep the raw output; the reporter truncates |
| `flutter gen-l10n` | Regenerate `AppLocalizations` after ARB edits | `l10n.yaml` wins over CLI flags |
| `xops/agent/safe-run.sh <tag> -- <cmd>` | Any long/risky command | Note the `--` separator; writes `/tmp/agent-runs/*.log` and a `last_failure.json` breadcrumb |
| `make up` / `down` | The real stack | The dev server hot-reloads on Go edits and can wedge; `docker compose restart server` clears it |
| Browser tools + `run_playwright_code` | Visual verification | **Flutter web renders to canvas — there are no DOM elements.** Selectors fail; click by coordinate read off a screenshot |
| `git stash --keep-index` | Prove a failure predates you | §5.10 |
| Throwaway debug test | Print the widget tree / measure a `RenderBox` | Delete it the moment it has answered |

### 6.1 Driving a Flutter web app from the browser tools

`click_element` with a selector will time out — the accessibility tree is empty
until the user enables it. Take a screenshot, read the coordinates off it, and
use `page.mouse.click(x, y)`. Long game phases fit naturally into a single
scripted run:

```js
await page.goto('http://127.0.0.1:8791/?run=7', {waitUntil: 'load'});
await page.waitForTimeout(7000);
await page.mouse.click(820, 267);    // Play
await page.waitForTimeout(32000);    // queue → backfill bots → round
await page.mouse.click(1017, 185);   // Ready
await page.waitForTimeout(7000);
await page.mouse.click(1064, 356);   // vote
```

---

## 7. Checklist for the next UI change

- [ ] Read the ROADMAP chapters the work touches, and the matching skill file.
- [ ] Extend tokens **additively**; never edit a brand-locked value in place.
- [ ] Build on `Ko*` primitives — no bare Material widget next to styled siblings.
- [ ] Tilt only decorative/celebratory surfaces; ballots, timers and forms stay aligned.
- [ ] Every state widget takes an icon **and** a label.
- [ ] Contrast-check any new colour pairing before shipping it.
- [ ] New strings go through the ARB; run `flutter gen-l10n`.
- [ ] `dart format` → `flutter analyze` → `flutter test` → `flutter build` → **run it**.
- [ ] Clear the service worker before trusting a web build.
- [ ] Locked-token change? Supersede the ADR, update the token snapshot, sync the ROADMAP chapter in the same change.
- [ ] Tests move with the code; a changed assertion needs a reason, not a shrug.
- [ ] Findings outside scope → `action=note` tracking row, not scope creep.
- [ ] Verify no temporary/`_v2` files survived (§5.1).

---

## 8. Deliberately left open

- **Bot seat names** — server-side plumbing (§5.11).
- **Body typeface** — still the platform sans. Inter or DM Sans would be the
  next candidates; the loud/calm contract means this is a small win, not a big one.
- **`internal/config TestLoadLocalCap`** — needs `KNOWOFF_*` secrets in the
  environment; unrelated to the client.
- **Bot policy loop** — `internal/bots` re-sends ready/vote intents twice a
  second regardless of phase. Noisy, harmless, out of scope.
- **Golden-image tests** — the suite asserts structure and tokens, not pixels.
  Worth adding once the layout settles.


## 2026-09-10 reconstruction and measured motion

The new implementation consolidates shared surfaces in `ko_ui.dart`, match
rendering in `game_screen.dart` / `game_surfaces.dart`, and account flows in
`account_screens.dart` with service widgets. The former layouts and their
UI-only tests were retired with owner approval, then replaced with fresh
behavior/privacy/layout tests. Tokens, localization, API/state rules and media
services were retained. Theatrical copy and tilted posters surround aligned
forms, cards, clocks and ballots. Timed desktop play puts the hand beside Nown;
large text moves header actions below the title. Avatar uploads explicitly
acknowledge pending review, while the active preset remains visible.

Motion uses static animation children and isolated repaint boundaries. Buttons
settle in 60 ms, decorative entrances in 150 ms, and the result has one finite
four-second reveal. Reduced motion renders equivalent static information. The
one-second clock lives in its own widget; it does not tick the whole game page.
No ambient repeating animation, blur or stacked opacity was introduced.

A foreground Flutter 3.38.5 / Dart 3.10.4 profile CanvasKit run measured the real
GameScreen at 806×1270 logical pixels, with six seats and text-only fixtures.
After warm-up, Round and Knowoff received state changes every 250 ms for 12 s
each; the result sample included the four-second animation. Flutter's
`SchedulerBinding.addTimingsCallback` recorded build/raster/total durations.

| Phase | Frames | Build p95 | Raster p95 | Total p95 | Total maximum | Frames >16.7 ms |
|---|---:|---:|---:|---:|---:|---:|
| Round | 61 | 11.7 ms | 0.9 ms | 13.2 ms | 40.8 ms | 3 |
| Knowoff | 92 | 10.4 ms | 0.5 ms | 11.5 ms | 33.2 ms | 1 |
| Result reveal | 606 | 0.8 ms | 0.3 ms | 2.1 ms | 15.7 ms | 0 |

The 16.7 ms **p95** target passed in this desktop sample. Four isolated state
updates exceeded that budget; this is not an every-frame or zero-jank claim.
Native mobile, low-end hardware, cold image decoding, GIFs and network loading
remain unprofiled. A hidden-tab sample was discarded because browser throttling
invalidated frame-delivery measurements. Narrower state subscriptions are a
possible follow-up for the build spikes, but the measurements do not establish
their cause.

Local evidence: `/tmp/agent-runs/ui_perf_report.md`, raw
`/tmp/agent-runs/ui_perf_metrics.json`, and build log
`/tmp/agent-runs/ui-perf-profile-stable--20260910T092248Z-196322.log`.
The ignored harness remains at `client/.dart_tool/ui_perf/main.dart` and builds
with `flutter build web --profile --target=.dart_tool/ui_perf/main.dart
--output=.dart_tool/ui_perf_web`; run in a **foreground** browser. These are
local artifacts, not a committed benchmark fixture or low-end certification.

## 2026-09-10 device experiences and restored developer controls

One Flutter state owner now feeds three deliberately different compositions.
The shared window policy uses available logical dimensions (including browser
windows and tablet split-screen), never a browser or device-model guess:

| Window | Navigation and match composition |
| --- | --- |
| Phone below 600 dp, or a short landscape window below 1000 dp | Bottom service destinations and Hand/Table/People workspaces; pinned turn, votes and Ready; compact Nown/latest-play summary; full Nown and attributed evidence together in Table. |
| Small phone below 380 dp or 700 dp tall | Immediate Quick Play, collapsed help, one readable hand column. |
| Large phone | More context on Home and room for two complete hand cards per row at 430 dp. |
| Tablet 600–1199 dp | Navigation rail, two-part service tasks and a two-pane match; evidence stays beside Hand/Now or People. |
| Desktop from 1200 dp | Persistent labeled service menu; three independently scrolling match panes for evidence, active task and people/chat. |

Card selection, scroll controllers, draft text, evidence history and all server
intents remain outside these compositions. Ordinary bot/turn updates never
switch phone workspaces; entering Knowoff, runoff or result opens Now so a timed
action cannot remain hidden. Play-to-discussion keeps hand geometry stable.
Hidden phone match panes are unmounted; cached service destinations suppress
focus and animation tickers. No ambient animation was added.

The historical dev controls were verified against CHANGELOG, tracking rows and
Go handlers, plus the deleted overlay in Git history. See
[CLIENT_DEV_TOOLS.md](CLIENT_DEV_TOOLS.md) for the restored controls and gates.
The client-only freeze also pauses the displayed clock. The server continues;
resuming processes buffered events in order. Random clears the stored override,
and both Quick Play and local-room handshakes carry preselected roles before
seat binding. Production rejects debug overrides.

Fresh proof includes small/large phone, landscape phone, 600 dp split window,
portrait/landscape tablet and desktop widget coverage; normal/expanded pseudo
copy; 2x text; reduced motion; complete card bounds; bot/selection/discussion
stability; semantic tab activation; scrollable Reveal/Leave dialogs; nickname
and store actions on each device class; draft persistence and failed-refresh
recovery. All 288 Flutter tests and the Python/Go checks passed; the full gate
and release build logs are linked in ROADMAP.md.
The Impeccable mechanical detector returned no findings for the changed Flutter
presentation targets; widget/render tests remain the stronger Flutter evidence.

A real local-stack debug match was inspected in the in-app browser at 360x800,
430x932, 834x1194 and 1440x900. It exercised pre-join Nower selection, bot play,
discussion/ballot, phone Table navigation and client freeze; its displayed clock
held while the server continued. The browser console had no errors/warnings.
These are browser-window checks, not physical Android/iOS/tablet certification.

**Motion measurement limit.** The attempted 1440x900 profile-mode trace of a
six-seat text fixture was visibility/throttling affected: only 13 samples each
for 12-second play/ballot periods despite 4 Hz stimulation. Browser visibility
could not be held (`visibility.get()` returned false after set(true)). Raw
samples are retained at `/tmp/agent-runs/device_perf_metrics.json`: play p95 total
21.401 ms, ballot 16.201 ms, and result 3001.5 ms (queue delay dominates that
result; build/raster maxima were 12.3/5.4 ms). This is diagnostic evidence only,
not a passing foreground frame-budget trace. The new device compositions still
need a reliable foreground/physical low-end profile before claiming p95<=16.7ms.
Automated tests do prove finite motion, reduced-motion equivalence and no idle
tickers; prior reconstruction timings do not certify these new layouts.
