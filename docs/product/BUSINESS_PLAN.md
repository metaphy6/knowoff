# Knowoff text transition: business and product plan

**Status: planning baseline, 2026-09-12.** The owner has selected text-only
Knowoff with five selectable gameplay modes. This plan prepares commercial
execution alongside the [Blueprint](../../BLUEPRINT.md) and existing
[Roadmap](../planning/ROADMAP.md). It does not implement, launch, purchase,
activate content or claim market validation. Rules and economy values belong
in the Blueprint and tuning; implementation order belongs in the Roadmap.

## Goal, boundaries and evidence

Build a sustainable social deduction game in which a short text prompt and
simple card choices give 4 or 6 people something worth arguing about, with
returning human tables funding content, moderation and reliable operation.

The intended offering contains **Missed the Briefing, Secret Scale, Make
Room, Bad Bargains and Top That**. Missed the Briefing is the onboarding
default. The initial text release has no specialties or production bot
backfill. Players choose a mode; rollout can keep an unready mode unavailable
without silently substituting another. All five remain intended deliverables.

This plan excludes market-size claims, a revenue forecast presented as fact,
unapproved price changes, paid advantages, a new currency, voice/video,
real-money item trading, AI scoring of player choices and a public launch
before operational gates pass. Bad Bargains exchanges fictional match cards;
it creates neither inventory value nor an external marketplace.

| Basis | What can be concluded | What remains unproven |
|---|---|---|
| Owner direction and [mode design](../design/DESIGN-text-game-modes.md) | Text-only, five intended modes, concrete interaction defaults | Fun, balance, retention, market demand and commercial viability |
| Blueprint economy and platform baseline | Shared fair economy; Android/PWA first, iOS fast-follow | Provider readiness, store acceptance, real prices and revenue |
| [Current mechanics audit](../code/MODULE-media-engine.md) and Roadmap | Existing server/client and content foundations with recorded gaps | A released mode selector, mode-safe dealing, migration integrity or production packs |
| [Community operations](../guides/COMMUNITY_OPERATIONS.md) | A documented text submission and challenge workflow | Full production moderation, pack activation and sustainable throughput |
| Numbers below | Explicit planning assumptions and decision rules | No playtest results, invoices, market surveys or live cohorts have been supplied |

Revisit this plan after each evidence gate. Record sample, dates, rule/pack
versions, exclusions and the accountable human's decision. A changed threshold
must be documented before the next cohort, never adjusted to make a completed
experiment look successful.

## Audience, jobs and proposition

These are recruitment hypotheses, not validated demographic segments. Humor
preferences do not establish age, identity or willingness to pay.

| Priority | Candidate player / job | Proposition to test | Evidence and stop signal |
|---|---|---|---|
| Primary | A person seeking a short social break with strangers | Join Quick Play, understand one action, bluff and vote without writing a performance | Observe first-match completion and D7 return; stop acquisition if queues strand recruits |
| Activation channel | A friend-group organizer with exactly 4 or 6 people together | Share a QR/code; browser guests join; everyone gets a role and a simple choice | Observe unaided joins and a second match; record group-size mismatch and device friction |
| Retained player | Someone who enjoys reading others but wants variety | Change the reasoning task while keeping familiar roles, evidence and votes | Compare voluntary mode trials and returns against confusion and queue fragmentation |
| Content contributor | A player who wants to make their group laugh and receive credit | Submit short original text, receive a reasoned review, see accepted work credited | Measure review burden and later certified usefulness, not raw submission volume |

Working positioning: **A text party game where someone missed the context.**
The differentiating hypothesis is several short card interactions around a
shared deduction loop. Test it against players' actual alternatives, such as
another party game, a group chat or doing nothing; no superiority or competitor
market-share claim is established here.

The first-run path should explain hidden context, one mode action and Knowoff,
then let the player experience a match. Make the selected language, room size,
absence of specialties, mode availability and access allowance visible before
Ready or queue entry. Marketing must describe only released functionality;
concept examples and synthetic fixtures are not store screenshots or proof of
a five-mode release.

