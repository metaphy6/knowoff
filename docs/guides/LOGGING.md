# Logging

Knowoff logs are readable for people without changing machine-facing protocols.
Services write to stdout/stderr so Docker captures them; do not add ANSI color,
terminal control sequences, or multiline values to application logs.

## Standard

Development console logs use an emoji pair, an uppercase severity, a concise
message, then optional fields separated from the message by `|`.

```text
13:04:12  ⚠️ 🐘 [WARN]  dependency degraded
    dependency=postgres  error=connection refused
```

| Severity | Emoji | Use for |
|---|---|---|
| `DEBUG` | `🔎` | Development diagnostics |
| `INFO` | `ℹ️` | Expected lifecycle events |
| `WARN` | `⚠️` | Recoverable degradation or retry |
| `ERROR` | `🚨` | Failed operations requiring attention |

| Topic | Emoji | Use for |
|---|---|---|
| Application | `📱` | Client lifecycle |
| Network | `🌐` | WebSocket and HTTP lifecycle |
| Authentication | `🔐` | Login and session events |
| Game | `🎲` | Match lifecycle |
| Postgres | `🐘` | PostgreSQL dependency state |
| Redis | `⚡` | Redis dependency state |
| Storage | `🗄️` | Retained asset/archive storage; current stack includes MinIO |
| Other backend work | `🧩` | Uncategorized backend events |

Never log passwords, secrets, bearer tokens, cookies, session tokens, raw
request bodies, WebSocket payloads, or personally identifying data. The Go and
Flutter loggers redact field names containing `password`, `secret`, `token`,
`authorization`, or `cookie`. Callers must still avoid passing sensitive values.

## Text-transition observability contract

The [Blueprint](../../BLUEPRINT.md) and
[transition design](../design/DESIGN-text-transition.md) require the following
before text modes ship. These are implementation gates, not a claim that the
current logger or audit stream already enforces them.

- Never log Nown text/IDs during active play, future schedules, unrevealed hand
  or reserve contents, relevance annotations, random seeds or privileged replay
  scripts. A safe-looking field name does not make its value safe. The current
  match startup logs its RNG seed; remove that general-log exposure and prove
  replacement diagnostics preserve secrecy.
- Record mode/rules/protocol/pack release, content language, table size and
  lifecycle/result reason with stable match/action identifiers where access is
  appropriate. Keep metric labels bounded; account, match and action IDs belong
  in access-controlled records rather than high-cardinality metric labels.
- Measure queue wait, action latency, snapshot bytes, disconnects, pending-offer
  resolution, activation failures and settlement retries. Aggregates by mode
  must not expose private cards, text, roles or live account balance changes.
- Separate ordinary telemetry from privileged replay/audit evidence. Define
  access, retention and deletion before retaining that evidence; test redaction
  in success, malformed-input, timeout and failure paths on both server/client.
- Escape authored text in Portal/Admin error views. Do not place content in an
  exception message that later reaches general logs. Database statement logs,
  proxy query strings and exported feedback snapshots are part of this audit.

Retire media-prefetch and production-backfill metrics/config when their last
consumer is removed. Keep avatar and other retained asset diagnostics; no
image-related keyword purge should remove valid account operations.

## Backend

The Go backend uses `log/slog`. Production keeps JSON output for Docker log
ingestion and other machine consumers. Local development uses the readable
console handler configured in `configs/local.yaml`.

```go
logger.Warn("dependency degraded", "dependency", "postgres", "error", err)
```

Use structured fields rather than embedding values in messages. One record may
occupy a message line and an indented field line; every field value is escaped
so it cannot inject terminal controls or extra records.

## Flutter

Use `AppLogger` from `client/lib/core/logging/app_logger.dart`. It logs only in
Flutter debug builds and uses `debugPrint`, so release builds do not emit client
debug logs.

```dart
AppLogger.warning(LogTopic.network, 'Game connection closed');
```

Log connection state and safe identifiers only. Do not log server messages,
auth headers, URLs containing credentials, or exception text that may contain
them.

## Postgres And Redis

Postgres and Redis retain their native stdout formats. PostgreSQL adds the
stable `[POSTGRES]` prefix, while Redis is explicitly configured for stdout at
its `notice` severity. Changing their wire or native log formats with emoji
prefixes would make Docker tooling, database parsers, and vendor support
procedures less reliable. Docker Compose provides the service name and timestamp
while the standard maps those services to `🐘` and `⚡` respectively.

```bash
cd infra/compose
docker compose logs --timestamps --follow postgres
docker compose logs --timestamps --follow redis
```

Postgres does not enable query logging by default because queries can contain
secrets or personal data and full statement logging has material overhead.
Redis retains its default `notice` level, which reports lifecycle and memory
pressure without exposing commands or values. Enable temporary database
logging only for a diagnosed incident and remove it afterward.

## Verification

Run the repository gate from the root:

```bash
python3 xops/test/tests-lints.py
```

The transition additionally requires the standalone-module and real-database
proofs in the [Roadmap](../planning/ROADMAP.md); required integration skips
cannot stand in for a passed release gate.

The focused logger contracts are:

```bash
cd server && go test ./internal/logging -count=1
cd client && flutter test test/core/logging/app_logger_test.dart
```
