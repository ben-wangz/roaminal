# 03 - 持久终端与 Worker

> 状态：Approved
> 上位文档：[MVP 计划索引](../README.md)

## Session 与 PTY

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
- shell execution marker 用于运行状态、完成状态和通知。
- 服务端 shadow terminal 与浏览器 main terminal 使用相同的
  `scrollbackLines`；浏览器 preview terminal 固定 `scrollback: 0`。
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

Go backend 启动一个独立 Node.js `terminal-worker` 子进程。Worker 固定使用
官方 `xterm-headless 5.3.0` 和 `xterm-addon-serialize 0.11.0`。这里的“原生
xterm”指 xterm.js 官方 JavaScript headless 实现，不是系统 `xterm` 程序或
自行编写的 terminal emulator。版本和替换规则见
[06-architecture-dependencies.md](./06-architecture-dependencies.md)。

## Backend 与 Worker 职责

每个持久 session 对应一个 Go session 和一个 worker 内的 headless xterm
实例及 `SerializeAddon`。Worker 中的实例是 PTY 的“影子终端”，不是浏览器
renderer。一个 worker 承载当前 backend 的所有 session。

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

Go backend 独占：

1. 监听 HTTP/WebSocket、认证用户并校验 same-origin。
2. 创建和管理 Linux PTY、Bash process group、session event loop 与浏览器
   client queues。
3. 过滤 private marker，解析 cwd/execution metadata，广播 live output，并
   处理 terminal-query response owner/claim。
4. 决定 checkpoint 时机，执行 snapshot 原子持久化和损坏隔离。
5. 启动、监督和关闭唯一的 terminal worker；浏览器和集群网络不能直接访问
   worker。

Terminal worker 只承担：

1. 顺序解析清理后的 PTY ANSI/VT output，维护 normal/alternate buffer、
   scrollback、cursor、cell attributes、terminal modes 和 resize/reflow 状态。
2. 在浏览器 attach/reconnect 时，把当前状态序列化成可由浏览器 xterm.js
   重放的 ANSI snapshot。
3. 为服务重启保存可恢复的视觉 terminal state；先把持久化 snapshot 写入新
   shadow terminal，再启动替代 Bash 并继续追加新 output。
4. 为 contract tests 暴露 buffer、cursor、mode 和文本状态。
5. 跟踪参考实现补充的 `?1005`、`?1006`、`?1015` mouse encoding mode，并把
   启用状态追加到 serialized snapshot。

集成约束：

- Go 为每个 session 的 `write` 和 `resize` mutation 分配从 1 开始严格递增的
  `sequence`；worker 按 session 串行执行，重复、跳号或倒序是 protocol error。
- Worker 对每个 session 使用 Promise chain；`write` 必须等待 xterm.js write
  callback 后才算完成。`snapshot` 只有在此前 mutation 全部完成后才能返回，
  并携带 `throughSequence`。
- Roaminal private shell markers 在写入 shadow terminal 和广播前移除；普通
  ANSI/OSC/DCS 必须保留。
- Go 对 PTY output 使用每 session streaming UTF-8 decoder；不完整 code point
  留到下一 chunk，非法序列统一替换为 U+FFFD。Worker payload 和 browser
  `output.data` 必须来自同一份 decoded stream。
- OSC 7 cwd 和 Roaminal execution markers 由 Go streaming parser 处理。
- attach 建立 serialization barrier；barrier 后到达的 output 先排队，直到
  worker 返回相同 `throughSequence`，且 `snapshot -> meta -> status` 发送完毕，
  再按原顺序发送，不能丢失或重复。
- Worker headless xterm 和 browser xterm.js main terminal 使用同一个
  `scrollbackLines` 值。
- output 或 resize 后把 snapshot 标为 dirty；首次 dirty 后等待 250ms 合并
  写入，但持续 output 不得把 checkpoint 推迟超过 1s。
- Snapshot 以 ANSI replay bytes、`0600` 权限和 temp + fsync + rename + parent
  directory fsync 原子替换持久化；SIGTERM 在 session output drain 后强制
  flush 所有 dirty snapshots。MVP 不实现 output WAL；存储正常时，SIGKILL
  或节点掉电最多可能丢失最后 1s 尚未 checkpoint 的视觉 output。持续存储
  写入失败不承诺该上限，必须记录错误并暴露 persistence degraded 状态。