## Five modes and queue liquidity

| Mode | One-line action | Additional release concern |
|---|---|---|
| Missed the Briefing | Play a response to a situation you may not know | Generic safe responses and first-seat disadvantage |
| Secret Scale | Place an item on a 1–5 scale with a hidden criterion | Readable ratings, explanation burden and repeated obvious placements |
| Make Room | Replace one item in a shared three-item bag | Legible before/after evidence and meaningful removals |
| Bad Bargains | Offer a swap that another player accepts or refuses | Response delays, collusion, card ownership and earnings per minute |
| Top That | Play an item claimed to top the last one | Arguable comparisons and repeated escalation patterns |

Quick Play eligibility partitions by **mode × 4/6 seats × content language**;
five modes and two languages already imply 20 possible queue cells. This is a
planning count, not measured capacity. UI language need not equal the locked
content language. Do not create paid-pack or region partitions casually; each
additional partition needs a documented matching and population rationale.

For a rough empty-queue check, if compatible arrivals average `lambda` players
per minute and table size is `n`, the first entrant's expected wait for the
other seats is approximately `(n - 1) / lambda` minutes. This assumes steady
independent arrivals, no departures and no reconnect or admission failures;
it is a sanity check, not a percentile forecast or a replacement for queue
simulation. Measure real arrivals and abandonment before interpreting it.

Exposure proceeds by evidence, not a promised calendar:

1. Invite-only prototypes test every mode in Local Rooms with live rewards and
   leaderboard credit disabled. Recruit complete groups to avoid conflating
   mode quality with empty matchmaking.
2. After mode/locale/size correctness and content gates, expose ready Local
   Room choices. Run scheduled, clearly announced Quick Play sessions for the
   first ready cells. The default is Missed the Briefing when available.
3. Expand a Quick Play cell only when its measured queue and retention gates
   pass; recruit into a declared session window before buying ongoing traffic.
   Keep unready cells out of matchmaking. Show availability honestly.
4. Offer **Keep waiting**, **Change mode** or **Leave queue** after the configured
   timeout. A voluntary change leaves the prior queue and joins the new one;
   there is no hidden mode, size, language or bot fallback.
5. Review the availability matrix weekly. If a cell regresses, stop new
   admissions through audited availability controls and let running matches
   finish under their pinned contracts. Offer alternatives explicitly. Record
   the work/evidence required to reopen it; do not quietly remove an intended
   mode from the roadmap.

Local and rematch settings require a full connected table and unanimous Ready;
membership or settings changes clear readiness. Commercial tests may change
exposure and explanation, never secretly change match rules, rewards or roles.

## Acquisition and retention experiments

Experiment budgets are **not authorized spending**. Before execution, the
owner records a cash limit, staff-hour limit, stop date and named operator.
Use one cohort identifier in server audit-derived reporting; do not add a
third-party client analytics SDK. Capture only the attribution required for
the experiment and include it in the privacy and deletion design.

| Experiment | Initial sample / timebox assumption | Primary decision rule | Guardrail / next action |
|---|---|---|---|
| Hosted friend-group play | 10 independent groups, at least 5 per table size, over two weeks in a ready pilot language | At least 8/10 start without staff taking over, and 6/10 voluntarily start a second match | Qualitative discovery only; record failures and repair onboarding before recruiting more |
| Mode comprehension | At least 3 fresh tables per mode × size × pilot language; each plays at least 2 matches | At least 80% of observed first turns complete legally without facilitator correction; no recurring misunderstood rule remains unresolved | Report counts and group clustering; this small sample is a usability screen, not balance proof |
| Small community session | Two separately recruited session windows, at least 100 human queue joins and 30 first-time human players each | Meet queue and completion gates in both windows; collect invitations and opt-in return intent separately from actual return | No unscreened content, unsolicited outreach or unstaffed moderation; a failed session gets a diagnosis before repetition |
| Positioning / store-message test | At least 20 target-player interviews or moderated page tests split between two literal explanations | At least 80% correctly explain hidden context, selected action and voting after exposure | Do not interpret a small preference count as a conversion uplift; use a later powered experiment if traffic supports one |
| Return and mode variety | Two weekly activation cohorts, each at least 100 new human accounts in the exposed cells | Apply D1/D7 and second-match gates below; inspect per-language and per-mode behavior | Distinguish hosted attendance from organic returns; do not merge a weak locale into an aggregate |
| Paid acquisition pilot | Only after reliability, retention and commercial gates; budget and minimum detectable effect recorded first | Observed CAC per activated player is below conservatively observed contribution LTV at the chosen payback horizon | Stop at the approved spend/time limit; if revenue history cannot support LTV, label exploratory and do not scale |

