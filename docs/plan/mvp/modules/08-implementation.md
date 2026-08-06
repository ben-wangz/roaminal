# 08 - 实施、测试与验收

> 状态：Approved
> 上位文档：[MVP 计划索引](../README.md)

实施 Agent 必须按顺序执行；每个 Phase 的 gate 通过后提交该阶段的原子 commit，
再进入下一阶段。

## Phase 0：冻结行为基线

1. 确认参考仓库 HEAD 为指定 commit；不匹配时停止。
2. 记录 Roaminal 开始前工作树状态，保留用户修改。
3. 建立 `docs/plan/mvp/implementation-log.md`。
4. 从参考实现提取 MVP 行为清单、API/WS fixtures、关键视口截图和 headless
   xterm input/snapshot fixtures，不复制实现代码。
5. 创建 `THIRD_PARTY_NOTICES.md`，记录行为参考和依赖许可证。

Gate：行为基线、来源版本、许可证和工作树均可审计。

## Phase 1：工程骨架

1. 创建 Go module、`cmd/roaminal` 和 `internal/*` 包。
2. 创建 React/TypeScript/Vite 模块化前端。
3. 创建 ESM `terminal-worker` package、framed IPC codec 和 Node test skeleton。
4. 创建统一 `Makefile`：`build`、`test`、`lint`、`dev`、`container`。
5. 配置 Go formatting/vet/static analysis、worker tests 和 frontend
   lint/typecheck。
6. 建立 Go embed 流程：先生成 `web/dist`，再编译 binary。
7. 建立 Roaminal favicon；不创建 manifest 或 PWA icons。

Gate：

- `go test ./...`、`go vet ./...` 可执行。
- frontend 与 terminal-worker 的 `npm ci`、lint/test 可执行。
- production build 生成 Go binary 和 worker artifact；Go 能从 embed 返回页面
  和本地资源，并完成 worker handshake。
- 页面静态资源请求全部指向当前 Origin。

## Phase 2：配置、认证与持久化

1. 实现完整配置优先级、严格验证和 auth policy 可配置项。
2. 实现 challenge、login、refresh、logout、session list/revoke。
3. 实现 access/refresh token rotation 和锁定。
4. 实现 session 和 auth session typed schemas。
5. 所有写入采用 temp file + fsync + rename，权限符合敏感性要求。
6. 前端使用 Web Crypto 实现 challenge response，使用单实例 auth storage。

Gate：认证和配置 tests 全部通过；不存在旧命名空间或排除数据字段。

## Phase 3：PTY 与终端快照

1. 实现 Bash PTY spawn、process group、resize、input/output 和 shutdown。
2. 实现 session event loop、client queues 和 query owner。
3. 实现 Bash hooks、title/cwd/execution parser 和 private marker 过滤。
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

## Phase 4：HTTP、WebSocket 与单实例同步

1. 实现 API allowlist、auth middleware、strict same-origin 和 body limit。
2. heartbeat 只接收 resize，只返回 sessions、system、runtime。
3. WebSocket 只允许 `/ws/:sessionId`。
4. WebSocket URL 只从当前页面 Origin 派生，并校验 Origin。
5. 实现单实例 Cloudflare Access 重新认证检测。
6. 实现 boot ID 和运行时替换检测。
7. 明确拒绝 `/api/cluster` 和跨 Origin browser requests。

Gate：API contract、WS auth、same-origin、heartbeat 和 reconnect tests 通过；
排除 API/namespace 均返回 404 或拒绝升级，不存在 Host registry 数据。

## Phase 5：模块化终端 UI

1. 按 [05-frontend.md](./05-frontend.md) 建立模块，不创建等价的单文件控制器。
2. 实现 terminal、preview、tabs、search、auth、status、toast 和 touch。
3. 保留无 session 时自动创建和关闭最后 session 后补建。
4. 删除所有文件/workspace/Agent UI 和快捷键。
5. 实现桌面、平板、手机竖屏和横屏响应式行为。
6. 对照参考截图和交互 fixtures 做功能回归。

