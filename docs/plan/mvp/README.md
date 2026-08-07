# Roaminal MVP 实施计划

> 文档版本：1.5
> 更新日期：2026-08-07
> 文档状态：**Approved / 可直接实施**
> 批准日期：2026-08-06
> 目标读者：后续负责完整实现的 Coding Agent
> 行为参考：`~/temp/Tabminal`，tag `v3.0.40`，commit
> `fbd26d3aff033fd850a6696eccb107520780fd8b`

## 文档目的

本计划锁定 Roaminal MVP 的产品范围、行为契约、Go 后端架构、Web 前端架构、
部署方式和验收方法。`DEC-001` 至 `DEC-044` 已全部确认，不存在待实施 Agent
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
| 8 | [08-test-environment.md](./modules/08-test-environment.md) | 项目依赖、Chrome、Podman、registry 和 develop namespace 测试规则 |
| 9 | [09-implementation.md](./modules/09-implementation.md) | Phase 顺序、gate、测试矩阵和 Definition of Done |
| 参考 | [10-rationale.md](./modules/10-rationale.md) | React、独立 worker 和 fail-fast 的已确认决策依据 |
| 增补需求 | [11-terminal-tabs-requirements.md](./modules/11-terminal-tabs-requirements.md) | 多终端 Tab、重命名、侧栏、xterm 和持久化目录问题单 |
| 增补方案 | [12-terminal-tabs-solution.md](./modules/12-terminal-tabs-solution.md) | 状态模型、协议、迁移、生命周期和回归测试方案 |
| 修复方案 | [13-single-terminal-sidebar-remediation.md](./modules/13-single-terminal-sidebar-remediation.md) | 取消 Tab、Sidebar 对齐、既有 MVP 缺口审查与完整修复方案 |

实施时，README 决策列表与各模块共同构成规范，不可只执行单个文件。若出现
文字冲突，优先级为：本 README 中编号较大的明确取代决策 > 其他 README 决策项
> 对应职责模块的精确 contract > 决策依据。`DEC-034` 至 `DEC-044` 是 MVP
验收后依据真实使用反馈增加的修订项；其明确取代的旧 contract 不再实施。
发现除此以外的真实冲突必须按停止条件报告，不能自行选择。

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
| `DEC-030` | 本地容器 build/test/push 只使用 Podman CLI。允许必要的 `podman build`、`podman run` 和 `podman push`；禁止 Docker、Docker Compose、Podman Compose 和兼容 alias。仓库使用 `Containerfile`，不交付 compose 文件。 | 已确认 |
| `DEC-031` | 项目不使用 Makefile；Agent 直接执行并记录 `go`、`npm`/`npx`、`podman` 和 `kubectl` 命令。缺少 `make` 不得中断实施或测试。 | 已确认 |
| `DEC-032` | Kubernetes 验证使用当前集群的 `develop` namespace，执行 server-side dry-run、实际 rollout 和 E2E；不要求 `kubeconform`/`kubeval`，缺少静态 schema 工具不得中断。 | 已确认 |
| `DEC-033` | React、Vite、Vitest、xterm、ESLint、Playwright runner 和 worker packages 都是仓库锁定的项目依赖，由实施 Agent 创建 lockfile、安装并使用项目内 binary。全局 npm packages 缺失或版本不同不得中断；允许使用可信镜像加速但不得改变版本或 integrity。 | 已确认 |
| `DEC-034` | Heartbeat 返回的 session 顺序必须稳定；前端选中 Tab、状态栏和唯一可见 xterm viewport 必须始终指向同一 session。该项修订现有未排序 map 输出和 DOM reattach 行为。 | 已确认 |
| `DEC-035` | Terminal session 是服务端运行实体，Tab 只是当前浏览器打开的视图。Tab 条不必展示所有 session；关闭 Tab 只隐藏视图并释放该浏览器 runtime，不调用删除 session API。Sidebar 展示全部 session，点击未打开 session 时重新打开 Tab。该项取代 `05-frontend.md` 中 Tab 与 session 一一覆盖及关闭 Tab 即关闭 terminal 的旧 contract。 | 已确认 |
| `DEC-036` | Tab 原 `x` 操作改为菜单触发器；菜单至少包含重命名、关闭 Tab 和终止 Terminal。关闭 Tab 非破坏性；终止 Terminal 是明确区分且需确认的破坏性操作。自定义标题持久化，并可恢复为 shell 自动标题。 | 已确认 |
| `DEC-037` | 前端 xterm core 与 addons 的 peer dependency 必须全部兼容；禁止安装 `npm ls` 判定为 invalid 的组合。当前只支持 xterm 5 的 CanvasAddon 不再与 xterm 6.1 beta 混用；使用 core renderer 或同发布线的受支持 renderer。该项取代 `06-architecture-dependencies.md` 中固定 `@xterm/addon-canvas 0.8.0-beta.48` 的要求。 | 已确认 |
| `DEC-038` | 一个进程只允许一个有效持久化 root。PVC mount root 权限不安全时有效 root 为 `.roaminal/state/`；`.roaminal/sessions/` 是旧启动遗留且为空时可清理，不得双写。两个候选目录均含数据时必须停止并报告，不得自动合并或删除。 | 已确认 |
| `DEC-039` | Sidebar toggle 在桌面必须真实收起并保留可发现的恢复入口，在移动端必须控制 overlay；按钮的 aria label、展开状态和焦点行为必须与视觉状态一致。 | 已确认 |
| `DEC-040` | 浏览器不再维护或显示 Terminal Tab；只保存一个 `activeSessionId`，点击 Sidebar session 直接切换唯一主 terminal。该项取代 `DEC-035`、`DEC-036` 和 05/11/12 模块中的 Tab/open/close contract；rename/reset/terminate 菜单继续保留。 | 已确认 |
| `DEC-041` | Sidebar session card 显示 effective title、短 ID、完整 PWD 和本地时间 SINCE；桌面 hover 按需创建只读实时 preview，移动端不创建，全页面最多一个 preview runtime。 | 已确认 |
| `DEC-042` | 每个 session card 显示独立 Agent、Files 和 Terminal actions 图标入口。Agent/Files 只是 core 的 extension affordance，未安装插件时显示明确 unavailable 状态；该项仅取代“不得出现入口/占位”，不把 Agent、AI 或文件 workspace 实现在 core。 | 已确认 |
| `DEC-043` | `13-single-terminal-sidebar-remediation.md` 审查列出的既有实现与证据缺口全部纳入 MVP DoD；implementation log 或手工 smoke 不能替代缺失的自动化 contract test。 | 已确认 |
| `DEC-044` | 后续 Kubernetes 验证直接访问 `http://roaminal.develop.svc.cluster.local:9846`，不再使用 port-forward。HTTP Service 仅可在测试 Chrome 中对精确 origin 设置 secure-context 例外；产品 runtime 不提供弱 crypto fallback，生产入口仍要求 HTTPS。 | 已确认 |

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

