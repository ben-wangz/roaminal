# 07 - 容器与 Kubernetes 部署

> 状态：Approved
> 上位文档：[MVP 计划索引](../README.md)

## 镜像

- 使用 multi-stage Docker build：Node 前端构建与 worker dependency stage、
  Go 后端构建、Linux runtime。
- Node build/runtime 固定 `24.13.1`，frontend 和 terminal worker 各自使用
  lockfile。
- Go builder 固定 Go `1.26.5`。
- Runtime 使用固定 Node Debian slim digest，包含 Go binary、Node.js runtime、
  terminal worker、worker production dependencies、Bash、CA certificates 和
  tini；不包含 npm、npx、corepack、编译器或 cloudflared。
- Go binary 以 `CGO_ENABLED=0` 构建。
- 使用非 root 用户，home 为 `/home/roaminal`。
- `/home/roaminal/.roaminal` 挂 state volume；`/workspace` 挂显式 workspace
  volume。
- 终端只能访问容器和显式 volume；不挂 Docker socket、不使用 privileged、
  不进入宿主机 namespace。
- “无文件工作区”只排除 Web 文件 UI/API，不限制用户在 Bash 中操作
  `/workspace`。

## 普通容器运行

- 提供 `compose.yaml`，固定 `restart: unless-stopped`。
- `docker run` 文档同样使用 `--restart unless-stopped`，使 worker fail-fast 后
  能自动拉起新实例。
- 对外端口固定为 `9846`；冲突时服务报错并退出。

## Kubernetes

使用 `apps/v1 Deployment`：

- `replicas: 1`
- `strategy.type: Recreate`
- 不配置 HPA
- state PVC 和 workspace PVC 使用 `ReadWriteOnce`
- Service 使用 `ClusterIP`
- startup/readiness/liveness 均调用 `/healthz`
- worker 未完成 handshake 或进入 fatal 状态时 `/healthz` 返回 503
- password 使用 Secret
- 非敏感配置使用 ConfigMap/env
- resources 固定为 requests `cpu: 100m`、`memory: 256Mi`，limits
  `cpu: "2"`、`memory: 2Gi`；部署者可直接修改普通 YAML
- startup probe：`periodSeconds: 2`、`timeoutSeconds: 1`、
  `failureThreshold: 15`
- readiness probe：`periodSeconds: 5`、`timeoutSeconds: 1`、
  `failureThreshold: 2`
- liveness probe：`periodSeconds: 10`、`timeoutSeconds: 1`、
  `failureThreshold: 3`
- `terminationGracePeriodSeconds: 30`
- restrictive securityContext

只提供普通 YAML：

```text
deploy/kubernetes/deployment.yaml
deploy/kubernetes/service.yaml
deploy/kubernetes/pvc.yaml
deploy/kubernetes/configmap.yaml
deploy/kubernetes/secret.example.yaml
deploy/kubernetes/ingress.example.yaml
```

不提供 Helm、Kustomize、Operator 或其他模板层。部署文档必须说明 TLS、
WebSocket proxy timeout、PVC 权限、备份恢复和升级中断。

## 运行时故障语义

Worker 在 backend 运行期间异常退出、protocol corruption 或持续超时时，整个
Go 服务 fail-fast：

1. `/healthz` 立即变为 503。
2. 停止接受新连接和 input。
3. 尽力保留最后一个成功 checkpoint，终止 PTY 并清理 worker。
4. Go 以非零状态退出。
5. Docker/Kubernetes restart policy 拉起新实例，并按持久终端恢复语义创建
   新 Bash。

MVP 不在同一 Go 进程中热重启 worker。应用 shutdown deadline 为 10 秒，Pod
grace 为 30 秒，worker handshake 为 5 秒，control request 为 30 秒，writer
queue stall 为 10 秒。
