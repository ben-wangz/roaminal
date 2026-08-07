# 06 - 架构、依赖与命名空间

> 状态：Approved
> 上位文档：[MVP 计划索引](../README.md)

## 目标架构

```text
Chrome desktop/tablet/mobile
          |
          | same-origin HTTPS: auth, heartbeat, session inventory
          | WSS: snapshot/meta/status/output/execution/input/resize
          v
Roaminal Linux container
  |- embedded Vite dist (no CDN, no Service Worker)
  |- roaminal Go backend (only network listener)
  |    |- net/http API, static server and coder/websocket
  |    |- auth store and Linux/cgroup system monitor
  |    |- terminal manager
  |    |    `- one event loop + one creack/pty Bash process per session
  |    `- terminal worker supervisor + framed stdio IPC
  |- Node terminal-worker (one headless xterm per session)
  `- /home/roaminal/.roaminal on state PVC
       `- state/ (private state root when the PVC mount is world-accessible)
            |- sessions/*.json
            |- sessions/*.snapshot
            `- auth-sessions.json
```

数据边界：

- Heartbeat 是 session inventory 的权威来源，WebSocket 不是唯一事实来源。
- 每个 session 对应一个服务端 Bash PTY；当前浏览器是否打开对应 Terminal Tab
  是独立的视图状态，不要求所有 session 同时出现在 Tab 条中。
- 浏览器拥有当前实例的 token；服务端拥有 refresh session。
- PTY 只存在于当前 Go 进程，其他 Pod 不能接管。
- Headless xterm state 只存在于当前 Node worker；Go backend 通过 sequence 和
  snapshot barrier 维持一致性，worker 不拥有持久卷。
- PVC 保存恢复材料，不保存正在运行的 Unix 进程。
- Go static embed 保证镜像包含所有浏览器资源。
- Node runtime、worker code 和 dependencies 全部在镜像内，不在启动时安装或
  联网获取。

## 参考行为映射

参考代码不得直接搬运。实施 Agent 必须先用测试描述行为，再在新架构中实现。

| 参考领域 | Go/worker/模块化 Web 实现 |
| --- | --- |
| `src/config.mjs` | `internal/config`，Go JSON/env/flags loader |
| `src/auth.mjs` | `internal/auth`，Go crypto + atomic persistence |
| `src/persistence.mjs` | `internal/persistence`，typed schema + atomic files |
| `src/system-monitor.mjs` | `internal/monitor`，Linux `/proc`/cgroup implementation |
| `src/terminal-manager.mjs` | `internal/terminal/manager.go` |
| `src/terminal-session.mjs` | `internal/terminal/session.go` + event loop + parser；headless xterm 进入 `terminal-worker` |
| `src/server.mjs` | `internal/server` + `internal/httpapi` |
| `shell/tabminal-*` | 新写 `shell/roaminal-bashrc` 和 `roaminal-hooks.bash` |
| `public/app.js` | `web/src` 领域 TypeScript 模块 |
| `public/styles.css` | `web/src/styles` 分层 CSS |
| `public/sw.js` | 不实现 |
| Node tests | Go unit/integration + worker `node:test` + Vitest + Playwright |

计划仓库结构：

```text
cmd/roaminal/main.go
internal/auth/
internal/config/
internal/httpapi/
internal/monitor/
internal/persistence/
internal/server/
internal/terminal/
internal/webassets/
shell/
terminal-worker/
  src/index.mjs
  test/
  package.json
  package-lock.json
web/
  src/
  public/
  package.json
  package-lock.json
  tsconfig.json
  vite.config.ts
