# Content generation inputs

Editorial guidance and inputs for the text-content pipeline. The
[Blueprint](../BLUEPRINT.md) holds the product contract; these guides describe
how to write, review and test content against it.

- [Tone matrix](tone-matrix.md) — the four required tone buckets, separate from
  editorial dimensions and relevance bands.
- [Curator guide](curator-guide.md) — contribution, human review, humor playtests
  and deal-certification rules.
- [Humor development](humor-development.md) — lo-fi text humor, mode-specific
  pilots, themes, freshness and cultural adaptation, with historical evidence.

The 2026-09-12 target is **plain-text Nowns and cards in five selectable modes**:
Missed the Briefing, Secret Scale, Make Room, Bad Bargains and Top That. Use
separate reusable response and item pools with explicit mode/language suitability.
Start a bounded pilot with three themes and two cultures/languages; this does
not promise both languages at launch or certify any mode. Follow the
[playable media direction](../BLUEPRINT.md#playable-media-direction) and the
[transition contract](../docs/design/DESIGN-text-transition.md).

Draft and review before technical certification. Record exact candidate revision,
mode, language, rights, editorial dimensions and topical review dates. The current
Contributor Studio supports submission/review but does not provide all these
fields or author/deploy mode-aware Nown decks. Accepted text is curation input,
not automatically playable content. See
[Community operations](../docs/guides/COMMUNITY_OPERATIONS.md) for the implemented
workflow and its limits. No pack, runtime behavior or content activation changes
as a result of this planning update.

Agents use the shared [creation](../.agents/skills/knowoff-content-create/SKILL.md),
[review](../.agents/skills/knowoff-content-review/SKILL.md) and
[integration](../.agents/skills/knowoff-content-integrate/SKILL.md) skills.
Their source links and editorial record keep handoffs grounded in these guides;
the skills do not replace the Blueprint or record approval on a human's behalf.
All content work automatically follows the [High/Distant/Chaos dealing path](curator-guide.md#read-the-dealing-path-before-authoring)
through current tuning, server band construction/dealing and client delivery.
The text-only decision is recorded in
[ADR-012](../docs/design/ADR-012-text-only-selectable-modes.md).
[ADR-011](../docs/design/ADR-011-static-image-and-text-content.md) records the
superseded image/text contract. Its assets and provenance are historical inputs,
not active text-mode candidates; image alt-text is not an approved replacement.

## Media tools across workspaces

The toolbox below is for separately authorized promotional/how-to media or
historical asset inspection. It is not a dependency or production step for
playable text bundles, and this plan does not authorize an OS installation.

FFmpeg and FFprobe are command-line tools used through the terminal. Agents
should use existing system tools first: check both commands on `PATH` before
installing anything, in this or another workspace.

```bash
command -v ffmpeg
command -v ffprobe
ffmpeg -version
ffprobe -version
```

If either command is missing on Ubuntu/Debian, install the distribution's
`ffmpeg` package, which supplies both tools:

```bash
sudo apt-get update
sudo apt-get install --no-install-recommends ffmpeg
```

OS package installation requires user authorization when it has not already
been granted for the task; reuse existing authorization. On other operating
systems, follow the [official FFmpeg download page](https://ffmpeg.org/download.html)
and the platform's installation instructions.

Verify access from outside the repository:

```bash
(cd /tmp && command -v ffmpeg && command -v ffprobe && ffmpeg -version && ffprobe -version)
```

A system installation is available to other workspaces through `PATH`; no
project-local binary path or per-project installation is needed. Check again
on each workstation: copying these instructions does not install software.

Do not use media conversion to create playable content for the text target.
Avatars, store art and the how-to clip remain separately governed non-playable
assets. Preserve historical rights/source records during retirement; no tool
conversion or editorial retirement note proves a deployed release changed.
