# API

All JSON requests use `Content-Type: application/json`, are limited to 1 MiB,
reject unknown fields, and must contain exactly one JSON value. Browser requests
must have the current page Origin. Errors are `{ "error": "message" }`.

Public endpoints are `GET /healthz`, `GET /api/version`, and the three auth
endpoints below. Other HTTP endpoints require `Authorization: Bearer <access>`.

| Method | Path | Result |
| --- | --- | --- |
| GET | `/healthz` | `200 {"status":"ok"}` or `503` while the worker is unavailable |
| GET | `/api/version` | name, version, API version, and per-process `bootId` |
| POST | `/api/auth/challenge` | challenge ID, salt, expiry, and algorithm |
| POST | `/api/auth/login` | access and refresh tokens |
| POST | `/api/auth/refresh` | rotated access and refresh tokens |
| POST | `/api/auth/logout` | `204`; refresh token body is optional and idempotent |
| GET | `/api/auth/session` | current authenticated session |
| GET | `/api/auth/sessions` | all refresh sessions |
| DELETE | `/api/auth/sessions/:id` | revoke one refresh session |
| POST | `/api/auth/logout-others` | revoke every other refresh session |
| GET/POST | `/api/heartbeat` | authoritative connection-instance list; POST accepts resize updates |
| GET | `/api/connection-instances` | list connection instances |
| GET | `/api/connection-instances/:id` | inspect one connection instance |
| POST | `/api/connection-instances` | create a local connection instance (`201`) |
| PATCH | `/api/connection-instances/:id/title` | update a connection title |
| POST | `/api/connection-instances/:id/close` | stop, archive, and retire a connection instance (`204`) |
| DELETE | `/api/connection-instances/:id` | stop, archive, and retire a connection instance (`204`) |
| GET | `/api/ssh-keys` | list detected Ed25519/RSA key metadata |
| DELETE | `/api/ssh-keys/:keyId` | delete a writable managed key pair (`204`) |
| GET | `/api/ssh-keys/:keyId/public-key` | read a detected public key |
| POST | `/api/ssh-key-generations` | start generation for an algorithm without an existing key |

`POST /api/connection-instances` accepts `connectionDefinitionId: "local"` and optional `initialCwd`, `cols`, and `rows`. `initialCwd` must be
an existing absolute directory. Dimensions are `cols: 2..1000` and
`rows: 1..1000`. Capacity returns `409`. Remote definitions are introduced by
the SSH connection manager in a later API section.

## WebSocket

Connect only to `/ws/connection-instances/:connectionInstanceId` from the current Origin. Send the access token
as the `roaminal.auth.<token>` subprotocol alongside `roaminal.v1`; the server
selects only `roaminal.v1` and never echoes the token. Attach messages are always
ordered `snapshot`, `meta`, `status`, then live `output`.

Client messages are:

```json
{"type":"input","data":"ls\n"}
{"type":"resize","cols":120,"rows":30}
{"type":"claim_terminal_control"}
{"type":"ping"}
```

The server emits `snapshot`, `meta`, `status`, `output`, `execution`, and `pong`
messages. Malformed or unknown messages close with WebSocket code `1008`;
messages over 1 MiB close with `1009`. A slow client is disconnected with `1013`.
