# Roaminal MVP 实施计划

> 文档版本：0.1（讨论稿）
> 文档状态：**Draft / 尚不可直接实施**
> 目标读者：后续负责完整实现的 Coding Agent
> 参考实现：`~/temp/Tabminal`，tag `v3.0.40`，commit
> `fbd26d3aff033fd850a6696eccb107520780fd8b`

## 1. 文档目的

本计划用于先锁定 Roaminal MVP 的产品边界、行为契约、技术路线、部署
方式和验收方法。完成讨论后，它应当能够直接交给另一个 Agent 执行，
执行过程中尽可能不再需要人为选择或补充需求。

当前版本是讨论稿。所有标记为 `待确认` 的阻塞决策必须先得到结论，
然后将本文件状态改为 `Approved`，实施 Agent 才能开始编码。

为便于阅读，正文暂时按第 11 节的“推荐结论”描述目标实现。凡正文内容
引用了尚未确认的 `DEC-*`，该内容仍是建议而不是已锁定需求；最终结论
必须在批准本文前同步回正文、阶段 gate 和 Definition of Done。

本文所说的“一比一复刻”是指：

- 对纳入 MVP 的功能，复刻 Tabminal 的用户可见行为、交互、协议语义和
  恢复能力。
- 产品名、命令、环境变量、存储目录和浏览器存储键统一改为 Roaminal。
- 不复制已锁定排除的文件工作区、Agent 和原生客户端代码；terminal-native
  AI 暂按 `DEC-002` 的推荐结论排除。
- 不要求逐文件或逐行相同；删除功能后必须同步删除死代码、依赖、接口、
  DOM、样式、测试和文档。
- MVP 阶段不借机重写框架或重新设计 UI。除范围裁剪、品牌迁移、部署和
  必要的解耦外，优先保持参考实现的结构与行为。

## 2. 已锁定的产品前提

以下内容来自当前项目目标和本轮范围划定，除非后续明确修改，否则视为
已经确定：

1. Roaminal 是纯 Web 应用，由浏览器客户端和容器内的服务端组成。
2. 不开发 macOS、Windows、Linux、iOS 或 Android 原生客户端。
3. 开发阶段只保证 Google Chrome 兼容性。
4. 页面需要适配桌面、平板和手机尺寸的 Chrome。
5. MVP 不包含文件工作区，后续将单独设计和实现。
6. MVP 不包含 ACP、Coding Agent、Agent 工作区或 Agent 管理能力。
7. MVP 通过普通容器运行时或 Kubernetes 部署。
8. 保留长期产品方向中的多 Host 访问能力。
9. MVP 的主体是持久终端，而不是服务器管理面板或 Web IDE。

## 3. MVP 成功定义

MVP 完成后，用户应当能够：

1. 在 Chrome 中打开 Roaminal 并通过密码登录。
2. 创建、切换和关闭多个真实 PTY 终端会话。
3. 刷新页面、临时断网或从另一台设备重新连接后继续使用已有会话。
4. 在桌面、平板和手机布局中完成相同的核心终端操作。
5. 通过一个 Roaminal 页面注册并访问多个 Roaminal Host。
6. 看到每个 Host 的连接状态、会话状态和基础系统状态。
7. 使用终端搜索、链接识别、终端进度、触控辅助键盘和常用快捷键。
8. 以容器启动服务，或用仓库提供的 Kubernetes 清单部署单实例服务。
9. 使用持久卷保存 Roaminal 的认证会话、Host 注册表、终端元数据、
   scrollback 日志和终端快照。

MVP 不承诺容器或 Pod 重启后仍保留原 PTY 进程。建议采用的精确定义见
`DEC-001`。

## 4. 范围清单

### 4.1 必须包含

#### 4.1.1 服务启动与配置

- Node.js `>= 22`、ESM 项目。
- CLI 命令为 `roaminal`。
- 支持配置优先级：内置默认值、`~/.roaminal/config.json`、
  `./config.json`、CLI 参数、环境变量。
- MVP 配置项：
  - bind host
  - port
  - password
  - shell
  - WebSocket heartbeat interval
  - terminal history limit
  - debug
  - accept terms
  - initial working directory
- 未配置密码时生成临时随机密码并输出到服务日志。
- 未确认风险条款时拒绝启动。
- 收到 `SIGINT`/`SIGTERM` 后关闭 HTTP、WebSocket 和 PTY，并有超时保护。
- 端口冲突时是否自动递增端口由 `DEC-011` 决定。

#### 4.1.2 认证与登录会话

复刻 Tabminal 当前认证模型，但使用 Roaminal 命名空间：

- 一次性 challenge + HMAC-SHA256 密码证明，浏览器不存储密码或可复用
  密码哈希。
