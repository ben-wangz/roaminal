# Roaminal MVP 实施计划

> 文档版本：1.0
> 更新日期：2026-08-06
> 文档状态：**Approved / 可直接实施**
> 批准日期：2026-08-06
> 目标读者：后续负责完整实现的 Coding Agent
> 行为参考：`~/temp/Tabminal`，tag `v3.0.40`，commit
> `fbd26d3aff033fd850a6696eccb107520780fd8b`

## 1. 文档目的

本计划用于锁定 Roaminal MVP 的产品范围、行为契约、Go 后端架构、Web
前端架构、部署方式和验收方法。最终版本应当能够直接交给另一个 Agent
连续执行，除文档明确列出的停止条件外，不再需要人为选择。

`DEC-001` 至 `DEC-029` 已全部确认，正文不存在待实施 Agent 选择的分支。

本文所说的“功能一比一复现”是指：

- 对纳入 MVP 的功能，复现 Tabminal 的用户可见行为、交互、协议语义和
  恢复能力。
- Tabminal 仅作为行为规格、交互参考和验收 oracle，不直接复制其 Node.js
  后端或前端单文件代码。
- 网络后端、PTY 和持久化使用 Go 重新实现；服务端 terminal emulator 是
  同容器内的独立 Node.js worker；前端按领域拆分为 TypeScript 模块。
- 产品名、命令、环境变量、协议、存储目录和浏览器存储键统一使用
  Roaminal 命名空间。
- 文件工作区、Agent、AI、PWA 安装和原生客户端必须连同死代码、依赖、
  接口、DOM、样式、测试和文档一起排除。
- UI 保持参考实现的终端布局和交互，不要求保留因删除功能而产生的空白
  区域，也不要求源码结构相同。

若实施中确实复制了参考项目的任何代码或资产，必须保留对应 MIT 版权
声明；默认路线是不复制，并在 `THIRD_PARTY_NOTICES.md` 中记录行为参考和
所有直接依赖的许可证。

## 2. 已锁定的产品与技术前提

1. Roaminal 是纯 Web 应用，由 Chrome 客户端、Go backend 和内部 terminal
   worker 组成。
2. 不开发任何桌面或移动原生客户端。
3. 开发阶段只保证 Google Chrome 兼容性。
4. 页面适配桌面、平板和手机尺寸的 Chrome。
5. MVP 不包含文件工作区；该部分以后独立设计。
6. MVP 不包含 ACP、Coding Agent、terminal-native AI 或任何模型服务。
7. MVP 通过普通容器运行时或 Kubernetes 部署。
8. 只连接页面来源对应的单个 Roaminal 实例；保留同一实例内的多 Terminal
   Tab，不实现多 Host 注册、切换或聚合。
9. 后端使用 Go 1.26.5；运行平台只承诺 Linux 容器。
10. 终端只承诺 Bash，不实现或测试 Zsh、Fish、PowerShell 等其他 shell。
11. 前端资源全部构建进镜像，运行时不依赖 CDN 或其他公网静态资源。
12. Kubernetes 使用单副本 Deployment，不使用 StatefulSet、HPA 或多副本。
13. Kubernetes MVP 只交付普通 YAML，不提供 Helm 或 Kustomize。
14. MVP 不提供 PWA 安装，也不注册 Service Worker。
15. 参考实现的前端单文件架构必须重构；复现的是功能，不是源码。
16. Go 是唯一监听网络端口的 backend；一个独立 Node.js 子进程只运行官方
    xterm.js headless terminal emulator，不提供 HTTP、WebSocket 或远程 RPC。

## 3. MVP 成功定义

MVP 完成后，用户能够：

1. 在 Chrome 中打开 Roaminal 并通过密码登录。
2. 创建、切换和关闭多个真实 Linux PTY + Bash 会话。
3. 刷新页面、临时断网或从另一台设备连接后继续使用已有 PTY。
4. 在桌面、平板和手机布局中完成相同的核心终端操作。
5. 在同一服务实例中通过多个 Terminal Tab 使用、预览和管理多个会话。
6. 看到当前服务连接状态、会话状态和基础系统状态。
7. 使用终端搜索、链接识别、终端进度、触控辅助键盘和常用快捷键。
8. 以容器启动服务，或使用仓库内普通 Kubernetes YAML 部署服务。
9. 通过持久卷保存认证会话、终端元数据、scrollback 和终端快照。

“持久终端”的精确定义已经锁定：

- 浏览器刷新、网络中断、浏览器关闭和其他设备连接不会终止服务端 PTY。
- Go 服务进程或 Pod 重启后，恢复相同 session ID、cwd、标题、尺寸和
  scrollback，并创建新的 Bash PTY。
- 重启前正在运行的 Unix 进程不会继续运行，也不得在 UI 中显示为仍存活。

## 4. MVP 范围

### 4.1 服务启动与配置

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
- 配置解析只接受文档定义的 canonical 字段；不兼容 Tabminal 旧字段。
- MVP 配置项：

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

- Duration 使用 Go duration 字符串；scrollback lines、port、attempts 和
  capacity 使用十进制整数。
- Access TTL、Refresh TTL、失败锁定阈值和 accept terms 均可配置，修改后
  重启服务生效；MVP 不做配置文件热加载。
- 新 TTL 只影响修改后签发的 token；已经签发的 refresh session 保持自身
  记录的到期时间。
- 配置边界固定为：
  - `host` 必须非空；`port` 为 `1..65535`。
  - 显式配置的 `password` 为 `1..1024` UTF-8 bytes；未提供和显式空字符串
    不等价，显式空字符串是配置错误。
  - `websocketPingInterval` 为 `1s..5m`。
  - `scrollbackLines` 为 `0..50000`。
  - `maxSessions` 为 `1..256`；`maxClientsPerSession` 为 `1..64`。
  - `authAccessTTL` 为 `1m..24h`。
  - `authRefreshTTL` 为 `1h..8760h`，且不得短于 `authAccessTTL`。
  - `authMaxAttempts` 为 `1..1000`。
  - `initialCwd` 必须是绝对路径，且启动时存在并可进入。
- 配置非法时启动失败，不能截断、clamp 或静默回退。
- 未配置密码时生成 32 字符随机密码，并只在启动日志中输出一次。
- 未接受风险条款时拒绝启动。
- 收到 `SIGINT`/`SIGTERM` 后停止接收连接、关闭 WebSocket、终止 PTY，
  刷新持久化写入；应用总 shutdown deadline 固定为 10 秒，超时后强制退出。

### 4.2 认证与登录会话

复现参考认证行为，但完全使用 Go 标准密码学库和 Roaminal 命名空间：

- 一次性 challenge + HMAC-SHA256 密码证明。
- 浏览器不持久化密码或可复用密码哈希。
- Challenge 默认有效期固定为 30 秒，单次使用，无论成功失败均消费。
- Access token 默认 15 分钟，可配置。
- Refresh token 默认 90 天，可配置，成功刷新后轮换。
- Refresh session 持久化到服务端。
- 浏览器只保存当前 Roaminal 实例的 access/refresh token 和到期时间。
- 支持查看登录会话、撤销指定会话、退出其他会话和当前登出。
- 默认连续 30 次验证失败后锁定服务，可配置；成功登录后失败计数清零。
- 锁定只能通过服务重启解除。
- WebSocket 使用 subprotocol 携带 access token，不把新 token 放在 URL。
- API 的 401/403 控制当前页面的全局登录框。
- Auth persistence 必须使用 `0700` 目录、`0600` 文件和原子替换写入。
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

持久化用的 password fingerprint 不能直接保存 `passwordKey`，固定计算为：

```text
passwordFingerprint = SHA-256(
  UTF-8("roaminal-password-fingerprint-v1:") || passwordKey
) as 64 lowercase hex chars
```

启动时删除 fingerprint 与当前密码不匹配的 refresh sessions 并原子重写 auth
file，因此修改密码会撤销此前所有登录。未配置密码时每次启动都会生成新随机
密码，也会使上次启动留下的 refresh sessions 失效；需要跨重启保留登录时必须
显式配置稳定密码。

### 4.3 持久终端服务端

- 使用 `github.com/creack/pty v1.1.24` 创建和 resize Linux PTY。
- 每个会话只启动 `/bin/bash --noprofile --rcfile <roaminal-bashrc> -i`。
- 不提供 shell 选择配置，不包含其他 shell 的 hooks 或兼容分支。
- 每个会话有独立 ID、创建时间、初始 cwd、当前 cwd、标题、尺寸、状态和
  最近 100 条执行记录。单条 execution 的 `command + input` 共享 64 KiB、
  `output` 最多 960 KiB UTF-8 bytes；超出时分别保留 command/input 前部和
  output 尾部，并标记 `truncated: true`。
- Title 最多 512 UTF-8 bytes，超出时在 code point boundary 截断；cwd 最多
  4096 UTF-8 bytes，超限 OSC 7 update 直接忽略。
- 捕获 OSC title、OSC 7 cwd 和 Roaminal shell execution markers。
- 内部 marker 从用户可见输出中移除，但普通 ANSI/xterm 序列保持原样。
- 支持输入、resize、PTY 输出广播和多个浏览器客户端同时连接。
- 多客户端保留 terminal-query response owner/claim 机制。
- 每个 session 由单独 event-loop goroutine 串行处理 PTY output、input、
  resize、attach、detach、snapshot 和 close，避免状态竞争。
- 每个 WebSocket client 的 live output 发送队列上限固定为 4 MiB encoded
  payload；慢客户端不得阻塞 PTY，队列溢出时以 WebSocket code `1013`、reason
  `slow_client` 关闭并允许其重新连接。Attach snapshot/control message 不计入
  该额度，但 attach 期间排队的 live output 计入。
