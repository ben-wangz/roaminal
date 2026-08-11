# Roaminal Helm Chart 需求

> 状态：需求草案，等待讨论与确认。
>
> 本阶段只定义需求。未阅读 `/root/code/github/k8s-at-home` 中的参考实现，也不得
> 创建 `chart/`、修改 `deploy/kubernetes/`、调整 CI/ForgeKit 配置、发布 Chart，
> 或变更任何运行中的 Kubernetes 资源。需求确认后，设计阶段必须先完成参考项目
> 调研，再另行产出可交给 agent 连续实施的设计文档。

## 1. 背景与目标

Roaminal 当前通过 `deploy/kubernetes/` 中的裸 Kubernetes YAML 部署。它们已经定义
了可运行的单副本应用，但缺少参数化、安装升级契约、values 校验、标准标签、资源
命名隔离及可复用的发布载体。

本需求的目标是把当前 Kubernetes 部署能力迁移并改进为 Helm v3 application
Chart：

1. Chart 源码直接位于仓库根目录的 `chart/`，不放在 `deploy/` 下。
2. 保留当前部署的运行、安全、存储和 WebSocket 行为，不因模板化发生功能回退。
3. 将镜像、应用配置、认证 Secret、单个持久卷中的三类数据目录、Service、Ingress、
   安全上下文和常见 Pod 调度能力整理成稳定且有校验的 values 接口。
4. 支持从现有裸 YAML 部署迁移。当前三个 PVC 中的数据需要通过显式、可回滚的步骤
   汇入统一 PVC，不能静默搬迁、删除或重建用户数据。
5. 将 Chart 纳入现有 ForgeKit 版本与 lint 工作流，并形成可验证、可打包的发布物。

## 2. 当前部署基线

后续设计与实施必须以当前仓库事实为基线，而不是重新发明一套运行模型：

- 一个 `Deployment`，固定单副本，升级策略为 `Recreate`。
- 一个 `ClusterIP` Service，默认端口和容器端口均为 `9846`，探针访问 `/healthz`。
- 独立的 state、workspace 和 SSH 三个 RWO PVC，当前默认容量分别为 `2Gi`、
  `10Gi` 和 `1Gi`。
- state 挂载到 `/home/roaminal/.roaminal`，workspace 挂载到 `/workspace`，SSH
  挂载到 `/home/roaminal/.ssh`，临时目录使用 `/tmp` `emptyDir`。
- Roaminal 以 UID/GID `1000` 运行，Pod 使用 `fsGroup: 1000`；容器禁止提权、丢弃
  全部 capabilities、使用只读根文件系统和 `RuntimeDefault` seccomp。
- 镜像内已使用 `tini` 作为 PID 1。Chart 不得覆盖现有 entrypoint 或引入第二套 init
  机制。
- 密码来自 Kubernetes Secret 的 `password` key；条款确认当前固定为 `true`。
- 配置包括 bind host、port、WebSocket ping、scrollback、connection/client 上限等。
- Ingress 当前只是示例，但明确要求 TLS、WebSocket upgrade，以及至少 3600 秒的
  read/send timeout。
- 应用不访问 Kubernetes API，不需要 RBAC，也不需要挂载 ServiceAccount token。
- 活动 PTY、SSH ControlMaster 和 session runtime 都在单个 Pod 内，不具备多副本
  调度或无损滚动迁移能力。

三个 PVC 只是待迁移的当前事实，不是新 Chart 要延续的目标结构。新 Chart 默认且
只管理一个持久化 PVC，在其中隔离 state、workspace 和 SSH 三个逻辑目录。

## 3. 范围

### 3.1 本次必须交付

- 根目录 `chart/` 下完整、可安装的 Helm v3 Chart。
- 覆盖当前 `Deployment`、`Service`、ConfigMap/配置注入、PVC 和可选 Ingress 能力。
- 外部认证 Secret、统一 PVC/已有 claim 复用、安全上下文、探针、资源配额和常用
  调度参数。
- `values.yaml`、`values.schema.json`、Chart README/安装说明和迁移说明。
- Helm lint、模板渲染、schema、Kubernetes API 校验和真实 develop namespace rollout
  验收。
- ForgeKit 对 Chart 版本和 Chart 质量门禁的管理。

### 3.2 本次不做

