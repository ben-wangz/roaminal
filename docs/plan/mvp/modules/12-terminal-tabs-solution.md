# 12 - Terminal Tab 与交互修订实施方案

> 状态：Implemented / 已落地
> 更新日期：2026-08-07
> 上位文档：[MVP 计划索引](../README.md)
> 输入需求：[11-terminal-tabs-requirements.md](./11-terminal-tabs-requirements.md)

## 方案目标

把“服务端 Terminal session”和“当前浏览器打开的 Tab”拆成两个生命周期，修复
多 Tab 顺序和 xterm DOM 身份问题，补齐持久标题、显式终止菜单和真正可用的
sidebar toggle，并消除 xterm renderer 的无效 peer dependency。

## 状态模型

```text
Heartbeat sessions (server authoritative, all sessions, stable order)
                  |
                  v
sessionsById + sessionOrder
                  |
       Sidebar renders all sessions
                  |
          open/select action
                  v
openTabIds (browser-local ordered subset) ---> Tab strip
                  |
                  v
activeTabId (null or member of openTabIds)
                  |
                  v
one TerminalViewport host <-> one active TerminalRuntime
```

React app state 增加：

```ts
type TerminalViewState = {
  sessionsById: Map<string, SessionSummary>;
  sessionOrder: string[];
  openTabIds: string[];
  activeTabId: string | null;
  sidebarOpen: boolean;
};
```

约束：

- `sessionOrder` 来自后端确定顺序，只用于 Sidebar 和 fallback 选择。
- `openTabIds` 只包含当前 heartbeat 中仍存在的 ID，顺序不被 heartbeat 重排。
- `activeTabId` 必须为 `openTabIds` 成员或 `null`。
- TerminalRuntime、WebSocket、xterm buffer 不进入 React state。
- 当前 Origin 的 localStorage 使用 `roaminal_terminal_tabs_v1`，只保存
  `openTabIds` 和 `activeTabId`；不保存 output、token、title 或 cwd。

## 后端 session 顺序

`Manager.Summaries()` 在释放 manager read lock 后按以下 key 排序：

```text
createdAt ascending, id ascending
```

同一 heartbeat 内每个 summary 先在 session lock 下复制，再统一排序，禁止在持有
manager lock 时长时间等待 session lock。增加单元测试：随机 map 插入顺序循环
100 次，JSON 响应顺序必须相同。

前端 reconciliation 不采用 response array 位置重排已打开 Tab：

1. 删除 `openTabIds` 中服务端已不存在的 ID。
2. 保留其余 ID 的原相对顺序。
3. active ID 已删除时选择被删 Tab 的右邻，否则左邻；都不存在则为 `null`。
4. heartbeat 新出现的 session 只加入 Sidebar，不自动打开。
5. 当前浏览器成功创建的 session append 到 `openTabIds` 并 active。
6. 只有服务端 session 清单为空时触发一次 initial create；`openTabIds` 为空不创建。

把 reconciliation 提取为纯函数并用 table tests 覆盖 stale heartbeat、本地 create
与 heartbeat 交错、delete 交错、重复 ID 防御和空 Tab 非空 session。

## Tab 生命周期

新增显式动作：

```text
openTab(sessionId)       browser-only; dedupe + activate
selectTab(sessionId)     browser-only; requires open
closeTab(sessionId)      browser-only; no HTTP DELETE
terminateSession(id)     server mutation; confirmed HTTP DELETE
```

`closeTab` 执行顺序：

1. 从 `openTabIds` 删除 ID 并同步 localStorage。
2. 如果是 active，按原位置选择右邻或左邻。
3. dispose 并删除该浏览器对应的 TerminalRuntime，关闭 WebSocket 和 observer。
4. 不调用 `/api/sessions/:id`，不改变 Sidebar session 数。

`terminateSession` 在确认后执行：

1. `DELETE /api/sessions/:id`。
2. dispose runtime，清理所有本地 view 引用。
3. 等待下一次 heartbeat 复核删除结果；404 视为已收敛。
4. 服务端清单为空时沿用 initial create guard 创建一个新 Terminal。

`Ctrl+Shift+W` 改为 `closeTab(activeTabId)`。真正终止只通过带确认的菜单操作，
不配置无确认快捷键。

## xterm DOM 与 runtime 生命周期

保持单 viewport host，但建立严格 DOM 所有权：

- `TerminalViewport` 以 `activeTabId` 为 React `key`。
- `runtime.attach(host)` 在 reattach 前从旧 parent 移除自己的
  `terminal.element`，并使用 `host.replaceChildren(terminal.element)`；首次 open
  后同样断言 host 只有一个直属 `.xterm`。
- `runtime.detach(host)` 只在 `terminal.element.parentElement === host` 时移除
  DOM，并断开该 host 的 ResizeObserver。
- Tab 间切换只 detach/reattach，保留已打开 Tab 的 buffer/socket；关闭 Tab 或
  终止 session 才 dispose runtime。
- `attach`、`detach`、`dispose` 全部幂等；dispose 后 attach 明确失败而非静默
  创建半初始化 runtime。