Retention hypotheses: a quick second match, familiar rematch controls,
meaningfully different modes, fresh certified text and visible contribution
credits will encourage return. Test these separately. The Weekly Nown Challenge
is a later repeat-visit channel only after screening, immutable voting,
terms-consent, safe publication and idempotent close/payout work end to end.
Do not substitute notifications, extra Noin or a more profitable mode for
evidence that players enjoy returning.

## Fair monetization and existing customer treatment

Retain the Blueprint economy; this document does not set new runtime prices:

- Noin buys Play Passes, theme packs and identity cosmetics. The existing pass
  baseline is 250 / 600 / 1,200 Noin for 1 / 3 / 7 days; point conversion is
  100 points to 1 Noin. Reconcile published amounts with tuning before launch.
- Free Quick Play allowance is currently three matches per server day. A pass
  or Premium removes that allowance; Local Rooms remain uncapped. Allowances,
  gameplay earning caps, first-win eligibility and leaderboard limits are
  shared across modes and cannot reset on switching or migration.
- Modes have no individual unlock fee. Quick Play uses core/free featured
  content. Paid theme packs retain the Host Pass rule for Local Rooms: guests
  never need to buy the host's pack. Content must still pass the same fairness
  and language gates. No purchase affects dealing, roles, votes or scoring.
- Premium is the cash subscription with unlimited Quick Play and ad removal;
  Noin passes do not remove ads. Noin-priced items are earnable; Premium's
  cash-only benefits must not be advertised as earnable with play.
- The planned optional post-match doubler requires verified, idempotent
  server-side ad settlement; Premium receives its defined ad-free equivalent.
  Neither an ad-client callback nor a receipt replay can grant currency.
- Prototype sessions receive no live rewards or leaderboard credit. Production
  rewards require human-count eligibility and mode-specific farming checks;
  no points or Noin are granted for rating values, accepted trades, item
  collections or a supposedly correct answer.

The target that free players earn a one-day pass in about two active play-days
is **an unvalidated economy target**. Measure from the real allowance, role
distribution, match outcomes, draw penalties, first-win grants, shared cap and
conversion rules. Simulate both typical and farming behavior and confirm live
cohorts before making a marketing promise. Earnings per minute matter because
trade acceptances preserve hand size while most other mode actions consume it.

Before cutover, inventory existing users, wallet balances, ledger entries,
passes, Premium expiry, point balances, packs, contributor credits and challenge
awards. Reconcile totals after migration and restore rehearsal. Replacing an
image pack must not silently erase its entitlement: record a replacement text
pack mapping, or an owner-approved compensation/refund disposition before that
SKU is retired. Preserve original transaction provenance, reconcile provider
records, avoid duplicate grants, and communicate the exact treatment. If no
real users or purchases exist, prove that using deployment/ledger evidence;
do not assume an empty production database.

## Content, localization and community operations

Text removes the playable image-production lane; it does not remove editorial
work. The limiting resources are strong reusable writing, cultural judgment,
mode-specific playtests and timely moderation. Follow the [Curator Guide](../../content/curator-guide.md)
and [humor development guide](../../content/humor-development.md); this plan
does not certify example cards or replace contribution approval.