- Snapshot 损坏时把文件重命名为带 UTC timestamp 的 `.corrupt` 文件，使用
  保留的 metadata 和空 scrollback 启动替代 Bash；不得影响其他 sessions 或
  整个服务启动。

## Worker 进程与 IPC Contract

Worker 是同一容器内由 Go 直接监督的子进程，不是 sidecar、独立 Deployment
或微服务：

```text
tini (PID 1)
  `- roaminal (Go backend, only network listener)
       `- node /opt/roaminal/terminal-worker/src/index.mjs
```

- Go 使用 worker stdin/stdout 做全双工 IPC；worker 不监听 TCP 或 Unix socket，
  不加入 Kubernetes Service，也不需要独立鉴权或 TLS。
- Production worker command/path 是镜像内固定实现，不增加 CLI、环境变量或
  JSON 配置；测试可通过 Go 内部 dependency injection 替换 fake worker。
- Go 在 Linux 上为子进程设置 parent-death signal，不能遗留 orphan worker。
- stderr 只用于 diagnostic log；stdout 只能写 protocol frame。
- Frame 先写 `uint32-be headerLength + uint32-be payloadLength`，随后写 JSON
  header 和 raw payload。Header 上限 64 KiB，payload 上限 256 MiB；越界、
  非法 JSON、未知 protocol version 或 operation 都是 fatal protocol error。
- PTY output 以 raw payload `Uint8Array` 交给 xterm.js，snapshot 以 UTF-8 bytes
  返回。
- 协议名固定为 `roaminal-terminal-worker/1`。Operation 只包含 `hello`、
  `create`、`restore`、`write`、`resize`、`snapshot`、`close`、`shutdown` 和
  对应 `ready`/`result`/`error`。
- 需要 response 的 control request 使用不可复用的 `requestId`；不等待
  response 的 `write`/`resize` 不带 `requestId`，但必须带 `sessionId` 和
  `sequence`。多 session 可以交错，同一 session 必须严格有序。
- `write` 不做逐条 response。Go 到 worker 的 writer queue 固定为 16 MiB raw
  payload budget，不能丢弃 mutation；队列满时施加 backpressure，连续 10 秒
  无 forward progress 按 fatal worker timeout 处理。
- Worker `hello`/`ready` deadline 为 5 秒；create、restore、snapshot 和 close
  deadline 为 30 秒；超时均进入 `DEC-027` fail-fast。
- Go 完成 `hello`/`ready` 和 version 校验后才开放 9846。Worker 缺失、版本
  不匹配或初始化失败时，Go 明确报错并非零退出。
- 服务就绪后 worker 异常退出、持续超时或发生 fatal protocol error 时，Go
  停止整个服务并非零退出，不原地重启 worker。
- Go 是 snapshot 文件、session metadata 和 checkpoint scheduler 的唯一 owner；
  worker 不读写 state volume。
- 正常 shutdown 顺序：停止新连接和 input，终止 PTY 并 drain output，写入
  dirty snapshots，发送 worker `shutdown`，等待退出；全部共享应用 10 秒总
  deadline，超时强制终止子进程。

Header 字段使用 lower camel case；`requestId` 和 `sessionId` 是 UUID string，
`sequence`/`throughSequence` 是无前导零的十进制 string。`ready`/`result`
必须 echo 对应 control request 的 `requestId`。

