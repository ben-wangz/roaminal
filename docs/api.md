# API

JSON API requests use `Content-Type: application/json`, are limited to 1 MiB, and
reject unknown fields. FileSystem uploads use multipart form data and are
limited to 10 GiB of file content plus multipart overhead. Browser requests
must use the current page Origin. The public endpoints are `/healthz`,
`/api/v2/version`, and the authentication challenge/login/refresh/logout
routes; other API endpoints require `Authorization: Bearer <access-token>`.
Errors use
`{"error":"message","code":"stable_code","retryable":false}` and may include `field`, `requestId`, and bounded `details`.

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/healthz` | Backend and terminal-worker health (`503` while unavailable) |
| GET | `/api/v2/version` | Product, API version, process `bootId`, and diagnostic capability |
| POST | `/api/v2/auth/challenge`, `/api/v2/auth/login`, `/api/v2/auth/refresh`, `/api/v2/auth/logout` | Login and session lifecycle |
| GET | `/api/v2/auth/session`, `/api/v2/auth/sessions` | Current or all login sessions |
| DELETE | `/api/v2/auth/sessions/:authSessionId` | Revoke one login session |
| POST | `/api/v2/auth/logout-others` | Revoke other login sessions |
| GET/POST | `/api/v2/heartbeat` | Read instances or submit resize updates |
| GET | `/api/v2/messages` | Read the authenticated operator's Agent message history |
| PUT | `/api/v2/messages/read-state` | Advance the account-wide Agent message read cursor |
| GET | `/api/v2/notifications/config` | Read authenticated Web Push availability and the public VAPID key |
| GET/PUT | `/api/v2/notifications/preferences` | Read or update per-connection browser notification preferences |
| PUT | `/api/v2/notifications/subscription` | Register or replace the current browser's Web Push subscription |
| DELETE | `/api/v2/notifications/subscription/:subscriptionId` | Remove one current login-session Web Push subscription |
| DELETE | `/api/v2/notifications/subscriptions` | Remove all current login-session Web Push subscriptions |
| GET/POST | `/api/v2/connection-instances` | List or create local/remote instances |
| PUT | `/api/v2/connection-instances/order` | Save the current login session's sidebar order |
| GET | `/api/v2/connection-instances/:connectionInstanceId` | Inspect an active instance |
| GET | `/api/v2/connection-instances/:connectionInstanceId/agent` | Read Agent support, binding, and status |
| POST | `/api/v2/connection-instances/:connectionInstanceId/agent/initializations` | Start or join Agent component initialization |
| GET | `/api/v2/agent/initializations/:initializationId` | Poll Agent initialization |
| PATCH | `/api/v2/connection-instances/:connectionInstanceId/title` | Set or clear a title |
| DELETE | `/api/v2/connection-instances/:connectionInstanceId` | Retire an active instance |
| GET | `/api/v2/connection-instances/:connectionInstanceId/remote-monitor` | Monitor a live SSH transport |
| GET | `/api/v2/connection-instances/:connectionInstanceId/filesystem/root` | Resolve the FileSystem root |
| GET | `/api/v2/connection-instances/:connectionInstanceId/filesystem/entries` | List one directory |
| GET | `/api/v2/connection-instances/:connectionInstanceId/filesystem/stat` | Inspect one entry |
| GET | `/api/v2/connection-instances/:connectionInstanceId/filesystem/content` | Read or download file content |
| POST | `/api/v2/connection-instances/:connectionInstanceId/filesystem/uploads` | Queue a FileSystem upload |
| GET/DELETE | `/api/v2/connection-instances/:connectionInstanceId/filesystem/uploads/:uploadId` | Read or cancel an upload |
| POST | `/api/v2/connection-launches` | Start an owned pending tmux launch |
| DELETE | `/api/v2/connection-launches/:launchId` | Abort an owned pending launch |
| GET/POST | `/api/v2/connection-instance-groups` | Read or create the current login session's groups |
| PUT | `/api/v2/connection-instance-groups/layout` | Replace the current login session's grouped layout |
| PATCH/DELETE | `/api/v2/connection-instance-groups/:groupId` | Rename or delete a group |
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
SSH instance projections may include an `endpoint` object containing only the
effective `user`, `host`, and `port` used for safe Terminal footer display; it
never contains credentials or raw SSH directives. Local instances omit it.
Sidebar layout is scoped to the current login session. New instances enter
`ungrouped`; named groups hold at most 10 instances, while `ungrouped` has no
group limit. A non-empty named group cannot be deleted. The flat order endpoint
is available only before named groups are used; grouped layouts use
`connection-instance-groups/layout`. Both forms use a monotonic `revision` and
reject stale writes with a conflict.

`PUT /api/v2/connection-instances/order` accepts `connectionInstanceIds`;
retired IDs are ignored and current omitted instances are appended.
Definition writes require the current `ETag` in `If-Match`; stale or missing
tags return `412` or `428`. Capacity and transport state errors return `409`
with stable codes such as `capacity`, `transport`, or `no_remote_transport`.

Local creation accepts `connectionDefinitionId: "local"`, optional absolute
`initialCwd`, and `cols: 2..1000` / `rows: 1..1000`. Remote creation uses an SSH
definition and may set `reuseFromConnectionInstanceId` to reuse a live
ControlMaster. Remote-monitor responses expose status, freshness, RTT, and
scoped CPU, memory, uptime, load, and disk values.

Heartbeat responses include a top-level `messageState` projection with the
current message revision, latest sequence, and unread count. Each connection
instance also contains the latest cached Agent projection and synchronization
metadata. Heartbeat never performs remote Agent reads. Message history is
newest-first and accepts an opaque `before` cursor; it is retained for at most
500 records or seven days. Read state is advanced monotonically with
`{"readThroughSequence":123}`. Message responses contain presentation-safe
metadata only; provider session IDs, tmux socket fingerprints, credentials,
endpoint keys, and remote state history are not returned.

Web Push sends only newly persisted Agent state transitions allowed by the
matching connection preference: `running -> relax` or `running -> error`. The
message center shows all standard state transitions. The server keeps
subscription endpoints and encryption keys in private state and never returns
them. The browser's global notification switch controls local delivery and
removes all subscriptions for the current login session when turned
off. If VAPID configuration is absent, the config endpoint reports Web Push
disabled.

## FileSystem

FileSystem operations are available only for live SSH connection instances.
The root is resolved from the active tmux pane when tmux is enabled. If that
probe fails, the connection definition's `filesystem.pwd` is used; its default
is `$HOME`. The tmux probe retries once per request and briefly caches a failed
probe, so a transient failure does not permanently disable FileSystem access.

The server does not persist a directory tree. `root` returns a revision,
`entries` lists one relative directory, and `stat`/`content` use that revision
to detect a root change. Directory pagination snapshots are in-memory and
short-lived. Paths are constrained below the resolved root; symlink content is
not read through the API. `stat` uses the filename extension first and uses the
SSH target's `file(1)` MIME result only when the extension is not recognized.

Uploads are asynchronous multipart requests with a file manifest and file parts.
Conflict policies are `refuse` (default), `overwrite`, and `update-if-newer`.
The server uses `rsync` when both sides support it and falls back to `scp`.
Poll the upload resource until it reaches a terminal status; `DELETE` cancels
an active upload.

## Agent

Agent initialization is supported for live SSH tmux connection instances with
remote `tmux` and Codex. Endpoint identity is normalized from SSH user, host,
and port. Each tmux target is further identified by its session name and tmux
runtime identity. Repeated or concurrent initialization for the same endpoint
joins the existing operation; the latest Agent projection and synchronization
metadata are persisted by the backend.

The Agent component bundle supports Linux and macOS on `amd64` and `arm64`. It
installs the hook binary at `$HOME/.roaminal/bin/roaminal-agent-hook`, the
Codex hook configuration under `$HOME/.codex/hooks.json`, and private local
component metadata under `$HOME/.roaminal/agent.json`. The hook writes only
the current tmux runtime's local state file under
`$HOME/.roaminal/state/agents/codex/`; it does not access the network. The
backend synchronizer reads state only through live connection instances and
stores one latest snapshot per tmux target. Standard Agent states are
`running`, `relax`, and provider-capability-gated `error`; synchronization
failures remain separate from Agent state and expose distinct `missing`,
`tmux_missing`, `stale`, `invalid`, and `unavailable` synchronization statuses.
Actual state changes create one
idempotent `agent_state_transition` message; ordinary hook events do not.

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
`pong`. The `meta` message includes the current `cwd`, `cols`, `rows`, and
effective `terminalType`; the backend uses `xterm-256color` when no explicit
terminal type is configured. Invalid messages close with `1008`, oversized
messages with `1009`, and slow clients with `1013`.
