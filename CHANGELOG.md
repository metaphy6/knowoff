# Changelog

All notable changes to Knowoff are documented in this file.

## [Unreleased]

### Changed

- Knowoff results now open with a four-second falling eliminated-player reveal, then show the Nower or Donower result card and Revote on the Knowoff screen without a Ready panel or duplicate result overlay.
- Knowoff result windows now finalize after four seconds instead of fifteen.
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