- 达到 `maxSessions` 时创建 session 返回 `409`；达到
  `maxClientsPerSession` 时新 WebSocket 在 upgrade 前返回 `429`。已有 session
  和 client 不受配置降低影响，新的创建/attach 被拒绝。
- 保留 shell execution marker，用于运行状态、完成状态和通知。
- 服务端 shadow terminal 与浏览器 main terminal 使用相同的
  `scrollbackLines`；浏览器 preview terminal 固定 `scrollback: 0`。
- 保留 headless terminal snapshot 和 attach snapshot。
- 元数据、序列化快照和执行记录按 session 持久化；MVP 不保存原始 output
  WAL。
- 关闭 session 时终止进程组，并删除 JSON 和 snapshot。
- 服务启动时加载 session 元数据和 snapshot，创建新的 Bash PTY，并明确
  清除旧 execution 的 running 状态。
- 恢复时持久化 cwd 不存在或不可进入，则记录 warning 并使用已验证的
  `initialCwd`；不能因单个已删除目录阻止其他 sessions 恢复。
- 服务端不自动创建默认 session；前端在 session 清单为空时创建一个。
- 新 session 默认继承最近活动 session 的 cwd；没有记录时使用
  `initialCwd`。
- `/workspace` 不存在或不可进入时启动失败，不回退到其他目录；是否可写由
  volume 的部署策略决定。

Go backend 启动一个独立 Node.js `terminal-worker` 子进程。Worker 使用
Tabminal 已验证的官方 `xterm-headless 5.3.0` 和
`xterm-addon-serialize 0.11.0` 实现 headless terminal 和 snapshot；这里的
“原生 xterm”指 xterm.js 官方 JavaScript headless 实现，不是系统 `xterm`
程序，也不是自行编写 terminal emulator。版本和替换规则见第 6.2 节及
`DEC-019`、`DEC-025`、`DEC-026`。

#### 4.3.1 Go backend 与 terminal worker 的精确职责

每个 Terminal Tab 对应一个 Go session 和一个 worker 内的 headless xterm
实例及 `SerializeAddon`。Worker 中的实例是 PTY 的“影子终端”，不是用户看到
的浏览器 renderer。一个 worker 承载当前 backend 的所有 session，不为每个
Terminal Tab 启动单独进程。

数据流固定为：

```text
Bash process
  <-> Linux PTY
        |- input/resize <- WebSocket client
        `- output -> Roaminal marker/metadata parser
                    |- cleaned output -> terminal-worker shadow terminal
                    |- cleaned output -> attached WebSocket clients
                    `- metadata/execution -> session state

attach/reconnect -> request snapshot at sequence barrier
                 -> serialize current worker state
                 -> snapshot -> meta -> status -> queued live output
```

Go backend 独占以下能力：

1. 监听 HTTP/WebSocket、认证用户并校验 same-origin。
2. 创建和管理 Linux PTY、Bash process group、session event loop 与浏览器
   client queues。
3. 过滤 private marker，解析 cwd/execution metadata，广播 live output，并
   处理 terminal-query response owner/claim。
4. 决定 checkpoint 时机，执行 snapshot 原子持久化和损坏隔离。
5. 启动、监督和关闭唯一的 terminal worker；浏览器和集群网络不能直接访问
   worker。

Terminal worker 只承担以下能力：

1. 顺序解析清理后的 PTY ANSI/VT output，维护 normal/alternate buffer、
   scrollback、cursor、cell attributes、terminal modes 和 resize/reflow 状态。
2. 在浏览器 attach/reconnect 时，把当前状态序列化成可由浏览器 xterm.js
   重放的 ANSI snapshot，避免从进程启动开始重放全部原始 output。
3. 为服务重启保存可恢复的视觉 terminal state；启动时先把持久化 snapshot
   写入新的 shadow terminal，再启动替代 Bash 并继续追加新 output。
4. 为 contract tests 暴露 buffer、cursor、mode 和文本状态，用于比较
   snapshot replay 与浏览器 xterm.js 的结果。
5. 跟踪参考实现额外补充的 `?1005`、`?1006`、`?1015` mouse encoding mode，
   并把启用状态追加到 serialized snapshot。

集成约束：

- Go 为每个 session 的 `write` 和 `resize` mutation 分配从 1 开始严格递增的
  `sequence`；worker 按 session 串行执行，遇到重复、跳号或倒序立即返回
  protocol error。
- Worker 对每个 session 使用 Promise chain；`write` 必须等待 xterm.js write
  callback 后才算完成。`snapshot` 只有在此前 mutation 全部完成后才能返回，
  并携带 `throughSequence`。
- Roaminal private shell markers 在写入 shadow terminal 和广播前移除；普通
  ANSI/OSC/DCS 必须保留。
- Go 对 PTY output 使用每 session streaming UTF-8 decoder；不完整 code point
  留到下一 chunk，非法序列统一替换为 U+FFFD。Worker payload 和 browser
  `output.data` 必须来自同一份 decoded stream，不能因 chunk boundary 分歧。
- OSC 7 cwd 和 Roaminal execution markers 由 Roaminal streaming parser
  在 Go 中处理，不依赖 worker。
- attach 建立 serialization barrier；barrier 后到达的 output 先排队，直到
  worker 返回相同 `throughSequence`，且 `snapshot -> meta -> status` 发送完毕，
  再按原顺序发送，不能丢失或重复。
- Worker headless xterm 和 browser xterm.js main terminal 使用同一个
  `scrollbackLines` 值，保证 snapshot replay 不因两端容量不同而再次裁剪。
- output 或 resize 后把 snapshot 标为 dirty；首次 dirty 后等待 250ms 合并
  写入，但持续 output 不得把 checkpoint 推迟超过 1s。
- Snapshot 以 ANSI replay bytes、`0600` 权限和 temp + fsync + rename + parent
  directory fsync 原子替换持久化；SIGTERM 在 session output drain 后强制
  flush 所有 dirty snapshots。MVP 不实现 output WAL；存储正常时，SIGKILL
  或节点掉电最多可能丢失最后 1s 尚未 checkpoint 的视觉 output。持续存储
  写入失败不承诺该上限，必须记录明确错误并暴露 persistence degraded 状态。
- Snapshot 损坏时把文件重命名为带 UTC timestamp 的 `.corrupt` 文件，记录
  明确日志，使用保留的 metadata 和空 scrollback 启动替代 Bash；不能导致
  其他 sessions 或整个服务启动失败。

#### 4.3.2 Terminal worker 进程与 IPC contract

MVP 的“独立进程”固定为同一容器内由 Go 直接启动和监督的子进程，不是
sidecar、独立 Deployment 或微服务：

```text
tini (PID 1)
  `- roaminal (Go backend, only network listener)
       `- node /opt/roaminal/terminal-worker/src/index.mjs
```

- Go 使用 worker 的 stdin/stdout 做全双工 IPC；worker 不监听 TCP 或 Unix
  socket，不加入 Kubernetes Service，也不需要独立鉴权或 TLS。
- Production worker command/path 是镜像内固定实现，不增加 CLI、环境变量或
  JSON 配置；测试可以通过 Go 内部 dependency injection 替换 fake worker。
- Go 在 Linux 上为子进程设置 parent-death signal；Go 异常退出时不能遗留
  orphan worker。
- stderr 只用于 worker diagnostic log；stdout 只能写 protocol frame，严禁
  console log 污染。
- Frame 先写 `uint32-be headerLength + uint32-be payloadLength`，随后写 JSON
  header 和 raw payload。Header 上限 64 KiB，payload 上限 256 MiB；越界、
  非法 JSON、未知 protocol version 或未知 operation 都是 fatal protocol
  error。
- `raw payload` 避免 base64 和 UTF-8 chunk boundary 问题；PTY output 作为
  `Uint8Array` 交给 xterm.js，snapshot 作为 UTF-8 bytes 返回。
- 协议名固定为 `roaminal-terminal-worker/1`。Operation 只包含 `hello`、
  `create`、`restore`、`write`、`resize`、`snapshot`、`close`、`shutdown` 和
  对应 `ready`/`result`/`error`；不得加入 PTY、文件、网络或认证操作。
- 每个需要 response 的 control request 使用不可复用的 `requestId`；不等待
  response 的 `write`/`resize` mutation 不带 `requestId`，但必须带 `sessionId`
  和 `sequence`。Snapshot result 带 `throughSequence`。多 session 可以交错，
  同一 session 必须保持严格顺序。
- `write` 不做逐条 response，以免在高频 output 下产生双倍 IPC；sequence
  gap、worker error、snapshot/close 等 control operation 必须响应。Go 到
  worker 的 writer queue 固定为 16 MiB raw payload budget，不能静默丢弃
  mutation；队列满时对 session event loop 施加 backpressure，连续 10 秒没有
  forward progress 按 fatal worker timeout 处理。
- Worker `hello`/`ready` deadline 固定为 5 秒；create、restore、snapshot 和
  close request deadline 固定为 30 秒；这些超时均进入 `DEC-027` fail-fast。
- 启动时 Go 先完成 `hello`/`ready` 和 protocol version 校验，再开放 9846；
  worker 不存在、版本不匹配或初始化失败时，Go 明确报错并非零退出。
- 服务就绪后 worker 异常退出、持续超时或发生 fatal protocol error 时，Go
  按 `DEC-027` 停止整个服务并非零退出，不尝试原地重启 worker。
- Go 保持 snapshot 文件、session metadata 和 checkpoint scheduler 的唯一
  ownership；worker 不读写 state volume。这样未来替换 emulator 不改变
  持久化 owner 或 Kubernetes YAML。