- 不把 Roaminal 改造成多副本、高可用或无状态服务。
- 不引入 StatefulSet、Operator、CRD、HPA、PDB、ServiceMonitor、PrometheusRule、
  Gateway API 或服务网格专用资源。
- 不在 Chart 中部署 SSH server、远端监控 agent、数据库、Ingress Controller、
  Secret 管理器或存储 provisioner。
- 不让 Roaminal Pod 获取 Kubernetes API 权限。
- 不改变应用业务配置、认证协议、session 生命周期、文件布局或容器镜像内容。
- 不为某个集群、Ingress Controller、CSI 或 GitOps 产品写死私有配置。
- 不将参考项目的品牌、应用专属逻辑或无法解释的模板直接复制进 Roaminal。

## 4. 参考项目调研要求

需求确认后的设计阶段必须阅读 `/root/code/github/k8s-at-home` 中与 ForgeKit 和 Helm
Chart 相关的代码，至少回答以下问题：

- Chart 的目录边界、helper、命名、labels/annotations 和 values 组织方式。
- `values.schema.json`、模板校验、测试矩阵和 lint 如何组织。
- Chart version、appVersion、容器版本与 ForgeKit version source 如何关联。
- CI 如何执行 Helm lint、render、package，以及是否有可复用的本地脚本。
- 哪些做法是通用约定，哪些只是参考项目自身的集群或应用约束。

调研结果必须在设计文档中注明“采用、调整、不采用”及原因。参考只用于建立成熟的
工程约定，Roaminal 的运行事实和本需求优先。

## 5. Chart 结构与元数据需求

- `HELM-STRUCT-001`：Chart 根目录固定为 `chart/`，至少包含 `Chart.yaml`、
  `values.yaml`、`values.schema.json`、`templates/`、Chart README 和 `.helmignore`。
- `HELM-STRUCT-002`：Chart 使用 Helm v3 `apiVersion: v2`、类型 `application`，名称
  为 `roaminal`。
- `HELM-STRUCT-003`：模板必须提供一致的 fullname、selector labels、common labels
  和 ServiceAccount name helper，不能在各资源中重复拼接命名规则。
- `HELM-STRUCT-004`：所有资源至少使用 Kubernetes recommended labels；selector
  labels 必须稳定，升级时不能因为 Chart/app version 改变。
- `HELM-STRUCT-005`：支持 `nameOverride`、`fullnameOverride`、额外 labels 和
  annotations，但用户值不能覆盖 selector 的身份语义。
- `HELM-STRUCT-006`：模板不能依赖在线查询、随机值或目标集群状态才能稳定 render；
  相同 Chart 与 values 必须得到语义一致的清单，适合 GitOps diff。
- `HELM-STRUCT-007`：用户提供的普通字符串默认按数据处理，不对任意 values 普遍
  执行 `tpl`，避免把 values 扩展成隐式模板执行面。
- `HELM-STRUCT-008`：必须声明并测试最低 Helm 版本和受支持的 Kubernetes 版本范围；
  `Chart.yaml` 的 `kubeVersion` 与模板所用 API 必须一致，不得生成已移除的 API。

## 6. Workload 与生命周期需求

- `HELM-WORK-001`：继续使用单副本 `Deployment` 和 `Recreate`，不得默认改为
  `RollingUpdate`。
- `HELM-WORK-002`：首版只允许 `replicaCount=1`；其他值必须在 schema 或模板阶段
  明确失败，不能生成表面可用但会分裂 session/transport 的多副本部署。
- `HELM-WORK-003`：保留 `terminationGracePeriodSeconds: 30`，并允许受控覆盖。
- `HELM-WORK-004`：不得覆盖镜像自带的 `tini` entrypoint。Pod 终止必须继续把
  SIGTERM 传递给 Roaminal 及其 terminal process group。
- `HELM-WORK-005`：startup、readiness 和 liveness probes 默认启用并继续访问
  `/healthz` 的 named HTTP port；允许分别调整阈值和周期，也允许显式关闭。
- `HELM-WORK-006`：ConfigMap 或由 Chart 管理的非敏感配置发生变化时，Pod template
  必须变化并触发重建。外部 Secret 内容变化无法由 Helm 感知时，文档必须说明重启
  方式，不能声称会自动 rollout。
