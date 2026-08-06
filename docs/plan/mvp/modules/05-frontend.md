# 05 - 浏览器前端

> 状态：Approved
> 上位文档：[MVP 计划索引](../README.md)

## 模块边界

前端功能与参考实现对齐，但不复用其 `public/app.js` 单文件结构。前端固定使用
React 19 + TypeScript 7 + Vite 8：

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

- 领域模块不得通过任意全局变量隐式耦合；只有 `app` 层负责跨领域编排。
- Session 数据以 `sessionId` 为一级 key。
- UI 模块不直接调用 `fetch` 或创建 WebSocket；网络模块不直接操作 DOM。
- xterm 实例、WebSocket、ResizeObserver、heartbeat timer 和 PTY output 都由
  React tree 之外的 `TerminalRuntime` 管理。
- 高频 PTY output 直接调用 `terminal.write()`，不得进入 UI render state。
- Session 在 UI 重挂载时只 detach/reattach DOM；只有显式关闭 session 才
  dispose terminal runtime 和 WebSocket。
- React state 只保存低频、可序列化 UI/session metadata，不保存 PTY bytes、
  xterm instance、WebSocket 或 mutable terminal buffer。
- `TerminalViewport` 通过 ref attach/detach 已存在的 runtime；component
  unmount 不等于关闭 session。
- 直接使用 xterm.js API，不使用第三方 React wrapper。
- 保持 React Strict Mode；effect 必须对 socket、listener、observer、timer
  和 DOM attach 做幂等 setup/cleanup。
- 不引入 Redux、Zustand、UI component library 或其他独立状态管理层；应用级
  状态使用 React state/reducer/context，外部 runtime snapshot 订阅使用
  `useSyncExternalStore`。
- 不创建与当前功能无关的抽象层。

## 终端体验

- Solarized Dark 风格、Monaspace Neon 字体和紧凑工作界面。
- xterm.js 及 fit、web-links、canvas、search、progress、ligatures addons。
- 标签展示标题、短 session ID、cwd、创建时间、运行/完成/attention 状态和
  可用时的终端进度。
- 桌面保留标签内的实时终端缩略预览；移动尺寸不创建预览。
- 创建、切换和关闭终端；关闭当前项后选择相邻项。
- Sidebar 展开/收起和移动端 overlay。
- 页面标题跟随当前终端。
- 搜索支持大小写、全词、正则、上一个和下一个结果。
- 浏览器复制粘贴和 xterm 原生选择。
- 触控设备保留 ESC、TAB、CTRL、ALT、SHIFT、SYM、方向键和软键盘。
- 正确处理 `visualViewport`、safe area 和软键盘高度变化。
- 不出现文件、workspace 或 Agent 入口和占位。

快捷键：

```text
Ctrl + Shift + T        新建终端
Ctrl + Shift + W        关闭终端
Ctrl + Shift + [ / ]    切换终端
Ctrl/Cmd + F            终端内搜索
Ctrl + Shift + ?        快捷键帮助
```

## 单实例多 Terminal Tab

- 浏览器只连接提供当前页面的 Roaminal 实例；HTTP 使用当前 Origin，WebSocket
  从页面协议派生 `ws://` 或 `wss://`。
- 不存在 Host entity、`hostId`、Host registry、Host picker 或跨 Host 聚合状态。
- 一个 Terminal Tab 对应一个 session ID 和一个独立 Bash PTY。
- 支持创建、切换和关闭多个 Tab；关闭当前 Tab 后选择相邻 Tab。
- session 清单为空时前端创建一个 Tab；关闭最后一个 Tab 后再创建一个。
- 同一 session 允许多个浏览器客户端 attach，属于协同连接，不是多 Host。
- 保留同源 Cloudflare Access 会话失效检测；需要重新认证时刷新或打开页面根
  URL。后端不启动 Tunnel，镜像不包含 cloudflared。
- `/api/cluster`、`cluster.json`、Host URL normalize/dedupe 和 Host-scoped
  localStorage 全部排除。

## 状态、提示与通知

- 保留当前服务 heartbeat 延迟图和连接状态。
- 保留 hostname、kernel、IP、CPU、内存、host uptime、Roaminal process
  uptime、FPS、延迟和 session 数。
- Linux system monitor 使用 `/proc`、cgroup v2 和 Go runtime 获取容器有效
  资源数据，不引入通用跨平台监控库。
- 保留连接丢失、恢复和终端退出 toast。
- Snapshot checkpoint 失败时 heartbeat `runtime.persistenceDegraded` 为 `true`
  并显示 warning toast；所有 dirty snapshots 成功写入后清除。
- 非当前终端的命令完成后显示 attention 状态。
- 用户允许时发送 Chrome 系统通知，否则降级为 toast。
- 内部 shell ready/bootstrap 命令不得产生用户通知。

## 静态资源与缓存

MVP 不提供 PWA 安装，不生成 Web App manifest，不设置 standalone display，也
不注册 Service Worker。核心工作流必须在线连接 Go 服务，本地镜像已包含全部
资源；Service Worker 只会增加旧资源缓存、更新竞态和移动端调试成本。

缓存规则：

- `index.html`：`Cache-Control: no-cache, max-age=0`。
- `/api/version` 和所有 `/api/*`：`Cache-Control: no-store`。
- Vite 内容哈希资源：`Cache-Control: public, max-age=31536000, immutable`。
- favicon 等非哈希资源：短期缓存并支持 ETag。
- Go binary 使用 `embed` 打包完整 `web/dist`，运行时不读取公网资源。
- HTML 不包含 CDN URL、`preconnect`、manifest link 或 Service Worker 注册。