- Access token 默认有效期 15 分钟。
- Refresh token 默认有效期 90 天，刷新时轮换。
- Refresh session 持久化到服务端。
- 浏览器按 Host 隔离保存认证状态。
- 支持查看登录会话、撤销指定会话、退出其他会话和当前登出。
- 30 次失败后锁定服务，重启后解锁。
- WebSocket 使用 subprotocol 携带 access token，不把新 token 放在 URL。
- 主 Host 的未认证状态控制全局登录框；子 Host 登录失败只影响该 Host。

#### 4.1.3 持久终端服务端

- 使用 `node-pty` 创建真实 PTY。
- 默认 shell 和可配置 shell 行为与参考实现一致；容器内至少提供 Bash。
- 每个会话有独立 ID、创建时间、shell、初始 cwd、当前 cwd、标题、尺寸、
  状态和执行记录。
- 捕获 OSC title 与 cwd 更新，并实时同步到客户端。
- 支持输入、resize、输出广播和多个浏览器客户端同时连接。
- 多客户端连接时保留 terminal-query response owner/claim 机制。
- 保留 shell execution marker，用于显示运行状态、完成状态和通知。
- 保留终端历史上限、xterm headless snapshot 和恢复逻辑。
- PTY 输出、日志、元数据和快照按会话持久化。
- 关闭会话时终止 PTY，并删除该会话的 JSON、日志和快照。
- 服务启动时加载持久化会话，按原 ID、cwd、标题、尺寸和 scrollback
  创建替代 PTY。
- 服务端不自动创建默认会话；前端在首次没有会话或关闭最后会话时创建
  一个主 Host 会话。
- 新会话默认继承最近活动终端的 cwd；无法确定时使用配置值或 home。

#### 4.1.4 HTTP 与 WebSocket 同步

- HTTP 用于权威状态和变更。
- WebSocket 用于终端实时流。
- `/api/heartbeat` 是每个 Host 的权威会话清单。
- 前端 heartbeat 固定为 1000 ms；异常后的重试节流为 5000 ms。
- WebSocket 中断后由 heartbeat 驱动重连和状态校准。
- `GET /api/version` 暴露每次进程启动变化的 boot ID。
- 客户端发现主 Host boot ID 变化后重新加载匹配版本的应用壳资源。