- `HELM-WORK-007`：支持 `podAnnotations`、`podLabels`、`priorityClassName`、
  `runtimeClassName`、`nodeSelector`、`tolerations`、`affinity` 和
  `topologySpreadConstraints`，默认不施加集群特定调度约束。
- `HELM-WORK-008`：默认禁用 ServiceAccount token 自动挂载；不创建 Role、
  ClusterRole 或 binding。若提供 ServiceAccount 定制，只服务于 Pod 身份和平台集成，
  不扩大默认权限。

## 7. 镜像与应用配置需求

- `HELM-IMAGE-001`：支持独立设置 image repository、tag、digest 和 pull policy；
  digest 存在时必须形成不可变镜像引用。
- `HELM-IMAGE-002`：支持 `imagePullSecrets`，不得写死当前开发 registry。
- `HELM-IMAGE-003`：Chart 的默认 appVersion 与 Roaminal 产品版本保持明确关系；
  开发 Git 版本和正式 release 的取值由 ForgeKit 统一产生，不手工维护重复版本源。
- `HELM-CONFIG-001`：至少暴露当前部署已有的 host、port、WebSocket ping interval、
  scrollback lines、最大 connection instances、每 connection 最大 clients、初始 cwd。
- `HELM-CONFIG-002`：还应覆盖已经公开且适用于部署的 debug、access token TTL、
  refresh token TTL 和 auth max attempts；镜像内部路径不作为常规 values 暴露。
- `HELM-CONFIG-003`：values 的默认值、类型、范围和 Go duration 格式必须与应用
  配置校验一致；明显无效的值应在 Helm schema/template 阶段失败，而不是等 Pod
  CrashLoop。
- `HELM-CONFIG-004`：container port、Service targetPort、探针端口和应用监听端口必须
  来自同一逻辑事实源，不能允许互相漂移。
- `HELM-CONFIG-005`：支持受控的 `extraEnv` 和 `extraEnvFrom`，但内置关键变量的
  优先级与覆盖规则必须有文档，不能出现同名 env 的不确定行为。
- `HELM-CONFIG-006`：条款确认必须是显式、可审计的安装选择；Chart 不应在用户
  无感知时替其接受条款。

## 8. 认证 Secret 需求

- `HELM-AUTH-001`：生产默认路径使用用户预先创建或 Secret manager 管理的现有
  Secret，并允许配置 Secret name 与 password key。
- `HELM-AUTH-002`：不得把密码渲染到 ConfigMap、Pod annotation、NOTES、命令行参数
  或非敏感文档示例中。
- `HELM-AUTH-003`：不得依靠应用的启动时随机密码作为 Kubernetes 正常部署方案；
  Pod 重建后认证连续性必须可控。
- `HELM-AUTH-004`：是否提供仅供本地测试的 Chart-managed Secret，留待设计决策；
  若支持，必须明确 Helm release storage 会保存该敏感值，并默认关闭。
- `HELM-AUTH-005`：密码 Secret 缺失、key 缺失或显式为空时，安装前检查或 Pod 启动
  错误必须清晰，不能静默生成新认证事实。

## 9. 存储与挂载需求

### 9.1 通用要求

- `HELM-STOR-001`：Chart 只创建或引用一个持久化 PVC。state、workspace 和 SSH
  使用该 claim 内固定且互不重叠的 `state/`、`workspace/`、`ssh/` 逻辑目录，并分别
  挂载到应用现有路径；不得再为三类数据各创建一个 PVC。
- `HELM-STOR-002`：统一存储支持 Chart 创建 PVC 或引用一个 `existingClaim`；
  existingClaim 模式下 Chart 不得创建、修改或接管该 PVC。
- `HELM-STOR-003`：Chart 创建统一 PVC 时支持 size、storageClass、accessModes、
  volumeMode、annotations 和 selector 等必要参数。容量只对整个 claim 生效，不提供
  虚假的逐目录 quota；默认总容量在设计阶段确认。
- `HELM-STOR-004`：默认升级和卸载策略必须保护用户数据，不能因 `helm uninstall`、
  release rename 或 Chart 升级意外删除统一 PVC。具体 retain 机制在设计阶段确定并
  写入恢复说明。
