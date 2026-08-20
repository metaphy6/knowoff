# ADR-004: Media engine lives in `server/pkg/media`

## Status
Accepted

## Context
The Media Engine is used by both the game server (`server/internal/*`) and the
pack-building CLI (`tools/mediapack`). Go modules do not allow an internal
package to be imported by another module, and `tools/mediapack` already replaces
`github.com/knowoff/knowoff/server` into `server/`.

## Decision
Move the media format, loader, relevance mesh, dealing, certification, and
signed-URL code from `server/internal/media` to `server/pkg/media`. The package
is imported by `server/cmd/knowoffd` and by `tools/mediapack`; no game logic
leaks into it.

## Consequences
- `tools/mediapack` can reuse pack structures and dealing code without code
  duplication or import hacks.
- The boundary between "media data" and "game state" stays explicit: the game
  engine consumes `media.Pack`/`media.Manager` but never makes pack-format
  decisions.
- Future clients (e.g. a headless simulator) can import `pkg/media` as well.