MVP 允许的路由应严格限制为：

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
GET    /api/cluster
PUT    /api/cluster
WS     /ws/:sessionId
```

`POST /api/sessions/:id/state` 属于原文件/工作区状态模型，MVP 建议不保留。
如果后续发现有纯终端客户端状态需要服务端保存，应先定义独立字段和接口，
不能把旧的 `editorState`/`workspaceState` 原样带入。

#### 4.1.5 浏览器终端 UI

- 保留 Tabminal 的 Solarized Dark 风格、字体、紧凑工作界面和响应式布局，
  全部品牌文字改为 Roaminal。
- 使用 xterm.js，并保留下列终端能力：
  - fit
  - web links
  - canvas renderer
  - search
  - progress
  - ligatures
- 终端标签展示：
  - 标题
  - 短 session ID
  - Host
  - cwd
  - 创建时间
  - 运行/完成/需要注意状态
  - 支持时显示终端进度
- 桌面尺寸保留标签内的实时终端缩略预览；移动尺寸不创建预览。
- 可创建、切换、关闭终端；关闭当前终端后选择相邻终端。
- Sidebar 支持展开/收起及移动端 overlay。
- 页面标题跟随当前终端/Host。
- 搜索栏支持大小写、全词、正则、上一个和下一个结果。
- 支持浏览器复制粘贴和 xterm 原生选择行为。
- 触控设备保留 ESC、TAB、CTRL、ALT、SHIFT、SYM、方向键和软键盘。
- 处理 `visualViewport`、安全区域和软键盘造成的可用高度变化。
- 不出现文件按钮、文件树、编辑器面板、工作区标签条、Agent 按钮或
  Agent 面板占位。

保留的快捷键：

```text
Ctrl + Shift + T        新建终端
Ctrl + Shift + W        关闭终端
Ctrl + Shift + [ / ]    切换终端
Ctrl/Cmd + F            终端内搜索
Ctrl + Shift + ?        快捷键帮助
```

所有文件工作区和 Agent 快捷键必须移除，不保留无效入口。

#### 4.1.6 多 Host

- 主 Host 为当前页面来源，ID 固定为 `main`。
- 用户可添加、重连和删除子 Host。
- 每个 Host 的会话、认证、连接状态、heartbeat 和终端 WebSocket 严格隔离。
- Host 注册表以主 Host 的 `GET/PUT /api/cluster` 为唯一持久化来源。
- 浏览器 localStorage 只保存每个 Host 的认证状态，不保存权威 Host 列表。
- Host 去重键为规范化后的 `hostname[:port]` 小写值，路径不参与去重。
- 加载注册表时跳过指向当前主节点的项目，避免自循环。
- 不支持用同一域名的不同 URL path 表示多个 Host。
- HTTPS 页面只能连接 HTTPS/WSS 子 Host。
- 子 Host 请求保留 `credentials: include`，并正确识别上游登录跳转。

#### 4.1.7 状态、提示和通知

- 保留 Host heartbeat 延迟图和连接状态。
- 保留当前 Host 的 hostname、kernel、IP、CPU、内存、uptime、Roaminal
  进程 uptime、FPS、延迟和会话数展示。
- 保留连接丢失、恢复、终端退出的 in-app toast。
- 当前终端之外的命令完成后显示 attention 状态；在用户允许时发送 Chrome
  系统通知，否则降级为 toast。
- 内部 shell ready/bootstrap 命令不得产生用户通知。

以上 UI 是否全部进入 MVP 由 `DEC-004` 最终确认。

#### 4.1.8 Web App/PWA

- 提供响应式 Web App manifest、图标、theme color 和 standalone display。
- Service Worker 只缓存静态应用壳，不缓存 API 响应。
- HTML 先读取 `/api/version`，再加载带 boot ID 的 CSS、JS 和 Service
  Worker，避免服务更新后的旧资源混用。
- 离线时可以显示已缓存的应用壳，但不承诺离线终端可用。

是否把 PWA 安装能力列为 MVP 必选项由 `DEC-003` 确认。

#### 4.1.9 容器和 Kubernetes

- 提供可重复构建、版本固定的 OCI 镜像。
- 镜像内包含 Node.js 22、运行时依赖、Bash、Roaminal 服务和前端静态资源。
- 使用非 root 用户运行服务。
- 使用 init 进程正确转发信号和回收子进程。
- 提供容器 healthcheck，调用 `/healthz`。
- 提供 `compose.yaml` 作为普通容器运行示例。
- 提供 Kubernetes 基础清单，默认单副本、有序身份和持久卷。
- 提供 readiness/liveness probe、Secret 注入、资源 request/limit、
  securityContext、Service 和可选 Ingress 示例。
- 文档明确 WebSocket、反向代理超时、TLS 和 sticky routing 要求。
- 默认不挂载宿主机 Docker socket，不使用 privileged，不提供宿主机 root
  shell。
- “不包含文件工作区”只排除浏览器文件 UI 和文件 API，不限制用户在容器
  shell 中通过普通终端命令访问显式挂载到 `/workspace` 的文件。

### 4.2 MVP 排除清单

以下内容不得出现在 MVP 的运行时代码、UI、API、依赖或部署配置中：
其中 terminal-native AI、Cloudflare Tunnel 和 Kubernetes HA 等建议项仍
需以第 11 节的对应决策为准。

- 文件列表、读取、创建、保存、重命名、删除和 raw preview API。
- 文件树、Monaco Editor、图片/PDF/Markdown 预览、文件图标下载构建。
- `memory.json` 和 expanded-folder API。
- `editorState`、`workspaceState`、workspace tabs、terminal pinning。
- ACP SDK、ACP manager、ACP test agent、Agent tab、Agent 配置、Agent 历史、
  prompt、attachment、permission、plan、usage HUD 和 managed terminal。
- Tabminal 的 terminal-native AI assistant、`#` prompt 劫持、失败命令
  auto-fix、OpenAI/OpenRouter/Google Search 配置和相关依赖。
- `apps/Apple`、Ghostty vendor、任何原生客户端或桌面安装包。
- macOS launchd、Linux systemd/pm2 和远程重部署脚本。
- 内置 cloudflared 二进制和自动创建 Cloudflare Tunnel。
- Firefox、Safari、Edge 等浏览器的兼容性承诺和专项 workaround。
- Kubernetes 多副本、高可用、HPA、PTY 跨 Pod 迁移和无缝滚动升级。
- 文件上传/下载、SFTP、端口转发、SSH 客户端和 Kubernetes 管理终端。

## 5. 参考代码迁移边界

### 5.1 直接复用并裁剪