- `HELM-STOR-005`：保留 `/tmp` writable `emptyDir`，并允许设置 sizeLimit；只读根
  文件系统开启时，应用所需的其它可写路径必须被显式列出，不能依赖镜像层可写。
- `HELM-STOR-006`：支持 `extraVolumes` 与 `extraVolumeMounts`，但必须记录与三个内置
  应用 mount path 冲突时的行为，并尽可能在模板阶段拒绝重复挂载。
- `HELM-STOR-007`：全新或空的统一 PVC 必须能够直接启动。Chart 必须在应用启动前
  安全创建三个逻辑目录并使 UID/GID 1000 可用，不得依赖 storage provisioner 恰好
  预建目录，也不得把整个卷放宽为 world-writable。
- `HELM-STOR-008`：三个目录共享容量、storageClass、access mode、快照和保留策略；
  values 与文档不得暗示它们仍可独立扩容或选择不同存储后端。
- `HELM-STOR-009`：统一 PVC 同时包含 SSH 凭据与普通 workspace 数据，因此整个 PVC、
  snapshot 和 backup 都必须按高敏感凭据材料保护，不能再把 workspace 备份视为普通
  非敏感数据。

### 9.2 State 与 workspace

- `HELM-STOR-STATE-001`：state 固定挂载到 `/home/roaminal/.roaminal`，用于 auth、
  active session snapshot 和 audit 材料；数据来源是统一 PVC 的 `state/` 目录，应用
  可见目录布局不得改变。
- `HELM-STOR-STATE-002`：workspace 默认挂载到 `/workspace`，并与默认
  `ROAMINAL_CWD` 对齐；数据来源是统一 PVC 的 `workspace/` 目录。自定义 cwd 时必须
  由用户保证对应路径存在且可写。
- `HELM-STOR-STATE-003`：为测试提供非持久 `emptyDir` 的必要性留待设计确认；若
  支持，必须由一个共享 `emptyDir` 提供同样的三个逻辑目录，显式启用并清楚标记所有
  state、workspace 和 SSH 数据都会随 Pod 删除。

### 9.3 SSH 数据

- `HELM-STOR-SSH-001`：默认 SSH 数据来自统一 PVC 的 `ssh/` 目录，并挂载到
  `/home/roaminal/.ssh`；不得另建 SSH PVC。
- `HELM-STOR-SSH-002`：引用统一 `existingClaim` 时必须使用其中的 `ssh/` 目录；也
  必须支持用户用 Secret、projected volume 或 CSI volume 替换这一处挂载，将整个
  `.ssh` 目录设为只读，而 state/workspace 仍使用统一 PVC。
- `HELM-STOR-SSH-003`：必须支持将 `config`、allowlisted private/public key 或
  known-hosts 文件直接挂载到 `.ssh` 下，而不强迫用户把整目录变成同一种来源。
- `HELM-STOR-SSH-004`：挂载矩阵必须保持现有产品语义：只读 config/key 可以建立
  connection，但结构化配置编辑、key generation、key delete 或 known-hosts 写入会
  因对应路径只读而不可用。
- `HELM-STOR-SSH-005`：Chart 不得把 private key 内容放入 values、ConfigMap、
  NOTES 或 rendered test fixture；只引用外部 Secret/CSI 对象及 key 名。
- `HELM-STOR-SSH-006`：文档必须说明 OpenSSH 对目录/文件权限的要求、UID/GID 1000
  的读取条件，以及某些 projected volume 权限由 kubelet 管理且不可由应用修复。

## 10. SecurityContext 与资源需求

- `HELM-SEC-001`：默认 Pod security context 保留 `runAsNonRoot: true`、UID/GID
  1000、`fsGroup: 1000`、`OnRootMismatch` 和 `RuntimeDefault` seccomp。
- `HELM-SEC-002`：默认 container security context 保留
  `allowPrivilegeEscalation: false`、`readOnlyRootFilesystem: true` 和 drop `ALL`。
- `HELM-SEC-003`：允许平台按需覆盖 security context，但安全默认值不能因为模板
  简化而消失；README 必须提示覆盖后的责任边界。
- `HELM-SEC-004`：默认 resources 保持当前 requests `100m/256Mi`、limits
  `2 CPU/2Gi`，并允许完整覆盖。
