# 🔒 `nginx/` — local reverse proxy

Containerized nginx in front of the Flutter web client and the Go game
server, publishing everything under friendly `*.knowoff.local` names with
TLS and a strong security-header baseline. This is a **local-development**
convenience layer. Public ingress remains an operator-selected deployment step;
Cloudflare Tunnel is currently commented out in Compose (see BLUEPRINT.md 📦 §2).

## Domains

| Domain | Proxies to | Notes |
|---|---|---|
| `app.knowoff.local` | `client-web:8000` | Requires explicitly enabling the currently commented-out client service. |
| `api.knowoff.local` | `server:8080` | The public game server — REST + `/ws/v2` WebSocket. |
| `admin.knowoff.local` | `server:9090` | The Admin Console / Contributor Portal admin routes / text content operations. Never expose this vhost outside your dev machine. |
| `adminer.knowoff.local` | `adminer:8080` | Postgres browser UI. |

Resolve these names to `127.0.0.1` with:

```bash
make localhostfile.add      # writes the *.knowoff.local block to your hosts file (admin/sudo required)
make localhostfile.remove   # removes it again
```

See [`xops/makefile/hosts_ops.py`](../xops/makefile/hosts_ops.py) for exactly what it writes.

## TLS certificates

`entrypoint.sh` runs before nginx starts and decides which certificate to serve:

1. If `certs/external/fullchain.pem` and `certs/external/privkey.pem` exist
   (bind-mounted into the container at `/etc/nginx/certs/external/`), they
   are used as-is — this is where you mount a real certificate later
   (e.g. from `mkcert`, or a CA-issued wildcard cert for a real domain).
2. Otherwise a self-signed wildcard certificate for `*.knowoff.local` is
   generated once into `certs/selfsigned/` and reused on every subsequent
   start (the `certs/` directory is a bind mount, so it survives
   `docker compose down`/`up`).

### Getting a browser-trusted cert with mkcert (recommended)

The self-signed fallback works everywhere but shows a browser warning on
first visit. [`MKCERT.md`](MKCERT.md) covers the zero-warning alternative
in full — what it is, how it actually works, day-to-day usage, and how to
remove it again (three levels, from "undo this repo's cert" to "fully
uninstall mkcert").

Either way the active pair is copied to `certs/active/`, the fixed path
every vhost config points its `ssl_certificate`/`ssl_certificate_key` at.

Because the default certificate is self-signed, your browser will show a
warning on first visit — click through / add the exception per-domain (or
trust the generated CA in your OS/browser store if you want to stop seeing
it). This is also why the bundled `Strict-Transport-Security` header uses a
short 5-minute `max-age` instead of a production value: pairing a long-lived
HSTS pin with an untrusted cert is a known way to lock yourself out of a
`.local` domain in Chrome-family browsers with no bypass. Once you mount a
real certificate, raise `max-age` in `conf.d/00-security-headers.conf` and
add `preload`.

## Security headers

`conf.d/00-security-headers.conf` is a snippet `include`-d from *inside*
every vhost's `server {}` block (not auto-loaded at the `http` level — see
the comment in `nginx.conf` for why: nginx's `add_header` does not merge
across nesting levels). It sets HSTS, `X-Content-Type-Options`,
`X-Frame-Options`, `Referrer-Policy`, `Permissions-Policy`, and the
cross-origin isolation headers (`Cross-Origin-Opener-Policy`,
`Cross-Origin-Resource-Policy: same-site` so the `*.knowoff.local`
subdomains can still talk to each other).

`Content-Security-Policy` is set per-vhost instead, because the right
policy differs by surface:

- `api.knowoff.local` — `default-src 'none'`. JSON/WebSocket plus the
  resource-free OAuth confirmation page.
- `admin.knowoff.local` — `default-src 'self'`, server-rendered HTML.
- `app.knowoff.local` — `'self'` plus `'wasm-unsafe-eval'` and `worker-src blob:`,
  which the Flutter web engine's CanvasKit/skwasm renderer needs to compile
  and run its WASM payload in a worker. Loosen further only if your
  browser's devtools console logs a CSP violation for your specific build.
- `adminer.knowoff.local` — no CSP. This third-party bundled UI needs inline
  script/style; it is a dev convenience, not a Knowoff product surface.

## Performance

`nginx.conf` keeps upstream keepalive pools (`upstream { keepalive ... }`)
per backend, gzip for text/JSON, and generous (not restrictive) buffer and
rate-limit settings — tuned to not get in your way during local
development while still exercising the same config shape you'd harden
further for a real deployment. Nothing here blocks or throttles normal
dev traffic; `limit_req`/`limit_conn` zones are sized for many concurrent
tabs/devices, not to reject them.

## Bumping the image label version

```bash
make label.version SERVICE=nginx VERSION=0.2.0
```

See [`xops/makefile/labels_ops.py`](../xops/makefile/labels_ops.py).

## Verified infrastructure boundary

The active proxy has no object-store upstream, console vhost, or playable-image
CSP origin. Avatar/blob images, web icons, Flutter assets and font origins remain.
Removing the old vhost does not remove any previously created archive volume.

`python3 -m unittest discover -s xops/test -p test_infra.py -v` (repository root)
builds this Dockerfile and exercises actual TLS/config syntax plus callback
success and stopped-upstream failure. Synthetic code/state/Referer sentinels must
be absent from access/error logs; ordinary path/status errors remain observable.
The test uses private temporary certificates and an isolated synthetic upstream,
not real provider credentials or a deployed game server. Callback access/error
logs are disabled in its exact location because nginx upstream error messages can
otherwise repeat sensitive query values even with a sanitized access format.