| 参考文件 | MVP 处理方式 |
| --- | --- |
| `src/auth.mjs` | 保留认证行为，完整迁移命名空间 |
| `src/config.mjs` | 删除 AI/Search/Tunnel 配置，只保留 MVP 配置 |
| `src/persistence.mjs` | 只保留 session、cluster、auth session 持久化 |
| `src/system-monitor.mjs` | 按 `DEC-004` 保留或删除 |
| `src/terminal-manager.mjs` | 删除 editor/workspace/managed-agent 分支，保留 PTY 生命周期 |
| `src/terminal-session.mjs` | 删除全部 AI 分支，保留终端解析、快照、执行事件 |
| `src/server.mjs` | 删除 FS、memory、Agent、AI、Tunnel 路由和初始化 |
| `shell/*` | 改名并保留 Bash/Zsh shell marker 能力 |
| `public/index.html` | 删除 Monaco、editor、Agent DOM，只保留终端 shell |
| `public/app.js` | 裁剪为 auth、Host、terminal、status、touch、PWA 逻辑 |
| `public/styles.css` | 删除 file/editor/Agent 样式，保留终端响应式样式 |
| `public/modules/url-auth.js` | 保留并迁移品牌命名空间 |
| `public/modules/session-meta.js` | 保留 Host/cwd 展示工具 |
| `public/modules/notifications.js` | 按 `DEC-004` 保留或删除 |
| `public/sw.js` | 按 `DEC-003` 保留并删掉文件资产 |
| `test/auth.mjs` | 保留并改名测试协议与存储 |
| `test/terminal-session.mjs` | 删除 AI case，保留纯终端 case |
| `test/terminal-manager.mjs` | 删除 workspace case，补充纯终端 cwd/restore case |
| `test/integration-shell.mjs` | 删除 managed Agent case，保留 shell/persistence case |

### 5.2 不迁移

```text
src/acp-manager.mjs
src/acp-test-agent.mjs
src/fs-routes.mjs
test/acp-manager.mjs
test/fs-routes.mjs
docs/ACP.md
apps/
public/icons/（文件类型图标）
scripts/acp-browser-smoke.mjs
scripts/mac_*.sh
scripts/*.plist
scripts/*.service
```

### 5.3 依赖目标

运行时依赖应收敛到实际使用项，预计包括：

```text
@fontsource/monaspace-neon
@koa/router
koa
koa-bodyparser
koa-static
node-ansiparser
node-pty
ws
xterm-addon-serialize
xterm-headless
```

不得保留以下仅为排除功能服务的依赖：

```text
@agentclientprotocol/sdk
@mozilla/readability
formidable
jsdom
openai
utilitas
```

前端 xterm 及 addons 使用 CDN 还是随镜像打包，由 `DEC-007` 决定。

## 6. Roaminal 命名空间迁移

最终运行时代码和用户界面不得残留 Tabminal 命名，MIT 版权归属和迁移说明
除外。

| Tabminal | Roaminal |
| --- | --- |
| package/bin `tabminal` | `roaminal` |
| `~/.tabminal` | `~/.roaminal` |
| `TABMINAL_*` | `ROAMINAL_*` |
| `TABMINAL_CWD` | `ROAMINAL_CWD` |
| `TABMINAL_SESSION_ID` | `ROAMINAL_SESSION_ID` |
| `tabminal.v1` | `roaminal.v1` |
| `tabminal.auth.` | `roaminal.auth.` |
| `tabminal-login-v1` | `roaminal-login-v1` |
| access token prefix `ta_` | `ra_` |
| refresh token prefix `tr_` | `rr_` |
| `tabminal_auth_state:<hostId>` | `roaminal_auth_state:<hostId>` |
| runtime boot ID key | `roaminal_runtime_boot_id` |
| shell hooks/functions | `roaminal-*` / `_roaminal_*` |
| `TABMINAL_SHELL_READY` | `ROAMINAL_SHELL_READY` |
| `TabminalPrompt` OSC marker | `RoaminalPrompt` |
| Web/PWA name and icons | Roaminal / `r>` 品牌 |

由于当前 Roaminal 仓库没有历史用户数据，建议不实现 Tabminal 配置、token、
协议或数据目录的兼容迁移。该建议由 `DEC-010` 确认。

## 7. 目标架构与数据边界

```text
Chrome desktop/tablet/mobile
          |
          | HTTPS: auth, heartbeat, session inventory, cluster registry
          | WSS: terminal input/output/snapshot/meta/status/execution
          v
Roaminal container (single runtime identity)
  |- Koa HTTP server
  |- WebSocket server
  |- Auth store
  |- Terminal manager
  |    `- one node-pty process per live session
  |- System monitor (pending DEC-004)
  `- ~/.roaminal on persistent volume
       |- sessions/*.json
       |- sessions/*.log
       |- sessions/*.snapshot
       |- auth-sessions.json
       `- cluster.json
