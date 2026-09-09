# Changelog

All notable changes to Knowoff are documented in this file.

## [Unreleased]

### Added

- Using the Free Card specialty now fires a loud "FREE CARD!" announcement for the whole table while the draw pile celebrates: the card count swells big for a beat and the price chip flips from −5 to a bouncing FREE until the free draw is spent.
- Debug builds now have a dev-only specialty picker in the bottom-right dev tools: pick any of the five specialty cards and the server drops it into your hand as if it had been dealt (disabled entirely in prod), then the normal use flow runs unchanged — handy for testing specialty behavior without waiting for the deal.
- Verdict screen now opens a Play Again window once a match finishes: every seat picks "same table" (rematch with this exact table once everyone agrees) or "new table" (leaves for a fresh Quick Play match); a seat that left or never reconnected in time opens up for backfill, so a new player who simply clicks Quick Play can land straight into the reopened table instead of a brand new one.

### Fixed

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

- `make git` never creates empty commits anymore: all pending tracking rows in a staging window now ride one real commit (first pending summary as the subject, the rest under "Also includes:", one `[run_id]` trailer each), and with a clean tree the rows simply wait for the next real commit instead of committing empty markers.
- Developer-granted specialty cards remain current-hand test overrides; subsequent rounds still deal a fresh role-blind specialty, as specified by the game rules.
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
