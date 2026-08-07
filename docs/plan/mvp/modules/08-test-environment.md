# 08 - 测试环境与工具约束

> 状态：Approved
> 上位文档：[MVP 计划索引](../README.md)

## 基础环境

实施 Agent 必须在 Phase 0 记录实际版本，并在缺少可自行安装的工具或项目依赖
时直接补齐，不把环境准备问题转交人工。锁定基线为：

```text
Linux amd64
Bash 5.2+
Go 1.26.5
Node.js 24.13.1
npm 11+
Podman 4.9+
kubectl 1.35+
Google Chrome stable（Playwright channel: "chrome"）
```

## 已验证的 Codespace 基线

2026-08-06 已在目标 Codespace 完成以下验证：

| 项目 | 已验证结果 |
| --- | --- |
| OS/shell | Ubuntu 24.04.4、Linux amd64、Bash 5.2.21 |
| Go race | Go 1.26.5、CGO enabled、GCC 13.3；race test binary 编译和启动通过 |
| Node | Node.js 24.13.1、npm 11.8.0 |
| Chrome | Google Chrome stable 151.0.7922.75；项目目标 `@playwright/test 1.62.1` 使用 `channel: "chrome"` 启动、移动视口和 canvas pixel smoke 通过 |
| Podman | Podman 4.9.3、rootful overlay/crun；实际 container create/run/remove 通过 |
| Kubernetes | kubectl 1.35.4、server 1.34.6+k3s1；context 和 namespace 均为 `develop`，所需 namespaced RBAC 和 Service DNS 访问已确认 |
| Registry | `container-registry.internal.pve.lab.geekcity.tech:32443` 的 HTTPS、`/v2/`、`ben-wangz/roaminal` push/pull 和 digest 一致性已确认 |
| 固定依赖 | [06-architecture-dependencies.md](./06-architecture-dependencies.md) 列出的 Go/npm 精确版本均可从当前配置的官方源取得 |

以上版本是环境验收证据，不把 Chrome patch、Kubernetes server patch 或 GCC
版本新增为产品锁定项。实施 Agent 在 Phase 0 重新记录动态版本、剩余磁盘、
9846 端口、API server、RBAC、registry health 和下载源可达性；基线状态变化时
先按本模块规则自行修复，只有命中 README 停止条件时才请求人工处理。

Phase 0 至少执行并把以下输出写入 implementation log：

```bash
uname -m
bash --version
go version
cc --version
CGO_ENABLED=1 go test -race -count=1 -run '^$' errors
node --version
npm --version
podman version
podman info
kubectl version
kubectl config current-context
kubectl config view --minify --output 'jsonpath={..namespace}{"\n"}'
kubectl auth can-i --list --namespace develop
google-chrome --version
curl --fail --silent --show-error \
  https://container-registry.internal.pve.lab.geekcity.tech:32443/v2/
ss -ltn 'sport = :9846'
df -h /var/lib/containers
```

`ss` 有输出表示端口已占用；Phase 0 必须定位占用者并释放测试端口，不能让应用
自动换端口。Chrome 的 Playwright 启动验证在项目依赖安装后使用项目内 runner
执行，不以全局 Playwright 代替。

本项目不需要 `make`、Docker CLI、Docker Compose、Podman Compose、
`kubeconform` 或 `kubeval`。这些命令缺失不是停止条件，实施中也不得安装或
引入它们。构建和测试命令直接使用 `go`、`npm`/`npx`、`podman` 和 `kubectl`，
并在项目文档中列出，不用 Makefile 包装。

Codespace 镜像安装 `gcc` 和 `libc6-dev`，不安装 `make` 或完整
`build-essential`。实施 Agent 仍显式使用 `CGO_ENABLED=1 go test -race ./...`；
production binary 继续固定 `CGO_ENABLED=0`。这不是引入 runtime CGO
dependency。若未来环境丢失 compiler，Agent 自行补齐相同的最小工具链。

## 项目依赖由仓库管理

React、Vite、Vitest、xterm、ESLint、Playwright test runner 和 terminal worker
依赖都是项目自身依赖，不是预装系统软件。实施 Agent 必须：

1. 按固定版本创建 `web/package.json` 和 `terminal-worker/package.json`。
2. 生成并提交各自的 `package-lock.json`；之后构建和 CI 只使用 `npm ci`。
3. 使用项目内 `node_modules/.bin`，通过 `npm exec` 或 npm scripts 调用工具；
   不得依赖全局 Playwright、Vite、Vitest 或 ESLint 版本。
4. 前端安装 `@playwright/test 1.62.1` 后，在 `web/` 中执行
   `npm exec -- playwright install --with-deps chrome`，并以 `channel: "chrome"`
   运行 E2E。
   不得用默认 bundled Chromium 冒充 Chrome channel 结果。
5. 使用 `go.mod`/`go.sum` 管理 Go dependencies，并先执行 `go mod download`。

全局 npm package 缺失或版本不同不构成阻塞。只有固定版本在官方源和允许的
镜像源均无法取得，经过重试和最小复现后，才触发 README 的停止条件。

## 下载加速

默认优先使用官方 registry。网络较慢时允许对单次命令使用以下公开镜像：

```bash
NPM_CONFIG_REGISTRY=https://registry.npmmirror.com npm ci
GOPROXY=https://goproxy.cn,direct go mod download
```