- 每个 callback 捕获所属 session ID；input、resize、search 和 touch keyboard
  只读取当前 active runtime，不持有旧 render closure。

开发模式增加断言或测试 helper：

```text
viewport direct .xterm count <= 1
viewport data-session-id == activeTabId
runtime.element.parentElement is null or current host
```

Playwright 为不同 session 写入唯一 marker，并通过 xterm accessibility/text
fixture 或可测试 runtime snapshot 断言 marker 与 active ID 匹配；不能只断言
canvas 非空或 CSS active class。

## xterm renderer 依赖修复

浏览器端继续采用当前锁定的 xterm 6.1 beta 发布线，但删除
`@xterm/addon-canvas@0.8.0-beta.48` 和 `new CanvasAddon()`。xterm 6 core 自带受支持
renderer；MVP 不依赖 xterm 5 CanvasAddon 的内部 API。fit、web-links、search、
progress 和 ligatures 保持与 core 完全相同的 beta build 编号，lockfile 重新生成。

依赖 gate：

```bash
npm --prefix web install
npm --prefix web ls @xterm/xterm @xterm/addon-fit \
  @xterm/addon-web-links @xterm/addon-search \
  @xterm/addon-progress @xterm/addon-ligatures
```

命令必须退出 0 且没有 `invalid`。E2E 输出一条 URL，hover 链接并移出，验证
underline 生命周期；首次 attach、100 次切换、刷新和重新打开 Tab 全程收集
`pageerror` 与 `console.error`，业务相关错误数组必须为空。

若 core renderer 无法满足现有 canvas pixel smoke，先把 E2E 改为同时支持 core
DOM renderer 的实际可见像素/文本断言，不能为保留测试实现而继续使用不兼容
addon。不得通过 `try/catch` 忽略 `onShowLinkUnderline`。

## 标题模型与 API

### 数据模型

把 shell 自动标题与用户自定义标题分开：

```text
automaticTitle  latest accepted OSC title
titleOverride   null or user title
effectiveTitle  titleOverride ?? automaticTitle
```

`SessionSummary.title` 继续返回 effective title，并新增：

```ts
titleMode: "automatic" | "custom"
```

避免把 renderer/UI 逻辑绑定到持久化字段名。OSC parser 始终更新
`automaticTitle`；custom 模式下它不改变 effective title，清除 override 后立即
显示最新 automatic title。

### HTTP contract

新增：

```text
PATCH /api/sessions/:id/title
Authorization: Bearer <access token>
Content-Type: application/json

{ "title": "custom title" }
{ "title": null }              // restore automatic title
```

成功返回 `200 SessionSummary`。未知字段、缺字段和错误类型返回 `400`，未知 session
返回 `404`。自定义 title：trim 后 1 至 128 Unicode scalar values、最多 512 UTF-8
bytes，不允许 C0/C1 control、DEL、bidi override/isolate 或换行。服务端执行全部
验证，前端验证只用于即时反馈。

### 持久化迁移

Session metadata 升级为 `formatVersion: 2`：

```json
{
  "formatVersion": 2,
  "automaticTitle": "shell title",
  "titleOverride": "custom title or null"
}
```

其余 v1 字段保持。启动时严格读取 v1/v2：

- v1 `title` 迁移为 v2 `automaticTitle`，`titleOverride: null`。
- 首次成功 checkpoint 原子写为 v2。
- v2 仍拒绝未知字段和非法 title。
- 不原地覆盖损坏文件；沿用 corrupt quarantine contract。
- 回滚到只理解 v1 的旧 image 不承诺读取 v2，部署前必须完成 PVC snapshot。

title PATCH 在返回 200 前完成 metadata 原子持久化；写入失败返回 500 并保持旧
内存值，避免 UI 显示未落盘标题。

## 操作菜单

Tab 和 Sidebar 共用 session action menu，但根据上下文显示：

```text
Rename title...          all sessions
Use automatic title     custom mode only
Close tab               open tabs only, non-destructive
Terminate terminal...   all sessions, destructive
```

触发器使用竖向三点图标和 `aria-label="Terminal actions"`。菜单采用语义化
`role=menu/menuitem` 或等价的可访问 popover，支持 Enter/Space 打开、方向键、
Home/End、Escape、外部点击和焦点恢复。菜单不能嵌套在 `<button>` 内；当前
`terminal-tab` 需要改为容器加独立选择按钮和菜单按钮，消除 nested interactive
content。

终止确认 dialog 显示 effective title 和短 session ID，不显示 terminal output。
确认按钮使用明确文案 `Terminate terminal`，不使用含糊的 `Close`。

## Sidebar 行为

桌面 `>800px`：

- open 时固定宽度 276px。
- closed 时宽度和 flex-basis 为 0、内容不可聚焦且不参与 pointer events。
- topbar 左侧显示 sidebar open icon；展开后焦点返回 sidebar header toggle。
- transition 完成后调用 active runtime fit，一次即可，禁止 ResizeObserver 循环。

移动/平板 `<=800px`：

- sidebar 作为 fixed overlay，closed 时 translateX(-100%)。
- open 时显示 backdrop，锁定背景 pointer interaction，但不破坏 xterm buffer。
- Escape、backdrop click、选择 session 后关闭；焦点被限制在 sidebar，关闭后
  回到触发器。