testdata/
deploy/kubernetes/
docs/
go.mod
go.sum
Containerfile
```

## Go 依赖

```text
github.com/creack/pty v1.1.24
github.com/coder/websocket v1.8.15
golang.org/x/sys v0.47.0
```

除以上依赖外优先使用 Go 标准库。所有 Go module 在 `go.mod` 和 `go.sum` 固定。

## Terminal Worker 依赖

```text
Node.js 24.13.1（build/test/runtime）
xterm-headless 5.3.0
xterm-addon-serialize 0.11.0
xterm 5.3.0（SerializeAddon peer dependency）
```

这些是 xterm.js 官方旧包名，已经 deprecated 并迁移为 `@xterm/*`；MVP 仍使用
它们，因为 Tabminal `v3.0.40` 的服务端快照路径已经用这组版本验证。不得自行
升级为 scoped package、换 beta、回退到 `xterm-go` 或手写 parser。版本由
`terminal-worker/package-lock.json` 固定，production dependencies 随镜像交付。

必须用参考实现提取的 fixtures 验证宽字符、组合字符、颜色、alternate buffer、
scrollback、cursor、resize 和 extra private mouse modes。固定版本出现问题时：

1. 用最小 fixture 记录 expected/actual、依赖版本和影响范围。
2. 能通过 worker 内有界 adapter 满足 contract 时完成实现并记录批准差异。
3. 固定版本无法满足 Definition of Done，且 adapter 也无法解决时，按 README
   的停止条件请求人工决策。

MVP 不包含第二套 emulator 或 runtime engine switch。MVP 完成后可另建实现相同
`roaminal-terminal-worker/1` protocol 的 `xterm-go` worker，固定候选 commit
`8e117204ebedc133bf33ee9eb759c8484f843cee`，使用相同 fixtures 和输入条件比较
correctness、throughput、write-to-barrier latency、snapshot latency/size、CPU
和 RSS。结果分别报告 emulator-only 与 IPC end-to-end 数据。发现不足时输出可
提交开源社区的最小复现、测试和优化提案，不阻塞或改变 MVP。

## 前端依赖

```text
Node.js 24.13.1（frontend build/test；同时也是 worker runtime）
TypeScript 7.0.2
Vite 8.1.5
Vitest 4.1.10
@playwright/test 1.62.1
ESLint 10.8.0
typescript-eslint 8.66.0
@fontsource/monaspace-neon 5.2.5
@xterm/xterm 6.1.0-beta.197
@xterm/addon-fit 0.12.0-beta.197
@xterm/addon-web-links 0.13.0-beta.197
@xterm/addon-search 0.17.0-beta.197
@xterm/addon-progress 0.3.0-beta.197
@xterm/addon-ligatures 0.11.0-beta.197
react 19.2.8
react-dom 19.2.8
@vitejs/plugin-react 6.0.4
@types/react 19.2.18
@types/react-dom 19.2.4
```

不引入 Redux、Zustand、xterm React wrapper 或 UI component library。低频状态
使用 React state/reducer/context，外部 runtime snapshot 订阅使用
`useSyncExternalStore`。

xterm core 和 addons 固定使用上述兼容发布线。渲染器使用 xterm core 自带的
DOM renderer；不得加入与 core peer dependency 不兼容的 CanvasAddon。若 beta
包无法通过本地构建或测试，实施 Agent 必须停止并记录兼容问题，不能自行升级。
全部 npm 依赖由 lockfile 固定并打包进 `web/dist`。

依赖安装、项目内 binary、下载镜像和 Chrome channel 的执行规则见
[08-test-environment.md](./08-test-environment.md)。

## Roaminal 命名空间

运行时代码、UI 和数据不得残留 Tabminal 命名，行为参考和版权说明除外。

| Tabminal | Roaminal |
| --- | --- |
| package/bin `tabminal` | binary `roaminal` |
| `~/.tabminal` | `~/.roaminal` |
| `TABMINAL_*` | `ROAMINAL_*` |
| `TABMINAL_CWD` | `ROAMINAL_CWD` |
| `TABMINAL_SESSION_ID` | `ROAMINAL_SESSION_ID` |
| `TABMINAL_SHELL_READY` | `ROAMINAL_SHELL_READY` |
| `tabminal.v1` | `roaminal.v1` |
| `tabminal.auth.` | `roaminal.auth.` |
| `tabminal-login-v1` | `roaminal-login-v1` |
| access prefix `ta_` | `ra_` |
| refresh prefix `tr_` | `rr_` |
| `tabminal_auth_state:<hostId>` | `roaminal_auth_state` |
| runtime boot storage key | `roaminal_runtime_boot_id` |
| shell functions/markers | `_roaminal_*` / `RoaminalPrompt` |
| Web name/icons | Roaminal / `r>` |

不读取、迁移或兼容 Tabminal 的配置、数据目录、token、协议和 localStorage。