```

核心数据边界：

- HTTP heartbeat 是终端清单的权威来源，WebSocket 不是唯一事实来源。
- 每个会话只属于一个 Host，不合并不同 Host 的运行时状态。
- 浏览器拥有短期访问状态，服务端拥有 refresh session 和 Host 注册表。
- PTY 只存在于创建它的 Roaminal 进程中，不能由另一个 Pod 接管。
- 持久卷保存的是恢复材料，不是正在运行的 Unix 进程。

## 8. 实施阶段

实施 Agent 必须按顺序完成，每个阶段通过对应 gate 后再进入下一阶段。

### Phase 0：冻结来源与建立追踪

任务：

1. 确认 `~/temp/Tabminal` 的 HEAD 正好是
   `fbd26d3aff033fd850a6696eccb107520780fd8b`。
2. 如果不匹配，停止并报告，不得自行改用新版。
3. 记录参考实现的 MIT `LICENSE` 并在 Roaminal 中保留原版权声明。
4. 建立实现日志 `docs/plan/mvp/implementation-log.md`，记录每阶段完成项、
   行为差异、测试命令和结果。
5. 记录开始前 Roaminal 工作树状态，保留所有用户已有改动。

Gate：参考版本、许可证和工作树状态都有可审计记录。

### Phase 1：项目骨架与品牌迁移

任务：

1. 创建 package、lint、test、build/start/dev 脚本。
2. 只导入 MVP 所需后端、shell、前端和测试文件。
3. 完成第 6 节列出的命名空间迁移。
4. 生成 Roaminal favicon；如果 `DEC-003` 确认保留 PWA，再生成 PWA 图标
   和 manifest。不得继续使用 Tabminal 产品图标。
5. 删除所有排除功能的依赖和 build 步骤。
6. 更新根 README 的 MVP 状态、启动方法、风险警告和部署入口。

Gate：

- `npm install` 成功。
- `npm run lint` 能执行。
- 除 LICENSE/迁移说明外，源码和 UI 中无 `Tabminal`/`tabminal` 标识。
- package lock 中无 Agent、AI、文件预览相关依赖。

### Phase 2：认证、配置与持久化

任务：

1. 迁移并测试配置优先级和参数验证。
2. 迁移完整认证 challenge/token/refresh/session/revoke 流程。
3. 将认证算法、WebSocket protocol、token prefix 和浏览器存储键改为
   Roaminal。
4. 精简 persistence，只保留 session、cluster 和 auth session。
5. 添加损坏持久化文件、过期 token、token rotation、锁定和恢复测试。

Gate：认证单元测试全部通过；排除 API 返回 404；磁盘不生成
`memory.json`、`agent-*.json` 或任何 editor/workspace 字段。

### Phase 3：PTY、终端协议与会话恢复

任务：

1. 迁移 TerminalManager 和 TerminalSession 的纯终端行为。
2. 删除 AI prompt interception、auto-fix、provider 调用和 managed session。
3. 重命名 Bash/Zsh hooks、环境变量和 OSC marker。
4. 保留 title/cwd/execution/snapshot/query-owner 行为。
5. 精简会话 JSON schema，不写入 editor/workspace/managed 数据。
6. 实现并测试进程重启后的替代 PTY + scrollback 恢复。
7. 保留 history trim、尺寸清洗、输出缓冲、异常 resize、PTY exit 行为。

Gate：纯终端 unit/integration tests 全部通过，并新增以下回归测试：

- 刷新式重新 attach 不丢 scrollback。
- 两个 WebSocket 客户端同时 attach 时输入和 query owner 正确。
- 进程重启后会话 ID/cwd/title/scrollback 恢复，但旧进程不被宣称存活。
- 关闭会话删除所有持久化文件。
- `#` 作为普通 shell 输入处理，不触发 AI。

### Phase 4：HTTP/WS 服务与多 Host

任务：

1. 实现第 4.1.4 节的 API allowlist。
2. heartbeat 只接收 terminal resize 更新，只返回 sessions、system 和
   runtime；不得保留 agents/fileWriteResults。
3. 终端 WebSocket 只允许 `/ws/:sessionId`。
4. 迁移 Host URL 规范化、去重、自节点跳过和 HTTPS/WSS 规则。
5. 迁移 backend-owned cluster registry。
6. 保留主/子 Host 认证隔离和 Cloudflare Access 浏览器跳转行为（取决于
   `DEC-005`）。
7. 保留启动 boot ID 和应用壳版本一致性。

Gate：API contract tests、WebSocket auth tests、多 Host 隔离 tests 通过；
所有 `/api/fs/*`、`/api/memory/*`、`/api/agents/*` 和 `/ws/agents/*` 均
不存在。

### Phase 5：终端专用 Web UI

任务：

1. 从参考 HTML 中移除 Monaco loader 和全部 editor/Agent DOM。
2. 从 `app.js` 中删除 EditorManager、AgentTab 和相关 helper/state/event。
3. 保留并解耦 AuthManager、ServerClient、Session、terminal tab、status、
   search、notification 和 touch keyboard。