- `HELM-SEC-005`：不得默认启用 privileged、hostNetwork、hostPID、hostIPC、
  hostPath 或容器 runtime socket。
- `HELM-SEC-006`：生成的清单应满足 Kubernetes Restricted Pod Security 的核心
  要求；存储驱动与 fsGroup 的兼容例外必须在文档中说明。

## 11. Service、Ingress 与网络需求

- `HELM-NET-001`：默认创建 `ClusterIP` Service，默认端口 `9846`，端口名保持
  `http`；允许设置 Service type、port、annotations、labels 和受支持的附加字段。
- `HELM-NET-002`：Service selector 必须只选择当前 release 的 Roaminal Pod，多个
  release 安装在同一 namespace 时不得互相选中。
- `HELM-NET-003`：Ingress 默认关闭；启用后支持 ingressClassName、annotations、
  多 host/path 和 TLS secret 配置，使用 `networking.k8s.io/v1`。
- `HELM-NET-004`：Ingress 文档必须明确 TLS、same-origin、WebSocket upgrade 和
  至少 3600 秒长连接 timeout 的要求。Controller 专用 annotations 由用户提供，
  Chart 不默认假设 NGINX。
- `HELM-NET-005`：不得为 terminal worker、存储或任何内部 runtime 创建额外 Service。
- `HELM-NET-006`：NetworkPolicy 不在首版范围；文档可给出流量需求，但不得提供
  看似安全却可能阻断 SSH egress、DNS 或 Ingress Controller 的不完整默认策略。

## 12. 兼容与迁移需求

- `HELM-MIG-001`：必须提供从当前 `deploy/kubernetes/` 迁移到 Helm 的步骤，包含
  备份、暂停旧 Deployment、创建或选择统一 PVC、把旧三个 PVC 的内容分别迁移到
  `state/`、`workspace/`、`ssh/`、复用密码 Secret、资源命名和回滚方式。
- `HELM-MIG-002`：Chart install/upgrade 不能自动复制、重命名或删除旧 PVC/Secret，
  也不能轮换密码。数据汇入必须是管理员显式执行、可核对且可重试的迁移步骤；旧 PVC
  在验收完成前保持不变。
- `HELM-MIG-003`：`existingClaim` 只接受已经符合统一目录布局的单个 claim。验收要
  证明迁移后的 connection config、SSH key、workspace 和 state 均可读取。
- `HELM-MIG-004`：默认 Service 名称变化不能被隐藏。文档必须说明如何用 release
  name/fullnameOverride 保持 `roaminal.<namespace>.svc.cluster.local` 地址，或如何更新
  调用方。
- `HELM-MIG-005`：裸 YAML 在 Chart 达到功能等价且迁移文档完成前仍是基线；最终
  如何退役 `deploy/kubernetes/` 留待设计决策，但不得长期维护两套会漂移的资源源码。
- `HELM-MIG-006`：升级仍会中断活动 local/SSH/tmux connection。Chart 和 NOTES
  必须如实说明 `Recreate` 的停机语义，不得宣传 zero-downtime upgrade。

## 13. Values 与文档质量需求

- `HELM-VAL-001`：`values.yaml` 按 image、application/auth、persistence、service、
  ingress、security、resources、scheduling 和 extensibility 分组，并附简短注释。
- `HELM-VAL-002`：`values.schema.json` 必须校验类型、枚举、范围、必填项及互斥存储
  来源；不能只提供一个无约束 values 文件。
- `HELM-VAL-003`：默认 values 必须能成功 `helm lint` 和 `helm template`；需要外部
  Secret 的事实不应妨碍离线 render，但安装说明必须在 apply 前创建该 Secret。
- `HELM-VAL-004`：至少提供三类经过验证的示例 values：默认统一 PVC、统一
  existingClaim、统一 PVC 配合只读 SSH Secret/投影挂载。示例不得含真实凭据。
- `HELM-DOC-001`：Chart README 包含前置条件、安装、升级、卸载、Secret、存储、
  Ingress/WebSocket、迁移、备份、故障排查和安全提示。
- `HELM-DOC-002`：仓库根 README、`docs/deployment.md`、`docs/backup-recovery.md`、
  `docs/security.md` 和 release 文档必须与 Chart 成为部署事实源后的行为一致。
