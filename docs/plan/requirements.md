# Roaminal 连接管理需求

状态：需求草案。本文档记录产品需求、调研结论和讨论中已经确认的设计约束，
但不是实施计划或完整 API 设计，也不授权修改产品代码。

调研日期：2026-08-07。

## 1. 产品目标

Roaminal 的目标不再是一个恰好提供 Web terminal 的容器，而是一个操作平台：
用户通过它打开并管理通往本地及远程环境的终端连接。

下一个功能必须将 `connection` 确立为负责终端创建与生命周期的产品概念：

- local connection 在 Roaminal 运行环境中打开一个 shell；
- remote connection 根据 OpenSSH 用户配置打开通往远程主机的 SSH shell；
- terminal 是附着在 connection 上的交互渲染界面，而不是用户创建或管理的
  顶层对象；
- SSH 配置和密钥继续使用标准 OpenSSH 文件，Roaminal 不得成为 SSH 身份或
  主机数据的第二事实源。

## 2. 术语

设计必须区分已保存的配置与运行状态。如果二者都只称为“connection”，就会
重现当前 terminal 定义与 terminal session 之间的歧义。

| 术语 | 本文档中的含义 |
| --- | --- |
| Connection definition（连接定义） | 一个可启动的目标。local definition 是内置项；remote definition 来自 SSH `Host` 条目。 |
| Connection instance（连接实例） | 根据连接定义创建的一个存活或历史 local/remote terminal 运行实例。 |
| Local connection（本地连接） | 由 Roaminal 容器内本地 Bash PTY 支撑的连接实例。 |
| Remote connection（远程连接） | 由 Roaminal 发起的 SSH client session 支撑的连接实例。 |
| Terminal（终端） | 附着在连接实例上的 PTY/xterm 交互界面。它是实现和 UI 层面的界面，不是顶层产品实体。 |
| Authentication session（认证会话） | Roaminal 浏览器登录/refresh session，与连接实例严格区分。 |

面向用户的文案必须使用 `connection` 表达新的产品模型。后续设计必须盘点
API、持久化元数据、前端存储、测试和文档中的现有 `terminal` 与 `session`
命名，同时保留 authentication session 的独立含义。

## 3. 当前状态

当前实现存在以下约束，后续设计必须纳入考虑：

- `POST /api/sessions` 只能根据可选工作目录和终端尺寸创建本地 Bash PTY；
- heartbeat、WebSocket 路径、持久化元数据、前端状态、快捷键、侧边栏卡片和
  测试都使用 `session` 或 `terminal` 术语；
- 主工作区和 terminal exited 状态都可以直接创建 terminal；
- 运行实例元数据和 scrollback 存储在 `~/.roaminal` 下，与 SSH 文件相互独立；
- 当前运行时镜像不包含 OpenSSH client 或 `ssh-keygen`；
- 当前容器和 Kubernetes Deployment 没有挂载
  `/home/roaminal/.ssh`；
- 容器以 UID/GID 1000 和只读根文件系统运行，因此 SSH 数据必须通过显式
  部署资源提供；该资源可以是可写持久卷，也可以是只读 Secret 整目录或
  单文件挂载，二者提供的产品能力不同；
- 现有 terminal worker 负责终端状态和渲染，不负责 SSH 配置或认证。

这些是迁移输入，不能成为把 SSH 数据复制进现有 Roaminal 状态存储的理由。

## 4. Terminus/Tabby 调研

名为 Terminus 的项目于 2021 年更名为 Tabby。本文中的“Tabby”指当前项目，
“Terminus”指它更名前的名称。

相关官方资料如下：

