# Changelog

All notable changes to Knowoff are documented in this file.

## [Unreleased]

### Added

- Verdict screen now opens a Play Again window once a match finishes: every seat picks "same table" (rematch with this exact table once everyone agrees) or "new table" (leaves for a fresh Quick Play match); a seat that left or never reconnected in time opens up for backfill, so a new player who simply clicks Quick Play can land straight into the reopened table instead of a brand new one.

### Fixed

- Reveal a Hand no longer depends on a separate button below the hand, publicly broadcasts hidden cards before anyone asks to view them, or remains usable during the final five seconds of a turn.
- Knowoff result posters no longer show the stale ballot countdown or remaining-votes wheels; the result owns the full stage during its four-second display.
- Knowoff result window is 8 seconds again (was cut to 4), restoring visible time for the Nower/Donower role-reveal poster — the four-second falling reveal previously consumed the entire window, so the poster and Revote button never rendered before the round advanced.
- Discussion's Ready flag no longer carries over into the next round — it now resets when a fresh discussion phase opens, instead of leaving the button permanently locked from a Ready tapped last round.
- Ready can now be taken back any time before the window finalizes (discussion, Knowoff ballot, and the post-ballot result window) — tapping Ready again un-readies instead of being ignored; the button only locks once every active seat has agreed or the window's timer runs out.
- "Back to menu" on the Verdict screen now clears the finished match's session state (room code, seat, phase) before popping back — previously the stale state made the next Quick Play jump straight back into the same finished match instead of queuing.

### Changed

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
