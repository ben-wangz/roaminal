# 10 - 关键决策依据

> 状态：Approved
> 上位文档：[MVP 计划索引](../README.md)
> 注意：本文件解释已确认结论，不提供实施分支。

## React 前端

React 是 UI library，仍由 Vite 负责开发和构建；它不改变 Roaminal 是 Go
服务端托管静态 Web App 的部署模型。

| 维度 | React + TypeScript + Vite | 原生 DOM + TypeScript + Vite |
| --- | --- | --- |
| xterm.js 集成 | 需要 ref/effect adapter；生命周期约束更严格 | 命令式 API 直接匹配，首期接入更简单 |
| PTY 高频输出 | runtime 直接 `terminal.write()` 时无额外 render | 天然直接写 xterm，无 framework render 风险 |
| 多 Session UI | 组件、单向数据流和可预测重渲染更易维护 | 需要手工同步 DOM、事件和状态 |
| 响应式交互 | Sidebar、modal、touch bar、search 和状态组合易拆分测试 | 复杂交互增长后容易形成 controller 和 DOM patch 代码 |
| 生命周期风险 | Strict Mode 检验 setup/cleanup，错误实现可能重复资源 | 没有 React 重挂载，但仍需手工完成 cleanup |
| 包与产物 | 增加 React runtime、Vite plugin 和类型依赖 | 依赖和 bundle 更小 |
| 后续文件工作区 | 适合以后增加复杂 workspace UI | 扩展时可能自建组件/状态机制或迁移 framework |

最终选择 **React 19 + TypeScript 7 + Vite 8**。现有范围包含多 terminal、
desktop preview、认证 modal、状态图和移动端触控布局，且文件工作区在 MVP 后
有大量改动计划。现在建立 React/runtime 边界，避免 MVP 后迁移视图层。

## 独立 Terminal Worker

```text
Go backend
  owns network + auth + PTY + session ordering + persistence
        |
        | roaminal-terminal-worker/1 over framed stdio
        v
Node worker (MVP)
  owns xterm-headless state + SerializeAddon only
```

这不是微服务边界。MVP 只有一个 image、container、Deployment、对外端口和
health model；Node worker 没有网络身份或独立发布单元。代价是 runtime 包含
Node，PTY output 多一次本地 IPC，并需要 sequence、barrier、backpressure 和
联合 shutdown。收益是 Go backend 边界稳定，emulator 可在不改 HTTP、PTY 和
persistence contract 的情况下替换。

MVP 后对比 `xterm-go` 时，不在 production binary 中加入动态开关。另建实现
相同 protocol 的 Go worker executable，让两者通过相同 conformance fixtures；
correctness 达到同一门槛后再比较 throughput、latency、CPU、RSS 和 snapshot
成本。结果同时包含 emulator-only 与 IPC end-to-end 数据。“`xterm-go` 应有
更强性能”是待验证假设，不是选型事实。

## Worker 故障策略

Worker 异常退出、protocol corruption 或持续超时时采用整服务 fail-fast；初始
handshake 失败时 Go 同样非零退出。

| 维度 | 整服务 fail-fast | Worker 热重启 + replay |
| --- | --- | --- |
| 行为 | Go 停止服务和 PTY，由容器重启并恢复新 Bash | Go 保留 PTY，启动 worker 并 replay output 缺口 |
| 额外状态 | 无；使用 <=1s checkpoint contract | 需要 sequence ACK、bounded replay 和重建 barrier |
| 一致性 | 与整个 Go/Pod 异常退出使用同一恢复语义 | attach/snapshot/resize 恢复窗口更复杂 |
| 可用性 | worker 故障中断全部 session | PTY 可继续运行 |
| MVP 风险 | 低，容易确定性验证 | 中高，新增高频状态和内存压力 |

MVP 没有 durable output WAL，热重启无法保证无损。具体行为固定为：health 变为
503，停止新连接/input，尽力使用最后一个成功 checkpoint，终止 PTY，清理
worker 并非零退出；容器策略拉起新实例。不得在同一 Go 进程中盲目重启 worker
或继续广播无法进入 shadow state 的 output。