- `HELM-DOC-003`：`NOTES.txt` 只输出 Service/Ingress 访问提示与必要的后续检查，
  不读取或输出密码，不建议生产使用 port-forward。

## 14. ForgeKit、版本与发布需求

- `HELM-REL-001`：设计阶段根据参考项目确认 ForgeKit 对 Helm Chart 的原生能力，
  优先复用其 version source、lint 和 package 流程，不另写重复脚本。
- `HELM-REL-002`：Chart `version` 与 Roaminal `appVersion` 的职责必须分开说明；二者
  都不得形成需要人工同步的隐蔽重复版本源。
- `HELM-REL-003`：版本变更必须通过 ForgeKit 完成，不允许手工编辑受管版本文件。
- `HELM-REL-004`：Chart package 必须可从干净 git commit 重现，并能关联源码 commit、
  Chart version、appVersion 和默认镜像版本。
- `HELM-REL-005`：首版是否发布到 OCI registry、GitHub Release 或只保留源码包，
  留待设计阶段确认；无论发布渠道为何，本地 package 与 install 验收不可省略。

## 15. 验证与 Definition of Done

实施完成必须同时满足以下条件：

1. `helm lint chart/` 通过，schema 对错误类型、非法副本数、冲突存储来源和必要字段
   能给出清晰错误。
2. `helm template` 至少覆盖默认统一 PVC、existingClaim、只读 SSH Secret、Ingress TLS、
   自定义调度和 security context 覆盖矩阵；输出稳定且无明文 Secret。
3. 渲染后的 Kubernetes 资源通过目标 Kubernetes 版本的 API/schema 校验和
   server-side dry-run。
4. ForgeKit 根级 lint 纳入并通过 Chart 检查，现有 backend、worker、frontend gate
   继续通过。
5. 在 develop namespace 使用独立 Helm release 完成安装、`/healthz`、登录、local
   connection、SSH connection、WebSocket、Pod 重建与 Helm upgrade 验收。
6. 验证 `Recreate` 行为、探针、termination、只读根文件系统、非 root、无 SA token、
   同一 PVC 的三个逻辑目录/mount 和资源 requests/limits 与 values 一致。
7. 使用旧三个 PVC、预创建的统一 PVC 和 Secret 完成一次裸 YAML 到 Helm 的迁移演练，
   证明目录映射正确、密码没有轮换、数据没有丢失且旧 PVC 未被修改或删除。
8. `helm uninstall` 后按最终保留策略验证用户数据仍在，并记录显式清理方法。
9. 根文档、部署、安全、备份、发布和迁移说明全部更新，不再给出互相冲突的裸 YAML
   与 Helm 操作路径。
10. `chart/` 成为唯一部署资源源码；`deploy/kubernetes/` 按确认后的退役策略处理，
    不保留需要双重维护的模板副本。

## 16. 留待设计阶段确认的问题

以下问题会改变公开 values 或发布/迁移契约，不能由实施 agent 临时决定：

1. 密码是否严格只支持 existing Secret，还是额外提供默认关闭的测试用 managed
   Secret？推荐严格使用 existing Secret。
2. 统一 PVC 的默认总容量是多少，以及是否默认使用 Helm keep 策略；release 重装后
   如何明确重新认领 retained PVC？推荐默认保留，并提供 existingClaim 重装路径。
3. SSH 的 Secret/projected/CSI 挂载使用一等 values 模型，还是仅通过通用
   extraVolumes/extraVolumeMounts 提供？推荐为常见 SSH Secret 建立受校验的一等模型，
   同时保留通用扩展。
4. `deploy/kubernetes/` 在迁移后是全部删除，还是只保留一个指向 Chart 的 README？
   推荐删除资源 YAML，只保留迁移入口或直接由 `docs/deployment.md` 承担说明。
5. Chart version 是否与 Roaminal 产品版本同步，还是采用独立版本线？推荐独立 Chart
   version、明确 appVersion 映射，并全部由 ForgeKit 管理。
6. Chart package 的首个发布渠道是什么？这影响 CI 凭据、provenance 和安装文档，
   但不影响 `chart/` 源码及本地验收。
7. 首版承诺支持哪些 Helm/Kubernetes 版本？设计阶段应结合 develop 集群、当前上游
   支持窗口和所用 API 给出明确范围，不能只写“支持 Kubernetes”。
