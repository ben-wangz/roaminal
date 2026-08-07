# 04 - HTTP 与 WebSocket 协议

> 状态：Approved
> 上位文档：[MVP 计划索引](../README.md)

## 通用约束

- 使用 Go `net/http` 和 Go 1.26 method-aware `ServeMux` 路由。
- 使用 `github.com/coder/websocket v1.8.15` 提供 WebSocket。
- HTTP 用于权威清单和变更；WebSocket 用于终端实时流。
- `/api/heartbeat` 是当前服务实例的权威 session 清单。
- 前端 heartbeat 固定为 1000 ms；异常重试节流固定为 5000 ms。
- WebSocket 中断后由 heartbeat 驱动重连和清单校准。
- `GET /api/version` 暴露每次 Go 进程启动变化的 boot ID；客户端发现变化后
  重新加载页面。
- JSON request 默认上限 1 MiB；超限返回 `413`。
- API error 固定返回 `{ "error": "message" }`，不得返回 Go stack trace。
- 浏览器 API 和 WebSocket 只接受同源请求；MVP 不提供跨 Origin CORS 配置。
- 带 JSON body 的请求必须使用 `Content-Type: application/json`；使用
  `DisallowUnknownFields`，第二个 JSON value、未知字段或类型不符均返回
  `400`。Timestamp 是 UTC RFC3339 string，ID 是 UUID v4。

## HTTP API

MVP API allowlist：

```text
GET    /healthz
GET    /api/version
POST   /api/auth/challenge
POST   /api/auth/login
POST   /api/auth/refresh
POST   /api/auth/logout
GET    /api/auth/session
GET    /api/auth/sessions
DELETE /api/auth/sessions/:id
POST   /api/auth/logout-others
GET    /api/heartbeat
POST   /api/heartbeat
POST   /api/sessions
PATCH  /api/sessions/:id/title
DELETE /api/sessions/:id
WS     /ws/:sessionId
```

`204` response 没有 body：

| Endpoint | Request | Success response |
| --- | --- | --- |
| `GET /healthz` | none | `200 {"status":"ok"}`；fail-fast 窗口为 `503 {"error":"terminal worker unavailable"}` |
| `GET /api/version` | none | `200 {"name":"roaminal","version":"semver","apiVersion":"roaminal.v1","bootId":"uuid"}` |
| `POST /api/auth/challenge` | `{}` | `200 AuthChallenge` |
| `POST /api/auth/login` | `{"challengeId":"uuid","response":"64 hex"}` | `200 AuthTokens` |
| `POST /api/auth/refresh` | `{"refreshToken":"rr_..."}` | `200 AuthTokens`，旧 refresh/access token 失效 |
| `POST /api/auth/logout` | `{"refreshToken":"rr_..."}`，Authorization optional | `204`，未知/已撤销 token 仍幂等成功 |
| `GET /api/auth/session` | none | `200 CurrentAuthSession` |
| `GET /api/auth/sessions` | none | `200 {"sessions":[AuthSessionSummary...]}` |
| `DELETE /api/auth/sessions/:id` | none | `204`；未知 ID 为 `404` |
| `POST /api/auth/logout-others` | `{}` | `204` |
| `GET /api/heartbeat` | none | `200 HeartbeatResponse` |
| `POST /api/heartbeat` | `HeartbeatUpdate` | `200 HeartbeatResponse` |
| `POST /api/sessions` | `CreateSessionRequest` | `201 SessionSummary`；capacity 满为 `409` |
| `DELETE /api/sessions/:id` | none | `204`；未知 ID 为 `404` |

Named JSON objects：