如果基础 Node.js 版本需要重新安装，可使用清华 TUNA 的 Node release mirror：

```bash
export NODE_MIRROR=https://mirrors.tuna.tsinghua.edu.cn/nodejs-release/
```

镜像只用于传输加速，不得修改锁定版本、package integrity、Go checksum 或测试
范围；镜像缺包时回退官方源。不得把代理凭据、用户级 npm config 或机器专用
配置提交到仓库。Playwright Chrome channel 优先使用官方 installer；网络受限时
使用环境已有的 `HTTP_PROXY`/`HTTPS_PROXY`，不使用未经验证的浏览器二进制镜像。

## Podman 测试流程

容器相关命令只能使用 Podman CLI，不得调用 `docker`、`docker compose`、
`docker-compose`、`podman compose` 或兼容 alias。

仅有 `podman run` 不足以完成测试：它不能从 `Containerfile` 构建镜像，也不能
把本机 image store 的镜像交给 Kubernetes 节点。完整流程允许且要求：

```bash
IMAGE=container-registry.internal.pve.lab.geekcity.tech:32443/ben-wangz/roaminal:$(git rev-parse --short HEAD)
podman build --file Containerfile --tag "$IMAGE" .
podman run --rm --name roaminal-mvp-test \
  --publish 9846:9846 \
  --env ROAMINAL_HOST=0.0.0.0 \
  --env ROAMINAL_ACCEPT_TERMS=true \
  --env ROAMINAL_PASSWORD=test-only-password \
  --volume roaminal-test-state:/home/roaminal/.roaminal \
  --volume roaminal-test-workspace:/workspace \
  "$IMAGE"
podman push "$IMAGE"
```

上述是命令形态，不要求串行手工执行：测试 harness 可以启动/停止进程并收集
日志。固定 Git SHA tag，不使用 `latest`。测试只清理自己创建的 container、
local image 和 test volumes，不运行 system-wide prune。

内部 registry 已完成一次真实 push、删除本地 image 后 pull 和 digest 对比。
Registry 禁用了 manifest delete，标准 V2 API 返回 `405`，所以实施 Agent 不再
创建额外的 disposable preflight tag，也不把远端删除作为 gate。Phase 0 只检查
HTTPS `/v2/` health；Phase 7 的第一个唯一 Git SHA image push 是写权限复核，
随后必须 pull 或 inspect remote manifest 并确认 digest 一致，再用于 Kubernetes。
Git SHA tag 不覆盖并作为测试证据保留。health、push 或 pull 失败时可切换用户
明确提供的 registry，不能改用 Docker 或跳过 Kubernetes rollout。

环境验证遗留 tag
`ben-wangz/roaminal:registry-check-20260806084929-397721`；它不是部署输入，
Agent 不尝试绕过 Registry policy 删除它，也不因此暂停实施。

## Kubernetes 验证

当前 kube context 的默认 namespace 是 `develop`。该 namespace 已确认允许管理
Deployment、Service、ConfigMap、Secret、PVC、Pod、Pod logs、Job 和 Ingress；
不得操作其他 namespace 或 cluster-scoped resources。

静态 YAML schema 工具不是要求。先使用 kubectl 的结构化 local transform 生成
测试 Deployment，并用 client-side generator 生成测试 Secret：

```bash
kubectl set image --local \
  --filename deploy/kubernetes/deployment.yaml \
  roaminal="$IMAGE" \
  --output yaml > /tmp/roaminal-deployment.yaml
kubectl create secret generic roaminal \
  --namespace develop \
  --from-literal=password="$ROAMINAL_TEST_PASSWORD" \
  --dry-run=client \
  --output yaml > /tmp/roaminal-secret.yaml
```

验证顺序固定为：

1. 确认两个临时文件只包含测试值且不进入 Git。
2. 对生成的 Deployment/Secret 及 `service.yaml`、`pvc.yaml`、`configmap.yaml`
   执行 `kubectl apply --server-side --dry-run=server -n develop`。不得 apply
   `secret.example.yaml` 或 `ingress.example.yaml`。
3. 在 `develop` 实际 apply 同一组已验证文件。
4. `kubectl rollout status -n develop deployment/roaminal --timeout=180s`。
5. 检查 Pod events/logs、probe、PVC、Secret/config injection 和 restart restore。
6. 通过 `http://roaminal.develop.svc.cluster.local:9846` 直接运行 Chrome
   E2E；HTTP develop 测试只对该精确 origin 添加 Chrome secure-context 例外，
   不使用 port-forward；没有测试域名时不部署 `ingress.example.yaml`。
7. 删除 `/tmp/roaminal-deployment.yaml` 和 `/tmp/roaminal-secret.yaml`。

所有测试资源使用 `app.kubernetes.io/name=roaminal` 和
`app.kubernetes.io/managed-by=roaminal-mvp-test` labels。测试只能删除这些明确
归属 Roaminal 的资源；不得清理 namespace 中的其他 workloads。真实 API server
validation、admission、rollout 和 E2E 是 Kubernetes 验收依据，不额外运行
`kubeconform`/`kubeval`。这一选择验证当前集群的真实兼容性，不额外承诺其他
Kubernetes minor versions 的离线 schema 兼容矩阵。
