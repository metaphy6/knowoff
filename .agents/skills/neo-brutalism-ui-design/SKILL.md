---
name: neo-brutalism-ui-design
description: "Neo-brutalism UI design. The Knowoff client UI (or any new screen/widget) needs to look distinctive, vibrant, and intentionally 'built' instead of generic Material — redesigning, beautifying, or extending client/lib/presentation against the 🎨 Soft Neo-Brutalism Design Matrix."
---

# Neo-brutalism UI design (Knowoff)

## Scope: interface around playable media

This skill styles UI chrome, controls and decorative interface art. Playable
Nowns/cards follow the Blueprint's [lo-fi, GIF-first media direction](../../../BLUEPRINT.md#playable-media-direction):
candid rough captures, modest detail and abrupt reaction/action loops. Its
anti-rawness, zero-blur, sparse-doodle and UI motion restrictions apply to the
interface, not the imagery or action inside media packs. Preserve media
readability and playback performance without polishing it into matching pastel
illustrations or static brand scenes. Use the [content creation](../knowoff-content-create/SKILL.md),
[review](../knowoff-content-review/SKILL.md) and
[integration](../knowoff-content-integrate/SKILL.md) skills for that pipeline.

## When to use

- The user says the UI looks "mundane", "generic", "too Material", "boring", or asks
  to "redesign" / "beautify" / "make it pop".
- Building a **new** screen or widget under `client/lib/presentation/`.
- Reviewing a diff that touches `presentation/theme/`, `presentation/widgets/`, or
  `presentation/screens/` against the [🎨 Visual Identity chapter](../../../BLUEPRINT.md#-visual-identity-soft-neo-brutalism-design-matrix)
  of the roadmap.
- Any task that would otherwise reach for a stock `Card`, `Chip`, `MaterialBanner`,
  `AppBar`, or default `ThemeData` widget without a design-system-approved reason.

This skill is long on purpose: it's a condensed education in a real design
movement (§1) plus a project-specific execution playbook (§2 onward) so an
agent with zero design background can still ship a defensible, on-brand result.

---

## §1. Neo-brutalism 101 (what it is, so you don't cargo-cult it)

### 1.1 Three generations — know which one you're building

| Generation | Era | Signature | Usability stance |
|---|---|---|---|
| Architectural Brutalism | 1950s–70s | Raw concrete (*béton brut*), exposed structure | Not a UI concept |
| Web Brutalism | 2014–2020 | Anti-template rawness, bare HTML aesthetics, sometimes anti-UX | Ideological pushback on "the pretty web" |
| **Neubrutalism (what we build)** | 2021–present | Thick outlines, hard offset shadows, flat high-contrast color, oversized type — **as a repeatable, teachable design system** | Pro-UX navigation, pro-impact visuals |

Knowoff builds the third one. It is **not** raw or anti-usability — it is a
loud, opinionated *skin* over a completely conventional interaction model. NN/g
puts it precisely: *"more colorful and orderly than pure brutalism."*

### 1.2 The seven characteristics (every real neubrutalist interface has these)

1. **High-contrast, bold, flat color.** No gradients (one deliberate exception
   at most). Colors *carve surfaces into discrete objects* — they're
   categorical, not ambient/decorative.
2. **Thick, solid outlines.** Every component reads as a physically separate,
   stamped/boxed object. 2–4px is the working range; pick one canonical width
   and only deviate for deliberate hierarchy.
3. **Hard, offset shadows — zero blur, always.** `offset(x, y)` with
   `blurRadius: 0`. Depth is anti-naturalistic: stacked, shifted, "printed
   layers that don't fully align" — never atmospheric/soft.
4. **Oversized, expressive typography.** Type *is* a graphic element, not just
   copy. The trick is contrast: a loud display face for headlines/numbers, a
   calm, highly legible face for body — never uniform volume throughout.
5. **Structured disruption in layout.** Grids exist, then get selectively
   broken — offset modules, slight rotation, overlap, asymmetric scale.
   *"Broken but not random."* Macro level gets the chaos; micro level (labels,
   fields, buttons, ballots) stays mechanically aligned.
6. **Retro / skeuomorphic touches (optional, in moderation).** Sticker-style
   doodles, monospace accents, deliberately simple illustration — signals
   "made by a person," never a mascot-heavy scene.
7. **Loud, obvious interaction states.** Press = the control visibly moves
   into its own shadow and the shadow disappears. Hover = it lifts and the
   shadow grows. No subtle opacity fades.

### 1.3 Color — canonical do/don't

**Do:** one neutral base + one dark outline/ink color + 2–3 saturated accents;
flat fills; verify every text/background pair against WCAG before shipping;
let color help carve the interface into legible objects.

**Don't:** use gradients as a crutch (flat is the grammar — the one Knowoff
reveal-header gradient is a deliberate, singular exception, not a precedent);
let every component compete at max saturation; assume "loud" implies
accessible (yellow-on-white famously fails contrast); let color alone convey
state (always pair with icon + label).