- 正常 shutdown 顺序为：停止新连接和 input，终止 PTY 并 drain output，请求
  所有 dirty snapshot 并原子落盘，发送 worker `shutdown`，等待退出，超时后
  强制终止子进程；所有步骤共享应用 10 秒总 deadline。

Header 字段使用 lower camel case；`requestId` 和 `sessionId` 是 UUID string，
`sequence`/`throughSequence` 是无前导零的十进制 string，避免 JavaScript
number 精度差异。`ready`/`result` 必须 echo 对应 control request 的
`requestId`。每种 frame
固定如下：

| Direction | `op` | Header fields | Raw payload | Response |
| --- | --- | --- | --- | --- |
| Go -> worker | `hello` | `requestId`, `protocol` | empty | `ready`，包含 `protocol`, `engine`, `engineVersion`, `serializeAddonVersion` |
| Go -> worker | `create` | `requestId`, `sessionId`, `cols`, `rows`, `scrollbackLines` | empty | `result` with `requestOp: "create"`, `throughSequence: "0"` |
| Go -> worker | `restore` | `requestId`, `sessionId`, `cols`, `rows`, `scrollbackLines`, `throughSequence` | UTF-8 ANSI snapshot | `result` with `requestOp: "restore"` and matching `throughSequence`；必须等 write callback |
| Go -> worker | `write` | `sessionId`, `sequence` | UTF-8 PTY output，单 frame 最大 256 KiB | none |
| Go -> worker | `resize` | `sessionId`, `sequence`, `cols`, `rows` | empty | none |
| Go -> worker | `snapshot` | `requestId`, `sessionId`, `throughSequence` | empty | `result` with `requestOp: "snapshot"`, matching `throughSequence` and UTF-8 snapshot payload |
| Go -> worker | `close` | `requestId`, `sessionId` | empty | `result` with `requestOp: "close"` |
| Go -> worker | `shutdown` | `requestId` | empty | `result` with `requestOp: "shutdown"`，随后正常退出 0 |
| worker -> Go | `error` | `requestId?`, `sessionId?`, `code`, `message`, `fatal: true` | empty | Go 执行 `DEC-027` |

Worker error code allowlist 为 `protocol_version`、`invalid_frame`、
`unknown_operation`、`duplicate_session`、`unknown_session`、
`sequence_mismatch` 和 `engine_failure`。Go 本地产生的 `worker_timeout`、
`worker_exit`、`worker_io` 进入同一 fatal path。实现不得增加自动容错分支；
新增 operation 或字段必须先升级 protocol version。Header 缺少必填字段、
出现未知字段或 payload 与 operation 不匹配都属于 `invalid_frame`。

`create` 后首个 mutation sequence 是 `"1"`。`restore` 的尺寸、scrollback 和
`throughSequence` 来自已验证的 snapshot envelope；worker 以该 sequence 作为
恢复基线，下一个 mutation 必须是 `throughSequence + 1`。Go 收到 restore
result 后，如 session metadata 的当前尺寸不同，再以该下一个 sequence 发送
`resize`。这样服务重启不会把已持久化 sequence 重置为 `"0"`。

Worker 明确不承担以下职责：

- 不创建 PTY，不启动、终止或恢复 Bash/Unix process。
- 不渲染 DOM/canvas，不处理字体、selection、search、links 或 touch input；
  这些由浏览器的 xterm.js 和前端 UI 负责。
- 不管理 HTTP、WebSocket、认证、Terminal Tab 或多个浏览器 client。
- 不解析 Roaminal private execution markers，也不维护 cwd、execution history、
  attention、toast 或系统通知。
- 不决定 snapshot 文件路径、原子写入、持久化频率或损坏恢复策略。
- 不直接作为审计日志或命令历史；terminal snapshot 是视觉恢复材料，不是
  shell command log。
- 不订阅或转发 headless xterm 的 `onData`/`onBinary`；server-side emulator
  产生的 DA/DSR response 不写回 PTY。当前 browser xterm.js client 负责
  terminal query response；多个 client attach 时由 query-response
  owner/claim 保证只接受一个 client 的 response。

#### 4.3.3 持久化文件 contract

Roaminal 只写以下 versioned 文件，不写 output log、command audit log 或其他
隐式状态：

```text
/home/roaminal/.roaminal/
  auth-sessions.json
  sessions/
    <sessionId>.json
    <sessionId>.snapshot
```

`<sessionId>.json` 固定 schema：

```json
{
  "formatVersion": 1,
  "id": "uuid",
  "title": "string",
  "initialCwd": "/absolute/path",
  "cwd": "/absolute/path",
  "cols": 120,
  "rows": 30,
  "createdAt": "RFC3339 UTC",
  "updatedAt": "RFC3339 UTC",
  "executions": [
    {
      "command": "string",
      "exitCode": 0,
      "input": "string",
      "output": "string",
      "startedAt": "RFC3339 UTC",
      "completedAt": "RFC3339 UTC",
      "durationMs": 12,
      "truncated": false
    }
  ]
}
```

只持久化已经完成的 execution：`command`、`input`、`output` 始终为 string，
`exitCode` 为 integer 或 JSON `null`，两个 timestamp 均为 RFC3339 UTC，
`durationMs` 为非负 integer，`truncated` 为 boolean。Executions 只保留最近
100 条并应用第 4.3 节 byte limits。Session file 不保存 `shell`、
`managed`、environment dump、workspace/editor state、running process PID 或
running execution；这些字段出现时视为 schema error，不做兼容读取。

Snapshot file 是一个自校验 envelope：第一行固定 ASCII magic
`ROAMINAL-SNAPSHOT/1`，第二行是单行 JSON header，第三行起是原始 UTF-8 ANSI
payload。Header 固定为：

```json
{
  "cols": 120,
  "rows": 30,
  "scrollbackLines": 1000,
  "throughSequence": "42",
  "byteLength": 12345,
  "sha256": "64 lowercase hex characters"
}
```

Restore 时先验证 magic、header schema、byte length、SHA-256、UTF-8 和 256 MiB
上限；使用 header cols/rows 创建 headless terminal，写入 snapshot 并等待
callback，再 resize 到 session metadata 的当前尺寸。验证失败时把 snapshot
原子重命名为
`<sessionId>.snapshot.corrupt.<YYYYMMDDTHHMMSS.nnnnnnnnnZ>`，使用 metadata 和空
scrollback 创建新 Bash。Snapshot 损坏不能隔离或删除 metadata。

`auth-sessions.json` 固定为：

```json
{
  "formatVersion": 1,
  "sessions": [
    {
      "id": "uuid",
      "passwordFingerprint": "64 lowercase hex",
      "refreshTokenHash": "sha256 hex",
      "createdAt": "RFC3339 UTC",
      "lastSeenAt": "RFC3339 UTC",
      "refreshExpiresAt": "RFC3339 UTC",
      "rotatedAt": "RFC3339 UTC",
      "userAgent": "string, max 500 UTF-8 bytes"
    }
  ]
}
```

Access token 只存在内存；refresh token 只持久化 SHA-256 hash。未知
`formatVersion`、未知字段、类型错误或 hash/UUID/timestamp 格式错误都不做
best-effort 兼容：单个 session metadata 损坏时隔离该 session 的 metadata 和
snapshot；auth file 损坏时隔离整个 auth file 并以空 refresh session store
启动。所有隔离都记录不含 token、terminal payload 或环境变量的错误日志。

State directory 固定 `0700`，JSON/snapshot/temp/corrupt files 固定 `0600`。
每次写入使用同目录 temp file、file fsync、rename 和 parent directory fsync；
不得依赖跨目录 rename。`formatVersion: 1` 只定义 Roaminal 自身未来迁移边界，
不构成对 Tabminal 数据的兼容。

### 4.4 HTTP 与 WebSocket 协议

- 使用 Go `net/http` 和 Go 1.26 method-aware `ServeMux` 路由。
- 使用 `github.com/coder/websocket v1.8.15` 提供 WebSocket。
- HTTP 用于权威清单和变更；WebSocket 用于终端实时流。
- `/api/heartbeat` 是当前服务实例的权威 session 清单。
- 前端 heartbeat 固定为 1000 ms；异常重试节流固定为 5000 ms。
- WebSocket 中断后由 heartbeat 驱动重连和清单校准。
- `GET /api/version` 暴露每次 Go 进程启动变化的 boot ID。
- 客户端发现 boot ID 变化后重新加载页面。
- JSON request 默认上限 1 MiB；超限返回 `413`。
- API error 固定返回 `{ "error": "message" }`，不得返回 Go stack trace。
- 浏览器 API 和 WebSocket 只接受同源请求；MVP 不提供跨 Origin CORS 配置。
- 带 JSON body 的请求必须使用 `Content-Type: application/json`；使用 Go
  `DisallowUnknownFields`，body 后存在第二个 JSON value、未知字段或类型不符
  均返回 `400`。所有 timestamp 是 UTC RFC3339 string，所有 ID 是 UUID v4。

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
DELETE /api/sessions/:id
WS     /ws/:sessionId
```

HTTP contract 固定如下；`204` response 没有 body：

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

Named JSON objects 固定为：

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
  cwd: string,
  cols: integer,
  rows: integer,
  closed: boolean,
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
  runtime: { bootId: string, persistenceDegraded: boolean }
}
```

`CreateSessionRequest.cwd` 必须是容器内存在、可进入的绝对目录；省略时按第
4.3 节继承 cwd，且 cwd 最多 4096 UTF-8 bytes。Heartbeat POST 的
`updates.sessions` 最多为当前 `maxSessions`；重复 session ID 或非法 resize
使整个 request 返回 `400` 且不做部分应用，已在并发流程中删除的未知 session
ID 则忽略。Heartbeat response 不包含 execution output；execution 只通过
WebSocket 实时消息传递并在服务端持久化。

