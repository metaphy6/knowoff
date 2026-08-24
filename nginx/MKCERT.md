# 🔐 mkcert — trusted local HTTPS for `*.knowoff.local`

This proxy defaults to a **self-signed** certificate, which works everywhere
but makes every browser show a "your connection is not private" warning.
[mkcert](https://github.com/FiloSottile/mkcert) replaces that with a
certificate your own machine already trusts — no warnings, no click-through.

This doc explains what it actually does, how to use it in this repo, and
how to remove it again. If you just want the commands, see
[TL;DR](#tldr). If you want to understand *why* it works, see
[How it works](#how-it-works).

## TL;DR

```bash
# 1. Install mkcert (once per machine)
sudo apt-get install -y mkcert        # Debian/Ubuntu; use your OS's package manager otherwise

# 2. Create + trust a local Certificate Authority (once per machine)
mkcert -install

# 3. Issue a certificate for knowoff.local + its subdomains (once per clone)
cd nginx/certs/external
mkcert -cert-file fullchain.pem -key-file privkey.pem knowoff.local "*.knowoff.local"
cd -

# 4. Make nginx pick it up
docker compose -f deploy/compose/docker-compose.yaml restart nginx
```

Open `https://app.knowoff.local` — no warning, green padlock.

## How it works

A normal **self-signed** certificate vouches for itself: "trust me, I'm
`knowoff.local`" — with no third party backing that claim, so nothing
trusts it by default. That's why browsers warn.

A **real** certificate (e.g. from Let's Encrypt) is signed by a
**Certificate Authority (CA)** that your browser is already configured to
trust out of the box. The browser checks: "is this cert's signature from a
CA I trust?" → yes → green padlock. Public CAs can only do this for domains
they can verify you own over the public internet, which is impossible for
`knowoff.local` (it isn't a real, publicly resolvable domain).

**mkcert's trick: become your own CA, locally.**

1. `mkcert -install` generates a private key + certificate for a brand
   new Certificate Authority that exists *only on your machine*, then adds
   that CA's public certificate into your OS's trust store (and Firefox's
   separate one, since Firefox ships its own instead of using the OS's).
   From this point on, your machine/browsers trust *anything signed by that
   CA* — exactly like they trust Let's Encrypt, DigiCert, etc.
2. `mkcert knowoff.local "*.knowoff.local"` generates a **new** key pair +
   certificate for those exact domain names, signed by the CA from step 1
   (not self-signed).
3. When nginx presents that certificate, your browser verifies the
   signature chain back to the local CA it already trusts → no warning.

Nothing changed about `knowoff.local` itself or DNS — only your machine's
opinion of who's a trustworthy CA. This is why the cert only becomes
trusted **on machines where you ran `mkcert -install`** — cloning this repo
onto a different computer (or a teammate's) does not carry that trust with
it; each machine needs its own `-install` + cert-issue pass.

## Where the files live

Two *completely separate* sets of files are involved — mixing them up is
the most common source of confusion:

| What | Path | Committed? | Scope |
|---|---|---|---|
| The local CA's key pair (`rootCA.pem`, `rootCA-key.pem`) | `$(mkcert -CAROOT)` — typically `~/.local/share/mkcert` on Linux, entirely outside this repo | Never | Your whole machine, every project that uses mkcert |
| This project's issued cert (`fullchain.pem`, `privkey.pem`) | [`nginx/certs/external/`](certs/external/) | **No** — gitignored | This repo, this machine |
| The copy nginx actually serves | `nginx/certs/active/` (copied there by [`entrypoint.sh`](entrypoint.sh) on container start; root-owned since it's written from inside the container) | No — gitignored, regenerated every start | Runtime only, never edit by hand |
| The self-signed fallback (used only if `external/` is empty) | `nginx/certs/selfsigned/` | No — gitignored | Runtime only |

`entrypoint.sh`'s precedence is: `external/` (real or mkcert cert) wins if
present, otherwise it generates/reuses a self-signed cert in
`selfsigned/`. Dropping mkcert's output into `external/` is what makes
nginx prefer it — no config changes needed.

## Day-to-day usage

- **Already set up on this machine** (you're reading this after the initial
  setup ran) — there's nothing to do. `docker compose up` just works, no
  warnings.
- **Fresh clone / new machine** — repeat all four steps in [TL;DR](#tldr).
  `mkcert -install` is idempotent (safe to re-run; it reuses the existing
  CA if one is already there).
- **Cert expired or you need to cover another subdomain** — re-run step 3
  with the full list of names, then restart nginx (step 4). The certificate
  issued for this repo is valid for the SAN list `knowoff.local`,
  `*.knowoff.local` until **24 Nov 2028** (check yours with
  `openssl x509 -in nginx/certs/external/fullchain.pem -noout -enddate`).
- **A teammate can't see the padlock** — they need to run steps 1–4
  themselves; your trust doesn't transfer to their machine, and `mkcert`
  intentionally never uploads or shares the CA's private key anywhere.

## Removing it

Three independent levels, from "undo this repo's cert" to "remove mkcert
entirely" — pick the one that matches what you actually want:

### 1. Stop using a trusted cert for this project only

```bash
rm nginx/certs/external/fullchain.pem nginx/certs/external/privkey.pem
docker compose -f deploy/compose/docker-compose.yaml restart nginx
```

`entrypoint.sh` falls back to its self-signed certificate automatically —
back to browser warnings, but harmless and fully reversible. mkcert itself
and its CA stay installed (other projects on this machine that rely on it
are unaffected).

### 2. Stop trusting mkcert's local CA (undoes step 1 of TL;DR)

```bash
mkcert -uninstall
```

⚠️ This removes the local CA from your OS/browser trust stores —
**every** mkcert-issued certificate on this machine, across **every**
project, goes back to being untrusted, not just Knowoff's. The CA's key
files under `$(mkcert -CAROOT)` are left on disk; re-running `mkcert
-install` later regenerates trust (either reusing the existing CA files or
minting a new CA, depending on whether you deleted them — see level 3).

### 3. Fully remove mkcert

```bash
mkcert -uninstall               # do this first, or the CA stays trusted with no way to manage it
rm -rf "$(mkcert -CAROOT)"      # deletes the CA's private key permanently
sudo apt-get purge -y mkcert    # removes the tool itself
```

After this, nothing on the machine remembers the local CA ever existed.
Repeating the [TL;DR](#tldr) from scratch creates a brand new CA (a new
private key, so a genuinely different trust anchor than before).

## Security notes

- `$(mkcert -CAROOT)/rootCA-key.pem` is effectively a skeleton key: anyone
  who has it can mint a certificate for *any* domain that your machine's
  browsers will trust silently. mkcert deliberately keeps it local-only —
  never commit it, never copy it to another machine, never share it.
- This is exactly why `nginx/certs/external/*.pem` (the cert mkcert issues
  *from* that CA) is gitignored too: even though it's scoped to
  `knowoff.local`, it's still per-machine material, not project source.
- Never reuse an mkcert-issued cert or its CA outside local development —
  it is a development convenience, not a substitute for a real CA
  (Let's Encrypt, etc.) on anything reachable by anyone else.

## Troubleshooting

- **Still seeing a warning after `mkcert -install`** — fully restart the
  browser (not just the tab/window); some browsers cache the trust store
  at startup.
- **Firefox still warns, Chrome doesn't (or vice versa)** — Firefox ships
  its own certificate store instead of using the OS one; mkcert handles
  this automatically via `libnss3-tools`, but only if that package was
  present when you ran `-install`. Install it and re-run `mkcert -install`.
- **`command not found: mkcert`** — it wasn't installed, or your shell's
  `PATH` doesn't include wherever your package manager put it. Re-run step
  1 of [TL;DR](#tldr).
- **nginx still serves the self-signed cert after issuing an mkcert one** —
  you need to `docker compose restart nginx` (step 4); `entrypoint.sh` only
  re-evaluates `external/` vs `selfsigned/` at container start.
