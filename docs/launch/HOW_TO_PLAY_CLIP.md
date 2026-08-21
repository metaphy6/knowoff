# How-to-Play Clip Storyboard

The clip is the ≤45-second onboarding asset. It uses only real production UI
captures, no narration, and captions on the lime highlighter sweep so it is
silent-autoplay safe and readable on a phone.

## Format

- Length: ≤45 seconds.
- Source: screen capture from the production native app.
- Audio: none. Captions carry every beat.
- Typography: chunky rounded display face for captions, with the lime marker
  sweep behind key phrases.

## Seven-beat storyboard

| Beat | Seconds | Visual | Caption (on lime sweep) |
|------|---------|--------|------------------------|
| 1 | 0–5 | Four phones around a table. Nown appears on three; the fourth shows the Donower placeholder static. | “One of you can’t see this.” |
| 2 | 5–9 | A finger presses and holds the role card; the card flips to reveal DONOWER. | “Check your role in secret.” |
| 3 | 9–14 | Nown loads on every screen at once; cut to one phone showing only the placeholder prompt. | “Donowers see nothing. Nobody knows who.” |
| 4 | 14–21 | Turn sequence: cards hit the table one by one, each revealed instantly with the player name. | “Play a card that fits the picture.” |
| 5 | 21–28 | A draw announcement, a screen-shake poke, the Shuffle alert, Quick Chat bubbles flying. | “Draw, poke, shuffle, accuse.” |
| 6 | 28–36 | Knowoff blind ballot closes; votes land; the most-voted player is eliminated, role revealed as NOWER. | “Vote wrong and a Nower is out.” |
| 7 | 36–45 | Verdict screen: Donowers grin, all Nowns are revealed, Noin falls onto the scoreboard. | “Donowers win together.” |

## Production notes

- Capture each beat on a 4-player match with one human and three gamebot seats
  so the pacing is controllable.
- Keep UI chrome unobstructed; no debug overlays.
- Export in vertical 9:16 and square 1:1 for store listings; keep file size
  under 10 MB.
- Localize captions for every launch locale; the English copy above is the
  source of truth and is derived from the Game Rules chapter of ROADMAP.md.

## Ship gate

The clip is produced only on final production UI. It releases with the public
launch build and is updated whenever the match flow changes.
