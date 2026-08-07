# 13 - 单终端视图、Sidebar 对齐与 MVP 缺口修复

> 状态：Approved
> 批准日期：2026-08-07
> 上位文档：[MVP 计划索引](../README.md)
> 行为参考：`/root/temp/Tabminal`，tag `v3.0.40`，commit
> `fbd26d3aff033fd850a6696eccb107520780fd8b`

## 目的

本模块记录 MVP 上线后的第二轮真实使用反馈、参考实现对照结果、代码审查发现的
既有缺口和完整修复方案。它不是未来路线图；本文列入 Definition of Done 的项目
全部属于当前 MVP 的修复范围。

本文明确取代此前所有“浏览器维护多个已打开 Tab”“顶部展示 Terminal Tab 条”及
“关闭 Tab”的 contract。服务端仍保留多个独立 session，但浏览器任何时刻只显示
一个 active session；用户直接通过 Sidebar 切换。

## 核验基线

### Roaminal 已部署版本

2026-08-07 通过
`http://roaminal.develop.svc.cluster.local:9846` 直接访问 `develop` namespace 中的
Service，核验镜像 commit
`2e7bbdcead1e56e6c6a1a0e0c329ec19b4c54715`：

- 服务端有 3 个 session，顶部只显示 1 个浏览器 Tab，Sidebar 显示全部 3 个。
- 点击第二个 Sidebar session 后，唯一 `.terminal-viewport` 的
  `data-session-id` 正确切换到第二个 session，直属 `.xterm` 始终为 1。
- 每个 Sidebar item 只有标题、cwd 和三点菜单；没有 ID、创建时间或独立扩展入口。
- Sidebar item 内 `.xterm` 数量为 0；hover 前后 DOM 和视觉均无缩略图变化。
- 普通 HTTP Service origin 不是 secure context；未配置 Chrome 测试例外时，登录
  报 `Cannot read properties of undefined (reading 'digest')`，来源是
  `crypto.subtle` 不存在。

### Tabminal 参考版本

同时通过 `/root/temp/Tabminal` 源码和运行中参考 Service 核验：

- 每个 Sidebar item 包含 `ID`、`HOST`、`PWD`、`SINCE`；Roaminal 是单实例，
  因此不复制 `HOST`。
- 桌面 item 内存在独立 preview xterm；hover 后信息 overlay 向下移动并露出实时
  缩略终端。实测 transform 从 `none` 变为向下约 97px。
- 文件、Agent 和 terminal lifecycle 是三个独立图标入口，不是一个三点菜单承载
  所有操作。
- 参考项目仍只作为行为和交互 oracle；不得复制其单文件实现、源码或资产。

## 新增需求

### RMT-007：取消 Terminal Tab

验收标准：

1. 页面不存在顶部 Terminal Tab 条、Tab 列表、Tab 新建按钮或 Close Tab 操作。
2. 前端状态只保存 `activeSessionId`，不保存 `openTabIds`。
3. 点击 Sidebar session 后，标题、状态栏、搜索、触控输入和唯一主 xterm 同时切换
   到该 session。
4. 切换 session 只释放浏览器 runtime/WebSocket，不删除服务端 session。
5. session 清单非空时始终选择一个 active session；只有权威清单为空时才创建
   一个新 session。
6. `Ctrl+Shift+W` 和“关闭 Tab”菜单项删除；终止 terminal 仍只能通过带确认的
   destructive action。

### RMT-008：Sidebar session 信息完整

每个 session card 必须显示：

```text
effective title
ID: <stable short id>
PWD: <current cwd>
SINCE: <createdAt in browser local time>
```

- short ID 固定取 UUID 最后一个 12 字符段，完整 ID 放在 `title`/accessible
  description 中。
- PWD 视觉上允许中间省略，但 DOM 和 tooltip 保留完整绝对路径；不得只显示目录
  basename。
- SINCE 使用 `<time datetime="RFC3339">`，显示格式固定为
  `MM-DD hh:mm AM/PM`，并随浏览器 locale timezone 计算。
- title、ID、PWD、SINCE 的尺寸必须稳定，长路径和长标题不得撑宽 Sidebar 或遮挡
  action buttons。

### RMT-009：桌面 hover 实时缩略终端

1. 仅在 `pointer: fine` 且宽度大于 800px 时启用 hover preview；触控布局不创建
   preview xterm。
2. card 默认显示信息 overlay；hover 100ms 后 overlay 下滑，露出该 session 的
   只读 xterm 缩略图；pointer leave、Sidebar 关闭或 session 删除时立即清理。