4. 删除所有 editor/workspace/Agent CSS selector，并确认无孤立 DOM 查询。
5. 重建纯终端 sidebar：终端标签、缩略预览、Host 操作、关闭按钮和状态。
6. 保留主 Host 无会话时自动创建、关闭最后会话后自动补建逻辑。
7. 更新帮助弹窗，只展示仍有效的快捷键。
8. 完成桌面、平板、手机竖屏和手机横屏响应式检查。

Gate：Chrome browser tests 通过，控制台无 error、failed request、空 DOM
引用或未处理 Promise；任何视口均无元素重叠、横向溢出或不可点击控件。

### Phase 6：PWA 与静态资源

任务：

1. 根据 `DEC-003` 实现或明确删除 PWA。
2. 若保留，更新 manifest、图标、cache namespace、boot ID 和 Service
   Worker asset allowlist。
3. Service Worker 不得缓存 `/api/*` 或包含认证数据的请求。
4. 根据 `DEC-007` 将 xterm 资源固定到 CDN 版本或打包进镜像。
5. 验证后端重启后页面不会混用旧 CSS/JS。

Gate：首次加载、刷新、Service Worker 更新、后端重启和无外网加载策略与
已确认决策一致。

### Phase 7：容器与 Kubernetes

任务：

1. 编写多阶段 Dockerfile，固定 Node 基础镜像版本，不使用 `node:latest`。
2. 构建 `node-pty`，最终镜像只保留运行所需内容。
3. 非 root 用户运行，接入 init 和 healthcheck。
4. 编写 `compose.yaml`，演示 password Secret/环境变量、state volume、
   workspace volume、端口和 accept terms。
5. 编写 Kubernetes StatefulSet、Service、PVC、Secret example 和可选
   Ingress example。
6. 默认 `replicas: 1`，更新策略不得同时启动两个会竞争同一 PVC 和同一
   Host 身份的 Pod。
7. 写明容器 shell 边界、volume 权限、WebSocket proxy timeout、TLS 和
   备份/恢复方法。
8. 容器和 Kubernetes 配置必须显式绑定 `0.0.0.0`；本地 CLI 的默认 bind
   address 仍可保持 `127.0.0.1`。

Gate：

- OCI 镜像可构建并以非 root 启动。
- `/healthz`、登录、创建终端、WebSocket 输入输出、容器停止均正常。
- 重建容器并挂载同一 state volume 后恢复会话材料。
- Kubernetes 清单通过 schema/dry-run 检查。

### Phase 8：最终验证与交付

任务：

1. 执行完整 lint、unit、integration、browser、container 测试。
2. 对照第 4 节逐项勾选，并对照第 4.2 节做负向扫描。
3. 更新 README、API、部署、安全和故障排查文档。
4. 记录所有已知差异；未批准的行为差异必须修复，不能只写入说明。
5. 检查仓库中没有临时文件、密钥、构建缓存或来源仓库的无关内容。

Gate：第 10 节 Definition of Done 全部满足。

## 9. 自动化测试矩阵

### 9.1 后端单元测试

- 配置优先级、非法端口、heartbeat/history 下限、随机密码。
- 认证 challenge 过期和单次使用。
- access/refresh token 签发、轮换、过期、撤销和服务重载恢复。
- WebSocket subprotocol 认证，响应不得回显 token-bearing protocol。
- 会话创建、默认 cwd、尺寸清洗、历史裁剪和删除。
- title/cwd OSC 解析和 execution marker 拆包。
- snapshot serialize/restore 和输出缓冲 attach。
- query response owner claim 和断开后的重新分配。
- cluster 规范化、持久化和损坏文件处理。

### 9.2 服务集成测试

- 未认证 API 为 401，公开 health/version 可访问。
- 登录后完整 HTTP 与 WebSocket terminal flow。
- heartbeat 是权威清单，resize 最终同步到 PTY。
- 刷新 token 后旧 refresh token 失效。
- 服务重启后 auth sessions、cluster 和 terminal restore 材料恢复。
- 关闭最后会话后由前端创建新会话，后端自身不自动创建。
- 子 Host 401 不触发主 Host 全局登出。
- 禁止的 API 和 WS namespace 返回 404/拒绝升级。

### 9.3 Chrome 端到端测试

建议使用 Playwright，并明确指定 Chrome channel。至少覆盖：

| 视口 | 用例 |
| --- | --- |
| 1440x900 | 桌面 sidebar、预览、终端、搜索、Host 管理 |
| 1024x768 | 平板横屏和软键盘前后布局 |
| 768x1024 | 平板竖屏、sidebar overlay、终端切换 |
| 390x844 | 手机竖屏、虚拟键、搜索和 modal |
| 844x390 | 手机横屏、visualViewport 和终端 resize |

