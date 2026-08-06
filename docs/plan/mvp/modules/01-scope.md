# 01 - 产品范围

> 状态：Approved
> 上位文档：[MVP 计划索引](../README.md)

## 产品与技术前提

1. Roaminal 是纯 Web 应用，由 Chrome 客户端、Go backend 和内部 terminal
   worker 组成。
2. 不开发任何桌面或移动原生客户端。
3. 开发阶段只保证 Google Chrome 兼容性；页面适配桌面、平板和手机尺寸。
4. MVP 不包含文件工作区；该部分以后独立设计。
5. MVP 不包含 ACP、Coding Agent、terminal-native AI 或任何模型服务。
6. MVP 通过 Podman 或 Kubernetes 部署；本地不使用 Docker/Compose。
7. 只连接页面来源对应的单个 Roaminal 实例；保留同一实例内的多 Terminal
   Tab，不实现多 Host 注册、切换或聚合。
8. 后端使用 Go 1.26.5；运行平台只承诺 Linux 容器。
9. 终端只承诺 Bash，不实现或测试 Zsh、Fish、PowerShell 等其他 shell。
10. 前端资源全部构建进镜像，运行时不依赖 CDN 或其他公网静态资源。
11. Kubernetes 使用单副本 Deployment，只交付普通 YAML，不使用
    StatefulSet、HPA、Helm 或 Kustomize。
12. MVP 不提供 PWA 安装，也不注册 Service Worker。
13. 参考实现的前端单文件架构必须重构；复现的是功能，不是源码。
14. Go 是唯一监听网络端口的 backend；一个独立 Node.js 子进程只运行官方
    xterm.js headless terminal emulator，不提供 HTTP、WebSocket 或远程 RPC。

## 成功定义

MVP 完成后，用户能够：

1. 在 Chrome 中打开 Roaminal 并通过密码登录。
2. 创建、切换和关闭多个真实 Linux PTY + Bash 会话。
3. 刷新页面、临时断网或从另一台设备连接后继续使用已有 PTY。
4. 在桌面、平板和手机布局中完成相同的核心终端操作。
5. 在同一服务实例中通过多个 Terminal Tab 使用、预览和管理多个会话。
6. 看到当前服务连接状态、会话状态和基础系统状态。
7. 使用终端搜索、链接识别、终端进度、触控辅助键盘和常用快捷键。
8. 以容器启动服务，或使用仓库内普通 Kubernetes YAML 部署服务。
9. 通过持久卷保存认证会话、终端元数据、scrollback 和终端快照。

“持久终端”的精确定义：

- 浏览器刷新、网络中断、浏览器关闭和其他设备连接不会终止服务端 PTY。
- Go 服务进程或 Pod 重启后，恢复相同 session ID、cwd、标题、尺寸和
  scrollback，并创建新的 Bash PTY。
- 重启前正在运行的 Unix 进程不会继续运行，也不得在 UI 中显示为仍存活。

## 明确排除

以下内容不得出现在 MVP 的运行时代码、UI、API、依赖或部署配置中：

- 对外 Node.js backend、Koa、node-pty 和 Node `ws`；Node.js 只允许用于
  frontend build/test 和内部 xterm.js terminal worker。
- 文件列表、读取、创建、保存、重命名、删除和 raw preview API。
- 文件树、Monaco、图片/PDF/Markdown 预览和文件图标构建。
- `memory.json`、expanded-folder API、`editorState` 和 `workspaceState`。
- workspace tabs 和 terminal pinning。
- ACP、Agent、prompt、attachment、permission、plan、usage HUD 和 managed
  terminal。
- terminal-native AI、`#` prompt 劫持、失败命令 auto-fix、OpenAI、
  OpenRouter 和 Google Search。
- Web App manifest、PWA 安装和 Service Worker。
- 任何原生客户端、桌面安装包、launchd、systemd/pm2 和远程部署脚本。
- 服务端 Cloudflare Tunnel 和 cloudflared。
- 多 Host registry、Host picker、`/api/cluster`、`cluster.json`、跨 Origin 子
  Host 连接和 Host-scoped auth storage。
- Zsh、Fish、PowerShell、Windows PTY 和 macOS 后端兼容代码。
- Firefox、Safari、Edge 的兼容性承诺和专项 workaround。
- StatefulSet、Helm、Kustomize、HPA、多副本、PTY 跨 Pod 迁移和无缝升级。
- 文件上传下载、SFTP、端口转发、SSH client 和 Kubernetes 管理终端。
- 运行时 CDN 或任何必须访问公网才能加载的浏览器资源。

## 行为复现边界

- 对纳入 MVP 的功能，复现 Tabminal 的用户可见行为、交互、协议语义和恢复
  能力。
- Tabminal 仅作为行为规格、交互参考和验收 oracle，不直接复制其 Node.js
  后端或前端单文件代码。
- 网络后端、PTY 和持久化使用 Go 重新实现；服务端 terminal emulator 是
  同容器内的独立 Node.js worker；前端按领域拆分为 TypeScript 模块。
- 产品名、命令、环境变量、协议、存储目录和浏览器存储键统一使用
  Roaminal 命名空间。
- 删除功能时必须同时删除死代码、依赖、接口、DOM、样式、测试和文档。
- UI 保持参考实现的终端布局和交互，不保留因删除功能产生的空白区域，也不
  要求源码结构相同。

若实施中确实复制了参考项目的任何代码或资产，必须保留对应 MIT 版权声明；
默认不复制，并在 `THIRD_PARTY_NOTICES.md` 中记录行为参考和所有直接依赖的
许可证。