3. preview 使用 `scrollback: 0`、`disableStdin: true`、隐藏 cursor，不加载 search、
   links、progress 或 ligatures addon。
4. 全页面同时最多存在一个 preview runtime 和一个 preview WebSocket。快速跨卡
   hover 必须通过 generation token/AbortController 防止旧 preview 回挂。
5. preview attach 先接收权威 snapshot，再接收 live output；不得复用主 xterm DOM，
   不得发送 input、resize 或 `claim_terminal_control`。
6. preview 容器使用稳定 aspect ratio 和 overflow clipping；hover 不改变 card、
   session list 或主 terminal 的几何尺寸。
7. `prefers-reduced-motion: reduce` 时取消滑动动画但保留内容切换。

### RMT-010：独立操作轨与扩展入口

session card 使用三个独立图标按钮：

```text
[Agent extension] [Files extension] [Terminal actions]
```

- 使用 `lucide-react` 的 `Bot`、`FolderOpen`、`EllipsisVertical`，并同步把新建、
  搜索和 Sidebar 开关替换为同一图标体系；不手写新的 SVG 或继续使用字符图标。
- `Terminal actions` 菜单只包含 `Rename title...`、custom 模式下的
  `Use automatic title` 和 `Terminate terminal...`。
- Agent/Files 是 core 提供的固定 extension affordance，不把 Agent、AI、文件树、
  文件 API 或 workspace state 实现在 core 中。
- 本轮没有已注册插件时两个入口仍显示；设置 `aria-disabled="true"`，点击或键盘
  激活显示 `Agent extension unavailable` / `Files extension unavailable` toast，
  tooltip 使用同一文案。不得显示空 panel 或发起不存在的 API 请求。
- 以后只有经单独批准的 extension capability contract 才能把入口切换为 available；
  core 不通过猜测全局变量、探测私有 endpoint 或内置插件代码来启用它们。

该项只取代“UI 中不得出现 Agent/Files 入口和占位”的旧条款。`DEC-002` 的无 AI
core、文件 workspace/API 排除项和插件代码独立分发边界继续有效。

### RMT-011：active session 生命周期

前端状态模型固定为：

```ts
type SessionView = {
  activeSessionId: string | null;
};
```

reconcile 规则：

1. 当前 ID 仍在 heartbeat 清单中时保持不变，不受清单重新排序影响。
2. 当前 ID 消失时，选择其原稳定顺序中的下一项，否则上一项，否则第一项。
3. 初次加载优先恢复 `roaminal_active_session_v1`；不存在时可从旧
   `roaminal_terminal_tabs_v1.activeTabId` 单次迁移，随后删除旧 key。
4. 用户选择新 session 时先更新 active ID，再 dispose 旧 main runtime，最后创建
   并 attach 新 runtime；异步 callback 必须捕获 session ID 并拒绝陈旧回调。
5. 浏览器最多持有一个 main runtime；刷新或切换依赖服务端 snapshot 恢复视图。
6. terminate active session 后按规则 2 选择 fallback；terminate 非 active session
   不干扰主 viewport、搜索状态或焦点。

## Sidebar 目标结构

```text
┌──────────────────────────────────┐
│ ● terminal title       [A][F][⋮] │
│ ID: d04611064326                 │
│ PWD: /workspace/project          │
│ SINCE: 08-07 11:39 AM            │
│                                  │
│  hover reveals read-only xterm   │
└──────────────────────────────────┘
```

card 自身不是一个包含其他 button 的 `<button>`。选择面使用独立 button 或可访问
link-like control，action rail 是 sibling，避免 nested interactive content。整个
card 支持 active、hover-preview、attention、terminated 和 disconnected 状态；
颜色不是唯一状态信号。

## 既有 MVP 缺口审查

下表区分“实现缺口”和“证据缺口”。仅有 implementation log 的手工结论不能替代
自动化测试。