每个关键视口必须保存 screenshot 供回归比较，并检查：

- 页面和终端 canvas 非空。
- 文本和按钮不越界、不重叠。
- 软键盘打开/关闭不导致布局振荡。
- 创建、切换、关闭终端后焦点正确。
- 搜索 UI、login、Host modal、auth sessions modal 可完整操作。
- WebSocket 断开后显示重连并自动恢复。
- 没有文件/Agent UI、请求或快捷键。

### 9.4 容器测试

- 镜像构建可重复且无 `latest` tag。
- 容器以非 root 用户运行。
- state/workspace volume 可写，其他路径权限符合预期。
- SIGTERM 能终止子 PTY，进程在超时内退出。
- healthcheck、端口、配置和 Secret 注入有效。
- 不包含 cloudflared、Agent CLI、OpenAI SDK、Monaco 或文件图标资产。

## 10. Definition of Done

以下条件必须全部满足，MVP 才算完成：

- [ ] 所有阻塞决策已写入本文件且状态为 `已确认`。
- [ ] 参考 commit 和 MIT 归属已记录。
- [ ] 用户可在 Chrome 完成登录、多终端和多 Host 核心流程。
- [ ] 页面刷新、网络断开重连和后端重启恢复符合已确认语义。
- [ ] 桌面、平板、手机 Chrome E2E 全部通过。
- [ ] 文件工作区的 API、UI、状态模型、依赖和资产均不存在。
- [ ] ACP、Agent、terminal AI、provider 和 managed terminal 均不存在。
- [ ] 无原生客户端、桌面打包和 host service 脚本。
- [ ] 所有产品命名空间均为 Roaminal。
- [ ] lint、unit、integration、browser 和 container tests 全部通过。
- [ ] OCI 镜像、compose 和 Kubernetes 单副本部署可用。
- [ ] 安全、API、配置、部署、备份恢复和故障排查文档完整。
- [ ] 实施日志记录命令、结果和与参考实现的全部批准差异。

## 11. 需要提前敲定的决策

下表中的推荐值只是为了推动讨论，不代表已经批准。