1. 先按阅读顺序读完 README 和 01 至 09 模块，再读 11 至 13 修订模块；
   `10-rationale.md` 仅在需要理解取舍时阅读，然后从修订实施顺序继续。
2. 直接执行已确认决策，不重新发起相同选择。
3. 不增加文件工作区、Agent/AI 功能、PWA、原生客户端或 Kubernetes HA；
   `DEC-042` 规定的两个 unavailable extension affordance 是唯一 UI 例外。
4. 参考代码只用于理解和行为对照，不直接复制实现。
5. 每个 Phase 完成后运行 gate，不把验证留到最后。
6. 每个 commit 是单一连贯变更，message 严格遵守 `DEC-018`。
7. 失败时先自行定位修复；只有下列情况才暂停：
   - 参考 commit 不匹配；
   - 已确认决策或模块 contract 互相矛盾；
   - 必须扩大容器权限或安全边界；
   - 无法保留用户已有修改；
   - 已确认可用的 `develop` namespace 或内部 registry 状态发生变化，且无同等
     的自动化替代路径；
   - 固定依赖无法取得，或尝试有界 worker adapter 后仍不能满足 contract tests。
8. Kubernetes 测试必须使用 `develop` namespace 和可访问的内部 registry；
   Docker、Compose、Make 或静态 YAML validator 缺失不构成阻塞。
9. 不把未运行测试写成通过；所有未运行项记录在 implementation log。
10. 最终报告只包含结果、验证证据、批准差异和真实残余风险。

## 已批准的实施启动

唯一启动指令为：

```text
严格按 docs/plan/mvp/README.md 及其模块索引实施 Roaminal MVP。
先读完 README、01 至 09 及 11 至 13 模块，再从当前修订实施顺序连续执行到所有
Definition of Done 满足。按文档自行创建原子 commits；除停止条件外，
不要等待人工确认。
```