| Responsibility | Required operating record / gate |
|---|---|
| Content lead | Shared response versus item pools; supported mode/language matrix; per-version accepted candidates, dead/unreachable cards, repeat exposure and full-schedule feasibility |
| Locale editor | Native-language rewrites, explanation burden, cultural/age suitability, text expansion, screen-reader reading, source/rights and expiry review; translation alone is insufficient |
| Curator | Automated screen plus human decision; actual 4/6-player hands and changing board states; explicit rejection/rewrite reasons; no claim that cosine bands measure humor or correctness |
| Community operator | Versioned terms, consent evidence, immutable submission history, queue capacity, response times, abuse escalation and contributor credits |
| Release operator | Certified immutable pack version, rollback/takedown mapping, active-match pinning and audit evidence; approval never directly activates content |

Start the editorial pilot with three themes and two cultures/languages, as the
humor guide proposes. Select those languages from recruitment and editor
capacity; an available UI translation is not a launch-ready content locale.
Every exposed mode/language/size cell needs its own content and play evidence.
Reusing item pools can lower authoring cost, but suitability for one mode is
not certification for another. No untranslated secret fallback may cross the
locked language boundary.

Track candidate-to-approval yield, approval-to-certification yield, hours per
released card, mode coverage, player-repeat exposure, rejection reasons and
cost per activated pack. A 70/20/10 evergreen/seasonal/topical release mix is
an editorial hypothesis from the humor guide, not an enforced quota. Topical
items require source, review and expiry records and human review at expiry.
Neither AI generation nor automated screening removes human responsibility.

Contribution rewards need one named milestone and an idempotency key. Retain
the current documented approval-time grant/credit behavior as the transition
baseline, and reconcile any legacy records using a different milestone before
activation. Publication or migration must not pay the same accepted item
again. Challenge rewards follow their separately defined settlement rules;
do not silently apply gameplay-cap semantics to every reward category.

For each language, budget `candidate count × author/rewrite minutes`,
`review count × review minutes`, playtest facilitation, moderation coverage,
provider calls and revision work. Measure acceptance rates separately by
language and mode. If review backlog exceeds available capacity, pause intake
or exposure with a clear notice; do not bypass screening to hit a content date.
Initial service targets to test are acknowledgement within two working days
and review within seven, with the staffed hours and escalation contact visible.

## Unit economics and funding assumptions

The following arithmetic is illustrative and uses **USD-equivalent accounting
units**, not store prices, quotes, provider fees, revenue evidence or an
approved budget. Replace every input with dated invoices/provider settlements
and measured workloads. Do not claim a zero-cost home server: staff time,
hardware, electricity, backups and downtime have costs even without a VPS bill.

Define monthly active humans `A`, completed **player-matches** per active human
`m` (one six-person match contributes six player-matches), Premium payer share
`p`, realized net subscription receipts per payer-month `S`, net Noin-bulk
receipts per active human `B`, verified rewarded impressions per non-Premium
active human `q`, and net receipts per thousand verified impressions `e`.
Use observed renewals/refunds and revenue recognition appropriate to monthly
and yearly subscriptions; do not count a full yearly payment every month.
Allocate queue, incomplete-match and retry infrastructure costs into the
runtime unit cost as well; counting only successful requests understates it.

```text
Net monthly receipts = A × (p × S + B + (1 - p) × q × e / 1,000)
Variable runtime cost = A × m × cost per completed player-match
Monthly operating contribution = receipts - runtime - content labor
                                - content/screening providers - moderation/support
                                - hosting/backups/monitoring
Acquisition contribution = operating contribution - acquisition expense
CAC per activated human = attributable acquisition spend / new activated humans
Observed contribution LTV per activated human =
  (cumulative net cohort receipts - attributable service/content/support costs)
  / activated humans in that cohort
Runway months = (available cash - protected reserve) / monthly cash burn
```

Do not count earned Noin as cash revenue, a card trade as a purchase, a repeated
receipt as a new payer, or unpaid founder work as free. LTV must specify its
observation horizon; no indefinite retention extrapolation supports spending.
Runway is undefined until cash/reserves are supplied and burn is positive;
actual financing, tax and accounting treatment need their own owner-reviewed
records before external use.