| ID | 问题 | 推荐结论 | 影响 | 状态 |
| --- | --- | --- | --- | --- |
| `DEC-001` | “持久终端”是否要求 Pod 重启后原进程继续运行？ | 不要求。浏览器刷新/断网时原 PTY 保持；服务重启后恢复同 ID、cwd、标题和 scrollback，但启动新 shell，旧命令进程已终止。 | 若要求真进程持久化，需要 tmux/screen 或外部 supervisor，已超出一比一复刻。 | 待确认 |
| `DEC-002` | Tabminal 的 terminal-native AI 是否属于 MVP？ | 排除。`#` 保持普通 shell 字符，删除全部 provider 配置与依赖。 | 决定 TerminalSession 裁剪范围和依赖。 | 待确认 |
| `DEC-003` | PWA 安装和 Service Worker 是否进入 MVP？ | 保留。它仍是纯 Web，不等于原生客户端，且有利于平板/手机使用。 | 决定 manifest、图标、缓存和 boot ID 验收。 | 待确认 |
| `DEC-004` | 系统监控、延迟图、命令完成通知是否全部保留？ | 保留，属于 Tabminal 纯终端体验；不保留文件/Agent 触发的通知。 | 决定 `SystemMonitor`、canvas、Notification 模块和大量 UI/CSS。 | 待确认 |
| `DEC-005` | Cloudflare 功能保留到什么程度？ | 保留客户端对 Cloudflare Access 登录跳转的兼容；删除服务端自动 Tunnel 和镜像内 cloudflared。 | 决定子 Host 登录逻辑和镜像体积/权限。 | 待确认 |
| `DEC-006` | 桌面 sidebar 中的终端实时缩略预览是否保留？ | 保留；删除预览周围的文件树和 Agent 控件。 | 对 xterm 实例数量、性能和 Tabminal UI 还原度有影响。 | 待确认 |
| `DEC-007` | xterm.js、addons 和前端依赖从 CDN 还是镜像本地加载？ | 镜像本地打包并固定参考版本；构建脚本从 npm 包复制 browser ESM/CSS 到 `public/vendor`，不为此引入前端框架。 | 会偏离参考实现的 CDN 交付方式，但提高可重复性和离线启动能力。 | 待确认 |
| `DEC-008` | 容器里的终端可以访问什么？ | 终端只运行在 Roaminal 容器内；通过显式 `/workspace` volume 提供工作目录，不挂 Docker socket、不进入宿主机。 | 这是最重要的安全和产品边界之一。 | 待确认 |
| `DEC-009` | Kubernetes 使用 Deployment 还是 StatefulSet，是否允许多副本？ | 单副本 StatefulSet + PVC；MVP 禁止多副本和 HPA。 | PTY 和 WebSocket 是进程本地状态，多副本需要额外路由和调度设计。 | 待确认 |
| `DEC-010` | 是否兼容既有 Tabminal 配置、数据目录、token 和协议？ | 不兼容。Roaminal 是新项目，全部使用新命名空间。 | 若兼容，需要迁移器、双协议和额外安全测试。 | 待确认 |
| `DEC-011` | 默认端口和端口冲突行为是什么？ | 默认继续使用 `9846`；本地 CLI 可自动寻找下一端口，容器/Kubernetes 模式端口冲突直接失败。 | 自动递增在容器中会让 Service/healthcheck 指向错误端口。 | 待确认 |
| `DEC-012` | CORS 是否一比一保留任意 Origin 反射？ | 默认同源；通过显式 `ROAMINAL_ALLOWED_ORIGINS` 开放跨域。 | 这是安全改进，但会改变参考实现的默认 CORS 行为。 | 待确认 |
| `DEC-013` | MVP 支持哪些服务端 OS 和 shell？ | 只承诺 Linux 容器；Bash 必选，Zsh 在镜像安装或用户指定时按参考 hooks 支持；不承诺 Windows PTY。 | 决定条件分支、镜像内容和测试矩阵。 | 待确认 |
| `DEC-014` | 是否原样保留 15 分钟 access、90 天 refresh、30 次失败锁定和 accept-terms？ | 原样保留。 | 决定认证兼容性、运维体验和安全文档。 | 待确认 |
| `DEC-015` | Kubernetes 交付使用普通 YAML、Kustomize 还是 Helm？ | MVP 提供普通 YAML + Kustomize base/example，不做 Helm chart。 | 决定交付文件结构和变量化能力。 | 待确认 |
| `DEC-016` | MVP 是否重构参考实现的前端单文件架构？ | 不做架构重写；先准确裁剪并增加测试，只把已存在的通用模块保留在独立文件。 | 大规模重构会让一比一验收失去稳定基线。 | 待确认 |
| `DEC-017` | UI 要求像素级一比一还是行为与布局一比一？ | 保持 Tabminal 终端 UI 的布局和交互，替换 Roaminal 品牌；不要求因删除功能导致的空白区域像素一致。 | 决定 screenshot 比较方式和品牌设计工作量。 | 待确认 |
| `DEC-018` | 实施 Agent 是否可以自行创建 Git commits？ | 默认不提交，只修改并验证；由调用它的任务显式授权提交策略。 | 影响执行可回滚性和用户工作树管理。 | 待确认 |

## 12. 实施 Agent 的自治规则

文档批准后，实施 Agent 应遵守：

1. 不再对已确认决策发起选择题，直接按结论实施。
2. 不擅自增加文件工作区、Agent、AI、原生客户端或 Kubernetes HA。
3. 不把“顺手重构”当作范围内工作；只有为删除排除功能而必要的解耦可以
   做。
4. 每个 Phase 完成后立即运行 gate，不把所有验证留到最后。
5. 失败时先自行定位和修复；只有出现以下情况才暂停请求人工介入：
   - 参考仓库 commit 不匹配；
   - 已确认决策互相矛盾；
   - 必须扩大权限或安全边界；
   - 无法保留用户已有修改；
   - 外部凭据、域名、镜像仓库或集群访问是完成任务的硬性前提。
6. 不需要真实集群或镜像仓库凭据即可完成本地镜像测试和 Kubernetes
   manifest dry-run；缺少外部发布权限不应阻塞代码交付。
7. 不声明未实际运行的测试为通过；所有未运行项写入实施日志。
8. 最终交付说明只报告结果、验证证据、批准差异和真实残余风险。

## 13. 讨论完成后的文档收口

讨论结束时必须做以下更新，之后本计划才可交给实施 Agent：

1. 将 `DEC-001` 至 `DEC-018` 全部改为 `已确认`，并把最终结论写入正文，
   不只留在表格里。
2. 删除所有相互冲突的“建议”“取决于”和可选表述。
3. 将文档状态从 `Draft` 改为 `Approved`，写入批准日期和参考 commit。
4. 检查范围、阶段任务、API allowlist、测试和 Definition of Done 一致。
5. 为实施 Agent 增加唯一、明确的启动指令，例如：

   ```text
   严格按 docs/plan/mvp/README.md 实施 Roaminal MVP，从 Phase 0 开始，
   连续执行至所有 Definition of Done 满足。除该文档列出的停止条件外，
   不要等待人工确认。
   ```