Gate：Chrome E2E 通过；控制台无 error、404、未处理 Promise 或空 DOM 引用；
所有视口无重叠、横向溢出或不可点击控件。React Strict Mode 的
mount/cleanup/remount 测试确认不会重复创建 WebSocket、xterm、listener、
observer 或 timer。

## Phase 6：本地资源与缓存

1. 固定 frontend 与 terminal worker 的 npm 版本并生成独立 lockfile。
2. Vite 构建内容哈希资源，字体、xterm 和图标全部进入 dist。
3. Go embed 完整 dist，设置 [05-frontend.md](./05-frontend.md) 的 cache headers。
4. 确认不存在 manifest、Service Worker、CDN URL 或公网请求。
5. 验证 Go backend 重启和版本升级后不会混用旧 CSS/JS。

Gate：断开容器外网后页面仍完整加载；连接本容器时核心终端可用；Chrome 中
没有 Service Worker registration。

## Phase 7：容器与 Kubernetes

1. 编写 multi-stage Dockerfile 并固定 image digest。
2. Runtime 仅包含 Go binary、Node runtime、terminal worker 及 production
   dependencies、Bash、tini、CA 和必要系统文件；删除 npm/npx/corepack。
3. 以非 root 用户启动并实现 healthcheck。
4. 编写 compose，挂 state/workspace volume 并注入配置。
5. 编写 [07-deployment.md](./07-deployment.md) 列出的普通 Kubernetes YAML。
6. Deployment 固定 `replicas: 1` 和 `strategy: Recreate`。
7. 验证 worker handshake/lifecycle、SIGTERM、PTY cleanup、PVC restore 和
   重新部署行为。

Gate：镜像构建、非 root 启动、登录、PTY、WebSocket、worker IPC、
healthcheck、stop 和 restart restore 全部通过；YAML 通过 server-side dry-run
或 schema 验证。

## Phase 8：最终验证与文档

1. 执行全部 Go、terminal worker、frontend、Chrome、container 测试。
2. 对照范围逐项验证，并扫描所有排除功能和旧命名空间。
3. 更新 README、API、配置、安全、部署、备份恢复和故障排查文档。
4. 记录所有与参考行为的差异；未批准差异必须修复。
5. 检查无 secret、临时文件、构建缓存或无关参考文件。

Gate：本文 Definition of Done 全部满足。

## 自动化测试矩阵

### Go 单元测试

- 配置优先级、精确边界、显式空密码、capacity、端口冲突和随机密码。
- Challenge 单次使用/过期、token rotation/expiry/revoke 和 lockout。
- Password fingerprint 域分隔、密码修改和随机密码重启撤销旧 refresh
  sessions；磁盘上不出现可直接用于 proof 的 `passwordKey`。
- 可配置 TTL/attempts 对新 token 的生效边界。
- WebSocket subprotocol 解析且不回显 token。
- Session cwd、resize、history、execution、snapshot 和 deletion。
- Session/client capacity 的 `409`/`429`、execution command/input 64 KiB +
  output 960 KiB 截断、4 MiB slow-client queue 和 `1013 slow_client`。
- OSC/title/cwd/marker 拆包和内部噪声过滤。
- Query owner claim/disconnect；worker `onData`/`onBinary` 不写入 PTY，非 owner
  browser query response 被丢弃。
- Worker frame codec、request correlation、per-session sequence 和 snapshot
  `throughSequence` barrier。
- Snapshot dirty/coalesce/max-delay/flush/corruption quarantine。
- Versioned session/auth JSON 和 snapshot magic/length/SHA-256 验证。
- Persistence degraded 状态、权限和原子替换失败恢复。
- 所有并发核心执行 `go test -race ./...`。

### Terminal Worker 测试

- `hello` version、create/restore/write/resize/snapshot/close/shutdown contract。
- Restore 使用非零 `throughSequence` 后，下一 mutation 从基线加一继续。
- 任意 PTY byte chunk boundary 不损坏 UTF-8 和 ANSI。
- xterm.js write callback 完成前 snapshot 不得越过 barrier。
- 重复、跳号、倒序 sequence，超限 frame 和未知 operation 明确失败。
- 宽字符、组合字符、颜色、alternate buffer、scrollback、cursor 和 resize
  golden fixtures 与参考结果一致。
