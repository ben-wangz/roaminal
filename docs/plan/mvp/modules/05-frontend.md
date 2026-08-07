# 05 - 浏览器前端

> 状态：Approved
> 上位文档：[MVP 计划索引](../README.md)
> 当前补充：[13 - 单终端视图、Sidebar 对齐与 MVP 缺口修复](./13-single-terminal-sidebar-remediation.md)

## 模块边界

前端固定使用 React 19 + TypeScript 7 + Vite 8。领域模块不得通过全局变量隐式
耦合，只有 `app` 层负责跨领域编排；UI 不直接调用 `fetch` 或创建 WebSocket，网络
模块不直接操作 DOM。

```text
web/src/
  main.tsx
  app/              active-session state and orchestration
  auth/             challenge, token rotation, session management
  terminal/         protocol, main runtime, preview runtime, viewport, search
  status/           heartbeat, system strip, notifications
  input/            shortcuts, touch modifiers, visualViewport
  ui/               sidebar cards, action menus, dialogs, toast
  styles/           tokens, layout, terminal, responsive
```

React state 只保存可序列化的低频 UI/session metadata，不保存 PTY bytes、xterm
instance、WebSocket 或 mutable terminal buffer。xterm 实例、WebSocket、
ResizeObserver、heartbeat timer 和 PTY output 由 React tree 外的 runtime 管理。
React Strict Mode 下所有 socket、listener、observer、timer 和 DOM attach 必须
幂等 setup/cleanup。

## 单主终端视图

- 浏览器状态只有 `{ activeSessionId: string | null }`；服务端可以有多个 session，
  但页面任何时刻只显示一个 `.terminal-viewport` 和一个 main runtime。
- Sidebar 是唯一 session navigation。点击 card 先更新 active ID，再释放旧
  runtime 并 attach 新 runtime；切换、刷新、登出和 boot ID 变化均幂等 dispose。
- heartbeat 清单是权威来源；清单重排不改变 active，active 消失时按稳定顺序选择
  下一项、上一项或第一项。清单为空才创建一个 session。
- 切换不删除服务端 session；只有 action menu 中带确认的 Terminate 才调用 DELETE。
- 页面标题、statusbar、搜索、触控输入和主 xterm 从同一个 active ID 派生。
- `Ctrl+Shift+W`、Close Tab 菜单和顶部 Terminal Tab 条不存在。

## Sidebar card 与 preview

桌面 card 固定显示 effective title、`ID`（UUID 最后 12 字符）、完整 `PWD`（视觉
省略但 DOM/title 保留完整路径）和 `<time>` 形式的 `SINCE`。有未查看输出的非当前
session 显示 attention 文案和非颜色状态信号。卡片尺寸稳定，长文本
不得撑宽或遮挡操作轨。

仅在 `pointer: fine` 且宽度大于 800px 时，hover/focus intent 延迟 100ms 后启动一个独立
`TerminalPreviewRuntime`。它使用独立 xterm、WebSocket、FitAddon 和 DOM，
`scrollback: 0`、`disableStdin: true`、隐藏 cursor，不加载 search、links、
progress 或 ligatures addon；只消费 snapshot/output，永不发送 input、resize 或
`claim_terminal_control`。全页面最多一个 preview runtime/socket，leave、关闭
Sidebar、切换或删除 session 立即 dispose。移动布局不创建 preview。

## 操作轨与快捷键

每个 card 有独立的 Lucide `Bot`、`FolderOpen`、`EllipsisVertical` 按钮。Agent/Files
在没有插件时保留为 `aria-disabled="true"` 的不可用入口，激活只显示 unavailable
toast，不发起 API 请求。Terminal actions 仅包含 Rename title、custom 模式下的
Use automatic title 和确认后的 Terminate terminal。

```text
Ctrl/Cmd + Shift + T  新建终端
Ctrl/Cmd + Shift + S  切换 Sidebar
Ctrl/Cmd + F          终端内搜索
```

触控设备保留 ESC、TAB、CTRL、ALT、SHIFT、SYM 和方向键；modifier 是状态机，按下
普通键后消费并清除。`visualViewport` resize 更新 CSS viewport 高度，不触发 layout
振荡。

## 状态、提示与认证

heartbeat 提供连接状态、hostname、session 数、延迟、scrollback 配置和 persistence
degraded warning。执行 started/completed 通过 WebSocket 进入运行状态、attention、
toast 和（用户已授权时）浏览器通知；非当前 session 的完成事件保留为 Sidebar
attention，内部 bootstrap marker 不通知。

登录前必须确认 `window.isSecureContext` 和 `crypto.subtle`；不安全的 HTTP origin
显示 `Secure HTTPS context required`，不提供纯 JavaScript crypto fallback。开发
环境的 direct Service E2E 只在 Chrome 启动参数中对精确 Service origin 设置例外。
Sign out 先调用幂等 `/api/auth/logout`，再清理本地 token；网络失败时明确提示当前
refresh session 可能仍在服务端。登录会话 dialog 支持查看、撤销单个和 logout-others。

## 静态资源与缓存

不提供 PWA manifest 或 Service Worker。所有资源由 Go `embed` 从当前镜像返回；HTML
和 API 使用 no-cache/no-store，Vite 内容哈希资源使用 immutable cache，页面不包含
CDN URL 或外部请求。终端主 runtime 不加载 web-links addon，以避免当前 xterm beta
初始化路径的 `onShowLinkUnderline` 空引用；preview 同样不加载任何交互 addon。