| Direction | `op` | Header fields | Raw payload | Response |
| --- | --- | --- | --- | --- |
| Go -> worker | `hello` | `requestId`, `protocol` | empty | `ready`，包含 `protocol`, `engine`, `engineVersion`, `serializeAddonVersion` |
| Go -> worker | `create` | `requestId`, `sessionId`, `cols`, `rows`, `scrollbackLines` | empty | `result` with `requestOp: "create"`, `throughSequence: "0"` |
| Go -> worker | `restore` | `requestId`, `sessionId`, `cols`, `rows`, `scrollbackLines`, `throughSequence` | UTF-8 ANSI snapshot | `result` with `requestOp: "restore"` and matching `throughSequence`；等待 write callback |
| Go -> worker | `write` | `sessionId`, `sequence` | UTF-8 PTY output，单 frame 最大 256 KiB | none |
| Go -> worker | `resize` | `sessionId`, `sequence`, `cols`, `rows` | empty | none |
| Go -> worker | `snapshot` | `requestId`, `sessionId`, `throughSequence` | empty | `result` with `requestOp: "snapshot"`, matching `throughSequence` and UTF-8 snapshot payload |
| Go -> worker | `close` | `requestId`, `sessionId` | empty | `result` with `requestOp: "close"` |
| Go -> worker | `shutdown` | `requestId` | empty | `result` with `requestOp: "shutdown"`，随后正常退出 0 |
| worker -> Go | `error` | `requestId?`, `sessionId?`, `code`, `message`, `fatal: true` | empty | Go 执行 `DEC-027` |

Worker error code allowlist 为 `protocol_version`、`invalid_frame`、
`unknown_operation`、`duplicate_session`、`unknown_session`、
`sequence_mismatch` 和 `engine_failure`。Go 本地产生的 `worker_timeout`、
`worker_exit`、`worker_io` 进入同一 fatal path。Header 缺少必填字段、出现未知
字段或 payload 与 operation 不匹配都属于 `invalid_frame`。新增 operation 或
字段必须先升级 protocol version。

`create` 后首个 mutation sequence 是 `"1"`。`restore` 的尺寸、scrollback 和
`throughSequence` 来自已验证的 snapshot envelope；worker 以该 sequence 作为
恢复基线，下一个 mutation 必须是 `throughSequence + 1`。Go 收到 restore
result 后，如 session metadata 当前尺寸不同，再以下一个 sequence 发送
`resize`。

Worker 不承担：

- 创建 PTY，启动、终止或恢复 Bash/Unix process。
- DOM/canvas 渲染、字体、selection、search、links 或 touch input。
- HTTP、WebSocket、认证、浏览器 client 或 active-session UI 管理。
- Roaminal private marker 解析及 cwd、execution history、attention、toast、
  notification 管理。
- snapshot 路径、写入策略、持久化频率或损坏恢复策略。
- 审计日志或命令历史。
- 转发 headless xterm 的 `onData`/`onBinary` 到 PTY。当前 browser xterm.js
  client 负责 terminal query response；query-response owner/claim 保证只接受
  一个 client 的 response。

## 持久化文件 Contract

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
  "formatVersion": 2,
  "id": "uuid",
  "automaticTitle": "string",
  "titleOverride": "string or null",
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

只持久化已完成的 execution：`command`、`input`、`output` 始终为 string，
`exitCode` 为 integer 或 JSON `null`，两个 timestamp 均为 RFC3339 UTC，
`durationMs` 为非负 integer，`truncated` 为 boolean。Executions 只保留最近
100 条并应用本文件开头的 byte limits。Session file 不保存 `shell`、
`managed`、environment dump、workspace/editor state、running process PID 或
running execution；这些字段出现时视为 schema error。

Snapshot file 是自校验 envelope：第一行固定 ASCII magic
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

Restore 时验证 magic、header schema、byte length、SHA-256、UTF-8 和 256 MiB
上限；使用 header cols/rows 创建 headless terminal，写入 snapshot 并等待
callback，再 resize 到 metadata 当前尺寸。验证失败时把 snapshot 原子重命名为
`<sessionId>.snapshot.corrupt.<YYYYMMDDTHHMMSS.nnnnnnnnnZ>`，使用 metadata 和空
scrollback 创建新 Bash；不得隔离或删除 metadata。

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
`formatVersion`、未知字段、类型错误或 hash/UUID/timestamp 格式错误不做
best-effort 兼容：单个 session metadata 损坏时隔离该 session 的 metadata 和
snapshot；auth file 损坏时隔离整个 auth file 并以空 refresh session store
启动。隔离日志不得包含 token、terminal payload 或环境变量。

State directory 固定 `0700`，JSON/snapshot/temp/corrupt files 固定 `0600`。
每次写入使用同目录 temp file、file fsync、rename 和 parent directory fsync；
不得依赖跨目录 rename。Session `formatVersion: 2`、auth `formatVersion: 1` 只定义 Roaminal 自身未来迁移边界，
不兼容 Tabminal 数据。