- `?1005`、`?1006`、`?1015` set/reset 在 snapshot round-trip 后一致。
- 多 session 交错不破坏各自顺序；关闭一个 session 不影响其他 session。
- worker stdout 无非 protocol bytes，stderr 不包含 PTY output 或 snapshot。

### 服务集成测试

- 公开 health/version；其他 API 未认证返回 401。
- 完整 login + HTTP + terminal WebSocket flow。
- 所有 HTTP named schema、strict unknown-field rejection、status code 和 1 MiB
  request limit。
- Heartbeat 权威清单和 resize；refresh rotation 后旧 token 失效。
- Process restart 后 auth 和 terminal materials 恢复。
- Worker 缺失或版本不匹配时启动失败；就绪后 kill worker 会使 health 变为
  503、Go 非零退出，容器重启后恢复材料。
- 后端不自动建 session，前端负责创建。
- API/WS 拒绝跨 Origin browser requests。
- `/api/cluster` 和其他禁止 API/WS 返回 404 或拒绝。
- 所有静态资源从当前 Origin 返回。

### 前端模块测试

- 单实例 auth storage 的 load/refresh/logout。
- Session reconciliation、active fallback 和 reconnect throttle。
- Terminal protocol message parsing。
- Command completion attention/notification 去重。
- Touch modifier、soft keyboard state machine 和 viewport layout 无振荡。
- Terminal viewport detach/reattach 不销毁 runtime，显式 close 才 dispose。
- React Strict Mode setup/cleanup/remount 后资源计数稳定。
- 模块依赖规则禁止 UI 直接网络访问和 network 直接 DOM 访问。

### Chrome E2E

使用 Playwright Chrome channel：

| Viewport | Coverage |
| --- | --- |
| 1440x900 | desktop sidebar、preview、terminal、search、auth |
| 1024x768 | tablet landscape、soft keyboard、resize |
| 768x1024 | tablet portrait、sidebar overlay、switching |
| 390x844 | phone portrait、virtual keys、search、modal |
| 844x390 | phone landscape、visualViewport、terminal resize |

每个视口保存 screenshot，并断言 terminal canvas 非空、文本控件不越界重叠、
软键盘不引起布局振荡、焦点正确、认证和搜索可操作、WebSocket 自动恢复、不
出现文件/Agent/PWA UI 或请求、Service Worker registration 为空，且不请求
当前 Origin 之外的资源。

### Container/Kubernetes

- 镜像无 floating `latest`，build stages 和 runtime digest 固定。
- Runtime 有固定 Node 与 worker dependencies，无 npm/npx/corepack/compiler/
  cloudflared/Agent CLI。
- 非 root，state/workspace 可写，其他权限符合预期。
- Worker 不监听网络端口；worker 缺失时容器启动失败。
- Kill worker 后服务非零退出，restart policy 拉起实例并恢复 metadata、
  snapshot 和新 Bash。
- SIGTERM 终止 Bash process group 并在 grace period 内退出。
- Port 9846 冲突时明确失败；healthcheck 和 Secret/config injection 有效。
- Resources、probe timings、10s shutdown 和 30s grace 与部署模块一致。
- Deployment 是单副本 Recreate，PVC 为 RWO；普通 YAML 通过 schema 和 dry-run。

## Definition of Done

- [ ] 所有决策均为 `已确认`，正文无待定实现分支。
- [ ] Go backend 和模块化 Web frontend 架构完成。
- [ ] 独立 Node terminal worker、framed IPC 和 xterm.js snapshot contract 完成。
- [ ] 用户可在 Chrome 完成登录和单实例多 Terminal Tab 核心流程。
- [ ] 不存在 Host model、Host management UI、cluster API 或 `cluster.json`。
- [ ] Refresh/reconnect/restart 语义符合产品范围模块。
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