HTTP status 固定使用：validation `400`、auth missing/expired `401`、service
locked/origin denied `403`、not found `404`、capacity conflict `409`、body too
large `413`、client capacity `429`、unexpected internal error `500`、terminal
worker unavailable `503`。每个 `5xx` response 必须生成 UUID v4 correlation
ID，同一值写入结构化日志并通过 `X-Roaminal-Request-ID` response header 返回；
response body 和日志都不得包含路径、token、PTY data 或 stack trace。

明确不保留 `POST /api/sessions/:id/state`，也不在 session schema 中写入
`editorState`、`workspaceState` 或 `managed`。

终端 WebSocket 保持参考消息语义：

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

WebSocket application message 是 UTF-8 JSON text，schema 固定为：

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

`ExecutionRecord` 与第 4.3.3 节相同。Client message 上限 1 MiB；超限以 code
`1009` 关闭。Malformed JSON、unknown type/field 或 schema violation 以 code
`1008`、reason `invalid_message` 关闭。达到 client capacity 时在 upgrade 前
返回 HTTP `429`。Server attach 顺序严格为 `snapshot -> meta -> status`，再
flush attach 期间按序排队的 `output`；正常 live output 不得在 snapshot 前
出现。Server 不在 attach 时重发历史 execution events。

### 4.5 模块化浏览器前端

前端功能与参考实现对齐，但不复用其 `public/app.js` 单文件结构。

前端固定使用 React 19 + TypeScript 7 + Vite 8，并满足以下领域边界：

```text
web/src/
  main.tsx
  app/
    app-state.ts
    app-shell.tsx
  auth/
    auth-client.ts
    auth-crypto.ts
    auth-storage.ts
    auth-session-ui.tsx
  terminal/
    terminal-protocol.ts
    terminal-runtime.ts
    terminal-transport.ts
    terminal-viewport.tsx
    terminal-tabs.tsx
    terminal-preview.tsx
    terminal-search.tsx
  status/
    heartbeat.ts
    system-status.ts
    notifications.ts
  input/
    shortcuts.ts
    touch-keyboard.ts
    viewport.ts
  ui/
    modal.tsx
    toast.tsx
    sidebar.tsx
  styles/
    tokens.css
    layout.css
    terminal.css
    responsive.css
```

约束：

- 领域模块不得通过任意全局变量隐式耦合。
- 只有 `app` 层负责跨领域编排。
- Session 数据以 `sessionId` 为一级 key。
- UI 模块不直接调用 `fetch` 或创建 WebSocket。
- 网络模块不直接操作 DOM。
- xterm 实例、WebSocket、ResizeObserver、heartbeat timer 和 PTY output
  都由 React tree 之外的 `TerminalRuntime` 管理。
- 高频 PTY output 直接调用 `terminal.write()`，不得进入 UI render state。
- Session 在 UI 重挂载时只 detach/reattach DOM；只有显式关闭 session 才
  dispose terminal runtime 和 WebSocket。
- React state 只保存低频、可序列化 UI/session metadata，不保存 PTY bytes、
  xterm instance、WebSocket 或 mutable terminal buffer。
- `TerminalViewport` 通过 ref attach/detach 已存在的 runtime；React component
  unmount 不等于关闭 session。
- 直接使用 xterm.js API，不使用第三方 React wrapper。
- 保持 React Strict Mode；effect 必须对 socket、listener、observer、timer
  和 DOM attach 做幂等 setup/cleanup。
- 不引入 Redux、Zustand、UI component library 或其他独立状态管理层；应用级
  状态使用 React state/reducer/context，外部 runtime snapshot 订阅使用
  `useSyncExternalStore`。
- 不创建与当前功能无关的抽象层。

保留的终端体验：

- Solarized Dark 风格、Monaspace Neon 字体和紧凑工作界面。
- xterm.js 及 fit、web-links、canvas、search、progress、ligatures addons。
- 标签展示标题、短 session ID、cwd、创建时间、运行/完成/attention
  状态和可用时的终端进度。
- 桌面保留标签内的实时终端缩略预览；移动尺寸不创建预览。
- 创建、切换和关闭终端；关闭当前项后选择相邻项。
- Sidebar 展开/收起和移动端 overlay。
- 页面标题跟随当前终端。
- 搜索支持大小写、全词、正则、上一个和下一个结果。
- 浏览器复制粘贴和 xterm 原生选择。
- 触控设备保留 ESC、TAB、CTRL、ALT、SHIFT、SYM、方向键和软键盘。
- 正确处理 `visualViewport`、safe area 和软键盘高度变化。
- 不出现文件、workspace 或 Agent 入口和占位。

保留快捷键：

```text
Ctrl + Shift + T        新建终端
Ctrl + Shift + W        关闭终端
Ctrl + Shift + [ / ]    切换终端
Ctrl/Cmd + F            终端内搜索
Ctrl + Shift + ?        快捷键帮助
```

### 4.6 单实例多 Terminal Tab

- 浏览器只连接提供当前页面的 Roaminal 实例；HTTP 使用当前 Origin，
  WebSocket 从当前页面协议派生 `ws://` 或 `wss://`。
- 不存在 Host entity、`hostId`、Host registry、Host picker 或跨 Host 聚合状态。
- 一个 Terminal Tab 对应一个 session ID 和一个独立 Bash PTY。
- 支持创建、切换和关闭多个 Tab；关闭当前 Tab 后选择相邻 Tab。
- session 清单为空时前端创建一个 Tab；关闭最后一个 Tab 后再创建一个。
- 同一 session 仍允许多个浏览器客户端 attach，这属于协同连接，不是多 Host。
- 保留同源 Cloudflare Access 会话失效检测；需要重新认证时刷新或打开当前
  页面根 URL。后端不启动 Tunnel，镜像不包含 cloudflared。
- `/api/cluster`、`cluster.json`、Host URL normalize/dedupe 和 Host-scoped
  localStorage 全部排除。

### 4.7 状态、提示和通知

- 保留当前服务 heartbeat 延迟图和连接状态。
- 保留 hostname、kernel、IP、CPU、内存、host uptime、Roaminal process
  uptime、FPS、延迟和 session 数。
- Linux system monitor 使用 `/proc`、cgroup v2 和 Go runtime 获取容器有效
  资源数据，不引入通用跨平台监控库。
- 保留连接丢失、恢复和终端退出 toast。
- Snapshot checkpoint 失败时 heartbeat `runtime.persistenceDegraded` 为 `true`
  并显示 warning toast；后续所有 dirty snapshots 成功写入后清除。
- 非当前终端的命令完成后显示 attention 状态。
- 用户允许时发送 Chrome 系统通知，否则降级为 toast。
- 内部 shell ready/bootstrap 命令不得产生用户通知。

### 4.8 静态资源、缓存与 Service Worker 结论

MVP 不提供 PWA 安装，不生成 Web App manifest，不设置 standalone display，
也不注册 Service Worker。

重新评估后，Service Worker 不保留，原因是：

1. 终端必须连接在线 Go 服务，离线应用壳没有可完成的核心工作流。
2. 所有 JS、CSS、字体、图标和 xterm 资源都随镜像本地交付，不需要用
   Service Worker 弥补 CDN 可用性。
3. PWA 安装已排除，Service Worker 不再承担安装能力。
4. Service Worker 会额外引入旧资源缓存、更新竞态和移动端调试成本。
5. Vite 内容哈希、HTTP cache headers 和 `/api/version` boot ID 已足够保证
   资源一致性。

缓存规则：

- `index.html`：`Cache-Control: no-cache, max-age=0`。
- `/api/version` 和所有 `/api/*`：`Cache-Control: no-store`。
- Vite 内容哈希资源：`Cache-Control: public, max-age=31536000, immutable`。
- favicon 等非哈希资源：短期缓存并必须支持 ETag。
- Go binary 使用 `embed` 打包完整 `web/dist`，运行时不读取公网资源。
- HTML 不包含 CDN URL、`preconnect`、manifest link 或 Service Worker 注册。

### 4.9 容器与 Kubernetes

- 使用 multi-stage Docker build：Node 前端构建与 worker dependency stage、
  Go 后端构建、Linux runtime。
- Node build/runtime 固定 `24.13.1`，frontend 和 terminal worker 各自使用
  lockfile。
- Go builder 固定 Go `1.26.5`。
- Runtime 使用固定 Node Debian slim digest，包含 Go binary、Node.js runtime、
  terminal worker、worker production dependencies、Bash、CA certificates 和
  tini；不包含 npm、npx、corepack、编译器或 cloudflared。
- Go binary 以 `CGO_ENABLED=0` 构建。
- 非 root 用户运行，home 为 `/home/roaminal`。
- `/home/roaminal/.roaminal` 挂 state volume。
- `/workspace` 挂显式 workspace volume。
- 终端只能访问容器和显式 volume；不挂 Docker socket、不使用 privileged、
  不进入宿主机 namespace。
- “无文件工作区”只排除 Web 文件 UI/API，不限制用户在 Bash 中操作
  `/workspace`。
- 提供 `compose.yaml` 作为普通容器运行示例，固定 `restart: unless-stopped`；
  `docker run` 文档同样使用 `--restart unless-stopped`，使 `DEC-027` 在非
  Kubernetes 部署中也能自动拉起新实例。
