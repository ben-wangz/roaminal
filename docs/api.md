# API

Requests use `Content-Type: application/json`, are limited to 1 MiB (or 256 KiB
for `/api/v2/client-diagnostics`), and reject unknown fields. Browser requests must use the current page Origin. Errors use
`{"error":"message","code":"stable_code","retryable":false}` and may include `field`, `requestId`, and bounded `details`.
Unless listed as public, endpoints require `Authorization: Bearer
<access-token>`.

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/healthz` | Worker health (`503` while unavailable) |
| GET | `/api/v2/version` | Product, API version, process `bootId`, and diagnostic capability |
| POST | `/api/v2/auth/challenge`, `/api/v2/auth/login`, `/api/v2/auth/refresh`, `/api/v2/auth/logout` | Authentication |
| GET | `/api/v2/auth/session`, `/api/v2/auth/sessions` | Current or all login sessions |
| DELETE | `/api/v2/auth/sessions/:authSessionId` | Revoke one login session |
| POST | `/api/v2/auth/logout-others` | Revoke other login sessions |
| GET/POST | `/api/v2/heartbeat` | Read instances or submit resize updates |
| GET/POST | `/api/v2/connection-instances` | List or create local/remote instances |
| PUT | `/api/v2/connection-instances/order` | Save the current login session's sidebar order |
| GET | `/api/v2/connection-instances/:connectionInstanceId` | Inspect an active instance |
| PATCH | `/api/v2/connection-instances/:connectionInstanceId/title` | Set or clear a title |
| DELETE | `/api/v2/connection-instances/:connectionInstanceId` | Retire an active instance |
| GET | `/api/v2/connection-instances/:connectionInstanceId/remote-monitor` | Monitor a live SSH transport |
| POST | `/api/v2/connection-launches` | Start an owned pending tmux launch |
| DELETE | `/api/v2/connection-launches/:launchId` | Abort an owned pending launch |
| GET/POST | `/api/v2/connection-definitions` | List or create structured SSH definitions |
| PUT/DELETE | `/api/v2/connection-definitions/:connectionDefinitionId` | Update or delete a definition |
| POST | `/api/v2/connection-definitions/:connectionDefinitionId/duplicate` | Duplicate a definition |
| GET | `/api/v2/ssh-keys` | List detected Ed25519/RSA keys |
| GET | `/api/v2/ssh-keys/:keyId/public-key` | Read a public key |
| DELETE | `/api/v2/ssh-keys/:keyId` | Delete a writable managed key pair |
| POST | `/api/v2/ssh-key-generations` | Generate an absent Ed25519/RSA key |
| POST | `/api/v2/client-diagnostics` | Submit an authenticated bounded browser error batch |

Connection-instance responses use `connectionInstanceId` as their only
instance identifier. Active lists do not include retired or exited instances.
Sidebar order is scoped to the current login session. `PUT /api/v2/connection-instances/order`
accepts `connectionInstanceIds`; retired IDs
are ignored and current omitted instances are appended.
Definition writes require the current `ETag` in `If-Match`; stale or missing
tags return `412` or `428`. Capacity and transport state errors return `409`
with stable codes such as `capacity`, `transport`, or `no_remote_transport`.

Local creation accepts `connectionDefinitionId: "local"`, optional absolute
`initialCwd`, and `cols: 2..1000` / `rows: 1..1000`. Remote creation uses an SSH
definition and may set `reuseFromConnectionInstanceId` to reuse a live
ControlMaster. Remote-monitor responses expose status, freshness, RTT, and
scoped CPU, memory, uptime, load, and disk values.

`POST /api/v2/client-diagnostics` accepts at most 20 redacted error events in a
256 KiB JSON body and returns `204` when accepted. It requires the current
access token and same-origin request. The endpoint returns `400` for invalid
schemas, `413` for oversized bodies, and `429` when the per-login-session
budget is exhausted. The server records only bounded event metadata; it does
not accept terminal content, SSH material, or request bodies.

## WebSocket

Connect to `/ws/v2/connection-instances/:connectionInstanceId` or
`/ws/v2/connection-launches/:launchId` from the current Origin with the
`roaminal.v2` and `roaminal.auth.<access-token>` subprotocols. Add
`?role=observer` for a read-only preview; the default `interactive` role may
claim terminal control and send input/resize commands. Pending launches
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
