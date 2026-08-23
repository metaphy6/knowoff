# Deployment artifacts

```text
deploy/
├── compose/    # Docker Compose local stack
├── k8s/        # Future Kubernetes manifests (scaffolded, not required to run)
└── terraform/  # Future Terraform modules (provider-agnostic)
```

## Local stack

```bash
cd deploy/compose
docker compose --profile core up --build
```

Local service configuration lives in `compose/config/<service>/environment.env`.
The files contain the complete development environment, including the visible
dev secrets used by the stack. Edit the Cloudflared token before using the
`edge` profile; production secrets must come from a deployment secret store.

Profiles:

- `core` — server, postgres, redis, minio, adminer
- `tools` — dev tooling
- `test` — test runners
- `edge` — adds cloudflared (beta-at-home only)