- Kubernetes 使用 `apps/v1 Deployment`：
  - `replicas: 1`
  - `strategy.type: Recreate`
  - 不配置 HPA
  - state PVC 和 workspace PVC 使用 `ReadWriteOnce`
  - Service 使用 `ClusterIP`
  - startup/readiness/liveness 均调用 `/healthz`
  - worker 未完成 handshake 或进入 fatal 状态时 `/healthz` 返回 503
  - password 使用 Secret
  - 非敏感配置使用 ConfigMap/env
  - resources 固定为 requests `cpu: 100m`、`memory: 256Mi`，limits
    `cpu: "2"`、`memory: 2Gi`；部署者可修改普通 YAML
  - startup probe：`periodSeconds: 2`、`timeoutSeconds: 1`、
    `failureThreshold: 15`
  - readiness probe：`periodSeconds: 5`、`timeoutSeconds: 1`、
    `failureThreshold: 2`
  - liveness probe：`periodSeconds: 10`、`timeoutSeconds: 1`、
    `failureThreshold: 3`
  - `terminationGracePeriodSeconds: 30`
  - restrictive securityContext
- 只提供普通 YAML：`deployment.yaml`、`service.yaml`、`pvc.yaml`、
  `configmap.yaml`、`secret.example.yaml`、`ingress.example.yaml`。
- 不提供 Helm、Kustomize、Operator 或其他模板层。
- 文档说明 TLS、WebSocket proxy timeout、PVC 权限、备份恢复和升级中断。

## 5. 明确排除

以下内容不得出现在 MVP 的运行时代码、UI、API、依赖或部署配置中：

- 对外 Node.js backend、Koa、node-pty 和 Node `ws`；Node.js 只允许用于
  frontend build/test 和内部 xterm.js terminal worker。
- 文件列表、读取、创建、保存、重命名、删除和 raw preview API。
- 文件树、Monaco、图片/PDF/Markdown 预览和文件图标构建。
- `memory.json`、expanded-folder API、`editorState` 和 `workspaceState`。
- workspace tabs 和 terminal pinning。
- ACP、Agent、prompt、attachment、permission、plan、usage HUD 和 managed
  terminal。
- terminal-native AI、`#` prompt 劫持、失败命令 auto-fix、OpenAI、
  OpenRouter 和 Google Search。
- Web App manifest、PWA 安装和 Service Worker。
- 任何原生客户端、桌面安装包、launchd、systemd/pm2 和远程部署脚本。
- 服务端 Cloudflare Tunnel 和 cloudflared。
- 多 Host registry、Host picker、`/api/cluster`、`cluster.json`、跨 Origin 子
  Host 连接和 Host-scoped auth storage。
- Zsh、Fish、PowerShell、Windows PTY 和 macOS 后端兼容代码。
- Firefox、Safari、Edge 的兼容性承诺和专项 workaround。
- StatefulSet、Helm、Kustomize、HPA、多副本、PTY 跨 Pod 迁移和无缝升级。
- 文件上传下载、SFTP、端口转发、SSH client 和 Kubernetes 管理终端。
- 运行时 CDN 或任何必须访问公网才能加载的浏览器资源。

## 6. 参考行为到新实现的映射

参考代码不得直接搬运。实施 Agent 必须先用测试描述行为，再在新架构中
实现。

| 参考领域 | Go/worker/模块化 Web 实现 |
| --- | --- |
| `src/config.mjs` | `internal/config`，Go JSON/env/flags loader |
| `src/auth.mjs` | `internal/auth`，Go crypto + atomic persistence |
| `src/persistence.mjs` | `internal/persistence`，typed schema + atomic files |
| `src/system-monitor.mjs` | `internal/monitor`，Linux `/proc`/cgroup implementation |
| `src/terminal-manager.mjs` | `internal/terminal/manager.go` |
| `src/terminal-session.mjs` | `internal/terminal/session.go` + event loop + parser；headless xterm 部分进入 `terminal-worker` |
| `src/server.mjs` | `internal/server` + `internal/httpapi` |
| `shell/tabminal-*` | 新写 `shell/roaminal-bashrc` 和 `roaminal-hooks.bash` |
| `public/app.js` | `web/src` 领域 TypeScript 模块 |
| `public/styles.css` | `web/src/styles` 分层 CSS |
| `public/sw.js` | 不实现 |
| Node tests | Go unit/integration + worker `node:test` + Vitest + Playwright |

计划中的仓库结构：

```text
cmd/roaminal/main.go
internal/auth/
internal/config/
internal/httpapi/
internal/monitor/
internal/persistence/
internal/server/
internal/terminal/
internal/webassets/
shell/
terminal-worker/
  src/index.mjs
  test/
  package.json
  package-lock.json
web/
  src/
  public/
  package.json
  package-lock.json
  tsconfig.json
  vite.config.ts
testdata/
deploy/kubernetes/
docs/
go.mod
go.sum
Dockerfile
compose.yaml
Makefile
```

### 6.1 Go 依赖基线

```text
github.com/creack/pty v1.1.24
github.com/coder/websocket v1.8.15
golang.org/x/sys v0.47.0
```

除以上依赖外优先使用 Go 标准库。所有 Go module 必须在 `go.mod` 和
`go.sum` 固定。

### 6.2 Terminal worker 依赖与替换基线

MVP worker 固定参考实现已验证的精确 runtime 版本：

```text
Node.js 24.13.1（build/test/runtime）
xterm-headless 5.3.0
xterm-addon-serialize 0.11.0
xterm 5.3.0（SerializeAddon peer dependency）
```

这些是 xterm.js 官方旧包名，已经 deprecated 并迁移为 `@xterm/*`；MVP 仍先
使用它们，是因为 Tabminal `v3.0.40` 的服务端快照路径已经用这组版本验证。
不得在实现过程中自行升级为 scoped package、换成不同 beta、回退到
`xterm-go` 或手写 parser。所有版本由 `terminal-worker/package-lock.json`
固定，production dependencies 随镜像本地交付。

Roaminal 必须用参考实现提取的 fixtures 验证宽字符、组合字符、颜色、
alternate buffer、scrollback、cursor、resize 和 extra private mouse modes。
固定版本出现问题时：

1. 用最小 fixture 记录 expected/actual、依赖版本和影响范围。
2. 能通过 worker 内有界 adapter 满足 contract 时先完成实现，记录批准差异。
3. 固定版本无法满足 Definition of Done，且 adapter 也无法解决时，才按第
   13 节停止请求人工决策。

MVP 不包含第二套 emulator，也不提供 runtime engine switch。MVP 完成后，
可以另建实现相同 `roaminal-terminal-worker/1` protocol 的 `xterm-go` worker，
固定候选 commit `8e117204ebedc133bf33ee9eb759c8484f843cee`，再执行：

1. 相同 VT/golden fixtures 和 snapshot replay conformance tests。
2. 相同输入数据、session 数、cols/rows、scrollback 和容器资源限制下的
   output throughput、write-to-barrier latency、snapshot latency/size、CPU 和
   RSS benchmark。
3. 分别报告 emulator-only benchmark 与包含 IPC 的 end-to-end benchmark，
   不能把预期的性能收益写成未经测量的结论。
4. 若发现 `xterm-go` 非阻塞不足，输出可提交开源社区的最小复现、测试和
   优化提案；这不阻塞或改变 MVP。

### 6.3 前端依赖基线

前端依赖固定为：

```text
Node.js 24.13.1（frontend build/test；同时也是 worker runtime）
TypeScript 7.0.2
Vite 8.1.5
Vitest 4.1.10
@playwright/test 1.62.1
ESLint 10.8.0
typescript-eslint 8.66.0
@fontsource/monaspace-neon 5.2.5
@xterm/xterm 6.1.0-beta.197
@xterm/addon-fit 0.12.0-beta.197
@xterm/addon-web-links 0.13.0-beta.197
@xterm/addon-canvas 0.8.0-beta.48
@xterm/addon-search 0.17.0-beta.197
@xterm/addon-progress 0.3.0-beta.197
@xterm/addon-ligatures 0.11.0-beta.197
react 19.2.8
react-dom 19.2.8
@vitejs/plugin-react 6.0.4
@types/react 19.2.18
@types/react-dom 19.2.4
```

不再引入 Redux、Zustand、xterm React wrapper 或 UI component
library。应用级低频状态优先使用 React 自带 state/reducer/context；只有
runtime 外部快照需要订阅时使用 `useSyncExternalStore`。

xterm 和 addons 固定使用参考实现的精确版本。如果这些 beta 包无法通过
本地构建或测试，实施 Agent 必须停止并记录兼容问题，不能自行升级到行为
不同的版本。全部 npm 依赖由 lockfile 固定并打包进 `web/dist`。

## 7. Roaminal 命名空间

运行时代码、UI 和数据不得残留 Tabminal 命名，行为参考和版权说明除外。

| Tabminal | Roaminal |
| --- | --- |
| package/bin `tabminal` | binary `roaminal` |
| `~/.tabminal` | `~/.roaminal` |
| `TABMINAL_*` | `ROAMINAL_*` |
| `TABMINAL_CWD` | `ROAMINAL_CWD` |
| `TABMINAL_SESSION_ID` | `ROAMINAL_SESSION_ID` |
| `TABMINAL_SHELL_READY` | `ROAMINAL_SHELL_READY` |
| `tabminal.v1` | `roaminal.v1` |
| `tabminal.auth.` | `roaminal.auth.` |
| `tabminal-login-v1` | `roaminal-login-v1` |
| access prefix `ta_` | `ra_` |
| refresh prefix `tr_` | `rr_` |
| `tabminal_auth_state:<hostId>` | `roaminal_auth_state` |
| runtime boot storage key | `roaminal_runtime_boot_id` |
| shell functions/markers | `_roaminal_*` / `RoaminalPrompt` |
| Web name/icons | Roaminal / `r>` |

不读取、迁移或兼容 Tabminal 的配置、数据目录、token、协议和 localStorage。

## 8. 目标架构与数据边界

