# 11 - Terminal Tab 与交互修订需求单

> 状态：Approved / 待实施
> 提出日期：2026-08-07
> 上位文档：[MVP 计划索引](../README.md)
> 对应方案：[12-terminal-tabs-solution.md](./12-terminal-tabs-solution.md)

## 背景

MVP 在 `develop` namespace 完成部署后，真实多终端使用暴露出 Tab 顺序、选中
状态、xterm DOM 生命周期、侧栏收起、操作语义和持久化目录可解释性问题。原
Chrome E2E 只验证单终端 canvas 非空，没有覆盖多 session 的稳定顺序、可见
viewport 身份、Tab 与 session 分离或链接 renderer，因此这些问题没有被 gate
捕获。

本需求单修订 Terminal Tab 的产品语义，并把已确认缺陷转成可自动验收的
contract。除本文明确修订的行为外，01 至 10 模块继续有效。

## 现场审查基线

审查对象：

```text
Git commit: b57b6eeb0691c869ea19b92e1d72cb7127576df9
Kubernetes context/namespace: develop/develop
Service: roaminal:9846
Chrome: project Playwright runner with channel chrome
Viewport used for desktop reproduction: 1440x900
```

浏览器认证使用运行时环境变量注入，不在文档、trace 或截图中记录密码。

## 已确认问题

### RMT-001 - Terminal Tab 顺序持续变化

严重度：High

Playwright 每 500ms 采样 Tab DOM。在同一组 3 个 session 不增不减的情况下，
顺序从：

```text
02c055, ce9b97, 37cbd2
```

变为：

```text
ce9b97, 37cbd2, 02c055
```

随后又恢复。高亮 session ID 没有变化，但高亮项在 Tab 条上的位置随 heartbeat
自动移动，用户感知为 Terminal 自动切换。

代码确认：`internal/terminal.Manager.Summaries()` 直接遍历 Go map；
`AppShell.sync()` 每秒用 heartbeat response 覆盖 `sessions`，前端按数组顺序重绘
全部 Tab。

需求：

- 同一组 session 在没有创建、删除或显式重排时顺序必须保持不变。
- Heartbeat metadata 更新不得改变 Tab 顺序或 active session。
- 后端返回顺序必须确定，前端打开 Tab 的顺序由浏览器视图状态独立维护。

### RMT-002 - 高亮 Tab 与实际可见 Terminal 不一致

严重度：Critical

Playwright 切换 session 后检查 `.terminal-viewport`，发现同一个 viewport 内同时
存在多个 `.xterm` 根节点。一次现场采样为：

```text
viewport: top=112, bottom=874
xterm[0]: top=112, bottom=874
xterm[1]: top=874, bottom=1636
```

旧 xterm 占据唯一可见区域，新 active xterm 被 append 到 viewport 下方。连续
切换后根节点数量从 2 增长到 4。Tab 高亮和 React metadata 已切换，但 canvas
仍显示旧 session。

代码确认：`TerminalRuntime.detach()` 只清空引用，不从旧 host 移除
`terminal.element`；`attach()` 又向同一 React host append 新 runtime DOM。

需求：

- `.terminal-viewport` 在任何时刻最多包含一个直属 `.xterm` 根节点。
- active Tab、页面标题、状态栏 cwd/尺寸、输入目标和可见 xterm 必须属于同一
  session ID。
- 切换、关闭 Tab、heartbeat、React Strict Mode remount 和 WebSocket reconnect
  均不得累积 xterm DOM、observer、timer 或 socket。

### RMT-003 - Tab 不应等于所有 Terminal session

严重度：High / Product change

现状把 heartbeat 中全部 session 渲染成 Tab，并让 Tab 上的 `x` 直接调用
`DELETE /api/sessions/:id`。这使 Tab 同时承担“打开视图”和“服务端运行实体”两种
互相冲突的语义。

修订需求：

- Sidebar 是当前服务端全部 Terminal session 的权威清单。
- Tab 条只展示当前浏览器已经打开的 session 子集。
- 点击 Sidebar 中未打开的 session 时，把它加入 Tab 条并设为 active；已打开时
  只激活，不重复添加。
- 关闭 Tab 只关闭当前浏览器视图，不终止 PTY，不删除 snapshot/metadata，不
  影响其他浏览器 client。
- 关闭 Tab 后再次从 Sidebar 打开，应 attach 到同一 session ID，并通过 snapshot
  和实时 output 继续使用原 Terminal。
- 新建 Terminal 时创建服务端 session，同时在发起创建的浏览器打开并选中 Tab。
- 服务端没有任何 session 时才自动创建初始 Terminal；不能因为当前浏览器没有
  open Tab 就创建新 session。

### RMT-004 - xterm CanvasAddon 兼容错误

严重度：High

用户观测到：

```text
Cannot read properties of undefined (reading 'onShowLinkUnderline')
```

当前部署经 20 次多 Tab 切换没有再次触发同一异常，因此它不是每次 attach 都
必现。代码和依赖审查仍确认存在不受支持组合：

```text
@xterm/xterm              6.1.0-beta.197
@xterm/addon-canvas       0.8.0-beta.48
CanvasAddon peer          @xterm/xterm ^5.0.0
npm ls result             ELSPROBLEMS / invalid
```