| Monthly input / result | Cautious assumption | Middle assumption | Stronger assumption |
|---|---:|---:|---:|
| Active humans `A` | 1,000 | 1,000 | 1,000 |
| Player-matches per active `m` | 8 | 8 | 8 |
| Premium share `p` | 1% | 3% | 6% |
| Net subscription receipt `S` | 3.50 | 3.50 | 3.50 |
| Net bulk receipt per active `B` | 0.02 | 0.07 | 0.15 |
| Verified impressions `q`; net per thousand `e` | 1; 1.00 | 3; 3.00 | 5; 5.00 |
| Net receipts from formula | 55.99 | 183.73 | 383.50 |
| Runtime (8,000 player-matches × 0.005) | 40.00 | 40.00 | 40.00 |
| Content labor (10 hours × 30) | 300.00 | 300.00 | 300.00 |
| Content/screening providers | 30.00 | 30.00 | 30.00 |
| Moderation/support (5 hours × 30) | 150.00 | 150.00 | 150.00 |
| Hosting/backups/monitoring | 50.00 | 50.00 | 50.00 |
| Contribution before acquisition and development | −514.01 | −386.27 | −186.50 |

All three examples lose money before acquisition, initial development,
transition work and other business overhead. The staffing assumptions may be
far too small for five modes and multiple languages; they illustrate the
sensitivity, not a staffing recommendation. Increasing active users does not
leave moderation or content costs constant. Run sensitivity cases for lower
retention, no ads/provider delay, twice the review hours, a content takedown,
more six-seat play and a second language requiring separate curation.

Before funding a release, the owner supplies current cash, protected reserve,
approved implementation/content hours, monthly maximum burn and spending
authority. Itemize one-time migration/rehearsal, legal/provider setup, launch
assets and localization separately from recurring costs. Release work stops
at the agreed cash/hour limit for a documented scope decision; this planning
change creates no spending authorization.

## KPI definitions and decision gates

The following thresholds are **initial decision assumptions**, not validated
benchmarks or promised outcomes. Freeze them before collecting a cohort.
Report numerator, denominator, cohort dates, rule version, pack version,
mode, table size, language and exposure state. Use server-derived events;
hash/pseudonymize identity in analytical access, retain no secret prompt or
private hand in public telemetry, and define access/deletion/retention before
collection. Economy grants remain private even in dashboards shared with
players or community moderators.

| Metric / gate | Definition and initial acceptance assumption |
|---|---|
| Integrity and safety | Zero unresolved role leaks, duplicate transfers/rewards, lost entitlements, unauthorized actions or critical moderation gaps; automated proofs and migration/restore drills are mandatory regardless of engagement |
| First-match activation | New eligible human starts and completes a valid match within the first session; report against all new arrivals and separately against queue entrants, including abandonment |
| Completion | At least 90% of started human matches reach a normal verdict without a forfeit, low-population ending or infrastructure failure in each exposed cell; retain abnormal endings in the denominator |
| Queue health | At least 90% of joins start within the configured queue timeout, and no more than 10% leave before start; at least 100 joins in each of two distinct staffed windows per exposed cell, with raw waits and p50/p95 published |
| Second match | At least 40% of activated humans voluntarily complete another match within seven days; exclude forced tutorial repeats and report hosted cohorts separately |
| Retention | D1 at least 25%, D7 at least 10% of activated new humans, in each of two weekly cohorts of at least 100; return means completing a valid match on UTC day 1/day 7 after activation, not merely opening the app |
| Durable participation | Weekly distinct humans completing at least two valid matches on different days; main product-health trend, segmented by mode/language and paid/free access |
| Balance | At least 200 completed human matches per mode/size/language release cell; report Nower-team win share with uncertainty, first-seat-role difference, forfeits, draw use and repeat opponents. Initial target: 95% interval for team win share contained in 35–65%; difference in Nower-team win share when the first seat is Nower versus Donower no more than 15 percentage points, with uncertainty explicitly reviewed |
| Duration and farming | Record p50/p95 duration and rewards per human-minute per cell; the existing approximately 5/8-minute match expectation is a hypothesis. Investigate a mode above 1.25× pooled reward rate before reward enablement; deliberate abuse tests and shared-cap proofs pass independently |
| Content readiness | No unreachable/dead cards, failed full-schedule deals, missing rights/consent, missing human review or missing language/mode certification in an activated pack; report repeat exposure and explanation burden |
| Commercial readiness | Provider settlement/refund/ad replay proofs, ledger/entitlement parity, real cost record, staffed support and owner-approved burn ceiling; no paid scaling while these or retention/liquidity gates fail |

