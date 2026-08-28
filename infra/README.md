# Deployment artifacts

```text
infra/
├── compose/    # Docker Compose local stack
├── k8s/        # Future Kubernetes manifests (scaffolded, not required to run)
└── terraform/  # Future Terraform modules (provider-agnostic)
```

The local reverse proxy lives at [`nginx/`](../nginx/README.md) (repo root,
alongside `client/` and `server/`), not under `infra/` — it's built and
versioned like the other service images, not a deployment artifact.

## Local stack

```bash
cd infra/compose
docker compose --profile core up --build
```

Opening this workspace in VS Code also runs this automatically (the
"Compose: Up (core)" task has `runOn: folderOpen` — see
[`.vscode/tasks.json`](../.vscode/tasks.json); port 443 is configured to
auto-open in VS Code's browser preview once nginx is reachable (see
[`.vscode/settings.json`](../.vscode/settings.json)).

Then resolve the local domains once (see [`nginx/README.md`](../nginx/README.md)):

```bash
make localhostfile.add   # adds *.knowoff.local -> 127.0.0.1 to your hosts file
```

Open `https://app.knowoff.local` for the Flutter web client. The browser
will warn about the self-signed certificate on first visit — see
[`nginx/README.md`](../nginx/README.md#tls-certificates).

Seed a dev-only Admin Console login (`https://admin.knowoff.local`):

```bash
cd infra/compose
docker compose --profile tools run --rm --build seed-admin
```

Local service configuration lives in `compose/config/<service>/environment.env`.
The files contain the complete development environment, including the visible
dev secrets used by the stack — see
[`docs/guides/DEV_CREDENTIALS.md`](../../docs/guides/DEV_CREDENTIALS.md) for
the full list. Edit the Cloudflared token before using the `edge` profile;
production secrets must come from a deployment secret store.

Profiles:

- `core` — server, postgres, redis, minio, adminer, client-web, nginx
- `tools` — dev tooling
- `test` — test runners
- `edge` — adds cloudflared (beta-at-home only)

## Volume snapshots

Use the Compose snapshot script directly. Create a snapshot while the `core`
profile is running:

```bash
./infra/compose/snapshot.sh create /path/to/snapshot
```

Restore from a complete snapshot with:

```bash
./infra/compose/snapshot.sh restore /path/to/snapshot
```

Restoring replaces the Compose data volumes, then starts the full `core` stack.
The snapshot directory must contain `postgres.dump`, `redis.rdb`, and `minio/`.

