# 02 - 配置与认证

> 状态：Approved
> 上位文档：[MVP 计划索引](../README.md)

## 服务启动与配置

- Go module，toolchain 固定为 Go `1.26.5`。
- 可执行文件和 CLI 命令均为 `roaminal`。
- 默认 bind address 为 `127.0.0.1`；容器和 Kubernetes 显式配置为
  `0.0.0.0`。
- 默认端口固定为 `9846`。
- 端口被占用时立即输出明确错误并非零退出，不自动寻找其他端口。
- 配置优先级固定为：
  1. 内置默认值
  2. `~/.roaminal/config.json`
  3. `./config.json`
  4. CLI 参数
  5. 环境变量
- 配置解析只接受本文定义的 canonical 字段；不兼容 Tabminal 旧字段。

| JSON | CLI | Environment | Default |
| --- | --- | --- | --- |
| `host` | `--host` / `-h` | `ROAMINAL_HOST` | `127.0.0.1` |
| `port` | `--port` / `-p` | `ROAMINAL_PORT` | `9846` |
| `password` | `--password` / `-a` | `ROAMINAL_PASSWORD` | 随机生成 |
| `websocketPingInterval` | `--websocket-ping` | `ROAMINAL_WEBSOCKET_PING_INTERVAL` | `10s` |
| `scrollbackLines` | `--scrollback-lines` | `ROAMINAL_SCROLLBACK_LINES` | `1000` |
| `maxSessions` | `--max-sessions` | `ROAMINAL_MAX_SESSIONS` | `32` |
| `maxClientsPerSession` | `--max-clients-per-session` | `ROAMINAL_MAX_CLIENTS_PER_SESSION` | `8` |
| `debug` | `--debug` / `-d` | `ROAMINAL_DEBUG` | `false` |
| `acceptTerms` | `--accept-terms` / `-y` | `ROAMINAL_ACCEPT_TERMS` | `false` |
| `initialCwd` | `--cwd` | `ROAMINAL_CWD` | `/workspace` |
| `authAccessTTL` | `--auth-access-ttl` | `ROAMINAL_AUTH_ACCESS_TTL` | `15m` |
| `authRefreshTTL` | `--auth-refresh-ttl` | `ROAMINAL_AUTH_REFRESH_TTL` | `2160h` |
| `authMaxAttempts` | `--auth-max-attempts` | `ROAMINAL_AUTH_MAX_ATTEMPTS` | `30` |

Duration 使用 Go duration 字符串；scrollback lines、port、attempts 和 capacity
使用十进制整数。Access TTL、Refresh TTL、失败锁定阈值和 accept terms 修改后
重启服务生效，MVP 不做配置文件热加载。新 TTL 只影响修改后签发的 token；
已经签发的 refresh session 保持自身记录的到期时间。

配置边界固定为：

- `host` 必须非空；`port` 为 `1..65535`。
- 显式配置的 `password` 为 `1..1024` UTF-8 bytes；未提供和显式空字符串不
  等价，显式空字符串是配置错误。
- `websocketPingInterval` 为 `1s..5m`。
- `scrollbackLines` 为 `0..50000`。
- `maxSessions` 为 `1..256`；`maxClientsPerSession` 为 `1..64`。
- `authAccessTTL` 为 `1m..24h`。
- `authRefreshTTL` 为 `1h..8760h`，且不得短于 `authAccessTTL`。
- `authMaxAttempts` 为 `1..1000`。
- `initialCwd` 必须是绝对路径，且启动时存在并可进入。

配置非法时启动失败，不能截断、clamp 或静默回退。未配置密码时生成 32 字符
随机密码，并只在启动日志中输出一次。未接受风险条款时拒绝启动。

收到 `SIGINT`/`SIGTERM` 后停止接收连接、关闭 WebSocket、终止 PTY 并刷新
持久化写入；应用总 shutdown deadline 固定为 10 秒，超时后强制退出。

## 认证与登录会话

- 一次性 challenge + HMAC-SHA256 密码证明。
- 浏览器不持久化密码或可复用密码哈希。
- Challenge 有效期固定为 30 秒，单次使用，无论成功失败均消费。
- Access token 默认 15 分钟，可配置。
- Refresh token 默认 90 天，可配置，成功刷新后轮换。
- Refresh session 持久化到服务端。
- 浏览器只保存当前 Roaminal 实例的 access/refresh token 和到期时间。
- 支持查看登录会话、撤销指定会话、退出其他会话和当前登出。
- 默认连续 30 次验证失败后锁定服务，可配置；成功登录后失败计数清零。
- 锁定只能通过服务重启解除。
- WebSocket 使用 subprotocol 携带 access token，不把 token 放在 URL。
- API 的 401/403 控制当前页面的全局登录框。
- Auth persistence 使用 `0700` 目录、`0600` 文件和原子替换写入。
- 受保护 HTTP endpoint 使用 `Authorization: Bearer <access-token>`；公开 endpoint
  仅为 `/healthz`、`/api/version`、challenge、login 和 refresh。Logout 可省略
  Bearer token，以 request body 中的 refresh token 完成幂等撤销。

Challenge proof 固定为：

```text
passwordKey = SHA-256(UTF-8 password) as 32 raw bytes
message = "roaminal-login-v1:" + challengeId + ":" + salt + ":" + expiresAt
response = HMAC-SHA256(passwordKey, UTF-8 message) as 64 lowercase hex chars
```

`salt` 是 32 random bytes 的 base64url without padding；challenge/session ID 是
UUID v4。Access token 格式为 `ra_<32 random bytes base64url>`，refresh token
格式为 `rr_<32 random bytes base64url>`。服务端比较 proof 和 token hash 时
必须使用 constant-time comparison。

持久化 password fingerprint 不能直接保存 `passwordKey`，固定计算为：

```text
passwordFingerprint = SHA-256(
  UTF-8("roaminal-password-fingerprint-v1:") || passwordKey
) as 64 lowercase hex chars
```

启动时删除 fingerprint 与当前密码不匹配的 refresh sessions 并原子重写 auth
file，因此修改密码会撤销此前所有登录。未配置密码时每次启动都会生成新随机
密码，也会使上次启动留下的 refresh sessions 失效；需要跨重启保留登录时必须
显式配置稳定密码。