报错字段位于该旧 CanvasAddon 的 `LinkRenderLayer`。它依赖 xterm 5 的内部
linkifier/renderer contract；项目其他 addon 均与 xterm 6.1 beta 同发布线。此前
`f103d73` 已把 addon activation 移到 `Terminal.open()` 之后，解决“open 前加载”
生命周期问题，但没有消除 peer dependency 不兼容。

需求：

- `npm ls` 不得报告任何 xterm core/addon invalid peer dependency。
- 不得捕获后忽略 renderer activation 异常；初始化失败必须显示可诊断错误并
  保持应用其他 UI 可操作。
- 链接识别、hover underline、搜索、fit、ligature、progress 在首次 attach、重复
  切换和 reattach 后均无 page error。

### RMT-005 - Sidebar toggle 在桌面无作用

严重度：Medium

Playwright 点击 `Toggle sidebar` 前后记录：

```text
before: class="sidebar open", width=276px, x=0, transform=none
after:  class="sidebar",      width=276px, x=0, transform=none
```

React state 和 class 确实变化，但桌面 media query 没有为 closed 状态定义宽度、
transform 或替代打开按钮；用户看到按钮文字改变，布局完全不变。

需求：

- 桌面点击后 sidebar 从布局中真实收起，terminal viewport 获得释放的宽度。
- 收起后必须在 topbar 留下可见、可聚焦的展开按钮，不能把唯一恢复入口一并
  移出视口。
- 移动端继续使用 overlay，打开时有 backdrop，点击 backdrop、按 Escape 或选择
  session 后关闭。
- toggle 使用明确图标、tooltip、`aria-expanded` 和 `aria-controls`。

### RMT-006 - Terminal 操作需要菜单和重命名

严重度：High / Product change

现状 Tab 和 Sidebar 各有裸 `x` 按钮，点击立即删除服务端 session；没有确认，
也没有重命名 API。服务端目前只支持创建和删除 session，持久化 title 会持续被
shell OSC title 更新。

需求：

- 原 `x` 改为“更多操作”菜单触发器，不再把破坏性操作绑定到单击图标。
- 菜单至少包含：`Rename title`、`Close tab`、`Terminate terminal`。
- `Close tab` 是默认非破坏性动作，不调用后端删除接口。
- `Terminate terminal` 必须明确描述会终止 PTY，并经过确认；确认后才调用删除
  API。
- `Rename title` 使用 modal 或可访问的 inline editor；空白、超长、控制字符和
  非法 UTF-8 必须被拒绝。
- 自定义标题必须跨 heartbeat、reconnect 和 Pod restart 保留，不能被下一次
  shell OSC title 覆盖。
- 用户可以选择 `Use automatic title`，恢复显示 shell OSC title。
- 菜单支持键盘打开、方向键导航、Escape 关闭、外部点击关闭和焦点归还。

## 持久化目录结论

现场目录为：

```text
/home/roaminal/.roaminal/state/sessions/  active, contains metadata/snapshot
/home/roaminal/.roaminal/sessions/        empty, legacy startup artifact
```

PVC mount root 是 root-owned `2777`。为保证敏感文件位于应用所有的 `0700`
目录，当前代码正确选择 `.roaminal/state/` 作为唯一有效 root。根下
`.roaminal/sessions/` 创建于 fallback 修复前的启动尝试，当前进程不读取也不
写入它，因此现场不存在双写或两份有效 session 数据。

要求：

- 启动日志以不含敏感路径内容的方式报告所选 state layout：`direct` 或
  `private-child`。
- 只允许一个 `Store.Root`，所有 auth、metadata、snapshot 和 corrupt 文件必须
  位于同一 root。
- fallback 模式下，根级 `sessions/` 为空时允许安全清理；非空时不得静默删除、
  合并或选择其中一份。
- 备份文档继续要求备份整个 mount root，以覆盖 direct 和 private-child 两种
  layout。

## 验收标准

- 10 个 server session、其中任意 3 个 open Tab 连续运行 60 秒，Tab 顺序和
  active session 不因 heartbeat 改变。
- 在 3 个 Tab 间切换 100 次，viewport 始终恰好一个直属 xterm，输入和输出均
  到达 active session，资源计数不增长。
- 关闭一个 Tab 后 server session 数不变；从 Sidebar 重开后 session ID、cwd 和
  scrollback 不变。
- 重命名跨 heartbeat、刷新和 Pod restart 保留；恢复自动标题后 OSC title 再次
  生效。
- 终止操作取消时 PTY 存活，确认时才删除；其他 session 不受影响。
- 桌面和全部移动 viewport 的 sidebar 打开/关闭、焦点、overlay 与 terminal fit
  通过 Playwright。
- xterm 依赖树无 invalid peer；链接 hover 和重复 reattach 无 console error 或
  page error。
- direct/private-child persistence 单元测试和 Kubernetes PVC 现场检查均证明只
  使用一个有效 root。

## 非目标

- 不增加 workspace/file tabs、pinning、Tab 跨用户同步或多 Host。
- 不在服务端持久化某个浏览器的 open Tab 集合。
- 不恢复 Pod 重启前的 Unix 进程；仍按 `DEC-001` 创建新 Bash。
- 不借此引入 UI component library、第三方 React xterm wrapper 或独立状态库。