| ID | 严重度 | 类型 | 已批准 contract | 当前证据 | 修复结论 |
| --- | --- | --- | --- | --- | --- |
| `MVP-GAP-001` | P0 | 实现 | `DEC-006` 桌面实时 preview | `TerminalPreview` 未被引用，Sidebar 内无 xterm | 按 RMT-009 实现 lazy preview runtime |
| `MVP-GAP-002` | P0 | 实现 | 浏览器 logout 撤销当前 refresh session；支持查看/撤销登录会话 | `signOut()` 只清 localStorage，auth session API 无 UI | 先调用 `/api/auth/logout`，增加登录会话管理 dialog 及 revoke/logout-others 流程 |
| `MVP-GAP-003` | P0 | 安全/部署 | Chrome 可完成登录；非 localhost 生产入口使用 TLS | HTTP Service origin 上 `crypto.subtle` 为 undefined，错误不可理解 | 登录前检查 secure context；非安全 origin 显示明确 TLS 错误，不引入纯 JS crypto 降级 |
| `MVP-GAP-004` | P0 | 实现 | 多 client query-response owner/claim | WebSocket `claim_terminal_control` 分支为空，input 不检查 owner | session 保存 control owner；main viewport 显式 claim，preview 永不 claim，非 owner input 被忽略 |
| `MVP-GAP-005` | P0 | 实现 | capacity 在 upgrade 前返回 429；slow client 以 1013/`slow_client` 关闭 | 当前先 upgrade 后以 1013 `client capacity reached` 关闭；queue close 未向 socket 传递 slow reason | 增加 attach reservation；writer 传播 typed close reason 并 cancel reader |
| `MVP-GAP-006` | P0 | 实现 | worker 16 MiB queue、10s stall、精确 engine version、恢复有序 | Go 直接同步写 pipe且不校验 engine/addon version；restore 在 Bash readLoop 启动后创建 worker state | 增加 bounded writer loop/deadline/version gate；先恢复 worker，再发布并读取 PTY，失败完整回滚 |
| `MVP-GAP-007` | P1 | 实现 | 完整 system status、延迟、attention、toast、通知、persistence warning | UI 只显示 connected/hostname/count；execution、notification、degraded 字段未接线 | 实现 status strip/details、heartbeat latency ring、execution attention 和有生命周期的 toast/Notification |
| `MVP-GAP-008` | P1 | 实现 | main/shadow 使用相同 `scrollbackLines` | 浏览器 `TerminalRuntime` 固定 `scrollback: 1000` | 在 authenticated bootstrap/runtime config 返回有效值，构造 main runtime 时注入 |
| `MVP-GAP-009` | P1 | 实现 | 触控 modifier、visualViewport、完整快捷键 | SHIFT/SYM 缺失，CTRL/ALT 是固定字节；`viewport.ts`、`shortcuts.ts` 未接线，切换/帮助快捷键未实现 | 建立 modifier state machine、visualViewport subscription、shortcut registry/help dialog |
| `MVP-GAP-010` | P1 | 实现 | 最近活动 cwd 继承、并发 capacity、degraded 恢复 | cwd 从 Go map 任取；capacity check/create 非原子；degraded flag 只置位不清除 | 使用确定性的 activity 排序和 create reservation；按 session dirty failure set 清除 degraded |
| `MVP-GAP-011` | P0 | 证据 | worker/Go/API/React/Chrome contract matrix 自动化 | worker 只有 1 条 fixture test；Go 无 server tests；React 只有 6 条纯函数测试；E2E 大量 project skip | 补齐下述测试矩阵；未覆盖项不得继续记为 passed |
| `MVP-GAP-012` | P1 | 文档 | 当前 protocol/schema/deploy 与实现一致 | API allowlist 未列 title PATCH，03 仍展示 v1 schema，07-09 仍要求 port-forward，旧 DoD 仍写多 Tab/无扩展入口 | 实现时同步修订 03-09 和 implementation log；高编号决策优先 |

## 修复方案

### 前端 active-session 架构

- 删除 `terminal-tabs.tsx` 及全部 `.terminal-tabs`/`.terminal-tab*` CSS。
- 把 `session-view.ts` 改为 active-session reconciliation，不保留“打开集合”。
- `AppShell` 只持有 `activeRuntimeRef`；切换、登出、boot ID 变化和 unmount 都执行
  幂等 dispose。
- session 新建成功后直接成为 active；Sidebar 是唯一 session navigation。
- 搜索只绑定 active runtime；切换时关闭搜索，避免搜索 addon 引用旧 runtime。
- 页面标题、statusbar 和 Sidebar `aria-current` 从同一个 `activeSessionId` 派生。

### Preview runtime

新增独立 `TerminalPreviewRuntime`，不从 `TerminalRuntime` 继承：

- 只实现 attach、snapshot/output apply、fit 和 dispose。
- 与 main runtime 共用 URL/auth token builder 和严格 protocol parser，但不共用
  xterm 实例、DOM、socket 或 ResizeObserver。
