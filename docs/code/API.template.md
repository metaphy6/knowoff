# 🔌 API: `<api-name>`

> Copy to `docs/code/API-<name>.md`. One per externally-visible API.

## Overview

What this API exposes, to whom, in what protocol (REST, gRPC, GraphQL, ...).

## Authentication

How callers authenticate. Token format, scopes, error responses.

## Endpoints / methods

### `POST /things`

Create a Thing.

**Request**

```json
{ "name": "string", "kind": "string" }
```

**Response**

```json
{ "id": "string", "name": "string", "kind": "string", "created_at": "RFC3339" }
```

**Status codes**

| Code | When |
|---|---|
| 201 | Created |
| 400 | Validation error |
| 401 | Unauthenticated |
| 409 | Duplicate `name` |

## Errors

Common error envelope, error codes, retry semantics.

## Versioning policy

Breaking-change rules, deprecation timeline.

## Rate limits

If applicable.
