# Original frame-animation experiments — 2026-09-11

Three new playable-format GIF experiments, samples 26–28. All 25 previous samples remain unchanged. This is a small method trial, not ten completed replacements or a certified media pack.

## Method and provenance

Built-in OpenAI imagegen produced one 4 × 4 sheet of sixteen progressive poses for each asset from an original text prompt. No images or footage were supplied as references. OpenArt was not used. The returned tool did not identify model version or seed; those remain unknown. The exact prompts, original source sheets, crop/timing parameters, output hashes and FFmpeg commands are retained.

FFmpeg 8.0.1 crops each square pose and assembles a silent, infinitely looping animation with timed holds and a blunt reset. These have real changing subject poses, but are choppy frame animations, not continuous generated video. The still sheets themselves are source material, not additional completed GIFs.

## Mechanical verification

All GIF and animated WebP outputs fully decoded with Pillow. Each has sixteen unique decoded frames, an infinite loop marker and no dimensions above 480 px or size above 2,000,000 bytes. Actual dimensions are 305 × 305; generated sheets are 1254 × 1254, retained only as sources. No output was upscaled.

| Sample | GIF bytes | WebP bytes | WebP duration |
| --- | ---: | ---: | ---: |
| 26 | 1060178 | 318466 | 3.666 s |
| 27 | 960521 | 338526 | 3.749 s |
| 28 | 1121487 | 67640 | 3.749 s |

SHA-256 comparison preserved all 113 prior batch files, including all earlier media and galleries. Production log: `/tmp/agent-runs/original-flipbooks-produce--20260911T131607Z-590935.log`.

## Editorial review

- 26: keep for owner review. The head disappears into the box, then rises with paws on the rim and wide eyes. The shared hiding/regret idea reads without a caption. Cat/box positioning shifts mildly between poses.
- 27: keep for owner review. The mug stays on the table while an empty hand performs a sip, then the face changes to confusion and eye contact. The hand-to-mouth action has visible jumps.
- 28: keep for owner review. A bright wave freezes and turns into a hand-behind-head recovery. Expressions and arm movement read clearly; the hair and background vary subtly.

All source poses were visually inspected in temporal order, and compressed playback was sampled in the browser at card size. Full uninterrupted real-time viewing: not run. The user should judge choppiness, timing, humor and reset before accepting this method. There is no continuous-video quality claim.

## Commercial-use basis

These new assets contain no downloaded Tenor footage, stock media, fonts, music or supplied reference images. Prompts request fictional people and unbranded objects rather than known characters or real-person likenesses. The inspected images showed no obvious logos or watermarks; incidental background shapes were not separately identified.

[OpenAI Terms of Use](https://openai.com/policies/row-terms-of-use/), checked 2026-09-11, assign output rights as between the user and OpenAI to the extent permitted by law. They also say output may not be unique, users remain responsible for respecting third-party rights, and disclaim non-infringement warranties. This is the commercial-use basis, not a promise of zero legal burden, exclusive copyright, or a public-domain/CC0 declaration. Account-specific agreement applicability and independent legal clearance were not established.

No Tenor permission is being claimed. Human rights/originality review, automated screening, 4/6-player playtests, compatible embeddings, technical certification, app integration and activation: **not run**.

## Repository handoff

Media checks passed. Staging remains blocked by the previously recorded Go native WebP build and Flutter test/analyzer failures; see [prior validation](validation.md). Those unrelated gates were not rerun because no application code changed and the same blockers are already evidenced. No existing content was reverted. No commit or push was performed.
