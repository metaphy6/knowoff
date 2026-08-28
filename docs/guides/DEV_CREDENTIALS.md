# 🔑 Local dev credentials

All values below are **local-only defaults** from `infra/compose/config/**/environment.env` —
they never apply to `staging`/`prod` (those get secrets from a deployment
secret store per [`AGENTS.md`](../../AGENTS.md) §4). Safe to keep visible in
this doc because they only ever unlock a throwaway local Docker stack.

## Postgres / Adminer

Open `https://adminer.knowoff.local` (or `http://localhost:8081` without the
proxy) after `make localhostfile.add` + `docker compose --profile core up`.

| Field | Value |
|---|---|
| System | PostgreSQL |
| Server | `postgres` |
| Username | `knowoff` |
| Password | `knowoff` |
| Database | `knowoff` |

## MinIO console

Open `https://minio.knowoff.local` (or `http://localhost:9001`).

| Field | Value |
|---|---|
| Username | `minioadmin` |
| Password | `minioadmin` |

## Admin Console

The Admin Console (`https://admin.knowoff.local`, or `http://localhost:9090`)
has no default login — seed one with:

```bash
cd infra/compose
docker compose --profile tools run --rm --build seed-admin
```

Prints an email, password, TOTP secret, and `otpauth://` URL to stdout (scan
it with an authenticator app, or compute a code with `oathtool --totp -b
"<secret>"`). Re-running is a no-op against an already-seeded email. Override
the identity with:

```bash
cd infra/compose
KNOWOFF_SEED_ADMIN_EMAIL=you@knowoff.local KNOWOFF_SEED_ADMIN_PASSWORD=somepassword docker compose --profile tools run --rm --build seed-admin
```

Refuses to run when `app.env=prod`. See `seedAdmin` in
[`server/cmd/knowoffd/main.go`](../../server/cmd/knowoffd/main.go).