- Sidebar 只在 hover intent 成立时通知 app 层打开 preview；app 层保证单例。
- preview 使用现有 WebSocket attach contract，但从不发送 claim/input/resize；由于
  control owner 初始为 null 且只能显式 claim，它不会取得 terminal control。
- preview error 只在 card 内显示 compact unavailable state，不覆盖主 terminal toast。

### Extension affordance

本轮 core 只定义 UI 状态：

```ts
type ExtensionAvailability = {
  agent: 'unavailable';
  files: 'unavailable';
};
```

不得提前设计专有插件加载器、传输协议或 API。未来启用能力时必须另立方案，明确
进程/iframe 边界、鉴权、版本、权限、CSP、失败隔离和许可证；在此之前两个入口只
提供一致、可访问、可测试的 unavailable feedback。

### 认证与 direct Service 验证

- `auth-crypto.ts` 在发 challenge 前检查 `window.isSecureContext` 和
  `crypto.subtle`；失败信息固定为 `Secure HTTPS context required`，不得暴露
  JavaScript property error。
- 不提供纯 JavaScript SHA/HMAC fallback。HTTP 下即使能生成 proof，access token、
  refresh token 和 terminal traffic 仍是明文，fallback 会制造虚假安全感。
- production 文档继续要求 HTTPS。`develop` 自动化可直接访问 HTTP Service，并
  仅在测试 Chrome 启动参数中把该精确 origin 标记为 secure；该例外不得进入应用
  runtime 或 production manifest。
- Playwright base URL 改由 `ROAMINAL_E2E_BASE_URL` 注入；Kubernetes gate 固定
  使用 `http://roaminal.develop.svc.cluster.local:9846`，后续不使用 port-forward。
- `signOut()` 在清本地 token 前调用幂等 logout；网络失败时明确提示“local sign-out
  completed, server session may remain”，并仍清理本地 secret。

### Backend/worker hardening

- 为 WebSocket attach 增加 reservation，容量判断、reserve、accept、commit/release
  必须在并发下不超限。
- session 的 control owner 初始为 null；main viewport 在 WebSocket open、terminal
  focus 和 pointer activation 时发送 `claim_terminal_control`，服务端原子转移 owner。
  preview 永不 claim，owner detach 后回到 null，其他 main client 必须显式 claim。
  非 owner 的 input 不写 PTY；上述路径全部加入 race tests。
- client close channel 携带 code/reason，slow queue 关闭必须终止 reader/writer
  goroutine；不得泄漏连接。
- worker client 使用唯一 writer goroutine、16 MiB payload budget 和 10s stall
  deadline；control request 统一 30s，hello 5s。
- ready 必须精确校验 protocol、`xterm-headless 5.3.0` 和
  `xterm-addon-serialize 0.11.0`。
- restore 顺序固定为：验证 snapshot -> 创建/恢复 worker shadow -> spawn Bash ->
  publish session -> start PTY read/wait loops。任一步失败都关闭已创建的 worker
  session、PTY 和进程，不把半初始化 session 留在 map。
- create 使用 reservation 计数；cwd 继承按 `updatedAt, createdAt, id` 确定性选择，
  不遍历 map 任取。
- persistence degraded 使用失败 session 集合；成功 checkpoint 清除对应失败项，
  集合为空才恢复 healthy。

## 实施顺序与原子提交

1. `docs(mvp): approve single-terminal sidebar remediation`
2. `refactor(web): replace terminal tabs with active session state`
3. `feat(web): add terminal session cards and action rail`
4. `feat(web): add lazy terminal sidebar previews`
5. `fix(auth): complete browser session lifecycle`
6. `feat(web): complete status input and shortcut contracts`
7. `fix(terminal): enforce client ownership and capacity`
8. `fix(worker): bound IPC writes and validate runtime versions`
9. `fix(terminal): make restore and session creation atomic`
10. `test(mvp): cover approved protocol and browser contracts`
11. `docs(mvp): reconcile contracts and validation evidence`
12. `deploy(develop): roll out single-terminal sidebar`

紧密耦合的测试可与对应实现同 commit；不同领域不得为减少 commit 数而混合。当前
工作树中的许可证/致谢变更必须保持独立，不得夹带进功能 commit。

## 测试矩阵

### Vitest / React

- 旧 Tab localStorage 单次迁移、active ID 稳定和删除 fallback。
- 快速切换 100 次后只有一个 main runtime、一个直属 xterm 和一个 WebSocket。
- card 的 ID/PWD/SINCE formatting、长文本 clipping 和完整 accessible name。
- hover intent、快速跨卡、pointer leave、Sidebar close 和 unmount 的 preview
  创建/取消/清理；全局 preview runtime 峰值为 1。