### 1.4 Shadows — the three-tier system

Real neubrutalist systems don't use one shadow everywhere — they use a small,
named scale so hierarchy survives the loudness:

| Tier | Offset | Used for |
|---|---|---|
| `sm` | `3,3` | badges, chips, inline/small controls |
| `md` | `5,5` (Knowoff's current default is `4,4` — keep it, just name it) | cards, buttons, panels |
| `lg` | `8,8` | overlays, hero/celebration moments, focus states |

Blur is **always** `0`. That's non-negotiable — see §5.

### 1.5 Typography — the loud/calm contract

- **Display** (headlines, hero numbers, timers, Noin deltas): heavy weight,
  tight tracking, geometric or quirky-grotesque. Reference faces: Baloo 2,
  Fredoka, Space Grotesk, Archivo Black, Syne.
- **Body**: a plain, highly legible geometric sans at generous line-height.
  Body text is deliberately "boring" — that calm is what makes the loud
  headline gestures sustainable. Reference: Inter, DM Sans, Plus Jakarta Sans.
- **Optional mono accent** for numeric/technical HUD elements (timers, seat
  ids, seeds): Space Mono, JetBrains Mono — reinforces "engineered, exposed
  structure," pairs naturally with a card-based game UI.
- The rule that matters most: **reserve the loudest typographic gestures for
  headlines and hero moments; keep everything else conventionally readable.**
  If every label shouts, nothing does.

### 1.6 Layout — structured disruption, precisely bounded

- **Macro level** (heroes, empty states, celebration screens, card stacks):
  asymmetry, rotation, overlap are all in-bounds and encouraged.
- **Micro level** (forms, ballots, timers, field alignment, anything a
  fairness rule depends on): stays mechanically aligned. The moment
  disruption interferes with comprehension or fairness, it has crossed from
  expression into sabotage — pull it back.

### 1.7 Accessibility — where the style helps and where it breaks

**Strengths:** big type and hard edges support scanning; visible borders beat
low-contrast ultra-flat UI for control discoverability; a well-built palette
can clear WCAG easily (see §4 — Knowoff's already does); hard shadows read as
obviously clickable.

**Common failure modes to actively guard against:**

| WCAG criterion | Requirement | Neubrutalism-specific trap |
|---|---|---|
| 1.4.3 Contrast (Minimum) | 4.5:1 normal text, 3:1 large text | Loud accent-on-accent combos (e.g. a colored badge on a colored background) can silently fail — check every *new* pair, not just text-on-canvas |
| 1.4.11 Non-text Contrast | 3:1 for UI component boundaries | A thick decorative border can visually swallow a real state-change border |
| 2.4.7 Focus Visible | Focus must be clearly indicated | A big hard shadow must not visually hide the platform focus ring — use `outline-offset`-equivalent spacing on Flutter focus decorations |
| 2.5.8 Target Size (AA) | ≥24×24px (Knowoff's touch UI should hold ≥48dp) | Thick borders visually imply a bigger hit-area than the actual `GestureDetector`/`InkWell` bounds — always match hit area to the drawn border, not just the inner content |
| 1.4.1 Use of Color | Never color-only signaling | Every verdict/state widget must carry icon + label, never hue alone (Knowoff already enforces this at the type level in `VerdictChip`/`KoChip` — keep doing it for every new state widget) |

### 1.8 When (not) to lean in — and where Knowoff sits

Best fit: creator/portfolio products, SaaS marketing, developer tools, games —
audiences that reward personality and a strong first impression. **Knowoff (a
party game about bluffing) is close to an ideal fit** — this is not a banking
app or a dense enterprise dashboard, both of which the literature explicitly
flags as poor fits for the style. That means Knowoff has *more* license to lean
into the loud end of the spectrum than the average product, not less.

### 1.9 Subtypes worth knowing (Knowoff is a hybrid of two)

- **Soft neubrutalism** — keeps the border/shadow grammar, relaxes color
  intensity and corner radius toward rounded. This is where Knowoff's design
  matrix (pastel canvas, rounded 16/12px corners) currently sits.
- **Cute-alism** — raw brutalist boxes + candy palette + sticker-style
  doodles + rounded controls; "kawaii energy inside a brutalist container."
  Knowoff's doodle-glyph plan and lavender/lime/pink palette already point
  here. **Leaning further into this subtype — bigger stickers, punchier
  motion, more physical card-table energy — is exactly the "abandon strict
  softness for a better look" move the project is asking for.** It does not
  require abandoning the brutalist grammar (borders/shadows/flat color)
  — it means turning up *scale, motion, and illustration*, not turning down
  the borders.

---

## §2. The house rule for this project

**Softness was always meant to be the *palette temperature*, not an excuse
for a timid execution.** The fixed palette, the 3px ink border, the zero-blur
hard shadow, and the press-collapses-into-its-shadow motion are **locked brand
grammar** (snapshot-tested — see §6). What was never locked is *intensity of
use*: scale, shadow hierarchy, typographic voice, motion, illustration.

**Status: the v1 build spent none of that budget; a full rebuild has since
spent it.** A named shadow tier scale, a bundled display face, 13 doodle
glyphs, hover/press/shake motion, structured disruption on decorative
surfaces, and total grammar coverage all landed — see §3 for the standing
rules that keep them, and
[`UI_REDESIGN_PLAYBOOK.md`](../../../docs/guides/UI_REDESIGN_PLAYBOOK.md) for
the method and the fail-and-learns.

**Conclusion for the agent: don't reach for new colors first.** The palette
passes accessibility with room to spare (§4). Reach for scale, shadow
hierarchy, typography, motion, and illustration before repainting anything.

A genuinely new hue or a second gradient is allowed, but it is a **locked-token
change**: it goes through [`adr-writing`](../adr-writing/SKILL.md) (supersede,
never edit, the ADRs in `docs/design/`), updates the token snapshot tests,
updates `guardrail_audit.dart` deliberately (§6), and syncs the ROADMAP
design-matrix chapter in the same change — exactly the path
[ADR-008](../../../docs/design/ADR-008-support-accents.md) took. It is never a
silent drive-by edit.

---

## §3. Standing rules (the offender list, and how it stays empty)

The original audit found seven concrete gaps. **All seven are closed.** They
are kept here as the standing rules that keep them closed — a diff that
reintroduces any of them should be rejected in review.

1. **No bare stock Material widget in a styled tree.** `hand_fan.dart` once
   wrapped `CardFace` in a `ChoiceChip` and `notice_banner.dart` rendered a raw
   `MaterialBanner`. Both are now `Ko*`-based, and `screens_test.dart` asserts
   no `ChoiceChip` / `MaterialBanner` / `Card` / `ListTile` survives on any live
   match screen.
2. **Headings use the bundled display face**, not a bold system font
   ([ADR-007](../../../docs/design/ADR-007-display-typeface.md)). `design_system_test.dart`
   asserts every display slot resolves to `KoFonts.display` and that body copy
   does *not*.
3. **Shadows come from the named tier scale** (`sm`/`md`/`lg`/`lift` plus the
   tinted celebration glows), never an ad-hoc `BoxShadow`. Snapshot-tested.
4. **The doodle set carries brand glyphs**, not `Icons.*`. 13 glyphs ship;
   Material icons are for utility chrome only (back, close, flag, wifi-off).
5. **Decorative surfaces are disrupted; fairness surfaces are not.** Tilt comes
   from the closed `KoTilt` set — hand fan, evidence table, stickers, banners
   yes; ballots, timers, forms, the Nown stage never.
6. **Pointer devices get a hover state.** `KoButton` lifts to `KoShadows.lift`
   and translates `(-2,-2)`; the PWA is a real deploy surface.
7. **New components are built on `Ko*` from day one** — not retrofitted later.

When asked to "redesign the UI" absent more specific direction, audit against
these seven, then reach for the per-component guide in §7.

---

## §4. Color — verified against WCAG (don't re-derive, reuse these numbers)

Computed relative-luminance contrast ratios for the actual token values in
`KoColors` (`ink` text/border is the constant; background varies):

| Pair (ink on…) | Approx. ratio | WCAG (normal text, AA=4.5, AAA=7) |
|---|---|---|
| `canvas` `#DCC8F7` | ~12.0:1 | AAA |
| `surface` `#F7F2E9` | ~16.5:1 | AAA |
| `whiteWell` `#FFFFFF` | ~18.5:1 | AAA |
| `violet` `#B49AF5` | ~7.8:1 | AAA |
| `lime` `#D4F04C` | ~14.4:1 | AAA |
| `pink` `#FF9ED2` | ~9.7:1 | AAA |

**Takeaway: the existing palette is not the problem.** Every accent already
clears AAA for ink text on it. That headroom is exactly why you should push
*scale, weight, motion, and shadow intensity* before ever touching a hex
value — there's no accessibility reason holding the current look back.

Any **new** color combination introduced during a redesign (e.g. a tinted
shadow next to a colored fill, or text on a semi-transparent tile) must be
verified the same way before it ships — don't assume; a canonical
neubrutalist trap is exactly this ("loud" ≠ accessible).

---

## §5. Non-negotiable guardrails (do not defeat these)

Enforced today by `client/lib/presentation/widgets/guardrail_audit.dart` and
exercised in tests — a redesign must pass, not route around, this file:

- **Zero blur on every shadow.** `BoxShadow.blurRadius` must always be `0`.
- **At most one gradient in the whole render tree** (reserved for the Knowoff
  reveal header). Wanting a second is a locked-token change (§2), not a
  patch.
- **No `BackdropFilter` / `ImageFiltered` blur, ever.**
- **No `Opacity`-based translucency.** Depth comes from shadow offset, never
  fade.
- **Color is never the sole signal.** Every verdict/state-bearing widget
  takes an icon *and* a label parameter (see `VerdictChip`, `KoChip`) — keep
  this true for anything new.
- **Fixed semantics stay fixed:** `violet` = interact, `lime` = truth/reward,
  `pink` = accuse/risk, `ink` = information. A redesign can make these bigger,
  louder, more animated — it cannot swap what they mean.
- **Low-end frame budget on `Round` and `Knowoff` screens** (Phase 3 proof
  test). Any new motion (hover-lift, rotation, sticker animation) added to
  these two screens must be cheap: `Transform`/`AnimatedContainer` at 60ms–
  150ms durations, no per-frame rebuilds of the whole tree, no rotation
  applied to timer-critical or vote-critical widgets in the first place (see
  §7 macro/micro rule).

---

## §6. The token system (as shipped — extend it the same way)

`presentation/theme/knowoff_tokens.dart` now carries the full scale below. The
rule that produced it still governs every future change: **add, don't replace**
the existing constants (they're brand-locked and snapshot-tested).

```dart
abstract final class KoShadows {
  // existing:
  static const BoxShadow hard = BoxShadow(color: KoColors.ink, offset: Offset(4, 4), blurRadius: 0);
  static const BoxShadow pressed = BoxShadow(color: KoColors.ink, offset: Offset.zero, blurRadius: 0);

  // add — a named three-tier scale (§1.4); `hard` above becomes the `md` tier:
  static const BoxShadow sm = BoxShadow(color: KoColors.ink, offset: Offset(3, 3), blurRadius: 0);
  static const BoxShadow md = hard;
  static const BoxShadow lg = BoxShadow(color: KoColors.ink, offset: Offset(8, 8), blurRadius: 0);

  // add — hover-lift target for pointer devices (web/desktop), grows on hover:
  static const BoxShadow lift = BoxShadow(color: KoColors.ink, offset: Offset(7, 7), blurRadius: 0);

  // add — reserved for celebratory/hero moments only (verdict, Week Winner,
  // streak badges): shadow color matches the accent instead of ink. Sparingly.
  static const BoxShadow limeGlow = BoxShadow(color: KoColors.lime, offset: Offset(6, 6), blurRadius: 0);
  static const BoxShadow pinkGlow = BoxShadow(color: KoColors.pink, offset: Offset(6, 6), blurRadius: 0);
}

// add — small, fixed rotation set for "structured disruption" (§7). Never
// randomize freely; pick from this closed set so the look stays deliberate.
abstract final class KoTilt {
  static const double none = 0.0;
  static const double subtle = -0.02; // ~-1.1°, radians for Transform.rotate
  static const double soft = 0.035;   // ~2°
  static const double loud = -0.05;   // ~-2.9°, hero/celebration only
}
```

Then wire `KoContainer`/`KoButton` to accept an optional `shadow` tier (default
`KoShadows.md`, i.e. today's behavior unchanged) and an optional `rotation`
(default `KoTilt.none`) so existing call sites keep compiling and opting in is
additive, not a breaking rewrite. Both do; `KoTilt.alternating(index)` gives a
deterministic tilt for a run of decorative items — never `Random()`, which
would make snapshot and golden tests impossible.

Also shipped: `KoBorders` (`thin`/`regular`/`thick`), `KoSpace` (a 4–32 scale),
`KoMotion` (`press`/`hover`/`pop`/`shake` durations) and `KoShadows.violetGlow`
alongside the lime and pink glows.

---

## §7. Component-by-component redesign guide

For each existing primitive/widget, the specific move — mapped to the
macro/micro rule from §1.6: **decorative & celebratory surfaces get the loud
treatment; anything a timer, vote, or form depends on stays calm and
mechanically aligned.**

| Component | File | Redesign move |
|---|---|---|
| `KoContainer` | `widgets/ko_container.dart` | Add `shadowTier` (§6) and optional `rotation` params, defaulted to no-op. This is the base every other primitive should route through. |
| `KoButton` | `widgets/ko_button.dart` | Add a `MouseRegion`-driven hover state (desktop/PWA pointer) that swaps to `KoShadows.lift` and translates `(-2,-2)` before the existing tap-press collapse takes over — today only touch/press is handled. |
| `KoChip` | `widgets/ko_chip.dart` | Fine as-is structurally; allow an optional small `rotation` for "badge" use (e.g. a "NEW" pack ribbon, Week Winner tag) — micro-level game chips (vote/poke) stay unrotated. |
| **`HandFan`** | `widgets/hand_fan.dart` | **Fix first (§3.1).** Replace the bare `ChoiceChip` with a `KoContainer`-based selectable card: ink border, `sm`/`md` shadow, `lime` fill + `pressed` shadow when selected. Give each unselected card in the fan a small alternating `KoTilt.subtle`/`KoTilt.soft` rotation for physical "hand of cards" energy; snap to `KoTilt.none` + lift on selection so the *selected* state is always the calm, obvious one. Keep the ≥48dp tap target the visual border implies. |
| `PlayTable` | `widgets/play_table.dart` | This is a read-only evidence surface, not a form — the best candidate in the whole app for maximal personality: scatter played cards with small alternating rotations and slight overlap (structured disruption, macro level) instead of a perfectly upright row. |
| `RoleCard` | `widgets/role_card.dart` | This is the "you're Donower" money-beat. On reveal, pair the existing lime/pink color flash with the highlighter-sweep motif and a brief scale/shadow pulse (`md` → `lg` → `md`, ~150ms) — a celebratory *micro*-moment exception to "no motion," bounded and cheap. |
| `NownStage` | `widgets/nown_stage.dart` | Keep the white-well frame calm and mechanically stable (Nowers must read it fast, every round) — but push the *frame* itself: `lg` shadow tier, thicker accent border color cue. Do not rotate; do not add motion — this is a micro/read-fast surface. |
| **`NoticeBanner`** | `widgets/notice_banner.dart` | **Fix first (§3.2).** Rebuild on `KoContainer` (border + shadow + typography), with the countdown digits rendered in the display face at a larger size — currently a bare `MaterialBanner` with default `TextStyle`. |
| `VoteBoard` | `widgets/vote_board.dart` | Micro/fairness-critical — keep the current `Wrap` grid mechanically aligned, no rotation, ever. It's already using `KoButton` + the correct `pink`/`lime` semantics; the only addition is the `lg` shadow tier to make the accusation surface feel weightier than a casual button. |
| `PokeNudge` | `widgets/poke_nudge.dart` | Fine structurally; add a quick shake/rotate burst (`KoTilt.loud` for ~80ms then back to `0`) on the *poked player's* nudge indicator to sell the "screen shakes" spec line (Rules §8) — cosmetic, bounded, not on a timer-critical widget. |
| Verdict / celebration screens | `screens/verdict_screen.dart` | The single best place to spend the reveal gradient, `limeGlow`/`pinkGlow` shadows, doodle glyphs (confetti-like sparkle/eye bursts), and the biggest type in the app. Motion budget is generous here — it's a post-match, non-timer screen. |
| Store / Leaderboard / Profile | `screens/store_screen.dart`, `leaderboard_screen.dart`, `profile_screen.dart` | Oversized display-face numerals for Noin/rank, sticker-glyph accents on empty/zero states, chip-based stat rows using the existing `KoChip` icon+label contract. |

---

## §8. Typography — landed (ADR-007)

The system-font stand-in was the single highest-leverage fix, and it is done:
**Baloo 2**, SIL OFL, bundled at `client/assets/fonts/Baloo2-Variable.ttf`
with its `OFL.txt`, wired through `KoFonts.display` and `koDisplayStyle()`.
See [ADR-007](../../../docs/design/ADR-007-display-typeface.md).

Two implementation facts worth carrying into any future face change:

- **Bundle the TTF; don't fetch at runtime.** `google_fonts` pulls from
  `fonts.gstatic.com` on first use — a network dependency, a third-party
  request, and a non-deterministic offline first launch. A committed TTF plus
  its licence file has none of those properties.
- **Variable fonts need the axis set explicitly.** `fontWeight` alone yields a
  synthesised weight; set `fontWeight` *and*
  `fontVariations: [FontVariation('wght', N)]`. `koDisplayStyle()` does both.

If you ever swap the face, the steps are unchanged: keep body copy on the plain
geometric sans, wire only `display*`/`headline*`/`titleLarge`/`labelLarge` and
the numeral styles, re-run the diacritics render check across every launch
locale plus the `en_XA` pseudo-locale, confirm the licence and size, and
**supersede the ADR — never edit it** ([`adr-writing`](../adr-writing/SKILL.md)).

---

## §9. Illustration — the doodle set

`presentation/icons/doodles.dart` ships 13 glyphs: `sparkle`, `staticBurst`,
`eye`, `cloud`, `placeholder`, `crown`, `coin`, `cards`, `check`, `cross`,
`poke`, `mask`, `clock` — the ~12 the design matrix calls for.

When you need a brand glyph, **add to this enum and its `CustomPainter`**
rather than reaching for a raster asset or a `Material` icon: single-weight,
ink-stroke, no gradient fill (at most one flat accent fill), consistent with
the border grammar. `Icons.*` is for utility chrome only — back, close, flag,
wifi-off, QR — and note that `uses-material-design: true` must stay on in
`pubspec.yaml` or every one of those renders as an empty box.

---

## §10. Verification before calling a redesign "done"

Follow [`self-review`](../self-review/SKILL.md) and
[`verification-before-completion`](../verification-before-completion/SKILL.md),
plus these design-specific checks:

- `guardrail_audit.dart`-backed tests still pass (zero blur, ≤1 gradient, no
  translucency) — or you deliberately updated them alongside an ADR (§2/§6).
- Token/primitive/press-motion golden tests updated and green (Phase 3 proof
  tests: token snapshot, primitive goldens, press-motion widget test).
- Any new/changed string still flows through the localization catalogs — the
  pseudo-locale + text-expansion sweep must stay green; a redesign is not an
  excuse to hardcode a string.
- Any new color pairing is contrast-checked (§4 method), not assumed.
- Low-end frame-budget trace unaffected on `Round`/`Knowoff` (§5).
- `dart analyze` / `dart format` clean; `flutter test` green.
- New components are built on `Ko*` primitives from the start — never a bare
  `Card`/`Chip`/`Banner`/`ListTile` sitting next to styled siblings (§3).

---

## §11. Anti-patterns

- ❌ **Softening back to generic Material** "to be safe" — defeats the ask;
  the palette already passes accessibility, so there's no safety reason to
  retreat.
- ❌ Reaching for `Opacity`, `BackdropFilter`, or blurred `BoxShadow` for
  "depth" — banned by the guardrail *and* by the style itself (§1.4, §5).
- ❌ Rotating, offsetting, or otherwise "loosening" a vote ballot, timer,
  form, or anything fairness-critical — structured disruption is macro-only
  (§1.6, §7).
- ❌ Leaving a stock Material widget (`ChoiceChip`, `MaterialBanner`, `Card`,
  default `Chip`) unstyled next to `Ko*`-styled siblings — the grammar must
  be total, not applied in patches (§3).
- ❌ Adding a second gradient or a brand-new hue without an ADR + updated
  token snapshot tests (§2, §6).
- ❌ Color-only state signaling anywhere (§1.7, §5).
- ❌ Treating a redesign as a one-shot visual pass on existing screens only —
  every *new* component from here on must be built on `Ko*` primitives from
  day one.
- ❌ Adding expensive per-frame motion to `Round`/`Knowoff` (§5, §7) in the
  name of "fresh".

---

## §12. Quick-reference cheat sheet

```
BORDER        3px solid ink (#141414) — canonical, don't vary casually
SHADOW sm     3,3  blur 0   — badges, chips, inline controls
SHADOW md     4,4  blur 0   — cards, buttons, panels (today's default `hard`)
SHADOW lg     8,8  blur 0   — overlays, hero/celebration, focus
SHADOW lift   7,7  blur 0   — hover state (pointer devices), grows from md
RADIUS        16 cards/sheets · 12 buttons · pill chips — soft neubrutalism, keep
GRADIENT      exactly one, reveal header only — locked token, ADR to change
PRESS         translate onto own shadow footprint, shadow → zero offset
HOVER         translate (-2,-2), shadow grows to `lift`
TILT          {0, ~1.1°, ~2°, ~2.9°} closed set — macro/decorative only, never on forms/votes/timers
DISPLAY FONT  Baloo 2 or Fredoka (OFL) — supersede ADR-006 to land it
BODY FONT     current geometric sans default is fine; Inter/DM Sans if swapping
COLOR MEANING violet=interact · lime=truth/reward · pink=accuse/risk · ink=information — fixed, never swap
CONTRAST      ink on every accent already clears AAA (§4) — verify new pairs, don't assume
NEVER         color-only signal · blur · >1 gradient · rotated forms/votes/timers · bare Material widgets
```

---

## Related

- [🎨 Visual Identity: Soft Neo-Brutalism Design Matrix](../../../BLUEPRINT.md#-visual-identity-soft-neo-brutalism-design-matrix) — the normative spec this skill implements.
- [`UI_REDESIGN_PLAYBOOK.md`](../../../docs/guides/UI_REDESIGN_PLAYBOOK.md) — the execution companion: order of operations, verification ladder, tooling, and the fail-and-learns from the rebuild that landed this matrix.
- [ADR-006 — Typography](../../../docs/design/ADR-006-typography.md) — superseded by [ADR-007](../../../docs/design/ADR-007-display-typeface.md), which locked Baloo 2 as the bundled display face (§8).
- [ADR-008 — Support accents](../../../docs/design/ADR-008-support-accents.md) — the `canvasDeep`/`tangerine`/`aqua` palette extension (§4).
- [`adr-writing`](../adr-writing/SKILL.md) — how to record any locked-token change (§2, §6, §8).
- [`self-review`](../self-review/SKILL.md), [`verification-before-completion`](../verification-before-completion/SKILL.md) — gates before staging (§10).
- [`minimal-change`](../minimal-change/SKILL.md) — extend tokens/primitives additively (§6), don't rewrite the theme.
- `client/lib/presentation/theme/`, `client/lib/presentation/widgets/`, `client/lib/presentation/icons/doodles.dart`, `client/lib/presentation/widgets/guardrail_audit.dart` — the code this skill operates on.
