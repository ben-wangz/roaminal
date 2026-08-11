# API

JSON requests use `Content-Type: application/json`, are limited to 1 MiB, and
reject unknown fields. Browser requests must use the current page Origin. Errors
are `{ "error": "message" }`. Unless listed as public, HTTP endpoints require
`Authorization: Bearer <access-token>`.

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/healthz` | `200 {"status":"ok"}`; `503` when the worker is unavailable |
| GET | `/api/version` | product, API version, and process `bootId` |
| POST | `/api/auth/challenge`, `/api/auth/login`, `/api/auth/refresh`, `/api/auth/logout` | authentication |
| GET | `/api/auth/session`, `/api/auth/sessions` | current or all refresh sessions |
| DELETE | `/api/auth/sessions/:id` | revoke one refresh session |
| POST | `/api/auth/logout-others` | revoke all other refresh sessions |
| GET/POST | `/api/heartbeat` | connection instances and resize updates |
| GET/POST | `/api/connection-instances` | list or create local instances |
| GET | `/api/connection-instances/:id` | inspect an instance |
| PATCH | `/api/connection-instances/:id/title` | change an instance title |
| POST/DELETE | `/api/connection-instances/:id/close`, `/api/connection-instances/:id` | retire an instance |
| GET | `/api/connection-instances/:id/remote-monitor` | cached monitor data for a live SSH transport |
| GET | `/api/ssh-keys`, `/api/ssh-keys/:keyId/public-key` | list keys or read a public key |
| DELETE | `/api/ssh-keys/:keyId` | delete a writable managed key pair |
| POST | `/api/ssh-key-generations` | generate an absent Ed25519/RSA key |

Local instance creation accepts `connectionDefinitionId: "local"`, optional
absolute `initialCwd`, and `cols: 2..1000` / `rows: 1..1000`. Capacity returns
`409`. Remote-monitor responses expose status, sample age, RTT, and scoped CPU,
memory, uptime, load, and disk values. They use an existing SSH ControlMaster,
never create a login session, and return `409 no_remote_transport` for local,
exited, or draining instances.

## WebSocket

Connect to `/ws/connection-instances/:connectionInstanceId` from the current
Origin with `roaminal.v1` and `roaminal.auth.<access-token>` subprotocols. The
server selects only `roaminal.v1`; attach order is `snapshot`, `meta`, `status`,
then live `output`.

```json
{"type":"input","data":"ls\n"}
{"type":"resize","cols":120,"rows":30}
{"type":"claim_terminal_control"}
{"type":"ping"}
```

Server messages are `snapshot`, `meta`, `status`, `output`, `execution`, and
`pong`. Invalid messages close with `1008`, oversized messages with `1009`, and
slow clients with `1013`.
