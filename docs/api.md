# API

Requests use `Content-Type: application/json`, are limited to 1 MiB (or 256 KiB
for `/api/client-diagnostics`), and reject unknown fields. Browser requests must use the current page Origin. Errors use
`{"error":"message","code":"stable_code"}` and may include `field`.
Unless listed as public, endpoints require `Authorization: Bearer
<access-token>`.

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/healthz` | Worker health (`503` while unavailable) |
| GET | `/api/version` | Product, API version, process `bootId`, and diagnostic capability |
| POST | `/api/auth/challenge`, `/api/auth/login`, `/api/auth/refresh`, `/api/auth/logout` | Authentication |
| GET | `/api/auth/session`, `/api/auth/sessions` | Current or all login sessions |
| DELETE | `/api/auth/sessions/:authSessionId` | Revoke one login session |
| POST | `/api/auth/logout-others` | Revoke other login sessions |
| GET/POST | `/api/heartbeat` | Read instances or submit resize updates |
| GET/POST | `/api/connection-instances` | List or create local/remote instances |
| GET | `/api/connection-instances/:connectionInstanceId` | Inspect an active instance |
| PATCH | `/api/connection-instances/:connectionInstanceId/title` | Set or clear a title |
| DELETE | `/api/connection-instances/:connectionInstanceId` | Retire an active instance |
| GET | `/api/connection-instances/:connectionInstanceId/remote-monitor` | Monitor a live SSH transport |
| POST | `/api/connection-launches` | Start an owned pending tmux launch |
| DELETE | `/api/connection-launches/:launchId` | Abort an owned pending launch |
| GET/POST | `/api/connection-definitions` | List or create structured SSH definitions |
| PUT/DELETE | `/api/connection-definitions/:connectionDefinitionId` | Update or delete a definition |
| POST | `/api/connection-definitions/:connectionDefinitionId/duplicate` | Duplicate a definition |
| GET | `/api/ssh-keys` | List detected Ed25519/RSA keys |
| GET | `/api/ssh-keys/:keyId/public-key` | Read a public key |
| DELETE | `/api/ssh-keys/:keyId` | Delete a writable managed key pair |
| POST | `/api/ssh-key-generations` | Generate an absent Ed25519/RSA key |
| POST | `/api/client-diagnostics` | Submit an authenticated bounded browser error batch |

Connection-instance responses use `connectionInstanceId` as their only
instance identifier. Active lists do not include retired or exited instances.
Definition writes require the current `ETag` in `If-Match`; stale or missing
tags return `412` or `428`. Capacity and transport state errors return `409`
with stable codes such as `capacity`, `transport`, or `no_remote_transport`.

Local creation accepts `connectionDefinitionId: "local"`, optional absolute
`initialCwd`, and `cols: 2..1000` / `rows: 1..1000`. Remote creation uses an SSH
definition and may set `reuseFromConnectionInstanceId` to reuse a live
ControlMaster. Remote-monitor responses expose status, freshness, RTT, and
scoped CPU, memory, uptime, load, and disk values.

`POST /api/client-diagnostics` accepts at most 20 redacted error events in a
256 KiB JSON body and returns `204` when accepted. It requires the current
access token and same-origin request. The endpoint returns `400` for invalid
schemas, `413` for oversized bodies, and `429` when the per-login-session
budget is exhausted. The server records only bounded event metadata; it does
not accept terminal content, SSH material, or request bodies.

## WebSocket

Connect to `/ws/connection-instances/:connectionInstanceId` or
`/ws/connection-launches/:launchId` from the current Origin with the
`roaminal.v1` and `roaminal.auth.<access-token>` subprotocols. Pending launches
are owned by the login session that created them. Attach order is `snapshot`,
`meta`, `status`, then live `output`.

```json
{"type":"input","data":"ls\n"}
{"type":"resize","cols":120,"rows":30}
{"type":"claim_terminal_control"}
{"type":"ping"}
```

Server messages are `snapshot`, `meta`, `status`, `output`, `execution`, and
`pong`. Invalid messages close with `1008`, oversized messages with `1009`,
and slow clients with `1013`.