```text
Chrome desktop/tablet/mobile
          |
          | same-origin HTTPS: auth, heartbeat, session inventory
          | WSS: snapshot/meta/status/output/execution/input/resize
          v
Roaminal Linux container
  |- embedded Vite dist (no CDN, no Service Worker)
  |- roaminal Go backend (only network listener)
  |    |- net/http API, static server and coder/websocket
  |    |- auth store and Linux/cgroup system monitor
  |    |- terminal manager
  |    |    `- one event loop + one creack/pty Bash process per session
  |    `- terminal worker supervisor + framed stdio IPC
  |- Node terminal-worker (one headless xterm per session)
  `- /home/roaminal/.roaminal on state PVC
       |- sessions/*.json
       |- sessions/*.snapshot
       `- auth-sessions.json
```

数据边界：

- Heartbeat 是 session inventory 的权威来源，WebSocket 不是唯一事实来源。
- 每个 session 对应一个 Terminal Tab 和一个 Bash PTY。
- 浏览器拥有当前实例的 token；服务端拥有 refresh session。
- PTY 只存在于当前 Go 进程，其他 Pod 不能接管。
- Headless xterm state 只存在于当前 Node worker；Go backend 通过 sequence 和
  snapshot barrier 维持一致性，worker 不拥有持久卷。
- PVC 保存恢复材料，不保存正在运行的 Unix 进程。
- Go static embed 保证镜像包含所有浏览器资源。
- Node runtime、worker code 和 worker dependencies 也全部在镜像内，不在
  容器启动时安装或联网获取。

## 9. 实施阶段

实施 Agent 必须按顺序执行，每个 Phase 的 gate 通过后再提交该阶段的原子
commit 并进入下一阶段。

### Phase 0：冻结行为基线

1. 确认参考仓库 HEAD 为指定 commit；不匹配时停止。
2. 记录 Roaminal 开始前工作树状态，保留用户修改。
3. 建立 `docs/plan/mvp/implementation-log.md`。
4. 从参考实现提取 MVP 行为清单、API fixtures、WebSocket fixtures 和关键
   视口截图；提取 headless xterm input/snapshot fixtures，不复制实现代码。
5. 创建 `THIRD_PARTY_NOTICES.md`，记录 Tabminal 行为参考和依赖许可证。

Gate：行为基线、来源版本、许可证和工作树均可审计。

### Phase 1：Go 与 Web 工程骨架

1. 创建 Go module、`cmd/roaminal` 和 `internal/*` 包。
2. 创建 TypeScript/Vite 模块化前端。
3. 创建 ESM `terminal-worker` package、framed IPC codec 和 Node test skeleton。
4. 创建统一 `Makefile`：`build`、`test`、`lint`、`dev`、`container`。
5. 配置 Go formatting/vet/static analysis、worker tests 和 frontend
   lint/typecheck。
6. 建立 Go embed 流程：先生成 `web/dist`，再编译 binary。
7. 建立 Roaminal favicon；不创建 manifest 或 PWA icons。

Gate：

- `go test ./...`、`go vet ./...` 可执行。
- frontend 与 terminal-worker 的 `npm ci`、lint/test 可执行。
- production build 生成 Go binary 和独立 worker artifact；Go 能从 embed 返回
  页面和本地资源，并能完成 worker protocol handshake。
- 页面发出的静态资源请求全部指向当前 Origin。

### Phase 2：配置、认证与持久化

1. 实现完整配置优先级、严格验证和 auth policy 可配置项。
2. 实现 challenge、login、refresh、logout、session list/revoke。
3. 实现 access/refresh token rotation 和锁定。
4. 实现 session 和 auth session typed schemas。
5. 所有写入采用 temp file + fsync + rename，权限符合敏感性要求。
6. 前端使用 Web Crypto 实现 challenge response，使用单实例 auth storage。

Gate：认证和配置 tests 全部通过；不存在旧命名空间或排除数据字段。

### Phase 3：PTY 与终端快照

1. 实现 Bash PTY spawn、process group、resize、input/output 和 shutdown。
2. 实现 session event loop、client queues 和 query owner。
3. 实现 Bash hooks、title/cwd/execution parser 和内部 marker 过滤。
4. 实现 Go worker client/supervisor、framed IPC、request correlation、sequence
   validation、bounded backpressure 和 startup/shutdown handshake。
5. 在 Node worker 实现 headless terminal state、per-session Promise chain、
   serialized snapshot 和 extra mouse mode adapter。
6. 实现 250ms 合并/1s 最大 checkpoint 延迟、SIGTERM flush 和损坏 snapshot
   隔离。
7. 实现 attach 时 snapshot/meta/status 顺序与实时 output 排队。
8. 实现 service restart 后新 Bash + 历史恢复。
9. 删除 session 时清理进程、worker state 和所有文件。
10. 加入 xterm.js golden fixtures、snapshot round-trip、sequence/barrier 和
    extra mouse mode tests。

Gate：

- `go test -race ./...` 无 data race。
- 两个客户端 attach、慢客户端、query owner、resize 和 disconnect 正确。
- refresh attach 不丢 scrollback。
- service restart 恢复材料但不宣称旧进程存活。
- 持续 output 时 snapshot checkpoint 间隔不超过 1s，且不存在 output WAL。
- 损坏 snapshot 只影响对应 session 的 scrollback restore。
- worker stdout framing、raw byte boundary、sequence gap 和 snapshot barrier
  contract 全部通过。
- `#` 是普通 Bash 输入。

### Phase 4：HTTP、WebSocket 与单实例同步

1. 实现 API allowlist、auth middleware、strict same-origin 和 body limit。
2. heartbeat 只接收 resize，只返回 sessions、system、runtime。
3. WebSocket 只允许 `/ws/:sessionId`。
4. WebSocket URL 只从当前页面 Origin 派生，并校验 Origin。
5. 实现单实例 Cloudflare Access 重新认证检测。
6. 实现 boot ID 和运行时替换检测。
7. 明确拒绝 `/api/cluster` 和跨 Origin browser requests。

Gate：API contract、WS auth、same-origin、heartbeat 和 reconnect tests 通过；
所有排除 API/namespace 均返回 404 或拒绝升级，不存在 Host registry 数据。

### Phase 5：模块化终端 UI

1. 按第 4.5 节建立模块，不创建等价的新单文件控制器。
2. 实现 terminal、preview、tabs、search、auth、status、toast 和 touch。
3. 保留无 session 时自动创建和关闭最后 session 后补建。
4. 删除所有文件/workspace/Agent UI 和快捷键。
5. 实现桌面、平板、手机竖屏和横屏响应式行为。
6. 对照参考截图和交互 fixtures 做功能回归。

Gate：Chrome E2E 通过；控制台无 error、404、未处理 Promise 或空 DOM 引用；
所有视口无重叠、横向溢出或不可点击控件。React Strict Mode 的
mount/cleanup/remount 测试确认不会重复创建 WebSocket、xterm、listener、
observer 或 timer。

### Phase 6：本地资源与缓存

1. 固定 frontend 与 terminal worker 的所有 npm 版本并生成独立 lockfile。
2. Vite 构建内容哈希资源，字体、xterm 和图标全部进入 dist。
3. Go embed 完整 dist，设置第 4.8 节 cache headers。
4. 确认不存在 manifest、Service Worker、CDN URL 或公网请求。
5. 验证 Go backend 重启和版本升级后不会混用旧 CSS/JS。

Gate：断开容器外网后页面仍完整加载；核心终端在连接本容器时可用；Chrome
中没有 Service Worker registration。

### Phase 7：容器与 Kubernetes Deployment

1. 编写 multi-stage Dockerfile 并固定 image digest。
2. Runtime 仅包含 Go binary、Node runtime、terminal worker 及 production
   dependencies、Bash、tini、CA 和必要系统文件；删除 npm/npx/corepack。
3. 以非 root 用户启动并实现 healthcheck。
4. 编写 compose，挂 state/workspace volume 并注入配置。
5. 编写第 4.9 节列出的普通 Kubernetes YAML。
6. Deployment 固定 `replicas: 1` 和 `strategy: Recreate`。
7. 验证 worker handshake/lifecycle、SIGTERM、PTY cleanup、PVC restore 和
   重新部署行为。

Gate：镜像构建、非 root 启动、登录、PTY、WebSocket、worker IPC、
healthcheck、stop 和 restart restore 全部通过；YAML 通过 server-side dry-run
或 schema 验证。

### Phase 8：最终验证与文档

1. 执行全部 Go、terminal worker、frontend、Chrome、container 测试。
2. 对照范围逐项验证，并扫描所有排除功能和旧命名空间。
3. 更新 README、API、配置、安全、部署、备份恢复和故障排查文档。
4. 记录所有与参考行为的差异；未批准差异必须修复。
5. 检查无 secret、临时文件、构建缓存或无关参考文件。

Gate：第 11 节 Definition of Done 全部满足。

## 10. 自动化测试矩阵

### 10.1 Go 单元测试

- 配置优先级、全部精确边界、显式空密码、capacity、端口冲突和随机密码。
- Challenge 单次使用/过期、token rotation/expiry/revoke 和 lockout。
- Password fingerprint 域分隔、密码修改和随机密码重启会撤销旧 refresh
  sessions，磁盘上不出现可直接用于 challenge proof 的 `passwordKey`。
- 可配置 TTL/attempts 对新 token 的生效边界。
- WebSocket subprotocol 解析且不回显 token。
- Session cwd、resize、history、execution、snapshot 和 deletion。
- Session/client capacity 的 `409`/`429`、execution command/input 64 KiB +
  output 960 KiB 截断、
  4 MiB slow-client queue 和 WebSocket `1013 slow_client`。
- OSC/title/cwd/marker 拆包和内部噪声过滤。
- Query owner claim 和 owner disconnect。
- Headless worker 的 `onData`/`onBinary` response 不写入 PTY，非 owner browser
  query response 被丢弃。
- Worker frame codec、request correlation、per-session sequence 和
  snapshot `throughSequence` barrier。
- Snapshot dirty/coalesce/max-delay/flush/corruption quarantine。
- Versioned session/auth JSON 和 snapshot envelope magic/length/SHA-256 验证。
- Snapshot 写入失败和恢复会设置/清除 persistence degraded 状态。
- Persistence 权限和原子替换失败恢复。
- 所有并发核心执行 `go test -race ./...`。

### 10.2 Terminal worker 测试

- `hello` version、create/restore/write/resize/snapshot/close/shutdown contract。
- Restore 使用非零 `throughSequence` 后，下一 mutation 从基线加一继续。
- 任意 PTY byte chunk boundary 经 raw payload 传输后不损坏 UTF-8 和 ANSI。
- xterm.js write callback 完成前 snapshot 不得越过 barrier。
- 重复、跳号、倒序 sequence，超限 frame 和未知 operation 明确失败。
- 宽字符、组合字符、颜色、alternate buffer、scrollback、cursor 和 resize
  golden fixtures 与参考结果一致。
- `?1005`、`?1006`、`?1015` set/reset 在 snapshot round-trip 后一致。
- 多 session 交错不破坏各自顺序；关闭一个 session 不影响其他 session。
- worker stdout 不含非 protocol bytes，stderr 不包含 PTY output 或 snapshot。

### 10.3 服务集成测试

- 公开 health/version；其他 API 未认证返回 401。
- 完整 login + HTTP + terminal WebSocket flow。
- 所有 HTTP named schema、strict unknown-field rejection、status code 和 1 MiB
  request limit。
- Heartbeat 权威清单和 resize。
- Refresh rotation 后旧 token 失效。
- Process restart 后 auth 和 terminal materials 恢复。
- Worker 缺失或 handshake 版本不匹配时启动失败；服务就绪后 kill worker
  会使 health 变为 503、Go 非零退出，容器重启后按 `DEC-001` 恢复材料。
- 后端不自动建 session，前端负责创建。
- API/WS 拒绝跨 Origin browser requests。
- `/api/cluster` 返回 404。
- 禁止 API/WS 返回 404/拒绝。
- 所有静态资源从当前 Origin 返回。

### 10.4 前端模块测试

- 单实例 auth storage 的 load/refresh/logout。
- Session reconciliation、active fallback 和 reconnect throttle。
- Terminal protocol message parsing。
- Command completion attention/notification 去重。
- Touch modifier 和 soft keyboard state machine。
- Viewport layout mode 切换无振荡。
- Terminal viewport detach/reattach 不销毁 runtime，显式 close 才完整 dispose。
- React Strict Mode setup/cleanup/remount 后资源计数保持稳定。
- 模块依赖规则测试，禁止 UI 直接网络访问和 network 直接 DOM 访问。

### 10.5 Chrome E2E

使用 Playwright Chrome channel：

| Viewport | Coverage |
| --- | --- |
| 1440x900 | desktop sidebar、preview、terminal、search、auth |
| 1024x768 | tablet landscape、soft keyboard、resize |
| 768x1024 | tablet portrait、sidebar overlay、switching |
| 390x844 | phone portrait、virtual keys、search、modal |
| 844x390 | phone landscape、visualViewport、terminal resize |

每个视口保存 screenshot 并断言：

- 页面和 terminal canvas 非空。
- 文本和控件不越界、不重叠。
- 软键盘不引起布局振荡。
- 创建、切换、关闭后的焦点正确。
- Login、auth sessions 和 search 可完整操作。
- WebSocket 断开后自动恢复。
- 不存在文件/Agent/PWA UI 或请求。
- `navigator.serviceWorker.getRegistrations()` 在测试 origin 下为空。
- 应用启动和核心操作不请求当前 Origin 之外的资源。

### 10.6 Container/Kubernetes

- 镜像无 floating `latest`，build stages 和 runtime digest 固定。
- Runtime 包含固定 Node runtime 和 worker production dependencies，但没有
  npm/npx/corepack/compiler/cloudflared/Agent CLI。
- 非 root，state/workspace 可写，其他权限符合预期。
- Worker 不监听网络端口，浏览器不能直接访问；worker 缺失时容器启动失败。
- Kill worker 后整个服务非零退出，restart policy 拉起新实例并恢复 session
  metadata、snapshot 和新的 Bash。
- SIGTERM 终止 Bash process group 并在 grace period 内退出。
- Port 9846 被占用时明确失败。
- Healthcheck 和 Secret/config injection 有效。
- Resources、probe timings、10s shutdown 和 30s grace 与第 4.9 节完全一致。
- Deployment 是单副本 Recreate，PVC 为 RWO。
- 普通 YAML 通过 schema 和 dry-run。

## 11. Definition of Done

- [ ] 所有决策均为 `已确认`，正文无待定实现分支。
- [ ] Go backend 和模块化 Web frontend 架构完成。
- [ ] 独立 Node terminal worker、framed IPC 和 xterm.js snapshot contract 完成。
- [ ] 用户可在 Chrome 完成登录和单实例多 Terminal Tab 核心流程。
- [ ] 不存在 Host model、Host management UI、cluster API 或 `cluster.json`。
- [ ] Refresh/reconnect/restart 语义符合第 3 节。
- [ ] 桌面、平板、手机 Chrome E2E 全部通过。
- [ ] 文件工作区 API/UI/state/dependency 均不存在。
- [ ] ACP、Agent、AI/provider/managed terminal 均不存在。
- [ ] PWA manifest、Service Worker、CDN 和公网静态资源均不存在。
- [ ] 只承诺 Linux container + Bash，无其他平台/shell 分支。
- [ ] 无原生客户端和 host service scripts。
- [ ] 所有运行时命名空间均为 Roaminal。
- [ ] Go race/unit/integration、worker、frontend unit、Chrome、container tests
  通过。
- [ ] OCI image、compose、单副本 Deployment 普通 YAML 可用。
- [ ] API、配置、安全、部署、备份和故障排查文档完整。
- [ ] 实施日志包含测试证据和全部批准差异。
- [ ] Git commits 和 commit messages 符合 `DEC-018`。

## 12. 决策记录

### 12.1 已确认

| ID | 结论 | 状态 |
| --- | --- | --- |
| `DEC-001` | Pod/服务重启只恢复 ID、cwd、标题、尺寸和 scrollback，创建新 Bash；不恢复原进程。 | 已确认 |
| `DEC-002` | 不包含任何 AI，包括 terminal-native `#` assistant 和 auto-fix。 | 已确认 |
| `DEC-003` | 不提供 PWA 安装；经重新评估也不保留 Service Worker，使用本地哈希资源、cache headers 和 boot ID。 | 已确认 |
| `DEC-004` | 保留系统监控、延迟图、attention、Chrome 通知和 toast。 | 已确认 |
| `DEC-005` | 只保留当前同源实例的 Cloudflare Access 重新认证兼容；删除多 Host Access 流程、Tunnel 和 cloudflared。 | 已确认 |
| `DEC-006` | 保留桌面 sidebar 的实时终端缩略预览。 | 已确认 |
| `DEC-007` | 所有前端资源从镜像本地加载，运行时不访问 CDN。 | 已确认 |
| `DEC-008` | 终端只运行在容器内，只访问显式 volume；不挂 Docker socket 或宿主 namespace。 | 已确认 |
| `DEC-009` | Kubernetes 使用单副本 Deployment，`strategy: Recreate`，不使用 StatefulSet/HPA。 | 已确认 |
| `DEC-010` | 不兼容 Tabminal 配置、数据、token、协议或 localStorage。 | 已确认 |
| `DEC-011` | 固定端口 9846，冲突时非零退出，不自动递增。 | 已确认 |
| `DEC-012` | 由于 MVP 只连接当前实例，API/WS 严格同源，不再提供 `ROAMINAL_ALLOWED_ORIGINS`。 | 已确认 |
| `DEC-013` | 只承诺 Linux 容器和 Bash，不考虑其他 OS/shell。 | 已确认 |
| `DEC-014` | 默认 15m access、90d refresh、30 次锁定和 accept-terms；这些策略支持配置，重启生效。 | 已确认 |
| `DEC-015` | Kubernetes MVP 只交付普通 YAML，不提供 Helm/Kustomize。 | 已确认 |
| `DEC-016` | 重构参考前端单文件架构；只做功能一比一，不复制代码结构。 | 已确认 |
| `DEC-017` | 保持终端 UI 布局和交互，替换品牌；删除功能后的空白不做像素级保持。 | 已确认 |
| `DEC-018` | 实施 Agent 可以自行提交。每个 commit 只包含一个连贯变更，message 只能客观描述本提交内容，不包含阶段状态、Agent/AI、计划进度、宣传语、无关 issue 或其他元信息。 | 已确认 |
| `DEC-019` | 接受 `github.com/gitpod-io/xterm-go` 当前实现成熟度，但它改为 MVP 后的对比候选，不是 MVP runtime dependency。候选固定 commit `8e117204ebedc133bf33ee9eb759c8484f843cee`；发现不足时可在对比完成后输出开源贡献提案。 | 已确认 |
| `DEC-020` | 前端使用 React 19 + TypeScript 7 + Vite 8；Vitest 做模块测试，Playwright Chrome 做 E2E，Go embed 打包 dist。采用 React/runtime 边界，不使用第三方 xterm wrapper、UI component library 或独立状态管理库。 | 已确认 |
| `DEC-021` | 删除参考实现的多 Host 能力，只支持页面来源对应的一个 Roaminal 实例及其多个 Terminal Tab。删除 Host model/UI、cluster API/persistence、Host-scoped auth、跨 Origin 子 Host 和相关测试。 | 已确认 |
| `DEC-022` | 不把 server-side headless emulator 的 `onData`/`onBinary` response 写回 PTY；由当前 browser xterm.js client 回答 terminal query，多 client attach 时保留 query-response owner/claim。 | 已确认 |
| `DEC-023` | 使用 atomic snapshot checkpoint，不做 output WAL；dirty 后 250ms 合并写，持续 output 最长 1s checkpoint，SIGTERM 强制 flush；存储正常时，异常终止最多丢失最后 1s 视觉 output。 | 已确认 |
| `DEC-024` | 删除 `historyLimit`，改用 `scrollbackLines` / `--scrollback-lines` / `ROAMINAL_SCROLLBACK_LINES`，默认 `1000`，范围 `0..50000`；server shadow 和 browser main 使用同一值，preview 固定 `0`。 | 已确认 |
| `DEC-025` | Terminal emulator 拆为同一容器内由 Go backend 监督的独立 Node.js 子进程；通过 stdio IPC，不做 sidecar、Deployment、Service 或远程微服务。Go 仍是唯一网络 backend，并继续拥有 PTY、session 和 persistence。 | 已确认 |
| `DEC-026` | MVP 使用参考实现已经验证的官方 `xterm-headless 5.3.0` + `xterm-addon-serialize 0.11.0`；不提供双引擎或 fallback。以 engine-neutral worker contract 保留 MVP 后对比 `xterm-go` 和迁移 scoped xterm package 的能力。 | 已确认 |
| `DEC-027` | 已就绪服务的 terminal worker 异常退出、持续超时或发生 fatal protocol error 时，整个 Go backend fail-fast 并由容器重启；以后具备 sequence ACK + bounded replay 机制时再设计 worker 热重启。 | 已确认 |
| `DEC-028` | Capacity 默认并允许配置为最多 32 sessions、每 session 8 clients；合法范围分别为 `1..256` 和 `1..64`。Execution 最近 100 条，client live queue 4 MiB，worker queue 16 MiB。 | 已确认 |
| `DEC-029` | Kubernetes 默认 resources 为 requests `100m/256Mi`、limits `2 CPU/2Gi`；应用 shutdown 10s、Pod grace 30s、worker handshake 5s、worker control 30s、worker stall 10s。 | 已确认 |

允许的 commit message 示例：

```text
feat(auth): implement challenge token rotation
feat(terminal): persist headless terminal snapshots
test(terminal): cover reconnect snapshot ordering
docs(deploy): document websocket proxy settings
```

禁止的 message 示例：

```text
phase 2 complete
implement MVP with agent
misc updates
fix stuff
generated by AI
```

### 12.2 已确认：前端工具链

本项目使用 React。准确地说，React 是 UI library，仍由 Vite 负责开发
和构建；它不会改变 Roaminal 是 Go 服务端托管静态 Web App 的部署模型。

项目级比较：

| 维度 | React + TypeScript + Vite | 原生 DOM + TypeScript + Vite |
| --- | --- | --- |
| xterm.js 集成 | 需要 ref/effect adapter；生命周期约束更严格 | 命令式 API 直接匹配，首期接入更简单 |
| PTY 高频输出 | runtime 直接 `terminal.write()` 时无额外 render；误放入 React state 会造成无意义重渲染 | 天然直接写 xterm，无 framework render 风险 |
| 多 Session UI | 组件、单向数据流和可预测重渲染更易维护 | 需要手工同步 DOM、事件和状态，边界更依赖纪律 |
| 响应式交互 | Sidebar、modal、touch bar、search 和状态组合更容易拆分测试 | 首屏依赖少，但复杂交互增长后容易形成 controller 和 DOM patch 代码 |
| xterm 生命周期风险 | 开发模式会额外检验 setup/cleanup；错误实现可能重复 socket/listener/terminal | 没有 React 重挂载，但仍需手工完成所有 cleanup |
| 包与产物 | 新增 React runtime、Vite plugin 和类型依赖，全部编译或固定在镜像构建中 | 依赖和 bundle 更小 |
| MVP 初始开发量 | 要先建立 runtime/React 边界，略高 | 最短路径更短 |
| 后续文件工作区 | 更适合以后增加复杂 workspace UI，避免届时再次迁移视图层 | 大规模扩展时可能需要自建组件/状态机制或再迁移 framework |
| 行为一比一 | 可以达到；样式继续使用项目 CSS，不使用组件库 | 可以达到；与参考实现的命令式 DOM 思路更接近 |

最终选择 **React 19 + TypeScript 7 + Vite 8**。现有范围包含多 terminal、
desktop preview、认证 modal、状态图和移动端触控布局，且文件工作区在 MVP
后有大量改动计划；现在建立清晰的 React/runtime 边界，避免完成 MVP 后再
迁移视图层。具体 lifecycle 和依赖约束已固定在第 4.5、6.3、9、10 节。

### 12.3 已确认：独立 terminal worker 与后续 engine 对比

独立进程让 Go backend 不需要链接 JavaScript terminal emulator，也不必因为
MVP 选择官方 xterm.js 而改回 Node backend。边界固定为：

```text
Go backend
  owns network + auth + PTY + session ordering + persistence
        |
        | roaminal-terminal-worker/1 over framed stdio
        v
Node worker (MVP)
  owns xterm-headless state + SerializeAddon only
```

这不是微服务边界。MVP 只有一个 image、一个 container、一个 Deployment、
一个对外端口和一个 health model；Node worker 没有网络身份或独立发布单元。
代价是 runtime 必须包含 Node，PTY output 经过一次本地 IPC，并需要 sequence、
barrier、backpressure 和联合 shutdown。收益是 Go backend 的领域边界保持
稳定，且 emulator 可以在不改 HTTP/PTY/persistence contract 的情况下替换。

MVP 后对比 `xterm-go` 时，不在 production binary 中加入动态开关。另建一个
实现相同 protocol 的 Go worker executable，让两个实现分别通过完全相同的
conformance fixtures；只有 correctness 达到同一门槛后，才比较 throughput、
latency、CPU、RSS 和 snapshot 成本。结果必须同时包含 emulator-only 与 IPC
end-to-end 两组数据，因为 IPC 或序列化成本可能掩盖或放大 engine 差异。
因此“`xterm-go` 应有更强性能”保留为待验证假设，不作为选型事实。

### 12.4 已确认：worker 运行时故障策略

Worker 在 backend 运行期间异常退出、protocol corruption 或持续超时时，MVP
采用整服务 fail-fast。初始启动 handshake 失败时，Go 同样直接非零退出。
与未采用的热重启路线对比如下：

| 维度 | 整服务 fail-fast | Worker 热重启 + replay |
| --- | --- | --- |
| 行为 | Go 停止服务、终止 PTY、非零退出，由容器策略重启并按 `DEC-001` 恢复新 Bash | Go 保留 PTY，启动新 worker，从 checkpoint 恢复并 replay 缺口 output |
| 额外状态 | 无；使用现有 <=1s checkpoint contract | 需要每 session sequence ACK/cutoff、bounded replay buffer 和重建 barrier |
| 一致性 | 与整个 Go/Pod 异常退出使用同一恢复语义 | 可以保住运行中的 Bash，但恢复窗口内 attach/snapshot/resize 语义更复杂 |
| 可用性 | worker 故障会中断全部 session | 故障隔离更好，PTY 可继续运行 |
| MVP 实施风险 | 低，容易做 failure injection 和确定性验证 | 中高，新机制本身成为高频路径和内存压力来源 |

整服务 fail-fast 与“先控制复杂度、后续引入新机制”一致，也避免在没有
durable output WAL 的前提下伪装成无损热恢复。具体行为固定为：`/healthz`
立即变为 503，停止接收新连接/input，尽力使用最后一个成功 checkpoint，
终止 PTY，清理 worker 并以非零状态退出；Docker/Kubernetes restart policy
拉起新实例。不得在同一 Go 进程中盲目重启 worker 或继续广播无法进入
shadow state 的 output。

## 13. 实施 Agent 自治规则

文档批准后，实施 Agent 必须遵守：

1. 直接执行已确认决策，不重新发起相同选择。
2. 不增加文件工作区、Agent、AI、PWA、原生客户端或 Kubernetes HA。
3. 参考代码只用于理解和行为对照，不直接复制实现。
4. 每个 Phase 完成后运行 gate，不把验证留到最后。
5. 每个 commit 是单一连贯变更，message 严格遵守 `DEC-018`。
6. 失败时先自行定位修复；只有下列情况才暂停：
   - 参考 commit 不匹配；
   - 已确认决策互相矛盾；
   - 必须扩大容器权限或安全边界；
   - 无法保留用户已有修改；
   - 外部凭据、域名、registry 或 Kubernetes 访问成为硬性前提；
   - 固定依赖无法取得，或按第 6.2 节尝试有界 worker adapter 后仍不能满足
     本文 contract tests。
7. 不需要真实集群或 registry 即可完成 image test 和 YAML dry-run；缺少发布
   权限不阻塞代码交付。
8. 不把未运行测试写成通过；所有未运行项记录在 implementation log。
9. 最终报告只包含结果、验证证据、批准差异和真实残余风险。

## 14. 已批准的实施启动

本计划已完成 scope、API、worker、持久化、phases、tests 和 Definition of
Done 一致性复核。交给实施 Agent 的唯一启动指令为：

```text
严格按 docs/plan/mvp/README.md 实施 Roaminal MVP，从 Phase 0 开始，
连续执行到所有 Definition of Done 满足。按文档自行创建原子 commits；
除文档列出的停止条件外，不要等待人工确认。
```
