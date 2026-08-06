# Roaminal MVP 实施计划

> 文档版本：1.1
> 更新日期：2026-08-06
> 文档状态：**Approved / 可直接实施**
> 批准日期：2026-08-06
> 目标读者：后续负责完整实现的 Coding Agent
> 行为参考：`~/temp/Tabminal`，tag `v3.0.40`，commit
> `fbd26d3aff033fd850a6696eccb107520780fd8b`

## 文档目的

本计划锁定 Roaminal MVP 的产品范围、行为契约、Go 后端架构、Web 前端架构、
部署方式和验收方法。`DEC-001` 至 `DEC-029` 已全部确认，不存在待实施 Agent
选择的分支。

“功能一比一复现”是指复现纳入范围的用户行为、交互、协议语义和恢复能力。
Tabminal 只作为行为规格与验收 oracle，不直接复制其 Node.js 后端、前端单文件
架构或源码结构。

## 阅读顺序

README 是决策索引和执行入口；详细 contract 按以下顺序读取：

| 顺序 | 模块 | 负责内容 |
| --- | --- | --- |
| 1 | [01-scope.md](./modules/01-scope.md) | 产品前提、成功定义、排除项、行为复现边界 |
| 2 | [02-configuration-auth.md](./modules/02-configuration-auth.md) | CLI/config、启动退出、challenge、token 与登录会话 |
| 3 | [03-terminal.md](./modules/03-terminal.md) | PTY/session、terminal worker、IPC、snapshot 与持久化 schema |
| 4 | [04-protocol.md](./modules/04-protocol.md) | HTTP allowlist、JSON schema、status 和 Terminal WebSocket |
| 5 | [05-frontend.md](./modules/05-frontend.md) | React 模块边界、terminal UI、响应式交互、缓存策略 |
| 6 | [06-architecture-dependencies.md](./modules/06-architecture-dependencies.md) | 目标架构、参考映射、固定依赖和命名空间 |
| 7 | [07-deployment.md](./modules/07-deployment.md) | OCI image、普通容器、Kubernetes YAML 和故障语义 |
| 8 | [08-implementation.md](./modules/08-implementation.md) | Phase 顺序、gate、测试矩阵和 Definition of Done |
| 参考 | [09-rationale.md](./modules/09-rationale.md) | React、独立 worker 和 fail-fast 的已确认决策依据 |

实施时，README 决策列表与各模块共同构成规范，不可只执行单个文件。若出现
文字冲突，优先级为：本 README 的决策项 > 对应职责模块的精确 contract >
决策依据。发现真实冲突必须按停止条件报告，不能自行选择。

## 决策项列表

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
| `DEC-019` | 接受 `github.com/gitpod-io/xterm-go` 当前实现成熟度，但它是 MVP 后的对比候选，不是 MVP runtime dependency。候选固定 commit `8e117204ebedc133bf33ee9eb759c8484f843cee`；发现不足时可在对比完成后输出开源贡献提案。 | 已确认 |
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

## Git 提交规则

允许的 commit message：

```text
feat(auth): implement challenge token rotation
feat(terminal): persist headless terminal snapshots
test(terminal): cover reconnect snapshot ordering
docs(deploy): document websocket proxy settings
```

禁止的 commit message：

```text
phase 2 complete
implement MVP with agent
misc updates
fix stuff
generated by AI
```

## 实施 Agent 自治规则

1. 先按阅读顺序读完 README 和 8 个实施模块，再从 Phase 0 开始；
   `09-rationale.md` 仅在需要理解取舍时阅读。
2. 直接执行已确认决策，不重新发起相同选择。
3. 不增加文件工作区、Agent、AI、PWA、原生客户端或 Kubernetes HA。
4. 参考代码只用于理解和行为对照，不直接复制实现。
5. 每个 Phase 完成后运行 gate，不把验证留到最后。
6. 每个 commit 是单一连贯变更，message 严格遵守 `DEC-018`。
7. 失败时先自行定位修复；只有下列情况才暂停：
   - 参考 commit 不匹配；
   - 已确认决策或模块 contract 互相矛盾；
   - 必须扩大容器权限或安全边界；
   - 无法保留用户已有修改；
   - 外部凭据、域名、registry 或 Kubernetes 访问成为硬性前提；
   - 固定依赖无法取得，或尝试有界 worker adapter 后仍不能满足 contract tests。
8. 不需要真实集群或 registry 即可完成 image test 和 YAML dry-run；缺少发布
   权限不阻塞代码交付。
9. 不把未运行测试写成通过；所有未运行项记录在 implementation log。
10. 最终报告只包含结果、验证证据、批准差异和真实残余风险。

## 已批准的实施启动

```text
严格按 docs/plan/mvp/README.md 及其模块索引实施 Roaminal MVP。
先读完 README 和 01 至 08 模块，再从 Phase 0 连续执行到所有
Definition of Done 满足。按文档自行创建原子 commits；除停止条件外，
不要等待人工确认。
```