- [Tabby 仓库与功能概览](https://github.com/Eugeny/tabby)；
- [项目更名说明](https://github.com/Eugeny/tabby/discussions/4087)；
- [SSH profile provider](https://github.com/Eugeny/tabby/blob/master/tabby-ssh/src/profiles.ts)；
- [OpenSSH config importer](https://github.com/Eugeny/tabby/blob/master/tabby-electron/src/sshImporters.ts)；
- [Profile 管理界面](https://github.com/Eugeny/tabby/blob/master/tabby-settings/src/components/profilesSettingsTab.component.ts)；
- [SSH transport multiplexer](https://github.com/Eugeny/tabby/blob/master/tabby-ssh/src/services/sshMultiplexer.service.ts)；
- [SSH tab/session 生命周期](https://github.com/Eugeny/tabby/blob/master/tabby-ssh/src/components/sshTab.component.ts)；
- [Tabby Web 架构](https://github.com/Eugeny/tabby-web)；
- [OpenSSH client 配置手册](https://man.openbsd.org/ssh_config.5)；
- [OpenSSH 密钥生成手册](https://man.openbsd.org/ssh-keygen.1)。

### 4.1 值得借鉴的模式

Tabby 将 profile 与运行中的 terminal tab 分离。Profile provider 提供 local
或 SSH 启动定义，打开 profile 才会创建新的运行 tab。这种分层适合 Roaminal
的连接定义/连接实例模型。

Tabby 的 profile selector 和 settings view 会对连接目标进行分组和过滤，
展示类型和说明，并从管理界面启动 profile。从 OpenSSH 导入的条目与应用自身
拥有的 profile 也有明确区分。

Tabby 的 OpenSSH importer 说明：实用的 connection manager 可以只展示具体
host alias，同时仍由 OpenSSH 承载更丰富的配置语言。它会递归读取 `Include`
文件、计算有效配置、从启动列表排除 wildcard alias，并只映射一组有限的常用
directive。

Tabby 支持两种彼此独立的复制行为：

- 把已保存 profile 克隆为另一个已保存 profile；
- 启用 session reuse 后，在兼容且已认证的 SSH transport 上打开另一个 shell
  channel。

第二种行为是 transport multiplexing，而不是复制 credential。Tabby 的
multiplexer 根据 destination、user、proxy 和 jump host 信息选择 session，
然后在已认证 transport 上打开新的 SSH shell channel。

### 4.2 与 Roaminal 需求冲突的模式

Tabby 把 custom profile 保存到自己的应用配置中。Roaminal 不能对 SSH
connection 这样做：复制 remote definition 必须创建新的 OpenSSH `Host`
条目。

Tabby 可以保存密码，也可以把 private key 导入加密 vault。Roaminal 明确
拒绝这两种能力，不得增加密码 vault、密钥 vault、密钥上传存储或私有 profile
数据库。

Tabby 的自动 key locator 接受范围较宽的 `id_*` 类名称，并把密钥内容读入
自身 SSH 实现。Roaminal 需要更严格的文件名和 key type allowlist，而且不得
向浏览器暴露 private key 内容。

Tabby Web 使用独立 connection gateway，因为浏览器不能直接建立任意 SSH TCP
连接。Roaminal 已经在 workload 内拥有可信后端，因此远程网络传输必须由
Roaminal runtime 持有，不能由浏览器 JavaScript 或新的第三方托管 gateway
持有。

### 4.3 与产品相关的 OpenSSH 约束

OpenSSH 配置具有顺序和条件语义。多数 directive 使用第一个取得的值；
`Host`、wildcard/negated pattern、`Match`、`Include`、token 和环境变量
展开都可能改变最终配置。因此，结构化编辑器不能只解析少数字段后，把整个文件
重新格式化写回。

OpenSSH 可通过 `ControlMaster` 和 `ControlPath` 共享一条网络连接；
内嵌 SSH 实现也可以在一个已认证 transport 上打开多个 shell channel。
但二者都无法事后克隆一个未预先暴露可复用 transport/control endpoint 的
任意手动启动 SSH 进程。

OpenSSH 支持生成 Ed25519 和 RSA 密钥。Private key 可以使用 passphrase
加密。`ssh-agent` 可以在独立进程中保存已解锁身份，但它不是复制 connection
definition、重复启动 connection instance 或通过 `ControlMaster` 复用已认证
transport 的必要条件，因此 Roaminal 不提供 external agent 集成。

## 5. 连接模型需求

### 5.1 通用模型

- `CON-001`：Roaminal 必须同时支持 local 和 remote connection definition。
- `CON-002`：每个 terminal runtime 都必须表示为拥有独立稳定 runtime ID
  的 connection instance。
- `CON-003`：连接实例必须记录自身类型为 `local` 或 `ssh`。
- `CON-004`：远程连接实例必须引用其来源 SSH host alias，不得把 host block
  复制到 Roaminal 持久化存储。
- `CON-005`：同一连接定义可以打开多个连接实例，各实例必须能够独立选择、
  关闭和观测。
- `CON-006`：现有本地 terminal 生命周期、scrollback、execution 状态、
  响应式布局和只读 closed history 必须继续通过本地连接实例提供。
- `CON-007`：SSH 配置读取失败或远程连接启动失败，不得导致本地连接或其他
  无关远程连接不可用。
- `CON-008`：API 响应和 UI 状态不得混淆连接定义与连接实例。

### 5.2 本地连接

- `LOC-001`：Connection manager 必须提供一个内置 local connection launcher。
- `LOC-002`：启动 local definition 必须创建新的本地 Bash PTY。启动请求可以
  携带一次性的可选 `cwd`；未提供时使用 deployment-level `ROAMINAL_CWD`，
  默认值为 `/workspace`，不得静默继承最近 local 或 remote instance 的目录。
- `LOC-003`：内置 local definition 不得用虚假的 SSH `Host` block 表示，
  也不得保存为 SSH 配置。
- `LOC-004`：内置 local definition 是唯一且不可变的系统 launcher；用户不能
  创建、编辑、复制、重命名或删除 local definition，也不提供具名 local
  launch template library。
- `LOC-005`：一次性 `cwd` 必须是 runtime 中存在、可访问的绝对目录；校验失败
  必须拒绝启动并返回明确错误。该值可以记录为新 connection instance 的
  `initialCwd`，但不得持久化为 definition 或改变后续启动默认值。
- `LOC-006`：Local launcher 固定启动 Roaminal 支持的交互式 `/bin/bash` 和
  application shell rc；connection manager 不提供 shell selector、任意启动
  command 或 environment variable editor。
- `LOC-007`：Local connection 的 title 属于 connection instance。Launcher
  不提供 title 字段；用户可以在实例创建后使用统一的 instance rename 能力。
- `LOC-008`：Terminal cols/rows 由实际 frontend viewport 和 resize protocol
  决定，不属于 local definition 或 launcher 表单字段。
- `LOC-009`：`ssh-keygen` 等由产品发起的受约束系统流程可以创建专用 local
  connection instance，但不得因此暴露通用 command template 或形成新的
  user-managed local definition。

### 5.3 远程连接

- `REM-001`：远程连接定义必须来源于用户 SSH 配置源可达的 OpenSSH
  `Host` 条目。
- `REM-002`：启动远程连接定义必须创建由 Roaminal backend/runtime 持有的
  交互式 SSH terminal，不能由浏览器持有。
- `REM-003`：启动目标必须是 SSH host alias，由 OpenSSH 配置语义决定最终
  destination 和 option。
- `REM-004`：OpenSSH 要求认证或 host verification 时，远程连接 terminal
  必须以普通 terminal I/O 承载对应 prompt。
- `REM-005`：远程断开、正常 shell exit、网络故障、删除和服务关闭必须产生
  明确的生命周期状态，不能冻结应用其他部分。
- `REM-006`：关闭远程连接实例必须释放其完整 SSH process/channel 资源，
  但不能删除对应 SSH definition。
- `REM-007`：删除 SSH definition 不得终止现有连接实例。实例必须继续运行并
  明确标记为 `source deleted`，直到用户关闭、远端退出或 transport 失败。
- `REM-008`：Live remote instance 的来源 Host block 被编辑后，既有 OpenSSH
  process/channel 不得重启或被动态改写；实例必须继续运行并明确标记为
  `source changed`。
- `REM-009`：来源被编辑或删除后，关联 authenticated transport 必须立即进入
  draining，拒绝创建新的 reuse channel；已经存在的 channel 可以继续运行。
- `REM-010`：来源被编辑后再次连接同一 alias，必须把当前 alias 交给 OpenSSH
  建立全新 transport 并重新认证，不得复用修改前的 transport。来源已删除时，
  connection manager 不得提供 reconnect 或 connect action。
- `REM-011`：删除后重新创建同名 Host alias 必须视为当前的新 definition
  revision；它不能重新绑定、恢复或解除旧 instance/transport 的 changed/deleted
  状态，新连接仍须使用全新 transport。
- `REM-012`：上述规则必须同等适用于 Roaminal 结构化修改、外部 editor 修改、
  Secret volume 轮换和其他可观测的主 config 更新。
- `REM-013`：Connection instance 可以持久化来源 alias，但不得持久化 Host block
  或 effective config 副本。用于识别 live source drift 的 definition revision
  token 只能是非敏感、不可逆且属于当前 runtime 的比较标识，不能成为配置缓存
  或跨 runtime 恢复 transport 的依据。

### 5.4 已确认的 SSH runtime

- `SSHRT-001`：系统 OpenSSH client 是 Roaminal 唯一的 SSH runtime；实际
  SSH transport 不得由 Go、Node 或浏览器 SSH library 建立。
- `SSHRT-002`：OpenSSH 负责完整解析 SSH config、建立网络连接、执行认证、
  校验 host key、维护 `known_hosts` 语义以及申请远程 PTY。
- `SSHRT-003`：Roaminal 负责启动和编排 OpenSSH 子进程，把它接入现有 PTY
  与 process group，并管理 terminal stream、连接生命周期和 history。
- `SSHRT-004`：Roaminal 可以调用 OpenSSH 辅助命令。首版允许使用
  `ssh-keygen` 生成密钥和读取 fingerprint，但不得使用 `ssh -G` 为结构化 UI
  查询、补全或展示 effective config。`ssh -O` 等 control command 只能用于
  已批准的 authenticated transport reuse lifecycle。
- `SSHRT-005`：启动远程连接时必须把用户选择的 `Host` alias 交给 OpenSSH，
  不能由 Roaminal 提取少量字段后重新构造一份实际连接配置。
- `SSHRT-006`：Roaminal application code 不得为了建立 SSH transport 解析
  private key，也不得实现 SSH authentication protocol。
- `SSHRT-007`：Password、keyboard-interactive、private-key passphrase 和
  host-key confirmation 均由 OpenSSH 在目标 terminal 中完成；Roaminal
  不得为它们建立结构化 secret flow。
- `SSHRT-008`：首版必须支持由用户对存活 remote connection 显式发起的
  authenticated transport reuse。未选择复用时，每个 remote connection
  instance 使用独立 transport；不得跨 definition 自动猜测或自动合并 transport。

## 6. SSH 配置事实源需求

### 6.1 规范存储

- `CFG-001`：`$HOME/.ssh/config` 是用户 SSH 配置的唯一事实源。
- `CFG-002`：Roaminal 不得在 `~/.roaminal`、数据库、浏览器存储或其他
  隐藏文件中持久化 SSH host definition、有效 SSH option 或持久化解析缓存。
- `CFG-003`：Connection manager 必须从 SSH 配置源读取当前数据；仅当实际
  config 具备已确认的安全写入能力时，才能把用户确认的结构化修改写回源文件。
- `CFG-004`：`$HOME/.ssh` 目录或 config 文件缺失时，必须表现为空白且
  可恢复的初始化状态，不能导致整个服务失败。
- `CFG-005`：用户通过 Roaminal 以外工具修改配置后，外部修改仍是权威数据，
  refresh/reload 后必须可见，不能要求 import 或同步。
- `CFG-006`：Roaminal 必须检测 edit form 加载后文件发生的变化，不能静默
  覆盖更新的外部内容。
- `CFG-007`：成功写入后必须留下由 runtime UID 拥有且符合 OpenSSH 安全要求
  的 config。Roaminal 不得对挂载或既有文件自动执行 `chown`、`chmod`，也不得
  为获得写权限而复制文件；新建临时文件和新 config 必须从创建时就使用安全
  ownership 和 permission。
- `CFG-008`：产品不得把用户 SSH config 烘焙、复制或迁移进容器镜像。
- `CFG-042`：主 config 可以是 `$HOME/.ssh` 下的普通文件，也可以是 Kubernetes
  Secret volume 使用 `..data` 布局提供的符号链接。读取符号链接时必须以已经
  打开的 `.ssh` 根目录为边界，限制链接深度，拒绝 magic link 和逃逸该根目录
  的目标，并要求最终目标是普通文件。
- `CFG-043`：只读或通过符号链接提供的有效 config 必须仍能被解析、展示并
  作为 OpenSSH launch target 使用；只读不能被误报为配置无效。
- `CFG-044`：Roaminal 的所有 config 写入都不得跟随符号链接。只有 runtime
  UID 拥有的普通 config 和可写父目录可以获得结构化写入能力；符号链接、
  只读 mount、非 runtime UID ownership 或不安全 permission 必须使 create、
  edit、duplicate 和 delete config 操作不可用，并展示具体原因。
- `CFG-045`：Root-owned config 是受支持的只读输入，前提是 runtime UID 可读、
  owner 为 UID 0、group/other 不可写且最终目标是普通文件。其他非 runtime UID
  ownership 不进入 managed config 模型，并必须报告安全状态。
- `CFG-046`：Config 是否可读、是否可写和具体不可写原因必须作为 capability
  元数据暴露；API 和 UI 不能通过一次失败的保存操作才发现只读状态。
- `CFG-047`：Secret volume 的原子 `..data` 目标切换必须被视为外部 config
  更新，refresh 或文件监控后重新读取实际目标；不得把旧解析结果变成事实源。
- `CFG-048`：通过 Kubernetes `subPath` 直接挂载的 config 可以读取和使用，
  但部署文档必须明确：Kubernetes 不会把后续 Secret 更新投射进已经运行的
  `subPath` mount，需要重建 Pod 才能取得新内容。

### 6.2 结构化 UI 支持范围

首版结构化编辑器的明确支持范围为：

- 一个不包含 wildcard 或 negation 的具体 `Host` alias；
- `HostName`；
- `User`；
- `Port`；
- 一个或多个引用受支持密钥的 `IdentityFile` directive；
- `IdentitiesOnly`；
- `StrictHostKeyChecking no`；
- `UserKnownHostsFile /dev/null`；
- `ServerAliveInterval`。

该子集需要满足：

- `CFG-009`：UI 必须允许创建、查看、编辑和删除能够安全识别的具体
  `Host` 条目。
- `CFG-010`：新 host alias 必须非空，在 UI 支持的具体 alias 模型中唯一，
  并且可作为有效 OpenSSH `Host` declaration。
- `CFG-011`：编辑受支持条目时，不得重排或规范化无关 block、comment、
  whitespace 或 directive。
- `CFG-012`：删除受支持条目时，只能删除用户选中的 block，不能删除继承的
  default 或 included content。
- `CFG-013`：当 inheritance 或 unsupported syntax 可能改变配置时，UI
  不得声称所展示字段是完整的 OpenSSH 有效配置。
- `CFG-014`：除上述明确列表外，`ProxyJump`、forwarding、`ProxyCommand`、
  `IdentityAgent`、`ControlMaster` 等 directive 不属于首版结构化编辑范围，
  但仍须按 advanced syntax 规则保留并交给 OpenSSH 执行。
- `CFG-023`：`IdentityFile` 必须支持零个、一个或多个值，并保持 OpenSSH
  中多个 identity 的原有顺序。
- `CFG-024`：`IdentitiesOnly` 结构化字段必须支持未设置、`yes` 和 `no`；
  未设置表示继承 OpenSSH 的有效配置。
- `CFG-025`：`StrictHostKeyChecking` 结构化字段首版只管理“未设置”和显式
  `no`；`UserKnownHostsFile` 结构化字段首版只管理“未设置”和显式
  `/dev/null`。文件中其他合法值必须保留并标记为 advanced syntax。
- `CFG-026`：Roaminal 不得默认或自动写入 `StrictHostKeyChecking no` 或
  `UserKnownHostsFile /dev/null`。用户对单个具体 `Host` 显式选择后才能写入，
  保存前必须说明：任一设置都会削弱 host verification；两者组合时，普通
  user-level host key 的持久化和变更保护将基本失效，并增加中间人攻击风险。
- `CFG-027`：只要某个连接显式使用上述任一高风险值，connection manager 和
  connection instance 必须持续显示 host verification weakened 状态，不能
  只在首次保存时提示。
- `CFG-028`：`ServerAliveInterval` 必须使用 OpenSSH 的秒数语义，接受非负
  整数；未设置表示继承有效配置，`0` 表示禁用该协议级 keepalive。

### 6.3 不支持和高级语法

- `CFG-015`：Roaminal 必须容忍结构化 UI 不理解的有效 SSH config 语法。
- `CFG-016`：只要 config 包含超出受支持子集的语法或语义，connection
  manager 就必须持续展示可访问的提示。
- `CFG-017`：在可行范围内，提示必须指出受影响的文件/block 和 directive，
  但不能把它误报为无效 OpenSSH 语法。
- `CFG-018`：Unsupported content 本身必须按字节保留。在同一 block 中编辑
  受支持字段时，不得改变 unsupported bytes。
- `CFG-019`：Unsupported syntax 不得妨碍用户使用外部工具编辑文件，也不得
  阻止主 config 中已由 connection manager 选中的具体 alias 继续由底层
  OpenSSH 按完整配置执行；本条不要求发现只存在于 `Include` file 的 alias。
- `CFG-020`：Wildcard/negated `Host` pattern、一个 `Host` 行上的多个
  alias、`Match`、`Include`、token expansion、environment expansion、
  任意 command、certificate 和 forwarding 均为 advanced syntax，不属于
  Roaminal 结构化 UI 支持范围。
- `CFG-021`：UI 不得从 wildcard 或 conditional block 虚构独立可编辑的
  host record。
- `CFG-022`：解析失败时，UI 必须提示该文件需要通过外部方式修复，并禁止
  destructive structured save。
- `CFG-029`：首版必须完全忽略 `Include` 内容；Roaminal 不得解析 include
  pattern、解析目标路径、读取 included file、递归遍历 include graph 或监控
  included file 的变化。
- `CFG-030`：只存在于 included file 的 `Host` 不得出现在 connection manager，
  也不能作为 Roaminal 首版的 launch target。用户需要把可管理的具体 Host
  定义在 `$HOME/.ssh/config` 主文件中。
- `CFG-031`：主 config 中的 `Include` directive 必须按字节保留，并触发
  unsupported syntax 提示，但 UI 不得展示 included file 的内容或路径展开
  结果。
- `CFG-032`：对于主 config 中可选择的具体 Host，实际启动后仍由系统 OpenSSH
  自行处理 `Include` 及其对 effective config 的影响；Roaminal 不得尝试模拟
  或覆盖该行为。
- `CFG-033`：Roaminal 不得编辑、创建、删除或重新格式化任何 included file。
- `CFG-034`：Roaminal Web UI 永远不得提供 `$HOME/.ssh/config` 的 raw editor
  或 raw viewer；该限制不是首版取舍，而是永久产品边界。
- `CFG-035`：HTTP API 不得向浏览器返回完整 raw config content。API 只能返回
  结构化 UI 支持的字段、安全的来源/版本元数据，以及 unsupported directive
  的名称和位置等提示信息。
- `CFG-036`：需要编辑高级配置的用户必须通过 local connection 中的终端工具
  或部署环境提供的外部工具直接修改 `$HOME/.ssh/config`；修改完成后，
  connection manager 必须按外部修改规则重新读取主文件。
- `CFG-037`：UI 不得把 wildcard/negated `Host` pattern、一个 `Host` 行上的
  多个 alias 或 `Match` block 解析、枚举或编辑为 connection definition。
- `CFG-038`：主 config 中的具体单 alias `Host` block 仍可作为 connection
  definition 展示和编辑；UI 只展示并修改该 block 中显式存在的受支持字段。
- `CFG-039`：具体 `Host` block 中未显式出现的受支持字段必须显示为“未设置”，
  不得用 wildcard、`Match`、OpenSSH default 或其他来源推导出的 effective
  value 填充。
- `CFG-040`：Connection manager 不得自动调用 `ssh -G` 或实现等价计算来
  渲染 effective config、生成字段预览或判断结构化表单的当前值。
- `CFG-041`：影响具体 alias 的 advanced block 必须原样保留并触发提示，但
  不能因此隐藏该 alias。实际启动连接时仍把 alias 交给系统 OpenSSH，由
  OpenSSH 按完整配置和顺序语义计算运行时 effective config。

## 7. SSH 密钥需求

### 7.1 来源和探测

- `KEY-001`：`$HOME/.ssh` 是唯一的密钥探测根目录。
- `KEY-002`：Roaminal 不得把 private/public key 文件复制到
  `~/.roaminal`、浏览器存储、数据库、缓存文件或镜像。
- `KEY-003`：探测必须是非递归的，并且仅限有明确文档的 Ed25519/RSA
  private key 文件名 allowlist。
- `KEY-004`：初始 allowlist 为 `id_ed25519`、以 `_ed25519` 结尾的名称、
  `id_rsa` 和以 `_rsa` 结尾的名称；相应 `.pub` 文件是关联 public key，
  不是 private key candidate。
- `KEY-005`：Allowlist 之外的名称不能作为可选 managed key 展示，即使其
  内容在技术上是可用 SSH key。
- `KEY-006`：文件名匹配本身不能作为 key type 证明；内容无效、不可读、
  类型不匹配或不受支持时，必须报告为 unavailable，且不能暴露其内容。
- `KEY-007`：引用探测策略以外密钥的 config 条目必须原样保留并标记为
  unmanaged；Roaminal 不得静默把它改写为探测到的密钥。
- `KEY-008`：Private key bytes 绝不能返回浏览器、渲染到 UI、写入日志、
  包含在诊断信息中或持久化到 terminal metadata。
- `KEY-009`：UI 可以展示 filename、algorithm、public fingerprint、
  public key 是否存在、文件状态和引用该 key 的 host alias 等安全元数据。
- `KEY-010`：用户可以显式复制 public key text；private key 的复制、下载和
  导入控件不在范围内。

### 7.2 生成

- `KEY-011`：Connection manager 必须支持在 `$HOME/.ssh` 下生成 Ed25519
  和 RSA key pair。
- `KEY-012`：生成时必须使用用户选择且符合 allowlist 的文件名，绝不能覆盖
  已存在的 private key 或 public key。
- `KEY-013`：生成的 private/public 文件必须具有与 OpenSSH 兼容的
  ownership、permission 和 format。
- `KEY-014`：新生成密钥必须通过普通探测流程出现；Roaminal 不得增加独立
  key registry。
- `KEY-015`：UI 不得包含 passphrase、confirm-passphrase、remember、
  reveal、recover 或 reset 字段。
- `KEY-016`：Key generation 必须使用原生交互式 `ssh-keygen` 流程。结构化
  UI 不得提供 passphrase 字段，也不得通过 `-N`、environment、HTTP API 或
  其他结构化 channel 提供 passphrase。
- `KEY-017`：Key deletion、key rotation、passphrase change、certificate
  generation、hardware-backed key 和 private key import 不属于本 feature
  基线的必需范围。
- `KEY-018`：Connection manager 只能收集 key algorithm、符合 allowlist 的
  filename、可选 comment，以及 RSA key size 等非敏感生成参数；完成参数和
  文件冲突校验后，必须创建独立 local connection 并把 `ssh-keygen` 接入 PTY。
- `KEY-019`：`ssh-keygen` 必须作为参数化子进程直接启动，不能把用户输入拼接
  成 shell command。Roaminal 不传递 `-N`，由 `ssh-keygen` 在 PTY 中原生询问
  passphrase；用户直接回车时可以生成无 passphrase key。
- `KEY-020`：Passphrase terminal input 只能作为普通 PTY input 瞬时透传；
  Roaminal 不得解析、验证、缓存、记录、回放或发送到 telemetry。PTY input
  logging 必须关闭，且 passphrase 不得进入 connection metadata 或 history。
- `KEY-021`：`ssh-keygen` 退出后必须展示其真实 exit 状态；只有成功退出后才
  重新执行普通 key discovery，不能根据预期 filename 虚构生成成功状态。
- `KEY-022`：Key discovery 可以按 `CFG-042` 的受约束规则读取 Kubernetes
  Secret `..data` 符号链接，但最终目标必须是 `$HOME/.ssh` 边界内的普通文件；
  不得递归扫描或跟随逃逸边界的链接。
- `KEY-023`：Root-owned、runtime-readable 的 Secret private key 是受支持的
  只读 key input。其 mode 不得包含 group/other write bit，并且必须让 runtime
  UID 实际可读；Kubernetes Secret 常见的 root-owned `0444` 或 `0644` 可以满足
  该条件。Runtime-owned private key 仍必须满足 OpenSSH 对其 owner 的严格
  private-key permission 检查。
- `KEY-024`：只读或符号链接 key 仍须出现在 key inventory 中并可供
  `IdentityFile` 选择和 OpenSSH 使用，但必须标记为只读。Roaminal 不得修改、
  删除、覆盖、`chmod`、`chown` 或复制该 key。
- `KEY-025`：Key generation 只有在 `.ssh` 目标目录可写、目标名称及其
  `.pub` 均不存在且可以安全创建普通文件时才可用；现有符号链接或只读文件
  必须按“已存在且不可覆盖”处理。

## 8. 连接管理界面需求

- `UI-001`：Roaminal 必须提供一个独立于 terminal workspace 的、需要认证的
  connection management view。
- `UI-002`：该界面必须展示内置 local launcher、remote SSH host definition、
  SSH config 健康状态/提示以及探测到的 key inventory。
- `UI-003`：Remote definition 必须可以按 host alias 和能够安全识别的
  destination metadata 进行搜索/过滤。
- `UI-004`：每个受支持 remote definition 必须提供明确的 connect、edit、
  duplicate 和 delete 操作。
- `UI-005`：该界面必须提供 create host、generate key 和 refresh from disk
  操作。
- `UI-006`：任何 local 或 remote terminal 的启动都必须从该管理界面发起。
- `UI-007`：现有主 terminal workspace 不得再通过 button、menu、empty
  state、exited-state action 或 keyboard shortcut 直接创建
  terminal/connection。
- `UI-008`：Workspace 中保留的任何 create 类入口只能导航到 connection
  manager，不能直接创建实例。
- `UI-009`：启动后必须进入 workspace，并选中新创建的连接实例。
- `UI-010`：Workspace 必须继续支持在现有 live/history connection instance
  之间切换，切换时不能重新创建它们。
- `UI-011`：Connection management view 必须适配现有 desktop、tablet 和
  phone viewport matrix，不能隐藏 destructive action，也不能发生文本重叠。
- `UI-012`：Unsupported config 提示和 key error 不能仅依赖颜色表达，并且
  在过滤结果后仍须可见。
- `UI-013`：Connection manager 必须分别显示 config read、config write、
  key read、key generation 和 host-trust persistence capability。只读来源仍可
  connect，但所有不可执行操作必须预先禁用并说明原因。
- `UI-014`：编辑或删除存在 live instance 的 Host 前，UI 必须展示受影响的
  live instance 数量，并说明它们会继续运行、但旧 transport 将停止接受复用。
- `UI-015`：`source changed` 和 `source deleted` 必须在 workspace 的对应
  connection instance 上持续可见，不能只显示一次 toast，也不能仅依赖颜色。
- `UI-016`：`source changed` instance 的 reconnect 必须明确表示会使用当前
  definition 创建新 instance 并重新认证；`source deleted` instance 不得显示
  reconnect、duplicate-source 或 reuse action，但仍可正常切换、使用和关闭。

## 9. 密码、认证和主机信任需求

- `AUTH-001`：Roaminal 不得存储、缓存、记忆、同步、记录日志或暴露远程账户
  密码。
- `AUTH-002`：Roaminal 不得存储、缓存、记忆、同步、记录日志或暴露 private
  key passphrase。
- `AUTH-003`：任何 HTTP API 或结构化 UI form 都不得接收上述两类密码。
- `AUTH-004`：原生 SSH password、keyboard-interactive 或 encrypted-key
  prompt 可以出现在 terminal byte stream 中，但 Roaminal 不得把对应值转化
  为结构化应用数据或历史记录。
- `AUTH-005`：Roaminal 不得以满足“无密码管理”为由引入 vault 或加密 secret
  store。
- `AUTH-006`：Roaminal 不提供 external `ssh-agent` 集成；Web UI、HTTP API
  和 runtime configuration 不得提供 agent socket path、agent identity list、
  agent health/status 或 agent lifecycle 管理。
- `AUTH-007`：Roaminal 默认必须保持 host identity verification 启用，不能
  为了简化 Web 流程自动设置 `StrictHostKeyChecking no`、丢弃
  `known_hosts` 或接受发生变化的 host key。只有用户按 `CFG-025` 至
  `CFG-027` 对单个 Host 显式选择时，才允许写入高风险 override。
- `AUTH-008`：如果 SSH client 写入 `known_hosts` 等用户 host trust 数据，
  它们必须归属 `$HOME/.ssh`，不能复制进 `~/.roaminal`。显式配置
  `UserKnownHostsFile /dev/null` 的 Host 不会持久化这类 trust data，UI 必须
  如实展示该状态。
- `AUTH-009`：安全敏感 SSH prompt 必须明确归属于目标连接实例，避免多个
  并发连接造成用户混淆。
- `AUTH-010`：官方 Podman 和 Kubernetes 部署不得挂载、创建或注入
  `SSH_AUTH_SOCK`，也不提供 agent sidecar。用户通过 unsupported advanced
  config 和自定义部署自行使 agent 可用时，属于系统 OpenSSH 的外部行为，
  Roaminal 不保证、探测或管理该行为，但也不得为阻止它而改写用户 SSH config。
- `AUTH-011`：Unknown/new host key confirmation 必须由首个 OpenSSH master
  在目标 connection PTY 中原生展示和读取；Roaminal 不得建立结构化 trust
  dialog、confirmation API 或独立 trust decision store。
- `AUTH-012`：Roaminal 不得使用 `ssh-keyscan`、预抓取 key、自动确认或其他
  preflight 结果代替 OpenSSH 在实际 transport handshake 中执行的 host-key
  verification。
- `AUTH-013`：用户接受 unknown host key 后，只能由 OpenSSH 按实际 effective
  config 写入 `UserKnownHostsFile`；Roaminal 不得复制、模拟或重复该写入。
- `AUTH-014`：Host key changed 时，必须保留 OpenSSH 的完整 terminal warning
  和真实非零 exit 状态。Roaminal 不得自动删除旧 key、自动接受替换 key 或
  提供一键绕过 verification 的操作。
- `AUTH-015`：由于 OpenSSH 没有为 changed host key 提供独立稳定的 exit code，
  Roaminal 不得依赖 locale-sensitive terminal 文本解析生成精确 failure type；
  结构化状态使用 SSH connection failed，安全细节以原生 terminal output 为准。
- `AUTH-016`：只有首个 master 执行 host verification；复用 channel 使用已经
  认证的同一 transport。Master verification 未成功时不得发布 reusable
  control endpoint，也不得创建任何复用 channel。
- `AUTH-017`：Roaminal 不得覆盖用户通过 advanced config 设置的
  `StrictHostKeyChecking`、`UserKnownHostsFile` 或其他 host trust 行为；显式
  weakened 配置仍必须按 `CFG-025` 至 `CFG-027` 持续提示。
- `AUTH-018`：当整个 `.ssh` 或实际 `known_hosts` 目标只读时，Roaminal 必须
  明确展示 host trust 无法持久化。OpenSSH 可以在用户原生确认 unknown key 后
  继续当前连接，但不能声称该信任已经保存；后续连接可能再次提示。要求已知
  host 的严格策略仍可让 unknown host 连接失败，Roaminal 不得绕过。

## 10. 运行时持久化和迁移需求

- `STATE-001`：禁止冗余存储的规则适用于 SSH definition、key file、key
  material 和 password；该规则不禁止 Roaminal 持久化现有产品所需的连接实例
  元数据与 terminal scrollback。
- `STATE-002`：持久化的 remote instance metadata 可以包含 host alias 和
  非敏感生命周期/显示数据，但不能包含复制的 config directive、effective
  config dump、private/public key bytes、password 或 passphrase。
- `STATE-003`：当前系统没有需要兼容的用户数据。新实现不得读取、迁移或继续
  写入旧 session metadata v1/v2、旧 session snapshot layout 或旧浏览器
  active-session state。
- `STATE-004`：新持久化模型必须从 connection-instance format v1 开始，不能
  为尚未发布的 session schema 保留 compatibility decoder、dual-read 或
  dual-write path。
- `STATE-005`：Local 和 remote connection instance 均不得跨 application、
  container 或 Pod restart 恢复 process、PTY 或 transport。Terminal snapshot
  只能恢复只读历史画面，不能作为 process continuation。
- `STATE-006`：SSH 文件的 backup/recovery 是独立于 `~/.roaminal`
  backup/recovery 的 volume 问题；文档必须分别说明，不能把一个 store 复制到
  另一个。
- `STATE-007`：Canonical runtime persistence directory 必须命名为
  `connection-instances`，metadata type 必须命名为 `ConnectionInstanceMeta`。
  检测到非空 legacy `sessions` directory 时必须以明确的 incompatible
  pre-release state 错误停止，不能静默删除、导入或忽略旧数据。
- `STATE-008`：持久化 lifecycle 至少必须区分 `live`、`exited` 和
  `interrupted`。启动时发现属于旧 runtime 且最后状态为 `live` 的实例，必须
  原子标记为 `interrupted`，不能为其自动启动 local shell 或 SSH client。
- `STATE-009`：`exited` 和 `interrupted` instance 的 terminal snapshot 必须
  保持可查看但只读；WebSocket attach 或前端恢复不得重新启用 input。
- `STATE-010`：对 historical local instance 执行 relaunch，或对 historical
  remote instance 执行 reconnect，必须创建新的 `connectionInstanceId`、PTY、
  process，并在 remote 情况下创建新的 SSH transport 和重新认证。
- `STATE-011`：新实例可以持久化非敏感的
  `reconnectFromConnectionInstanceId` 或 `relaunchFromConnectionInstanceId`
  以表达来源关系，但不得继承旧 lifecycle、exit status、terminal output、
  process state 或 authenticated state。
- `STATE-012`：只有 backend process 仍属于同一 runtime 时，浏览器刷新、
  WebSocket 暂时断开、界面切换或另一个浏览器 client attach 才是同一
  connection instance 的 continuation。
- `STATE-013`：正常 shutdown 必须在终止 process 前把仍 live 的实例标记为
  `interrupted`；异常退出后必须在下次启动根据 runtime identity 完成同样归类。

### 10.1 术语与契约命名

- `NAME-001`：产品领域模型必须始终区分 `connection definition` 和
  `connection instance`；API、JSON、前端 state 和 backend model 不得使用
  无限定的 `session` 或含义不明的 `connection` 表示二者之一。
- `NAME-002`：Connection definition 的 canonical HTTP resource 名称为
  `/api/connection-definitions`，运行实例的 canonical HTTP resource 名称为
  `/api/connection-instances`。
- `NAME-003`：Connection instance terminal stream 的 canonical WebSocket
  path 为 `/ws/connection-instances/{connectionInstanceId}`。Runtime contract
  中必须使用 `connectionDefinitionId`、`connectionInstanceId`、
  `connectionDefinitions`、`connectionInstances` 和
  `activeConnectionInstanceId` 等明确名称。
- `NAME-004`：现有 `/api/sessions`、`/ws/{sessionId}`、heartbeat `sessions`
  field、`SessionSummary` 和相应 runtime contract 必须直接删除，不提供 alias、
  redirect、`410 Gone` tombstone 或 deprecation window。
- `NAME-005`：Runtime HTTP/WebSocket contract 继续作为首个正式
  `roaminal.v1` 定义；不得仅为从未发布的旧 contract 保留兼容而升为 v2。
- `NAME-006`：Capacity 配置的 canonical 名称为 `maxConnectionInstances` 和
  `maxClientsPerConnectionInstance`；CLI 使用 `--max-connection-instances`、
  `--max-clients-per-connection-instance`，environment 使用
  `ROAMINAL_MAX_CONNECTION_INSTANCES`、
  `ROAMINAL_MAX_CLIENTS_PER_CONNECTION_INSTANCE`。旧名称必须直接删除。
- `NAME-007`：浏览器 active selection key 必须使用
  `roaminal_active_connection_instance_v1`，value 使用
  `activeConnectionInstanceId`。旧 localStorage key 不读取、不迁移。
- `NAME-008`：Authentication login session 继续使用 `/api/auth/sessions`、
  `auth-sessions.json` 和 auth session terminology；它与 connection instance
  是不同领域概念，不能随本次改名。
- `NAME-009`：`terminal-worker`、xterm、PTY、terminal viewport 和 terminal
  snapshot 等真实 terminal-engine 概念保留 terminal 命名。Worker protocol
  内部对象使用 `terminalId`，不得使用含义冲突的 `sessionId`。
- `NAME-010`：Backend 必须由 connection domain 负责 definition、instance、
  SSH transport 和 lifecycle；terminal domain 只负责 PTY/stream/rendering
  能力。代码 package、type 和 variable 命名必须体现该 ownership boundary。

## 11. 容器和部署需求

- `DEP-001`：Production runtime 必须具备已批准设计所需的 SSH client/key
  generation 能力；当前镜像尚不具备。
- `DEP-002`：`/home/roaminal/.ssh` 可以由 runtime UID 可写的持久卷提供，也
  可以由只读 Kubernetes Secret 整目录挂载提供；还必须支持把 `config` 或
  allowlisted key 直接挂载到对应路径。写 config、生成 key 和持久化
  `known_hosts` 分别取决于相关目录/文件的实际写入能力，不能要求整体同为
  可写或只读。
- `DEP-003`：SSH config 和 key 必须来自能够跨 Pod replacement 保留的外部
  来源，例如持久卷或 Kubernetes Secret，不能存入镜像或 Roaminal state
  directory。只读来源由部署系统负责更新，Roaminal 不得制作可写副本。
- `DEP-004`：没有 SSH mount 的部署必须进入明确的初始化状态，并保留 local
  connection 功能。
- `DEP-005`：必须保持只读根文件系统、非 root UID/GID、drop capabilities、
  no privilege escalation 和 `tini` process model。
- `DEP-006`：Remote connection 从 Roaminal workload 网络发起；operator
  必须能够明确判断 Pod egress、DNS、firewall、proxy 和 bastion 可达性。
- `DEP-007`：建立 SSH 连接不得依赖浏览器、第三方托管 gateway、host
  filesystem scan 或 container runtime socket。
- `DEP-008`：Mount 和 backup 文档必须把 `.ssh` 视为高度敏感、由用户控制
  的数据，不能把其内容放入日志或 support bundle。

### 11.1 挂载方式与能力矩阵

下表是必须支持的最低能力；单文件场景仍以其余 `.ssh` 路径的真实权限为准。

| 挂载方式 | 读取 config/key 并连接 | 修改 config | 生成 key | 持久化 `known_hosts` |
| --- | --- | --- | --- | --- |
| Runtime UID 拥有的可写 `.ssh` 持久卷 | 支持 | 支持 | 支持 | 支持 |
| 整个 `.ssh` 为只读 Secret volume | 支持 | 不支持 | 不支持 | 不支持 |
| 只读 `config` 单文件挂载，其余 `.ssh` 可写 | 支持 | 不支持 | 支持 | 支持 |
| 只读 private key 单文件挂载，其余 `.ssh` 可写 | 支持 | 支持 | 支持其他未占用名称 | 支持 |
| 没有 `.ssh` 来源且只读 root filesystem 无法创建 | 仅 local connection | 不支持 | 不支持 | 不支持 |

- `DEP-009`：整目录 Secret volume 的符号链接轮换必须在同一 Pod 中通过重新读取
  生效；`subPath` 单文件 mount 的非实时更新限制必须写入部署文档和运维提示。
- `DEP-010`：Roaminal 不得为了把只读部署提升为可写能力而引入 init container
  复制、自动 ownership 修复或 shadow `.ssh`。需要编辑能力的部署必须显式提供
  runtime UID 可写的持久存储。

## 12. 连接复制和复用需求

“复制 connection”包含三种本质不同的含义。设计必须分别命名，不能把其中
一种伪装成另一种。

### 12.1 复制连接定义

- `COPY-001`：可以把受支持的具体 SSH definition 复制为新的具体
  `Host` 条目。
- `COPY-002`：复制必须要求新的唯一 host alias，并写入 OpenSSH 事实源，
  不能写入 Roaminal profile store。
- `COPY-003`：只能复制能够安全保留的内容。不能把 unknown 或 inherited
  effective option 展平为误导性的 block。
- `COPY-004`：复制连接定义不能复制运行 process state、terminal output、
  password、passphrase、host trust decision 或 unlocked credential。

### 12.2 打开另一个连接实例

- `COPY-005`：用户必须能够从同一个 local 或 remote definition 启动多个
  连接实例。
- `COPY-006`：每个新实例都必须具有独立 runtime ID、terminal state、
  title/lifecycle state 和 close/delete 行为。
- `COPY-007`：重复启动 remote connection 时可能需要重新认证，除非可信的
  transport reuse mechanism 使其不再需要；external agent 不是满足该需求的
  Roaminal 产品能力。

### 12.3 复用已认证 SSH 传输

- `COPY-008`：首版必须交付 authenticated transport reuse。用户必须能够从
  一个仍然存活且由 Roaminal 建立的 remote connection 显式打开复用同一
  transport 的新 connection instance，而不重新执行 SSH authentication。
- `COPY-009`：Reuse 评估必须限定在 OpenSSH 持有 transport 的机制内，重点
  评估 `ControlMaster`、`ControlPath`、`ControlPersist` 和相应 control
  command；application-owned SSH library multiplexing 已不再是候选方案。
- `COPY-010`：Reuse identity 必须包含所有可能改变安全或路由上下文的
  option，包括 effective destination、port、user、identity/agent、
  jump/proxy path、host-key policy 和 forwarding context。
- `COPY-011`：复用的 channel 必须拥有独立 terminal lifecycle 和 failure
  isolation；共享 transport 的 teardown 必须明确且可观测。
- `COPY-012`：Reuse 绝不能依赖 Roaminal 记住 password 或 private-key
  passphrase。
- `COPY-013`：设计必须定义 idle lifetime、maximum channel、ownership、
  fallback behavior，以及原始实例关闭后会发生什么。
- `COPY-018`：Transport reuse 只能由用户从明确选中的 live remote instance
  发起；Roaminal 不得仅凭 Host alias、destination 或部分配置自动匹配其他
  reusable transport。
- `COPY-019`：首个连接必须从建立之初就由 Roaminal 准备 reusable OpenSSH
  control endpoint；不得声称可以把普通既有 SSH process 事后转换为 master。
- `COPY-020`：每个复用 channel 必须表现为独立 connection instance，并拥有
  自己的 runtime ID、PTY、title、scrollback、close 和 exited state。
- `COPY-021`：关闭任一 channel 不得终止仍被其他 live instance 使用的共享
  transport；共享 transport 失效必须让所有受影响实例得到明确状态。
- `COPY-022`：来源 Host definition 被修改后，不得再向旧 transport 添加新
  channel；现有 channel 必须按 `REM-008` 和 `REM-009` 继续运行并展示
  `source changed`，旧 transport 进入 draining。
- `COPY-023`：Reusable transport 和 control endpoint 只属于当前 Roaminal
  runtime，不得跨 application、container 或 Pod restart 恢复或持久化。
- `COPY-024`：每个新 reusable transport 的首个交互式 OpenSSH connection
  必须同时充当 `ControlMaster`，使 password、passphrase 和 host-key prompt
  继续在该 connection 的原生 PTY 中完成；不得为认证另建隐藏 terminal。
- `COPY-025`：每个 transport 必须使用不可预测的 runtime transport ID 和
  独立 `ControlPath`。Control socket 必须位于 Roaminal 专用、权限为 `0700`
  的短路径 runtime 临时目录，不能位于 `$HOME/.ssh`、`~/.roaminal` 或其他
  持久化 volume。
- `COPY-026`：Roaminal 必须通过当前 OpenSSH process 的命令行 runtime option
  设置所需的 `ControlMaster`、`ControlPath` 和 `ControlPersist` 行为，不得把
  产品管理的 multiplexing directive 写入用户 SSH config。
- `COPY-027`：创建复用 channel 前必须验证目标 master 和 control socket 属于
  所选 live transport。验证失败或存在 lifecycle race 时，复用操作必须明确
  失败，不能让 OpenSSH 静默 fallback 为新的独立 transport。
- `COPY-028`：`ControlPersist` 只用于让 master 生命周期脱离首个 channel；
  Roaminal 不提供长期 idle transport pool。最后一个 live channel 关闭后必须
  使用 OpenSSH control command 终止 master 并清理 control socket。
- `COPY-029`：Host definition 修改后，共享 transport 必须进入 draining 状态，
  并通过 OpenSSH `stop` control command 拒绝新 multiplex request；现有 channel
  在各自结束前可以继续运行。
- `COPY-030`：Application 正常关闭时必须使用 OpenSSH control command 终止
  所有 master。异常退出遗留的 socket 必须随 runtime 临时 volume 消失，且
  启动恢复不得把单独存在的旧 socket 视为 authenticated transport。

### 12.4 手动建立的嵌套 SSH 会话

- `COPY-014`：Roaminal 不得承诺克隆用户在 local terminal 内手动输入
  `ssh ...` 启动的任意 SSH session。
- `COPY-015`：除非该进程从一开始就使用由产品持有的 reusable control
  endpoint，否则通常无法在事后重建或接管它。
- `COPY-016`：Command history parsing 不能视为 effective SSH config 或
  authentication state 的证明，也不能用于提取 credential。
- `COPY-017`：未来可以探索显式“保存为 SSH config entry”流程，但它不在
  本需求基线内，且需要单独制定需求。

## 13. 数据完整性和并发需求

- `DATA-001`：多个浏览器 tab、外部 editor 或 automation 并发修改 SSH
  config 时，structured edit 必须具备 conflict safety。
- `DATA-002`：Parse、validation、write、permission update 或 durability
  步骤失败时，最后一个有效文件必须可恢复，并给出可操作的错误信息。
- `DATA-003`：Partial write 和意外替换为空文件是不可接受的 failure mode。
- `DATA-004`：成功写入后，UI 必须重新读取并展示实际从磁盘读回的数据。
- `DATA-005`：通过 alias、key filename、include path 或 API input 发起的
  path traversal，不能向浏览器暴露任意 server file。
- `DATA-006`：对 `.ssh`、主 config 和 key 的路径访问必须使用 descriptor-
  relative、抗 TOCTOU 的受约束解析；安全读取只允许落在已固定 `.ssh` 根目录
  下的普通文件，写入则额外禁止跟随任何符号链接。Included file 已明确不读取、
  不解析和不写入。
- `DATA-007`：Config 原子写入只能在可写普通文件模型中进行，必须在同一目录
  安全创建临时普通文件、完成 durability 后以原子 rename 替换，并在每一步
  验证目标仍位于固定目录且没有变为符号链接。
- `DATA-008`：只读来源的 refresh、Secret symlink target 轮换和 writable
  config 外部替换都必须使 capability、版本和解析结果一起更新，不能沿用旧
  inode 的可写判断。
- `DATA-009`：每个 live remote transport 必须绑定其启动时的 runtime-only
  definition revision。检测到对应 Host block 内容变化、删除或同名重建时，
  状态转换和 transport draining 必须是幂等的，并且不能因快速连续文件更新而
  重新启用旧 transport。

## 14. 必需用户流程

后续设计和验收测试至少必须覆盖：

1. 打开 connection manager，启动 local connection，并进入可用的本地
   terminal。
2. 读取现有具体 SSH host 条目并启动它，在 terminal 中完成任何原生 SSH
   prompt，然后使用 remote shell。
3. 使用探测到的 key 创建具体 SSH host 条目；页面 reload 后，从
   `.ssh/config` 读取到同一定义，且 Roaminal 没有保存副本。
4. 编辑一个受支持 host 字段，同时保留 comment 和无关 advanced syntax。
5. 打开包含 wildcard、`Match`、`Include` 或 unknown directive 的主 config；
   看到提示且文件内容得到保留。只存在于 included file 的 alias 不出现在
   connection manager；主 config 中可选择的 alias 仍由 OpenSSH 应用完整配置。
6. 探测 Ed25519/RSA key pair，查看安全元数据，并复制选中的 public key，
   private key 不得暴露。
7. 生成 Ed25519 或 RSA key pair，不覆盖现有文件，并在新 host 条目中使用。
8. 把受支持 host definition 复制为新 alias，确认两个条目都存在于
   `.ssh/config`。
9. 从同一 SSH definition 打开两个独立 runtime instance；无论是否启用
   transport reuse，关闭一个都不能关闭另一个。
10. Edit form 打开期间从外部修改 `.ssh/config`，收到 conflict 而不是丢失
    外部修改。
11. 在没有 `.ssh` mount 或 config file 的环境运行；local terminal 仍可用，
    SSH 界面展示明确初始化状态。
12. 尝试复制手动输入命令建立的嵌套 SSH session，得到如实说明：无法克隆其
    authenticated transport。
13. 分别以整目录 Secret、只读 config 单文件和只读 key 单文件启动；所有可读
    definition/key 均可用于连接，不可写操作已禁用，且 Roaminal 没有修改权限
    或制作副本。
14. 更新整目录 Secret 的 config/key 后，refresh 得到新内容；使用 `subPath`
    时界面或部署说明明确需要重建 Pod 才能取得 Secret 更新。
15. 在整个 `.ssh` 只读且遇到 unknown host 时，通过 OpenSSH 原生 prompt 可以
    完成当前连接，同时 UI 不声称 host trust 已经持久化。
16. 在 remote instance 存活时编辑其来源 Host；旧 terminal 不受中断并显示
    `source changed`，旧 transport 不再接受复用，再次连接使用新配置、新
    transport 并重新认证。
17. 在 remote instance 存活时删除其来源 Host；旧 terminal 继续运行并显示
    `source deleted`，reconnect、duplicate-source 和 reuse 不可用。重新创建
    同名 Host 后，新连接也不得绑定或复用旧 transport。
18. 通过外部 editor 和 Secret volume 轮换重复上述编辑/删除流程，得到与
    Roaminal 结构化编辑完全相同的状态转换。

## 15. 本需求基线的非目标

以下是永久非目标，不得由后续普通 feature 或设计变更引入：

- 在 Web UI 中提供 SSH config raw editor 或 raw viewer；
- 通过 HTTP API 向浏览器提供完整 raw SSH config content。

除非后续经单独评审的需求明确纳入，否则不要求：

- 保存 remote password 或 private-key passphrase；
- Roaminal key/config vault 或 profile database；
- import、upload、download、reveal 或 delete private key；
- 结构化 UI 支持全部 `ssh_config` directive；
- SFTP/file browser、SCP、port-forward management、X11、Telnet、serial、
  RDP 或 Kubernetes exec connection；
- hosted SSH connection gateway；
- 克隆任意嵌套 SSH process；
- named local profile library；
- 自动向远端 `authorized_keys` 分发密钥；
- SSH certificate、CA management、FIDO/security-key generation、PKCS#11
  或 cloud provider host discovery；
- External `ssh-agent` socket 注入、agent identity 探测以及 agent lifecycle
  管理；
- 实施阶段、package boundary 或 API schema；SSH runtime 选择已经在
  `SSHRT-*` 需求中确定。

## 16. 验收基线

只有设计能够满足以下全部可观测结果，才可以进入实施阶段：

- Local 和 SSH target 被表示为连接定义，并生成可独立管理的连接实例；
- 系统 OpenSSH 是唯一实际 SSH runtime，Roaminal 仅编排其主命令和辅助命令；
- 所有创建流程都从独立 connection manager 发起；
- 主 terminal workspace 不再包含直接创建操作；
- `.ssh/config` 是 remote connection definition 的唯一持久化存储；
- `$HOME/.ssh` 是唯一的密钥探测/生成存储；
- `~/.roaminal`、浏览器存储、日志或诊断信息中不出现 SSH config/key 的
  持久化副本、password 或 passphrase；
- Private key bytes、remote password 和 key passphrase 绝不出现在 HTTP API
  payload；认证响应可以携带受支持 config view，且只有用户显式执行复制
  public key 操作时才可以返回 public key text；
- Web UI 和 HTTP API 均不提供完整 SSH config 的 raw editor、raw viewer 或
  raw content；高级配置只能通过 local connection 或外部工具直接编辑；
- 结构化修改可以保留无关和 unsupported config content；
- 首版结构化编辑器支持 `Host`、`HostName`、`User`、`Port`、
  `IdentityFile`、`IdentitiesOnly`、`StrictHostKeyChecking no`、
  `UserKnownHostsFile /dev/null` 和 `ServerAliveInterval`；
- 高风险 host verification override 只能由用户对单个 Host 显式启用，保存前
  必须警告，并在连接定义和运行实例上持续显示 weakened 状态；
- Unknown host key 由首个 OpenSSH master 在目标 terminal 中原生确认；changed
  host key 保留原生 warning 并失败，产品不提供自动信任或一键删除旧 key；
- Unsupported syntax 会得到明确提示，同时不阻止有效外部编辑或底层
  OpenSSH 行为；
- `Include` directive 得到原样保留，但 Roaminal 不读取 included file，且只
  存在于 included file 的 Host 不会出现在 connection manager；
- Wildcard/negated/multi-alias `Host` 和 `Match` block 不进入结构化模型；具体
  单 alias Host 只展示自身显式字段，未设置字段不显示推导出的 effective value；
- 只有 allowlisted Ed25519/RSA key filename 会被管理；展示安全元数据前必须
  验证 key content；
- Key generation 不覆盖文件，也不引入 password manager；
- Key generation 从结构化 UI 收集非敏感参数后，在独立 local connection 中
  运行原生交互式 `ssh-keygen`；passphrase 不进入结构化表单、API、日志或
  持久化状态；
- 外部 config 修改不能被静默丢失；
- 可以从同一连接定义启动多个 runtime instance；
- 产品明确区分 profile duplication、repeated launch、transport reuse 和不受
  支持的 arbitrary nested-session cloning；
- 用户可以从明确选中的 live remote connection 打开复用同一已认证 OpenSSH
  transport 的独立 connection instance，且不重新认证；
- 首个交互式 connection 作为该 transport 的 OpenSSH master；control socket
  不持久化，复用不允许静默 fallback，最后一个 channel 关闭后 master 被终止；
- Connection copy 和 authenticated transport reuse 均不依赖 external
  `ssh-agent`；官方部署不注入或管理 agent socket；
- 容器部署支持可写持久卷、整目录只读 Secret 和 config/key 单文件挂载，同时
  保留现有 non-root/read-only 安全姿态；只读来源可以连接但不会被 Roaminal
  修改或复制；
- Kubernetes Secret `..data` 符号链接只能在固定 `.ssh` 边界内受约束读取，
  所有写入均禁止跟随符号链接；config/key/known-hosts capability 必须如实展示；
- Legacy session API、配置、浏览器 state 和 runtime persistence 不提供兼容；
  开发部署必须在切换到新模型前清理未发布的 legacy state。
- Service/runtime 重启后，原 live local 和 remote instance 只作为只读
  `interrupted` history；任何 relaunch/reconnect 都创建全新 instance 和 ID；
- Live remote instance 的来源 Host 被编辑或删除时继续运行并显示 source
  drift 状态；旧 transport 进入 draining，新连接使用当前 definition 创建全新
  transport，删除后同名重建也不会重新绑定旧 transport；

## 17. 设计问题与决策记录

### 17.1 已确认决策

1. SSH runtime 已确定为系统 OpenSSH。OpenSSH 持有实际 transport、配置解析、
   认证、host verification 和远程 PTY；Roaminal 只负责编排 OpenSSH 主命令
   与辅助命令，并管理本地 PTY、process 和 terminal lifecycle。Go/Node SSH
   library 不得用于建立实际连接。首版不自动启用 multiplexing。
2. 首版结构化 SSH config editor 支持 `Host`、`HostName`、`User`、`Port`、
   `IdentityFile`、`IdentitiesOnly`、显式 `StrictHostKeyChecking no`、显式
   `UserKnownHostsFile /dev/null` 和 `ServerAliveInterval`。两个 host
   verification override 不是默认值，只允许按 Host 显式启用，并必须警告和
   持续标识中间人攻击风险。
3. 首版完全忽略 `Include` 内容。Roaminal 只原样保留主 config 中的
   `Include` directive 并显示 unsupported syntax 提示，不读取、解析、监控
   或编辑 included file，也不展示只存在于 included file 的 Host。系统
   OpenSSH 在实际启动主 config Host 时仍自行执行完整 Include 语义。
4. Web UI 现在和未来都不提供 SSH config raw editor 或 raw viewer，HTTP API
   也不返回完整 raw config。高级用户通过 local connection 中的终端工具或
   外部工具直接编辑 `$HOME/.ssh/config`，随后由 connection manager 重读。
5. Wildcard/negated/multi-alias `Host` 和 `Match` block 完全排除在结构化 UI
   之外。具体单 alias Host 仍可管理，但只显示其 block 内显式存在的字段；
   未显式设置的字段显示为“未设置”。UI 不计算或展示 effective config，也不
   使用 `ssh -G` 补全字段。高级 block 原样保留并提示，实际连接仍由 OpenSSH
   按完整配置执行。
6. Key generation 使用原生交互式 `ssh-keygen`。结构化 UI 只收集算法、
   filename、comment 和 RSA key size 等非敏感参数，随后创建独立 local
   connection 并把 `ssh-keygen` 接入 PTY。Passphrase 完全由原生命令提示，
   Roaminal 只瞬时透传 terminal input，不解析、保存、记录或回放。
7. Roaminal 不支持 external `ssh-agent` 集成，因为 connection definition
   duplication、repeated launch 和 OpenSSH transport reuse 都不依赖它。
   官方部署不挂载或注入 agent socket，产品不探测 identity 或管理 agent
   lifecycle。高级用户在自定义部署中自行启用的 OpenSSH agent 行为不属于
   产品支持范围。
8. 首版必须支持 authenticated SSH transport reuse。该能力只能由用户从
   明确选中的 live remote connection 显式发起；新 channel 是独立 connection
   instance，并且不重新认证。Roaminal 不自动匹配不同 connection definition，
   也不尝试复用手动嵌套 SSH process 或跨 runtime 恢复 control endpoint。
9. 复用采用“首个交互式 connection 同时作为 OpenSSH master”的模型。每个
   transport 使用 runtime 随机 ID 和临时 `ControlPath`；`ControlPersist` 只
   用于保持其他 live channel。最后一个 channel 关闭后终止 master，配置修改
   后 transport 进入 draining。复用失败必须硬失败，不能静默建立新 transport。
10. Host-key verification 完全保留 OpenSSH 原生交互。Unknown key 在首个
    master connection 的 PTY 中确认；changed key 保留完整 warning 并以失败
    结束。Roaminal 不提供结构化 trust dialog、`ssh-keyscan` preflight、自动
    接受、一键删除或基于 terminal 文本的精确错误分类。
11. 当前系统没有用户，不提供任何 session-to-connection compatibility。
    Canonical contract 明确区分 `connection-definition` 和
    `connection-instance`，旧 REST、WebSocket、JSON、配置、localStorage 和
    persistence 名称直接删除。新 runtime contract 仍作为首个正式
    `roaminal.v1`；auth session 和真实 terminal-engine 概念保持原名。
12. Service/runtime 重启后，原来仍 live 的 local 和 remote connection
    instance 均变为只读 `interrupted` history，不自动启动 shell 或 SSH。
    Relaunch/reconnect 始终创建新 instance 和 ID；只有同一 backend runtime
    内的浏览器或 WebSocket 重新 attach 才属于 continuation。
13. `.ssh` 支持 runtime UID 可写持久卷、整目录只读 Secret，以及直接挂载到
    `config` 或 allowlisted key 路径的只读文件。读取可以为 Kubernetes Secret
    `..data` 布局受约束地跟随符号链接，但解析结果必须留在固定 `.ssh` 根目录
    内且最终是普通文件；任何写入都不得跟随符号链接。Root-owned、可读且权限
    安全的 config/key 可以作为只读输入，Roaminal 不自动 `chmod`、`chown` 或
    复制。UI 必须按实际能力启用 config 修改、key generation 和 host-trust
    persistence；整个 `.ssh` 只读仍可连接，但不能修改配置、生成密钥或持久化
    `known_hosts`。
14. 内置 Local definition 是唯一、不可变且没有持久化用户字段的系统
    launcher。它固定启动交互式 `/bin/bash`，不提供 shell、command、environment、
    title 或 terminal size 配置。用户只能在启动时临时提供一个可访问的绝对
    `cwd`；未提供时使用 `ROAMINAL_CWD`（默认 `/workspace`），且不自动继承最近
    connection 的目录。Title 在实例创建后单独重命名；`ssh-keygen` 等专用
    local instance 不会成为通用 local template。
15. Live remote instance 的来源 Host 被编辑或删除时，现有 channel 继续运行，
    分别标记为 `source changed` 或 `source deleted`；关联 transport 立即进入
    draining，不再接受 reuse。编辑后再次连接同一 alias 必须使用当前 config
    创建新 transport 并重新认证；删除后 reconnect、duplicate-source 和 reuse
    不可用。重新创建同名 alias 也不会重新绑定旧 instance 或 transport。UI 和
    外部文件更新遵循相同规则，Roaminal 只保留来源 alias 和 runtime-only revision
    比较标识，不保存 Host block 或 effective config 副本。

### 17.2 未决问题

当前需求阶段的问题已经全部形成明确决策；进入设计阶段前没有剩余的产品决策
阻塞项。
