# Content generation inputs

Editorial guidance and inputs for the media pipeline. The
[Blueprint](../BLUEPRINT.md) holds the product contract; these guides describe
how to write, review and test content against it.

- [Tone matrix](tone-matrix.md) — the four required tone buckets, separate from
  editorial dimensions and relevance bands.
- [Curator guide](curator-guide.md) — contribution, human review, humor playtests
  and deal-certification rules.
- [Humor development](humor-development.md) — restored lo-fi visual direction,
  GIF-led pilots, themes, freshness and cultural adaptation, with historical evidence.

Start by planning three themes for two target cultures/languages, prioritizing
rough, low-resolution reaction/action GIFs, followed by stills and supporting
text. Follow the [playable media direction](../BLUEPRINT.md#playable-media-direction);
the UI design matrix does not style game media. Draft and review candidates
before pack certification. Planning records hold editorial
dimensions and topical review dates; the current Contributor Studio does not
provide these fields or author/deploy Nown decks. Accepted text submissions are
curation inputs, not automatically playable content. See
[Community operations](../docs/guides/COMMUNITY_OPERATIONS.md) for the implemented
text workflow and its limits.

Agents use the shared [creation](../.agents/skills/knowoff-content-create/SKILL.md),
[review](../.agents/skills/knowoff-content-review/SKILL.md) and
[integration](../.agents/skills/knowoff-content-integrate/SKILL.md) skills.
Their source links and editorial record keep handoffs grounded in these guides;
the skills do not replace the Blueprint or record approval on a human's behalf.

## Media tools across workspaces

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

Use FFmpeg for trimming, cropping, frame-rate adjustment, audio removal and
animated WebP encoding; inspect metadata with FFprobe and watch the actual
loop. Follow the Blueprint's media limits and preserve the intended roughness.
Conversion is preparation, not editorial approval or pack certification.
