# 📖 BLUEPRINT — moved into the roadmap monolith

The product spec and the roadmap are now **one document**, so an agent can
never implement the plan without the spec in hand:

➡️ **[`docs/planning/ROADMAP.md`](docs/planning/ROADMAP.md)** — spec
chapters first, the roadmap chapter ("🗺 Roadmap — Step-by-Step
Implementation Lifecycle") at the end.

- Each phase's **Spec (required reading)** line names the chapters it
  implements; when bullet text and a spec chapter disagree, the spec
  chapter wins.
- `make roadmap.status` parses the checkboxes in
  `docs/planning/ROADMAP.md`.
- This stub exists only so older links keep resolving — **do not add spec
  content here**.
