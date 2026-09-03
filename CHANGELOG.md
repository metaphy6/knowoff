# Changelog

All notable changes to Knowoff are documented in this file.

## [Unreleased]

### Fixed

- Knowoff result window is 8 seconds again (was cut to 4), restoring visible time for the Nower/Donower role-reveal poster — the four-second falling reveal previously consumed the entire window, so the poster and Revote button never rendered before the round advanced.
- Discussion's Ready flag no longer carries over into the next round — it now resets when a fresh discussion phase opens, instead of leaving the button permanently locked from a Ready tapped last round.
- Ready can now be taken back any time before the window finalizes (discussion, Knowoff ballot, and the post-ballot result window) — tapping Ready again un-readies instead of being ignored; the button only locks once every active seat has agreed or the window's timer runs out.
- "Back to menu" on the Verdict screen now clears the finished match's session state (room code, seat, phase) before popping back — previously the stale state made the next Quick Play jump straight back into the same finished match instead of queuing.

### Changed

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