所有 viewport 设置 `aria-expanded`、`aria-controls="terminal-sidebar"` 和一致的
tooltip。图标使用项目既有图标策略；实现时若引入 icon dependency，必须先更新
依赖模块和 lockfile，不能手写不一致 SVG。

## 持久化 root 收敛

`persistence.New` 返回并记录结构化 layout：

```go
type Layout string
const (
    LayoutDirect       Layout = "direct"
    LayoutPrivateChild Layout = "private-child"
)
```

规则：

1. mount root 可安全设为 `0700` 时使用 direct：`.roaminal/sessions/`。
2. mount root 有 world permission 且应用可写时使用 private-child：
   `.roaminal/state/sessions/`。
3. private-child 模式发现根级 `sessions/` 为空，只记录可清理状态；应用启动路径
   不因清理失败而失败。
4. 两处任一存在 metadata/snapshot/auth 文件时先分类；如果两个候选 root 均有
   有效或未知文件，返回 `ambiguous state layout` 并非零退出。
5. 不在普通启动中自动迁移非空目录。另提供文档化停机迁移步骤：PVC snapshot、
   校验、同文件系统 rename、fsync、重启和恢复验证。

当前 `develop` 的根级 `sessions/` 为空，可在停机维护时删除；活动数据只在
`state/`。`.probe` 是验证遗留文件，也只可在确认非产品文件后由部署维护清理，
应用不得通过模糊文件名规则删除 mount root 内容。

## 实施顺序与原子提交

1. `fix(terminal): stabilize session summary ordering`
2. `fix(web): separate terminal tabs from server sessions`
3. `fix(web): enforce single active xterm viewport`
4. `fix(web): remove incompatible canvas addon`
5. `feat(terminal): persist custom session titles`
6. `feat(web): add terminal action menu`
7. `fix(web): implement responsive sidebar toggle`
8. `fix(persistence): detect ambiguous state layouts`
9. `test(web): cover multi-terminal view lifecycle`
10. `docs(mvp): record terminal interaction validation`

可以在实现中按实际依赖合并紧密耦合项，但每个 commit 仍必须满足 `DEC-018`，
不得把无关后端、UI、部署和文档变更混在一个提交。

## 测试矩阵

### Go

- Summary 排序确定性和并发读。
- PATCH title strict schema、validation、atomic rollback 和 404。
- OSC title 在 automatic/custom 两种模式下的 effective title。
- v1 到 v2 migration、v2 strict decode、corrupt quarantine。
- direct/private-child/empty legacy/ambiguous layouts。

### Vitest / React

- session reconciliation 不重排 open tabs。
- open/select/closeTab/terminate 四种动作边界。
- localStorage stale ID 清理和 active fallback。
- attach/detach/dispose 幂等及 Strict Mode resource counts。
- 菜单键盘、焦点、rename validation 和确认取消。
- desktop/mobile sidebar state transitions。

### Playwright Chrome

- 10 sessions、3 open tabs、60 秒 heartbeat 顺序稳定。
- 为每个 session 写唯一 marker，100 次切换后 active ID、状态栏、可见内容和输入
  目标一致，直属 xterm 数恒为 1。
- Close tab 后 heartbeat session 数不变，Sidebar 重开恢复相同 ID/scrollback。
- Terminate cancel/confirm 两条路径和相邻 Tab fallback。
- Rename 跨 heartbeat、reload；Kubernetes case 再覆盖 Pod restart。
- URL link hover/leave 不产生 `onShowLinkUnderline` 或其他 page error。
- 1440x900、1024x768、768x1024、390x844、844x390 全部验证 sidebar、菜单、
  modal 无重叠和焦点可达。

### Podman / Kubernetes

- read-only root、UID/GID 1000 和两种 state layout。
- `develop` PVC 中只有一个 active root；空 legacy 目录清理前后数据不变。
- rollout 前 PVC snapshot；v1 metadata 升级 v2 后 session ID、scrollback、cwd、
  automatic title 不变。
- Pod delete/recreate 后 custom title 和 automatic mode 均正确恢复。

## Definition of Done

- [ ] RMT-001 至 RMT-006 的验收标准全部自动化并通过。
- [ ] Tab close 不发送 DELETE，Terminate confirm 才发送 DELETE。
- [ ] 多 Tab 运行期间顺序稳定，active metadata 与唯一可见 xterm 一致。
- [ ] xterm dependency tree 无 invalid peer，链接 hover 无 page error。
- [ ] 自定义 title 使用 versioned schema 原子持久化并可恢复 automatic title。
- [ ] Sidebar 在桌面和移动端均真实可开关且符合键盘/ARIA contract。
- [ ] persistence 只选择一个 root，ambiguous layout 不静默处理。
- [ ] Go race/unit、frontend unit/typecheck/lint/build、五 viewport Chrome E2E、
  Podman smoke 和 `develop` rollout/recovery 全部通过。
- [ ] implementation log 记录命令、版本、截图/trace 路径和真实残余风险。