```text
AuthChallenge = {
  challengeId: string,
  salt: string,
  expiresAt: string,
  algorithm: "roaminal-hmac-sha256-login-v1"
}

AuthTokens = {
  accessToken: string,
  accessTokenExpiresAt: string,
  refreshToken: string,
  refreshTokenExpiresAt: string
}

CurrentAuthSession = {
  authenticated: true,
  sessionId: string,
  accessTokenExpiresAt: string,
  refreshTokenExpiresAt: string
}

AuthSessionSummary = {
  id: string,
  createdAt: string,
  lastSeenAt: string,
  refreshExpiresAt: string,
  userAgent: string,
  current: boolean
}

CreateSessionRequest = {
  cwd?: string,
  cols?: integer 2..1000,
  rows?: integer 1..1000
}

ExitStatus = {
  exitCode: integer | null,
  signal: integer | null
}

SessionSummary = {
  id: string,
  createdAt: string,
  updatedAt: string,
  shell: "/bin/bash",
  initialCwd: string,
  title: string,
  titleMode: "automatic" | "custom",
  cwd: string,
  cols: integer,
  rows: integer,
  closed: boolean,
  attention: boolean,
  exitStatus: ExitStatus | null
}

HeartbeatUpdate = {
  updates: {
    sessions: [{ id: string, resize?: { cols: integer, rows: integer } }]
  }
}

SystemStats = {
  hostname: string,
  kernel: string,
  ip: string,
  cpu: {
    model: string,
    count: integer,
    speedGHzMin: number,
    speedGHzMax: number,
    usagePercent: number
  },
  memory: { totalBytes: integer, usedBytes: integer, freeBytes: integer },
  hostUptimeSeconds: number,
  processUptimeSeconds: number
}

HeartbeatResponse = {
  sessions: SessionSummary[],
  system: SystemStats,
  runtime: { bootId: string, persistenceDegraded: boolean, scrollbackLines: integer }
}
```

`CreateSessionRequest.cwd` 必须是容器内存在、可进入的绝对目录；省略时按
[03-terminal.md](./03-terminal.md) 的继承规则处理，且最多 4096 UTF-8 bytes。
Heartbeat POST 的 `updates.sessions` 最多为当前 `maxSessions`；重复 session ID
或非法 resize 使整个 request 返回 `400` 且不做部分应用，已在并发流程中删除
的未知 session ID 则忽略。Heartbeat response 不包含 execution output；
execution 只通过 WebSocket 实时传递并在服务端持久化。

HTTP status 固定使用：validation `400`、auth missing/expired `401`、service
locked/origin denied `403`、not found `404`、capacity conflict `409`、body too
large `413`、client capacity `429`、unexpected internal error `500`、terminal
worker unavailable `503`。每个 `5xx` response 生成 UUID v4 correlation ID，
同一值写入结构化日志并通过 `X-Roaminal-Request-ID` response header 返回；
response body 和日志不得包含路径、token、PTY data 或 stack trace。

不保留 `POST /api/sessions/:id/state`，也不在 session schema 中写入
`editorState`、`workspaceState` 或 `managed`。

## Terminal WebSocket

消息方向：

```text
server -> client: snapshot, meta, status, output, execution, pong
client -> server: input, resize, claim_terminal_control, ping
```

认证 subprotocol：

```text
roaminal.v1
roaminal.auth.<access-token>
```

服务端只在响应中选择 `roaminal.v1`，不得回显携带 token 的 subprotocol。

Application message 是 UTF-8 JSON text：

```text
server -> client
{ type: "snapshot", data: string }
{ type: "meta", title: string, cwd: string, cols: integer, rows: integer }
{ type: "status", status: "ready" }
{ type: "status", status: "terminated", code: integer, signal: integer | null }
{ type: "output", data: string }
{ type: "execution", phase: "started", executionId: string,
  command: string, startedAt: string }
{ type: "execution", phase: "completed", executionId: string,
  entry: ExecutionRecord }
{ type: "execution", phase: "idle", executionId: string }
{ type: "pong" }

client -> server
{ type: "input", data: string }
{ type: "resize", cols: integer 2..1000, rows: integer 1..1000 }
{ type: "claim_terminal_control" }
{ type: "ping" }
```

`ExecutionRecord` 与 [03-terminal.md](./03-terminal.md) 的持久化 schema 相同。
Client message 上限 1 MiB；超限以 code `1009` 关闭。Malformed JSON、unknown
type/field 或 schema violation 以 code `1008`、reason `invalid_message` 关闭。
达到 client capacity 时在 upgrade 前返回 HTTP `429`。

Server attach 顺序严格为 `snapshot -> meta -> status`，再 flush attach 期间按序
排队的 `output`；正常 live output 不得在 snapshot 前出现。Server 不在 attach
时重发历史 execution events。
