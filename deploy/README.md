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

Profiles:

- `core` — server, postgres, redis, minio, adminer
- `tools` — dev tooling
- `test` — test runners
- `edge` — adds cloudflared (beta-at-home only)