- Agent/Files unavailable button 的鼠标、键盘、ARIA、tooltip 和 toast。
- terminal action menu 无 Close Tab，rename/reset/terminate 行为不回归。
- execution attention、notification fallback、persistence warning 和恢复 toast。
- touch modifier state machine、visualViewport resize 和 shortcut registry。
- auth secure-context guard、logout 成功/网络失败、本地 secret 清理及 session revoke。

### Go / worker

- claim 原子转移、owner disconnect fallback、非 owner input、preview read-only 和 race。
- capacity 并发 reservation 在 upgrade 前返回 429；slow client 收到
  1013/`slow_client`，goroutine 可回收。
- 16 MiB worker queue 边界、10s stall、control timeout、异常 exit、版本不匹配和
  fatal protocol error。
- worker 全 operation、UTF-8 任意分块、sequence 重复/跳号/倒序、snapshot barrier、
  golden 宽字符/组合字符/颜色/alternate buffer/mouse modes。
- restore 期间 Bash 早期 output、worker restore 失败回滚、并发 maxSessions、确定性
  cwd 继承和 persistence degraded 恢复。
- API strict schema/status/correlation ID、auth session lifecycle、WebSocket attach
  顺序和完整 login -> PTY -> reconnect integration flow。

### Playwright Chrome

- 使用 1440x900、1024x768、768x1024、390x844、844x390 五个 viewport。
- 所有 viewport 断言 `.terminal-tabs` 数为 0，session 非空时主 viewport 为 1。
- 创建至少 3 个 session，从 Sidebar 切换 100 次；active card、document title、
  statusbar、viewport ID、可见 marker 和输入目标一致。
- 桌面 hover 每个 card：overlay 发生预期状态变化、preview xterm 像素/文本非空、
  pointer leave 后 preview socket/runtime 清零；移动端 preview xterm 恒为 0。
- 三个 action 按钮布局不重叠；Agent/Files unavailable feedback 正确；菜单只包含
  rename/reset/terminate。
- 终止 cancel/confirm、rename/reload、Sidebar desktop/mobile、search、touch keyboard、
  auth sessions 和 logout 全流程。
- 收集 `pageerror`、`console.error`、failed request、WebSocket close 和外部资源；
  业务错误数组必须为空。
- 每个 viewport 保存 screenshot；hover preview 额外保存 before/after screenshot，
  trace 路径写入 implementation log。

### Podman / Kubernetes

- 全量 Go race、worker、frontend 和 Chrome gate 通过后才构建镜像。
- Podman read-only root smoke 覆盖登录、Sidebar 切换、preview attach、PTY 和 logout。
- 唯一 Git SHA image push 后，在 `develop` namespace 做 server-side dry-run 和实际
  rollout。
- Kubernetes Chrome E2E 直接使用 Service DNS，不启动或依赖任何 port-forward。
- Pod recreate 后 session ID、PWD、SINCE、title 和 scrollback 恢复；新 Bash 状态
  不伪装成旧进程。

## Definition of Done

- [ ] RMT-007 至 RMT-011 全部自动化并通过。
- [ ] 顶部 Tab 条、openTabIds、Close Tab、Tab CSS/测试/localStorage 主模型全部删除。
- [ ] Sidebar 每个 session 显示准确 ID、PWD、SINCE，并可直接切换唯一主 terminal。
- [ ] 桌面 hover preview 是真实只读 xterm，移动端不创建，且全局最多一个 preview。
- [ ] Agent、Files、Terminal actions 三个入口视觉和交互一致；core 不包含 Agent/文件
  功能，unavailable 状态明确。
- [ ] `MVP-GAP-001` 至 `MVP-GAP-012` 均关闭，并有自动化证据。
- [ ] auth logout/session management、secure-context 错误、control owner、capacity、
  slow client、worker queue/version 和 restore 原子性 contract 全部通过。
- [ ] status/attention/notification、configured scrollback、touch/viewport/shortcut 和
  persistence degraded 恢复完成。
- [ ] Go race/unit/integration、worker、frontend unit/typecheck/lint/build、五 viewport
  Chrome、Podman 和 `develop` rollout/recovery 全部通过。
- [ ] Kubernetes E2E 直接访问 Service 地址，不使用 port-forward。
- [ ] 03 至 09 模块、API/部署/安全文档和 implementation log 已与最终行为一致，
  不再把未覆盖项或被 skip 的 case 记为通过。