Balance counts **one team outcome per match**, not one independent observation
per player. Repeated players/tables create correlated observations; use
cluster-aware analysis by recruitment group/player when possible and report
effective sample limitations. An ordinary binomial interval is only a
screening approximation for independent matches. Small first-seat subgroups,
rare abuse, repeated queues and simultaneous mode comparisons need additional
sampling; an inconclusive interval stays inconclusive. Do not describe all
five modes as balanced because a pooled average passes.

Compare earnings per minute within the same size/language and standardize
role mix and prior daily-cap usage. A difference caused by a different player
mix is not by itself a profitable mode exploit. Report credited rewards and
pre-cap eligible rewards separately without exposing either to other players.

If a quantitative gate fails, inspect the linked recordings/events and decide
whether the cause is rules, content, onboarding, outages, acquisition mix or
low sample quality. Change one relevant lever, record a new version and test
on a fresh cohort. Never hide a failing cell by excluding disconnects,
switching denominators, rewarding repeat play or pooling it with a larger one.

## Platform policy checkpoints — checked 2026-09-12

These primary sources support release-review tasks, not a compliance approval.
Recheck the exact storefront/market policy before submission and retain the
reviewed version alongside actual implementation evidence.

Apple's UGC rules require filtering, reporting with timely response, abusive-user
blocking and published contact details, and restrict services primarily used for
random/anonymous chat. **Planning inference:** Knowoff's bounded game discussion
and anonymous accounts need a documented product/policy review; calling it a game
does not by itself establish acceptance. Review login, deletion and purchase
flows with the same guidance before the iOS fast-follow.
[Apple App Review Guidelines](https://developer.apple.com/app-store/review/guidelines/).

Google Play's UGC policy includes accepted user terms, moderation and appropriate
report/block functionality. Complete a tested abuse-control journey; an admin
queue alone is not evidence of the required player-facing controls.
[Google Play UGC policy](https://support.google.com/googleplay/android-developer/answer/9876937?hl=en-GB).

Google's digital-purchase rules include market/program exceptions. Platform
billing remains Knowoff's chosen plan; confirm the applicable requirements and
net proceeds rather than assume one universal fee or alternative-payment rule.
[Google Play Payments policy](https://support.google.com/googleplay/android-developer/answer/9858738?hl=en).

No fixed age-rating number or self-declared-age gate alone establishes store
acceptance. Record per-market content/age/privacy decisions and actual platform
rating questionnaires; hold incompatible content/markets rather than silently
claiming the existing adult-pack policy is approved.

## Responsibilities, handoff and acceptance

The owner is the accountable decision maker. The roles below are required
responsibilities, not claims that a staffed team exists. One person may hold
several roles; assign a named human and weekly capacity before release, and
retain a separate human review step for publication-sensitive decisions.

| Owner role | Deliverable and evidence |
|---|---|
| Product owner | Signed exposure matrix, hypothesis/threshold revisions, budget, existing-customer disposition and release decision |
| Engineering lead | Mode contracts, capability/version cutover, shared caps, removal inventory, full test results and data restore/rollback parity |
| Content/locale lead | Certified pack inventory, review/playtest records, capacity and expiry/takedown plan for each enabled cell |
| Community/safety lead | Staffed report/intake hours, escalation contacts, response-time evidence and challenge readiness |
| Growth/research lead | Recruitment consent, experiment brief, acquisition costs, cohort definitions and unbiased results |
| Operations/finance lead | Cost ledger, invoices, settlement/refund reconciliation, continuity plan and burn review |

Business execution is attached to the existing Roadmap, not a second technical
phase sequence. The following are small reviewable planning/evidence artifacts
for its content, realtime, hardening, economy/launch and community work:

- [ ] Record named owners, pilot languages, staffed hours and an explicit cash/hour ceiling; accept when each field and approver is present.
- [ ] Record the mode × language × size exposure matrix; accept when every intended cell is either gated-open with evidence or closed with its next proof named.
- [ ] Complete one mode/locale playtest brief with recruitment, versions and scoring sheet; accept after a dry run produces the required observations without revealing private roles.
- [ ] Reconcile the product price/access table with tuning and current entitlements; accept when discrepancies and grandfather/replacement decisions have an owner and migration test.
- [ ] Record the contribution reward milestone and deduplication proof; accept when approval, re-review, publication and migration cannot grant twice.
- [ ] Prepare the first acquisition brief with sample, stop rule, spend limit and attribution contract; accept when no metric depends on an unavailable event or client analytics SDK.
- [ ] Replace illustrative cost inputs with the first dated cost/settlement record; accept when formulas reconcile and cash versus earned Noin is separated.
- [ ] Record platform, privacy, age/content, billing, ad-consent and contributor-terms review evidence for each intended market; accept when placeholders and unsupported store claims are removed from launch assets.
- [ ] Review one launch listing and the onboarding clip against the enabled matrix; accept when mode names, screenshots, rewards and accessibility statements match tested production behavior.
- [ ] Execute a go/hold review against the KPI and integrity gates; accept when failed/inconclusive cells remain unavailable and rollback/support owners are reachable.

Touched planning surfaces: this plan and [product index](README.md),
[Glossary](../project/GLOSSARY.md), Blueprint, Roadmap, mode design,
[launch copy](../launch/STORE_COPY.md), [clip brief](../launch/HOW_TO_PLAY_CLIP.md)
and [community operations](../guides/COMMUNITY_OPERATIONS.md). Technical
acceptance requires the Roadmap's protocol, unit/integration/race, database,
client/accessibility, migration/restore and repository test/lint proofs; a
business review cannot waive them. No runtime feature is completed by checking
a planning artifact.

## Risks and decision triggers

| Risk | Detection | Mitigation / decision |
|---|---|---|
| Five-mode queues fragment a small audience | Queue arrivals, timeout/abandonment by cell | Staged availability and scheduled sessions; explicit player choice; no concealed bots |
| Text humor fails in another language | Explanation burden, low completion/return, editor rejection | Rewrite and retest per culture; hold that locale rather than ship literal translations |
| A mode becomes repetitive or unfair | Repeat exposure, role/seat results, draws and farming probes | Refresh certified pools or revise one rule under a new version; retain common economy |
| Public trade/board history exposes private data | Role-scoped payload and reconnect proofs | Hold release, repair serialization boundaries and audit traces before further play |
| Legacy images are removed too broadly | Entitlement inventory and dependency/retention audit | Remove playable image paths only after cutover; retain required avatars, branding, consent/audit/history and backups under policy |
| Old clients or old matches corrupt cutover | Capability/version rehearsal and restore reconciliation | Drain/pin contracts, explicit upgrade path and bounded rollback window; do not erase old migrations/history |
| Contribution volume overwhelms people | Age of queue, reviewer hours and adverse reports | Cap intake, staff review or pause calls; never auto-publish to meet a release date |
| Store/provider requirements or costs differ from assumptions | Dated official-source/provider review and test settlements | Revise launch budget/copy before purchase integration or submission; no forecast from undocumented fees |
| Revenue cannot cover content/safety effort | Actual contribution and burn versus ceiling | Reduce exposure/acquisition, improve proven retention/costs, or make an explicit funding/scope decision |

No evidence yet establishes a flawless transition. The release case is a
traceable set of passing product, technical, migration, content, operating and
commercial gates, with unresolved items visible and disabled until proven.
