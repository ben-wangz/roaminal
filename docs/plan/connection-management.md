# Roaminal Connection 管理实施设计

> 策略更新：connection 退出后不再保留可挂载的 history。系统会先将最新
> metadata 和 terminal snapshot 复制到 audit 目录，再删除活动 instance 目录。
> audit 副本目前仅作为审计材料保留，不提供 UI 或查询 API；前端自动选择下一个
> live connection，没有可选项时返回 connection manager。

状态：设计已确认，已于 2026-08-08 授权实施。执行 agent 按本文阶段连续实施，除本文定义
的停止条件外不得等待人工确认。

## 1. 文档目标

本文把已确认的 Connection 管理需求转换为可直接实施的：

- 模块边界和依赖方向；
- HTTP、WebSocket 和 worker 契约；
- SSH config、SSH key、connection instance 与 authenticated transport 的数据模型；
- 文件系统安全、并发写入、来源漂移和进程回收机制；
- Connection manager 与 workspace 的完整交互；
- 容器、Kubernetes、测试、迁移和版本发布方案；
- 原子提交顺序、质量门槛、停止条件和 Definition of Done。

需求基线只有 [requirements.md](./requirements.md)。两份文档冲突时，需求文档优先；
执行 agent 不得通过实现细节扩张产品范围。

## 2. 设计原则

1. `$HOME/.ssh/config`、受支持名称的 key 文件和 OpenSSH 本身分别是配置、密钥与
   SSH 语义的事实源。Roaminal 不建立镜像数据库。
2. Connection definition 是启动来源，connection instance 是一次可观测的运行记录；
   两者在模型、API、UI 和持久化中始终分离。
3. 后端只编排系统 `ssh`、`ssh-keygen` 与现有 PTY/worker，不实现 SSH 协议，不解析
   private key 内容，也不保存 password、passphrase 或认证输入。
4. 结构化编辑必须是 lossless patch：只改用户明确操作的受支持字段，未知语法原样保留。
5. 文件“可读”不等于“可写”。能力由真实挂载、ownership、mode、symlink 和父目录条件
   推导，UI 只展示后端判定的能力与阻塞原因。
6. transport reuse 必须显式、同 runtime、来源一致且失败关闭；任何复用失败都不能
   退化为新的独立 SSH 网络连接。
7. 不提供旧 session 契约、旧持久化数据或旧前端 localStorage 的兼容层。发现旧数据时
   明确失败，不静默忽略。
8. 每个阶段保持可构建、可测试、可回滚；测试与对应能力同提交落地，不把风险集中到结尾。

### 2.1 已确认的设计补充决策

以下需求阶段未规定的产品策略已经用户明确接受，实施时不得重新留作待定项：

1. main config 任意内容变化都会使既有 transport context 失效并进入 draining；若变化仅位于
   其他具体 Host block，现有 channel 不中断，目标实例也不误标为 source changed。
2. Close 终止 live process/channel 并保留只读 history；Delete 才永久删除 metadata 和
   terminal snapshot。
3. 失败或中断的 key generation 不自动删除可能含有 key material 的 staging directory，
   只报告位置并由用户通过 local connection 处理。
4. Kubernetes 默认部署提供 1 Gi writable `roaminal-ssh` PVC，使 SSH config、key generation
   和 default known_hosts persistence 开箱可用；只读 Secret/direct-file 仍是受支持替代方案。
5. 本 feature 完成后使用 ForgeKit 把产品版本提升为 `0.2.0`。

本文给出的 RSA 3072-bit 默认值、文件大小限制和 1 秒 config polling interval 等工程默认值
也按设计执行；只有真实测试证明其不可行时，才按停止条件处理。

## 3. 目标架构

### 3.1 运行边界

```text
Browser
  | HTTP / WebSocket
  v
backend/internal/server
  |-- connection.Manager --------> terminal PTY/shadow ----> terminal-worker
  |       |-- local /bin/bash
  |       `-- system ssh + ControlMaster registry
  |-- sshconfig.Repository ------> secure SSH filesystem
  `-- sshkey.Repository ---------> secure SSH filesystem + system ssh-keygen

Persistent facts
  |-- $HOME/.ssh/config           OpenSSH configuration source
  |-- $HOME/.ssh/<allowed keys>   key source
  `-- $ROAMINAL_STATE_DIR/
        `-- connection-instances/ runtime metadata and terminal snapshots only

Ephemeral facts
  `-- /tmp/rm-<runtime>/          ControlMaster sockets and runtime locks
```

浏览器永远不直接连接 SSH server。`terminal-worker` 仍是低层 terminal stream engine；
它不知道 connection definition、SSH config 或认证复用策略。

### 3.2 后端包边界

实施后采用以下职责划分，名称可在不改变边界的前提下做很小的 Go 风格调整：

```text
backend/internal/connection/
  manager.go          instance CRUD、启动、关闭、重启后状态恢复
  model.go            definition/instance/lifecycle/source-state 模型
  local.go            固定 Bash launcher
  ssh.go              OpenSSH process 构造和生命周期
  transport.go        ControlMaster registry、reuse reservation、draining
  sourcewatch.go      source/context revision 检测和 drift 事件

backend/internal/sshfs/
  root.go             固定 SSH 根目录、安全读、能力探测
  write.go            dirfd、O_NOFOLLOW、atomic replace、fsync
  policy.go           ownership/mode/symlink/readonly 判定

backend/internal/sshconfig/
  syntax.go           lossless concrete syntax tree
  parse.go            结构化字段识别和 warning
  repository.go       读取、ETag、CRUD、copy
  edit.go             byte-span patch 和原子写
  revision.go         runtime-only source/context revisions

backend/internal/sshkey/
  inventory.go        allowlist 探测和 fingerprint
  public.go           显式 public-key 读取
  generation.go       交互式 ssh-keygen task 和无覆盖 promotion

backend/internal/terminal/
  pty.go              PTY/process group
  shadow.go           scrollback 和只读历史
  protocol.go         terminal-worker 协议与 marker

backend/internal/persistence/
  connection.go       connection instance format v1
  snapshot.go         terminal snapshot
  legacy.go           仅检测并拒绝旧 sessions，不做解码迁移
```

依赖方向固定为：

```text
server -> connection -> terminal -> worker client
server -> sshconfig -> sshfs
server -> sshkey    -> sshfs
connection -> sshconfig, sshkey, persistence
```

`sshconfig` 和 `sshkey` 不依赖 `connection`；`terminal` 不依赖任何 SSH 领域包；
`persistence` 只保存传入模型，不参与 OpenSSH 或来源判定。

### 3.3 前端模块边界

前端保留单页应用，不引入 URL router。顶层使用内部 view 状态避免静态服务器的路径回退
问题：

```text
frontend/src/features/connections/
  api.ts, types.ts, hooks.ts
  ConnectionManager.tsx
  ConnectionList.tsx
  ConnectionEditor.tsx
  ConnectionActions.tsx
  KeyInventory.tsx
  KeyGenerationDialog.tsx

frontend/src/features/workspace/
  Workspace.tsx
  ConnectionSidebar.tsx
  InstanceActions.tsx

frontend/src/features/terminal/
  TerminalView.tsx
  runtime.ts
```

`AppShell` 只负责认证后在 `connections` 与 `workspace` 视图之间切换、保存当前实例 ID，
不直接创建实例。组件应继续遵守仓库的文件体积约束，并复用现有图标、样式和测试模式。

## 4. 领域模型

### 4.1 ConnectionDefinition

Definition 是每次启动时读取事实源得到的投影，不单独持久化。

```go
type ConnectionDefinition struct {
    ConnectionDefinitionID   string
    Type                     ConnectionType // local | ssh
    HostAlias                string         // ssh only
    HostName                 *string
    User                     *string
    Port                     *uint16
    IdentityFileNames        []string
    IdentitiesOnly           *YesNo
    StrictHostKeyChecking    *NoOnly
    UserKnownHostsFile       *DevNullOnly
    ServerAliveInterval      *uint32
    AdvancedDirectiveCount   int
    UnmanagedIdentityCount   int
    Warnings                 []DefinitionWarning
    Capabilities             DefinitionCapabilities
    HostVerificationAssessment HostVerificationAssessment // default | weakened | unknown
    LiveInstanceCount        int
}
```

- 内置 local definition ID 固定为 `local`，唯一、不可编辑、不可复制、不可删除。
- SSH definition ID 为 `ssh.` 加 Host alias 的 base64url（无 padding）编码。ID 可逆仅用于
  路径定位，不是秘密，也不能代替 request body 中的 alias 校验。
- SSH 字段只表示该受管理 Host block 中明确出现且能无歧义读取的值，不声称是 OpenSSH
  effective config。
- `Warnings` 只返回 directive 名称、行号、分类和安全说明，不返回未知 directive 的原始
  argument，也不提供 config raw view。
- `HostVerificationAssessment` 在具体 block 明确出现 `StrictHostKeyChecking no` 或
  `UserKnownHostsFile /dev/null` 时为 weakened；存在可能影响 trust 的高级/global/wildcard/
  Match/Include 内容时为 unknown；只有没有相关证据时才为 default。它不是 effective config。

### 4.2 ConnectionInstance

```go
type ConnectionInstanceMeta struct {
    FormatVersion              int // always 1
    ConnectionInstanceID       string
    BackendRuntimeID           string
    ConnectionDefinitionID     string
    Type                       ConnectionType
    Purpose                    InstancePurpose // interactive | ssh_key_generation
    SourceHostAlias            *string
    Lifecycle                  Lifecycle // live | exited | interrupted
    SourceState                SourceState // current | changed | deleted
    Title                      string
    TitleMode                  TitleMode // automatic | custom
    InitialCwd                 *string
    Cols                       uint16
    Rows                       uint16
    CreatedAt                  time.Time
    UpdatedAt                  time.Time
    ExitedAt                   *time.Time
    ExitCode                   *int
    ExitSignal                 *string
    Attention                  bool
    HostVerificationAssessment HostVerificationAssessment
    ReuseFromConnectionInstanceID     *string
    ReconnectFromConnectionInstanceID *string
    RelaunchFromConnectionInstanceID  *string
}
```

对外 summary 在上述安全字段之外增加运行时派生数据：当前 PWD、transport 状态、当前
channel 数、连接时长和可用操作。不得持久化：Host block、effective config、private/public
key 内容、password、passphrase、terminal 输入、ControlPath、transport ID 或跨 runtime
可复用的 revision。

实例 ID 使用不可预测的固定长度随机标识。每次启动都生成新 ID；duplicate definition、
reconnect、relaunch 和 transport reuse 也不复用旧 ID。

### 4.3 状态机

Instance lifecycle：

```text
create -> live -> exited
              `-> interrupted   (backend shutdown/restart/crash recovery)
```

- 正常 shell/SSH/ssh-keygen 退出都进入 `exited`，并保存 exit code/signal。
- 应用执行受控关闭时，先持久化 `interrupted`，再回收 process group 和 transport。
- 启动时发现上一 `BackendRuntimeID` 的 `live` 记录，直接改为 `interrupted`；不得重启进程。
- `exited` 和 `interrupted` 的 snapshot 可只读 attach，输入、resize ownership 和复用均拒绝。

SSH source state 对每个实例是单调状态：

```text
current -> changed -> deleted
    `----------------> deleted
```

来源恢复、回改或重新创建同名 alias 都不能把旧实例恢复为 `current`。来源变为 changed 或
deleted 时，关联 transport 立即 draining；已有 channel 不受影响。

Transport state：

```text
starting -> ready -> draining -> closed
     |        |          |
     `--------+----------+-> failed
```

- `starting` 实例的 PTY 立即对前端可见，以承载 host-key、password 和 passphrase prompt。
- 只有 `ready` transport 接受 reuse reservation。
- `draining` 拒绝新 reservation；现有 channel 退出后关闭。
- transport owner 即第一次独立连接实例；owner shell 退出但仍有 reuse channel 时，
  ControlPersist master 可以继续存在，直至最后 channel 退出。

### 4.4 本地启动规则

普通 local instance 只能用固定 argv 启动受支持的交互式 Bash，例如
`/bin/bash --rcfile <application-shell-rc> -i`，并沿用现有安全 shell integration/marker。
backend 在 spawn 前验证 `initialCwd` 是当前 runtime 可进入的绝对目录；缺省严格使用
`ROAMINAL_CWD`，未配置时为 `/workspace`。请求不能携带 shell、command、environment、title
或固定 terminal size，cols/rows 只来自实际 viewport/resize protocol。

`ssh-keygen` 是唯一由本 feature 增加的受约束专用 local process；它走显式 purpose 和固定
argv builder，不能把 local launcher 扩展成任意 command template。任何 local/remote process
退出都会触发 manager lifecycle 更新，不能让 terminal view 留在“live 但不可操作”状态。

## 5. SSH 文件系统边界

### 5.1 根目录固定与安全读取

启动时根据运行用户 HOME 计算 `$HOME/.ssh`，并建立专用 `sshfs.Root`：

1. 读取使用 Go `os.OpenRoot` 固定已解析根目录；所有调用只接受程序生成的相对文件名，
   禁止空值、绝对路径、`..` 和尾随 `/`。
2. 允许 Kubernetes projected volume 的目录内 symlink 链，但每一步必须保持在已固定根内，
   最终对象必须是普通文件；逃逸 symlink、magic link、目录、device 和 socket 全部拒绝。
3. symlink 深度沿用 `os.Root` 的有限上限。能力响应将超限、逃逸和非普通文件区分为稳定
   error code，但不泄露宿主路径。
4. `.ssh` 本身可通过部署挂载被解析后固定供读取；写入能力要求它在打开时是直接目录，
   不能是应用可替换的 symlink。
5. 所有固定文件都有大小上限：config 1 MiB、public key 64 KiB、private key 验证输入
   1 MiB。超限文件不可由产品解析或编辑，但 local connection 仍可用。

### 5.2 安全写入

写路径不复用“跟随 symlink 的安全读取”语义：

- 使用固定 `.ssh` directory fd 和 `fstatat(..., AT_SYMLINK_NOFOLLOW)` 检查目标。
- 任何写能力都要求固定 `.ssh` directory 非 symlink、runtime UID 实际可创建文件且不可被
  other 写。允许 runtime UID owner 的私有目录，也允许 Kubernetes `fsGroup` 形成的
  root:runtime-group、group-writable 目录；后者只授权目录内创建，最终 config/key 仍必须由
  runtime UID 以安全 mode 创建。Root-owned readonly Secret mount 只作为读取来源。
- 目标存在时必须是 runtime UID 拥有的普通、非 symlink 文件；root-owned、其他 UID、
  projected symlink 和只读 mount 只读可用。
- 临时文件在同一 directory fd 下以不可预测名称、`O_CREAT|O_EXCL|O_NOFOLLOW`、`0600`
  创建。写完依次 file `fsync`、close、二次版本校验、`renameat`、directory `fsync`。
- 不调用 `chmod`、`chown`，不复制挂载内容到私有目录，也不把 symlink 替换成普通文件。
- missing `.ssh` 只有在 HOME 直接父目录可安全写时才以 `0700` 创建；readonly root filesystem
  中会得到明确的只读能力，不能影响 local launcher。

Linux 实现允许引入 `golang.org/x/sys/unix` 以使用 dirfd、`renameat2` 和 no-follow flag；
依赖版本通过 Go module 正常管理。

### 5.3 Ownership 和 mode 策略

安全与能力判断如下：

| 对象 | 可读条件 | 可写条件 |
| --- | --- | --- |
| `config` | runtime 可读的普通安全文件，owner 为 runtime UID 或 root，且无 group/other write | runtime UID owner、无 group/other write、目标非 symlink、父目录可写 |
| private key | 文件名允许、runtime 可读、owner 为 runtime UID 或 root、无 group/other write | Roaminal 不编辑；仅生成新文件 |
| public key | 同名 `.pub` 普通安全文件且 runtime 可读 | Roaminal 不编辑；仅随新 key 生成 |
| `known_hosts` | 是否存在不阻止 SSH；由 OpenSSH 处理 | 仅用于 capability 提示，不由 Roaminal 编辑 |

runtime-owned private key 若有任何 group/other permission 必须标记 invalid；root-owned Secret
key 可为 `0444`/`0644`，只要 runtime 可读且不存在 group/other write。最终是否被 OpenSSH
接受仍由 OpenSSH 决定，Roaminal 不修改 mode。

`known_hosts` 能力为 `available`、`unavailable`、`unknown` 或
`disabled_by_definition`。判定顺序固定如下：

1. 具体 Host 显式 `UserKnownHostsFile /dev/null` 时为 disabled；
2. main config 中存在可能影响该 alias 的 Include、global/wildcard/Match trust directive 或
   其他高级 trust path 时为 unknown，且同时报告默认 `.ssh` trust store 的独立写入能力；
3. 否则按默认 `$HOME/.ssh/known_hosts` 目标和父目录的真实能力判定 available/unavailable。

这个状态只是产品能安全判断的持久化能力，不冒充 OpenSSH effective config。整体可写 PVC
通常为 available；无相关高级配置的 Secret/readonly `.ssh` 为 unavailable；高级自定义
path 为 unknown。OpenSSH 仍是实际写入是否成功的最终裁决者。

## 6. SSH Config 设计

### 6.1 Lossless parser

不能使用会规范化或重排文件的通用 SSH config writer。实现一个受限 concrete syntax tree：

```go
type Document struct {
    Bytes      []byte
    Newline    string
    Lines      []PhysicalLine
    Blocks     []Block
}

type Block struct {
    Kind       BlockKind // global | host | match
    Start, End int       // byte offsets
    Header     Directive
    Directives []Directive
}
```

scanner 必须保存每个 physical line 的原始 bytes、缩进、keyword、`=` 形式、argument span、
尾部注释和换行符。tokenizer 支持 OpenSSH config 的空白、引号、转义与单个可选 `=`；
keyword 大小写不敏感。编辑器不重新序列化整个文档。

产品 parser 看到 `Include` 时只把这一行当未知高级 directive：不展开、不 stat、不读取、
不 watch 其参数，也不向 API 返回参数。实际 `ssh <alias>` 仍由系统 OpenSSH 正常处理
Include 和系统配置。

### 6.2 可管理 block 判定

一个 block 只要 header 是唯一的单一、非空、无 wildcard、无 `!` 否定的具体 `Host`
alias，且不处于 `Match` block，就映射为可见、可连接的 SSH definition。unknown directive
和高级继承不能隐藏它。

在此基础上，block boundary 和 header argument span 可被无歧义定位时，definition 获得基础
edit/delete capability。每个受支持字段再独立获得 field capability：

- scalar directive 在 block 内至多出现一次且值能安全解析时，该字段可编辑；
- 重复、非法 quoting 或受支持 keyword 的高级 value 只让该字段只读，其他安全字段仍可编辑；
- 整个 block 的边界或 tokenization 无法证明时，所有 destructive structured mutation 禁用。

wildcard、否定、多 pattern、重复具体 alias 和 `Match` 不在连接列表中；可见具体 block
中的 field 歧义不隐藏 definition。duplicate 只复制能安全读取的 managed fields，delete 只在
block boundary 安全时可用。config 状态区域报告分类、directive/行号和数量，不展示 raw
内容。未知 directive 不阻止一个结构清晰的具体 block 被连接或编辑；编辑只触及受支持字段。

受支持字段严格为需求已确认的九项。特别规则：

- `IdentityFile` 可重复；只有值精确映射为 `~/.ssh/<allowlisted-name>` 且文件被 key inventory
  识别时才进入 `IdentityFileNames`。其他值原样保留并计入 unmanaged identity warning。
- `StrictHostKeyChecking` 只接受未设置或 `no`；其他已有值保留并把该字段标记只读歧义。
- `UserKnownHostsFile` 只接受未设置或 `/dev/null`；其他已有值保留并标记高级值。
- `IdentitiesOnly` 接受未设置、`yes`、`no`。
- `ServerAliveInterval` 接受非负十进制整数。

### 6.3 输入约束

新建和 UI 修改采用保守、确定的输入集合：

- alias：ASCII `[A-Za-z0-9][A-Za-z0-9._-]{0,254}`，拒绝 wildcard 和 `!`；
- HostName：单个 hostname/IP token，拒绝 whitespace、comment marker 和 `%`/`$` expansion；
- User：单个无 whitespace/control 的用户名 token；
- Port：`1..65535`；
- identity：只能从 key inventory 返回的 managed filename 多选；key basename 不含路径分隔符、
  最长 255 bytes，并且精确为 `id_ed25519`、`id_rsa` 或安全 basename 加 `_ed25519`/`_rsa`；
- ServerAliveInterval：`0..2147483647`；
- 枚举只接受 API schema 明确列出的值。

既有 config 中超出“可新写”正则但仍是安全单一 alias 的 definition 可以显示和连接；
UI 不允许把非法值写回。所有 `ssh`/`ssh-keygen` argv 都直接传给 `execve`，不用 shell，
且 alias 前使用 `--`。

### 6.4 Patch 行为

更新现有 block 时生成 byte-span patch，按 offset 倒序应用：

- 唯一受支持行仅替换 argument span，保留 indentation、keyword casing、`=` 风格和尾注释。
- 修改 Host alias 只替换 header argument，并要求新 alias 唯一；对 live instance 而言这是
  旧 alias 删除和新 definition 创建，旧实例永久变为 source deleted，transport drain。
- 删除字段时删除该 physical line；若无法证明整行只含该 directive，则拒绝保存。
- 新字段在 block 结束前插入，沿用该 block 最常见缩进和文档换行符。
- `IdentityFile` 只删除/替换产品已识别的 managed 行；unmanaged 行永远保留。
- 写回多个 managed `IdentityFile` 时严格采用请求数组顺序，不按 filename 排序。
- 新 block 追加到文件末尾，确保前置换行，固定顺序为 Host、HostName、User、Port、
  IdentityFile、IdentitiesOnly、StrictHostKeyChecking、UserKnownHostsFile、
  ServerAliveInterval。
- duplicate 只复制上述受支持字段到新 alias，不复制未知 directive、注释或 unmanaged
  IdentityFile；UI 必须在确认前明确提示该边界。
- delete 仅删除被精确识别的具体 Host block byte range，不触及相邻高级 block。

### 6.5 ETag 与并发写

每次读取 config 都计算 content hash 和安全 source fingerprint（存在状态、最终 file identity、
owner/mode、symlink/read/write capability）；HTTP `ETag` 使用每 runtime 随机 key 对二者做
HMAC-SHA256，生成 opaque strong ETag。这样即使内容相同，普通文件被换成 readonly Secret
或 symlink target 轮换也会更新 version/capability，同时不会把 config 内容 hash 暴露为长期
指纹。Runtime revisions 仍只使用第 6.6 节定义的内容 bytes，纯挂载形态变化不会无故中断
连接。

- `GET /api/connection-definitions` 返回当前 ETag。
- 所有 config mutation 必须带 `If-Match`；缺失返回 `428`，不匹配返回 `412`。
- mutation 在进程内 config mutex 下重新安全打开文件并校验 ETag；写临时文件后、rename
  前再次打开目标校验，降低外部 editor 与应用并发覆盖风险。
- rename 后重新读取和解析；若发生不可恢复错误，返回稳定错误并保留可诊断日志，不把
  目标文件替换成空/部分内容。
- POSIX 无法对不协作的外部 writer 提供严格 compare-and-swap；上述“双校验 + 同目录原子
  rename”是支持范围。测试必须有 hook 精确制造两个校验窗口的冲突。

### 6.6 来源 revision 与外部变更

后端每秒安全重读 main config；显式 UI refresh 立即触发一次读取。无需 fsnotify，因定时
读取同时覆盖普通文件替换和 Kubernetes `..data` symlink 轮换；使用 `subPath` 挂载时
Kubernetes 本身不会传播更新，文档必须说明。

每个 live SSH transport 在内存中绑定两个不可逆、runtime-only token：

```text
sourceRevision = HMAC(runtime-key,
  exact bytes of this Host block ||
  exact main-file bytes outside all other concrete single-alias Host blocks)

transportContextRevision = HMAC(runtime-key, exact bytes of the entire main config)
```

`sourceRevision` 决定实例的 source state，能捕获目标 block 和
global/wildcard/Match/Include directive 文本变化，又不会把另一个普通具体 alias 的编辑误报
为本实例来源变化。`transportContextRevision` 决定旧 control socket 是否还能接收新 channel；
它保守覆盖整个 main config，因为其他具体 Host 仍可能通过 ProxyJump、canonicalization 或
高级命令参与实际 route/security context。修改无关具体 block 时，既有 channel 继续且来源仍
为 current，但旧 transport 会 drain；UI 显示 transport 因 SSH config 变化而不再可复用。

Included file 内容永远不读取、watch 或计算，OpenSSH 在新连接时自行看到其当前值。两个 token
都不持久化、不给 API，也不能在 backend restart 后恢复 transport。

后续规则：

- sourceRevision 不同：实例 source state 单调变为 changed；
- transportContextRevision 不同：transport 进入 draining；
- alias 消失：变为 deleted；
- config unreadable 或主文件整体 parse 失败：无法证明来源相同，现有实例变为 changed 并
  drain，新 SSH 创建暂时不可用，本地连接不受影响；
- key/known_hosts 文件变化不改变上述 revisions，不中断既有已认证 transport；
- 产品 mutation 成功后同步广播变更事件，不等待下一次 poll；外部 poll 检测后同样原子更新
  metadata，并通过对应 instance WebSocket metadata event 和 heartbeat summary 通知前端。

## 7. SSH Key 设计

### 7.1 Inventory

只非递归探测：`id_ed25519`、`*_ed25519`、`id_rsa`、`*_rsa`。排除 `.pub`、临时目录、
非法 mode/owner、逃逸 symlink 和非普通文件。文件名是 key ID 的 base64url 编码来源。

为避免把 private key bytes 载入 Go heap，fingerprint 验证通过安全打开后的 file descriptor
作为 stdin 调用：

```text
ssh-keygen -lf /dev/stdin -E sha256
```

设置 `LC_ALL=C`，只解析 bits、SHA256 fingerprint 和 algorithm；不把 command output 的
comment 作为产品数据。解析出的 algorithm 必须与 filename suffix 匹配；`.pub` 只有在自身
格式有效且 fingerprint 与 private key 一致时才标记 available。该调用的可行性必须由真实
`ssh-keygen` integration test 锁定。
工具失败只把单个 key 标记 invalid，不影响其他 key 或 local connection。

Inventory 仅返回 filename、algorithm、bits、fingerprint、publicKeyAvailable、readOnly、
status，以及引用它的 definition ID/Host alias。API、日志和 state 不返回 private key bytes。

### 7.2 Public key 显式读取

只有用户点击 copy public key 时才调用独立 endpoint。后端安全打开 allowlisted `.pub`，
检查普通文件、大小和 OpenSSH public-key 格式，并只返回该 public key 文本。没有 `.pub`
时不从 private key 自动导出；UI 提示用户通过 local connection 自行处理。endpoint 必须从
当前 inventory key ID 解析固定 filename，不能把 path 当输入，并返回 `Cache-Control: no-store`。

### 7.3 交互式生成

POST generation 创建 `purpose=ssh_key_generation` 的专用 local connection instance，并
在 PTY 中直接执行 `/usr/bin/ssh-keygen`：

- type 为 `ed25519` 或 `rsa`；RSA bits 允许 2048/3072/4096，默认 3072；
- filename 必须匹配 allowlist 且目标 private/public 两个文件都不存在；
- comment 是最长 255 bytes、无 control/newline 的可选参数；
- 不传 `-N`，让 ssh-keygen 在 terminal 中原生询问 passphrase；
- 不启用 terminal input tracking，不持久化输入；snapshot 只包含程序输出。ssh-keygen
  自身关闭 echo 的 passphrase 输入不得出现在历史、HTTP、日志或事件中。

为保证无覆盖且兼容成对生成，先在 `.ssh/.roaminal-keygen-<instance-id>/` 的 `0700`
目录中生成。成功后校验 private/public pair，再用 Linux
`renameat2(RENAME_NOREPLACE)` 提升到最终名称并 fsync。先提升 public key，再以 private key
rename 作为 pair 的提交点；任一步骤都不得覆盖已有文件。第二个 rename 失败时对已提升的
public key 做最小回滚并产生显式诊断。崩溃最多留下不进入 private-key inventory 的孤立
`.pub` 或 staging，不能出现“已发现 private key 但 public 尚未提交”的成功假象。

生成失败或中断后不得自动删除可能包含用户 key material 的 staging 目录。UI 显示安全
warning 和目录名，引导用户从 local connection 检查或清理；Roaminal 不提供 private key
下载/查看接口。生成任务的实际 ssh-keygen exit 与 promotion 结果分别记录，避免 exit 0
但提交失败被误报为成功。

## 8. OpenSSH 进程与 Transport Reuse

### 8.1 工具发现

backend 启动时分别发现 `ssh` 和 `ssh-keygen` 绝对路径并记录 feature capability：

- 缺少 `ssh`：SSH connect/reuse unavailable，config/key inventory 和 local 仍可用；
- 缺少 `ssh-keygen`：fingerprint/generation unavailable，已有 SSH 仍可由 OpenSSH 使用；
- 不因 SSH 子系统异常阻止认证或 local connection manager 启动。

生产 image 必须安装 OpenSSH client。不得安装/启动 sshd 或 external ssh-agent sidecar。
官方 container/Kubernetes 配置也不得注入或管理 `SSH_AUTH_SOCK`；用户在自定义部署和高级
config 中自行启用 agent 是 OpenSSH 外部行为，产品不探测、不保证，也不改写 config 阻止。

### 8.2 独立远程连接

每次未选择 reuse 的 SSH 启动都创建独立 master transport：

```text
ssh
  -o ControlMaster=yes
  -o ControlPersist=yes
  -o ControlPath=/tmp/rm-<runtime>/t-<random>/ctl
  -- <host-alias>
```

命令通过 PTY/process group 启动，所选 alias 原样交给 OpenSSH，因此 user config、Include、
system config、ProxyJump、认证和 host trust 都由 OpenSSH 决定。ControlPath 目录使用短随机名、
mode `0700`，socket 和目录只在当前 runtime registry 中存在，不写入 state。

ControlPath 的固定父目录为短路径 `/tmp/roaminal-mux`，启动时以 no-follow 方式验证它由当前
UID 拥有且 mode `0700`。每个 backend runtime 使用新的短随机子目录；启动时安全清理同一
UID 的 stale runtime 子目录，从而覆盖 Kubernetes 同一 Pod 内 container restart 时 emptyDir
仍保留的情况。旧 socket 永不注册、检查或恢复为 authenticated transport。

terminal chrome 必须持续显示目标 instance title 和 Host alias，让 password/passphrase/host-key
prompt 明确归属当前连接。Roaminal 不调用 `ssh-keyscan`、不预抓取或自动接受 host key，也不
解析 locale-sensitive terminal 文本：changed host key 等失败保留完整 OpenSSH output 和真实
非零 exit，结构化 lifecycle 只报告通用 SSH connection failed。

本设计依赖 OpenSSH 的 ControlPersist detach 行为：认证和 forward confirmation 完成后，初始
进程把 master 放到后台，并在原前台/PTY 启动 mux client 承载第一个交互 session。因此首个
terminal 既承载真实认证 prompt，又能在 ready 后独立于后台 master 关闭。backend 把前台 mux
client process group 归属 connection instance，把 control socket 归属 transport；不通过解析
`-O check` 文案持久化 master PID。

创建后并行轮询 control socket：

```text
ssh -F none -S <control-path> -O check -- <host-alias>
```

`check` 成功才把 transport 从 starting 标记 ready；期间 PTY 已可交互。不能通过解析
terminal 文案判断登录、password 或 host verification。在 ready 发布前必须再次比较 source
和 transport-context revisions；若认证期间 config 已变，首个 channel 可以继续，但 transport
直接进入 draining，不得出现短暂可复用窗口。

### 8.3 显式复用与禁止 fallback

复用请求必须指定一个当前存活 remote source instance ID。后端在 registry lock 下验证：

1. source 属于当前 backend runtime，instance live，类型为 SSH；
2. transport 为 ready，未 draining，来源 state 为 current；
3. source alias 与目标 definition 一致，且当前 source/transport-context revisions 都相同；
4. ControlPath 是 registry 创建的路径，目录/socket ownership 和类型正确；
5. `ssh -F none -S ... -O check` 成功；
6. 全局 instance limit 和 transport reservation 可用。

选中的 registry entry 与 control socket 本身就是 reuse identity：它代表首个 OpenSSH master
实际采用的 destination、user、identity/agent、proxy/jump、host-key 和 forwarding 上下文。
Roaminal 不用部分结构化字段重新计算或猜测这份 identity；runtime-only source revision 只
负责阻止 config 变化后继续加 channel。

预检查失败返回 `409`，不创建实例。reservation 成功后用以下新 PTY 启动 channel：

```text
ssh
  -o ControlMaster=no
  -o ControlPersist=no
  -o ControlPath=<validated-control-path>
  -o CanonicalizeHostname=no
  -o ProxyCommand=/bin/false
  -- <host-alias>
```

OpenSSH 在 control socket 可用时先进入 mux client；若 socket 在检查后的竞态窗口失效，
命令行中优先取得的 `ProxyCommand=/bin/false` 使任何独立网络路径失败，从而满足“不能
fallback”；用户 config 中后出现的 ProxyCommand/ProxyJump 不能替换它。该 override 只用于
已选择 reuse 的新 channel，不改变首个独立连接。真实 fixture 还必须覆盖用户 config 自带
ProxyJump 和 ProxyCommand 的场景，验证目标 OpenSSH 版本的 first-obtained-value 行为。

真实 OpenSSH fixture 必须证明：正常 reuse 不增加 server transport；删除 socket、drain 或
关闭 master 后请求不会新建网络连接。若目标 OpenSSH 行为不能稳定证明此性质，属于停止
条件，不能用“多数时候成功”替代。

### 8.4 计数、drain 与回收

- channel count 包含已 reservation、starting 和 live 的 owner/channel；失败 spawn 必须释放。
- 不设置隐藏的单 transport channel 上限；只受全局 connection instance limit 和 server
  自身限制影响。
- transport ready 后关闭任一实例只终止其前台 mux-client process group。不得按 UID、祖先
  process tree 或原始 fork 关系误杀已 detach 的 master；真实 fixture 必须先关闭 owner 并证明
  reuse channel 仍可交互。
- owner terminal 退出但仍有 channel 时，`ControlPersist=yes` 保持 master；最后 channel
  退出后发送 `ssh -F none -S <path> -O exit -- <alias>` 并删除 runtime 目录。
- source drift 或用户开始关闭 transport 时发送 `-O stop`，拒绝新 mux 请求，现有 channel
  继续；全部结束后 `-O exit`。
- backend shutdown 先持久化 instance interrupted，transport 全部 stop/exit，再对残留
  process group TERM，超时后 KILL，最终清除当前 runtime 临时目录。
- 后台定期 `-O check`；失效时标记 transport failed，让受影响实例自然得到明确 exit/状态，
  不能冻结 manager。

不做 idle pool、跨 runtime 恢复、跨 definition 自动匹配、嵌套手动 SSH 识别或 external
ssh-agent 集成。

## 9. 持久化与启动迁移

### 9.1 新格式

新目录固定为：

```text
$ROAMINAL_STATE_DIR/connection-instances/<instance-id>/metadata.json
$ROAMINAL_STATE_DIR/connection-instances/<instance-id>/terminal.snapshot
```

`metadata.json` 只写第 4.2 节字段，`formatVersion=1`。terminal snapshot 可复用现有安全
shadow 编码和 magic，但由新目录、新 instance model 管理。auth session 存储保持原样。

backend 每次启动先生成 `BackendRuntimeID`，再加载 metadata。旧 runtime 的 `live` 全部原子改为
`interrupted`，只恢复只读 terminal history，不创建 Bash/SSH/transport。当前“重启后自动
恢复 Bash”行为必须删除。

### 9.2 不兼容数据

实施前识别现有 persistence root 解析方式，检查所有可能的旧 `sessions` 目录位置。任意旧
目录非空时，backend 启动必须失败并输出明确、可操作且不含 session 内容的错误：旧数据与
Connection schema 不兼容，需要操作者备份/清理后重启。

不得读取 v1/v2 session decoder、自动迁移、重命名目录、静默忽略或删除旧数据。旧 decoder、
compatibility endpoint 和测试在同一迁移提交中删除。

## 10. 外部契约

### 10.1 通用规则

- 现有登录、cookie、CSRF 和 origin 防护继续应用；本文不改变 auth 模型。
- Authentication login session 继续使用 `/api/auth/sessions`、`auth-sessions.json` 和 auth
  session 术语，不参与 connection 改名。
- JSON error 统一为 `{"error":"safe message","code":"machine_code","field":"optional"}`。
- 对资源 ID 使用 path escaping/解码后再做严格格式校验。
- 所有响应和日志都经过敏感信息审查；未知 config argument 和 terminal input 不进入 API。
- 容量配置精确改为 `maxConnectionInstances`、`maxClientsPerConnectionInstance`；CLI 使用
  `--max-connection-instances`、`--max-clients-per-connection-instance`，environment 使用
  `ROAMINAL_MAX_CONNECTION_INSTANCES`、`ROAMINAL_MAX_CLIENTS_PER_CONNECTION_INSTANCE`。
  旧名称直接删除，不接受 alias。

### 10.2 Connection definitions

```text
GET    /api/connection-definitions
POST   /api/connection-definitions
PUT    /api/connection-definitions/{connectionDefinitionId}
POST   /api/connection-definitions/{connectionDefinitionId}/duplicate
DELETE /api/connection-definitions/{connectionDefinitionId}
```

GET 返回：

```json
{
  "configSource": {
    "status": "available",
    "readable": true,
    "writable": true,
    "warnings": [],
    "blockers": []
  },
  "definitions": []
}
```

响应带 `ETag`。所有 mutation 要求 `If-Match`。POST/PUT body 使用完整受支持视图：

```json
{
  "type": "ssh",
  "hostAlias": "prod-api",
  "hostName": "10.0.0.12",
  "user": "deploy",
  "port": 22,
  "identityFileNames": ["id_ed25519"],
  "identitiesOnly": "yes",
  "strictHostKeyChecking": null,
  "userKnownHostsFile": null,
  "serverAliveInterval": 30
}
```

nullable 表示删除/未设置。duplicate body 只含新 alias。local 不接受 mutation。成功 mutation
返回更新后的完整 definition collection 和新 ETag，使 frontend 不在本地猜测 patch 结果。
每个 definition response 还返回 field-level capability；PUT 中对只读/advanced 字段提交不同于
当前结构化投影的值返回 `422 field_not_structurally_editable`，相同的 null/安全投影则保留原始
advanced bytes，不把它解释为删除。

即使 config 缺失、不可读或 parse 失败，GET 也返回 `200`、内置 local definition 和明确的
configSource 状态；只在安全可创建时开放 POST。frontend refresh 必须并发重取 definitions 与
key inventory，不能用缓存补齐失败的事实源。

### 10.3 Connection instances

```text
GET    /api/connection-instances
GET    /api/connection-instances/{connectionInstanceId}
POST   /api/connection-instances
PATCH  /api/connection-instances/{connectionInstanceId}/title
POST   /api/connection-instances/{connectionInstanceId}/close
DELETE /api/connection-instances/{connectionInstanceId}
```

创建 body：

```json
{
  "connectionDefinitionId": "ssh.cHJvZC1hcGk",
  "cols": 120,
  "rows": 36,
  "initialCwd": null,
  "reuseFromConnectionInstanceId": null,
  "reconnectFromConnectionInstanceId": null,
  "relaunchFromConnectionInstanceId": null
}
```

- `initialCwd` 只允许 local；必须是可访问绝对目录，缺省使用 `ROAMINAL_CWD`，默认
  `/workspace`。
- backend 必须从同一次当前 definition collection 中解析 `connectionDefinitionId`；remote ID
  解码得到 alias 也不能绕过 main config 可见性检查。只存在于 Include、advanced pattern、
  已删除或当前不可读来源中的 alias 一律不能启动新连接。
- 三个来源关系最多一个；只有 reuse 改变 transport 行为，reconnect/relaunch 仅记录安全
  关联用于 UI/审计，仍按当前 definition 创建全新 transport/process。
- `close` 终止 live 资源并保留只读历史；对历史实例幂等。
- `DELETE` 删除历史和 snapshot；若实例仍 live，先按 close 完整回收。不得删除 definition。
- title patch 适用于 live 和历史实例；空值恢复自动 title。

### 10.4 SSH keys

```text
GET  /api/ssh-keys
GET  /api/ssh-keys/{keyId}/public-key
POST /api/ssh-key-generations
```

generation body：

```json
{
  "algorithm": "ed25519",
  "rsaBits": null,
  "fileName": "id_ed25519",
  "comment": "roaminal"
}
```

成功返回新建的专用 connection instance summary；后续交互和普通 terminal 相同。只读挂载、
目标存在、工具缺失或能力不安全在 spawn 前明确拒绝。

### 10.5 WebSocket 与 heartbeat

```text
GET /ws/connection-instances/{connectionInstanceId}
Sec-WebSocket-Protocol: roaminal.v1
```

保留经过验证的 binary terminal frame 格式与 subprotocol；仅路径和领域事件改名。初次 attach
必须得到 snapshot、instance metadata 和 lifecycle。历史实例只允许读取，input、resize 和
write claim 返回协议错误并关闭对应写能力。

heartbeat GET 响应中的 `sessions` 完全替换为 `connectionInstances`；不要同时返回旧字段。
resize 以 WebSocket 当前 owner 协议为事实源，不再保留旧 session HTTP alias。

### 10.6 Worker 协议

内部协议 bump 为 `roaminal-terminal-worker/2`，所有 `sessionId` 改为 `terminalId`。worker 只
看到 terminal ID、PTY stream 和 marker，不看到 connection instance metadata。backend 与
worker 版本不一致必须启动失败，不能做 v1 fallback。

旧 `/api/sessions`、`/ws/{id}`、session JSON 字段、CLI/env/config 名称和 TypeScript session
types 同步删除。产品没有用户，不增加兼容 alias。

## 11. 前端体验

### 11.1 顶层导航

认证成功后：

- 没有 connection instance 时默认进入 Connection manager；
- 有历史或 live instance 时恢复 localStorage 中仍存在的 active instance，否则选择最近实例；
- 唯一 localStorage key 为 `roaminal_active_connection_instance_v1`，不读/迁移旧 key；
- `Ctrl+Shift+T`、workspace 空态、sidebar plus、exited overlay 的新建动作都只导航到 manager，
  绝不直接创建 local/remote instance。
- 任一 manager create/connect/generate 成功后立即进入 workspace 并选中新返回的
  `connectionInstanceId`，不依赖异步列表排序猜测 active instance。

view 使用 `connections | workspace` 内部状态。返回 workspace 的按钮在没有实例时 disabled。

### 11.2 Connection manager

首屏是全视口、安静且高信息密度的操作界面，不做 landing page，不把页面 section 包成装饰
card。顶部包含返回 workspace、Connections/Keys tabs、搜索、refresh icon 和认证菜单。

Connections tab：

- Local launcher 固定第一行，显示 Local、默认 cwd；主 play icon 立即使用默认 cwd 创建，
  菜单提供“Start in directory…”输入一次性绝对路径。
- SSH definition 使用紧凑 table/list，显示 alias、明确字段组成的 `user@host:port`、identity
  数量、weakened/unknown host verification assessment、warning、只读状态和 live count。
- 搜索只针对 Host alias 和可安全展示的明确 destination metadata，不读取 raw/advanced
  argument；config/key 全局 warning 固定在过滤结果之外，过滤后仍可见且不只依赖颜色。
- 每行主 play icon 创建独立 transport；kebab 菜单提供 edit、duplicate、delete。
- config source 状态是独立 full-width band，显示 readable/writable、挂载限制、advanced syntax
  数量和安全 blocker；没有 raw viewer/editor 入口。
- 状态带分别显示 config read、config write、key read、key generation 和 host-trust
  persistence capability，不能用单一“SSH ready”概括不同能力。
- create/edit 使用 panel 或 dialog；文本用 input，枚举用 select/segmented control，二元设置
  用 checkbox/toggle，identity 用多选，风险项有邻近 warning。不可写时控件只读且说明原因。
- 启用任一 host-verification weakened 设置时，保存前必须确认 MITM 风险；两个设置同时启用
  时明确说明普通 user-level trust 持久化和变更保护基本失效。
- edit、Host rename 或 delete 前若 `liveInstanceCount > 0`，确认界面显示准确数量，并说明
  现有实例继续运行、source 状态会改变且旧 transport 停止接受 reuse。

Keys tab：

- table 显示 filename、algorithm/bits、fingerprint、public 状态、read-only 和引用数；
- copy icon 只在 public endpoint 可用时出现；hover tooltip 说明图标；
- Generate key 打开受约束表单，提交后立即切回 workspace 中的新 keygen terminal；
- 失败/遗留 staging 用醒目但不泄露内容的状态带显示。

### 11.3 Workspace

左侧所有术语改为 Connection。点击一行直接切换唯一 terminal viewport，不恢复顶部 Tab。

- local 行显示 `ID`、`PWD`、`SINCE`；
- remote 行显示 `ID`、`TARGET`（alias，可辅以明确 user/host）、`SINCE`；
- keygen 行显示 task 类型、目标 filename 和状态，不显示 passphrase；
- desktop hover 保留 snapshot preview，mobile 不显示 hover preview；
- changed/deleted source 是持续可见标签，不因 config 后来恢复消失。

live instance 菜单：rename、close；live remote 且 transport ready/current 时增加“Open over
existing transport”。该动作进入 manager 的明确 reuse mode，页头展示来源 instance/alias，
用户再次执行 connect；不得在 workspace 内隐式创建。

local instance 不可能证明其中手动启动的嵌套 SSH transport。其菜单中 reuse action 以 disabled
状态显示，tooltip/辅助文本说明只有 Roaminal 创建的 remote connection transport 可复用；
后端收到 local 或未管理 process 的 reuse 请求也返回 `409 unmanaged_ssh_transport`，不得解析
command history 猜测连接或 credential。

historical 菜单：rename、delete、relaunch local 或 reconnect remote。动作同样进入 manager；
source changed 的 reconnect 文案明确说明会按当前 definition 新建 transport 并重新认证；
source deleted 不提供 reconnect、duplicate-source 或 reuse。exited/interrupted overlay 只提供
返回 manager 与查看历史。

### 11.4 交互与可访问性

- 使用仓库现有 lucide icon；熟悉动作使用 icon button，全部有 aria-label 和 tooltip。
- keyboard focus、dialog focus trap、Escape、screen reader 状态文本完整。
- 稳定 terminal/sidebar/action 尺寸，动态 badge 不引发布局跳动。
- 320/390/768/1440 px 视口无文字溢出、遮挡或不可达操作。
- 不在 UI 中放使用说明、快捷键宣传或产品功能介绍；必要说明只与当前 validation/warning
  直接相关。

## 12. 容器与 Kubernetes

### 12.1 Image

- `container/` 中生产 image 安装 `openssh-client`，不安装 ssh server。
- image 内建立 `/home/roaminal/.ssh`，owner `1000:1000`、mode `0700`；不声明隐式 Docker
  `VOLUME`，实际持久化由部署显式控制。
- 保持 `tini` PID 1、readonly root filesystem、non-root UID/GID 和现有 `/tmp` emptyDir；
  ControlPath 只使用 `/tmp`。
- 若项目第三方 notices 记录系统工具，补充 OpenSSH 相关声明。

Podman/Docker 文档默认示例增加：

```text
-v roaminal-ssh:/home/roaminal/.ssh
```

并分别说明整目录 Secret、直接 config 文件、直接 key 文件的只读行为。

### 12.2 Kubernetes 默认部署

保持 `deploy/kubernetes/` 为普通 YAML，不引入 Helm/chart。默认 develop deployment 增加独立
`roaminal-ssh` RWO PVC（建议 1 Gi）并挂载 `/home/roaminal/.ssh`，沿用 `fsGroup: 1000` 并设置
合适的 `fsGroupChangePolicy`，使 runtime UID 能在新卷创建自己的 config/key/known_hosts，
但不引入 init ownership 修复。现有 state/workspace PVC 职责不变。

同时在运维文档给出支持矩阵：

| 挂载 | 读取 | 修改 config | keygen | known_hosts 更新 | 外部轮换 |
| --- | --- | --- | --- | --- | --- |
| writable `.ssh` PVC | 是 | 是 | 是 | 是 | 由用户/备份流程负责 |
| Secret 挂载整个 `.ssh` | 是 | 否 | 否 | 否 | projected volume 可更新 |
| Secret 直接挂 `config` | 是 | 否 | 取决于 `.ssh` 其余目录 | 取决于目录 | `subPath` 不自动更新 |
| Secret 直接挂 key | 是 | config 能力独立 | 不能覆盖该 key，可生成其他名 | 目录能力独立 | `subPath` 不自动更新 |

不增加 init container 复制、chmod/chown 或 Secret 到 PVC 同步逻辑。不可写只影响对应操作，
不会阻止读取 config/key、建立 SSH 或使用 local connection。

运维文档还必须把 SSH egress 的来源明确为 Roaminal Pod，给出 DNS、NetworkPolicy、firewall、
proxy/bastion 可达性的诊断边界；不得依赖浏览器网络、host filesystem scan、第三方 gateway
或 container runtime socket。`.ssh` volume 与 Roaminal state volume 必须分别备份和恢复，
并把 `.ssh` 标记为高敏感用户数据；support bundle 和普通诊断不得收集其内容。

## 13. 并发、限额与故障隔离

- connection manager 内分别使用 instance registry lock、transport registry lock 和 config
  mutation mutex；不得持锁等待 PTY、OpenSSH、磁盘 fsync 或 WebSocket I/O。
- 启动采用 reserve -> spawn -> publish/rollback；每条错误路径释放 instance slot、transport
  reservation、PTY fd、process group 和 temp directory。
- config poll 读取失败采用有界日志节流，不把每秒失败刷屏；状态变化才广播 UI event。
- SSH/key/config 的错误均以 subsystem capability/status 体现；local Bash、auth、静态资源和
  其他 live terminal 保持工作。
- 沿用全局最大 connection instance 数并重命名配置项；keygen task 和 reuse channel 计入同一
  限额，避免绕过资源控制。
- terminal output snapshot 继续使用现有大小限制；terminal input 永不进入持久化。

## 14. 测试设计

### 14.1 单元与组件测试

必须随模块实现覆盖：

- sshconfig golden：所有支持字段、大小写、`=`、quote/comment/newline、unknown、wildcard、
  negation、multi Host、Match、duplicate、managed/unmanaged IdentityFile；
- lossless edit：未改 bytes 完全相同、span 顺序、create/copy/delete、冲突窗口、1 MiB 限额；
- parser fuzz：任意 bytes 不 panic、不越界、不意外读取 Include；
- sshfs：普通文件、projected `..data` symlink、目录内 symlink、逃逸、循环、magic link、
  owner/mode、readonly、root-owned Secret、final nonregular、rename race；
- key：allowlist、fingerprint parser、public endpoint、无覆盖 promotion、partial failure 和 staging；
- persistence：format v1、live -> interrupted、history attach、旧 nonempty sessions 拒绝、空旧
  目录行为、无 process restore；
- instance/source/transport 状态机：所有单调转换、reserve rollback、owner 先退出、last channel、
  drain、shutdown、limit；
- HTTP/WS/worker：新路径、新字段、ETag 428/412、auth/CSRF、readonly history、v1 worker 拒绝；
- frontend：manager 导航、无自动创建、表单能力、reuse mode、source badges、keygen、菜单和
  localStorage clean rename。

### 14.2 真实工具 integration

测试不能只依赖 fake runner。增加固定版本、仅测试使用的 OpenSSH fixture，相关容器资产放在
`container/test-ssh/` 或现有 test fixture 约定位置，生产 image 不包含 sshd。

真实 `ssh-keygen` 测试证明 stdin fingerprint、Ed25519/RSA 生成、交互 passphrase、不覆盖和
pair validation。真实 sshd 测试证明：

- alias 和 main config 由 OpenSSH 解析；
- unknown/changed host key prompt 通过 PTY；
- password、key、key passphrase prompt 不被结构化 API 捕获；
- 独立连接拥有不同 server transport；
- 显式 reuse 增加 channel 而不增加 transport；
- socket race、drain、master failure 时 reuse 不 fallback；
- owner 先退出、最后 channel、`-O stop`/`-O exit` 的资源回收；
- config 外部编辑/删除/同名重建导致正确 source 状态；
- Include 不被产品读取，但 OpenSSH 新连接仍按自身规则处理。

fixture 应通过 server 侧可观测计数或 sshd 日志断言 transport 数，不能只从客户端进程数推断。

### 14.3 浏览器与部署验证

在 develop namespace 使用不可变 image 部署，直接访问 Service 地址，禁止 port-forward。
Playwright 覆盖需求文档第 14 节全部 18 个用户流程，至少包含 desktop 1440x900、tablet
768x1024、mobile 390x844 和 320x568。

验证内容包括 screenshot、terminal canvas 非空像素、真实键盘输入/resize、hover preview、
菜单/dialog、readonly Secret 能力、PVC 可写能力、外部 config 轮换、unknown/changed host、
transport reuse、断网/exit、浏览器 console 和后端错误日志。结束后恢复默认 writable PVC
deployment，不把测试 Secret 或 credential 留在 namespace。

### 14.4 敏感数据审计

使用自动测试和静态搜索确认 private key bytes、public key（除显式 endpoint）、password、
passphrase、terminal input、unknown config argument、ControlPath 和 transport registry ID
不会进入：普通 API、heartbeat、structured log、metadata、snapshot metadata、浏览器存储或
错误消息。terminal output 按产品既有历史语义保存，但 keygen secret input 必须由禁用 input
tracking 和 PTY echo 行为双重验证。

该审计约束产品拥有的 config/key/API 流程。用户仍可在通用 local shell 中主动执行 `cat` 等
命令并让任意内容成为 terminal output；Roaminal 不解析或审查用户命令，也不能把这类输出
误分类为产品 key/raw viewer。它继续遵守普通 terminal scrollback/history 的既有语义。

## 15. 连续实施阶段与原子提交

得到用户明确“开始实施”后，agent 先完整重读需求与本文，再按以下阶段连续工作。每个提交
必须聚焦、通过对应测试、保持仓库可构建；发现相邻未提交用户改动时保留并做兼容合并。

### Phase 0：基线与可复现性

- 记录 clean/dirty worktree、当前 ForgeKit lint/test/build 结果、develop Service 与 deployment
  状态；不修改业务行为。
- 固化真实 OpenSSH test fixture 的可行性，尤其是 no-fallback server transport 断言。
- 若基线已有失败，先判断是否与本计划相关；能在范围内修复则原子提交，不能则按停止条件。

### Phase 1：低层 terminal 命名

原子提交建议：`refactor(worker): use terminal IDs in the engine protocol`

- worker protocol bump v2，内部 `sessionId` 全部改 `terminalId`；
- backend terminal 包只保留 PTY/shadow/marker 职责；
- 更新单元、integration 和协议不匹配测试。

### Phase 2：Connection instance 核心

原子提交建议：`refactor(connection): replace sessions with connection instances`

- 建立 connection manager、local launcher、新 HTTP/WS/heartbeat 契约；
- 新 persistence format v1、BackendRuntimeID、历史只读、旧 sessions fail-fast；
- 删除 session compatibility、自动进程恢复和旧命名；
- frontend 先切换新契约，允许临时保持最小 local manager 入口，但不能保留旧 endpoint alias；
- 更新配置名、CLI/help、测试与文档，提交结束时 local connection 全流程可用。

### Phase 3：SSH 事实源

原子提交建议：`feat(ssh): add secure config and key sources`

- 引入 sshfs、lossless sshconfig、ETag/CRUD/copy、key inventory/public endpoint；
- image 安装 OpenSSH client；
- 完成 symlink/mount/ownership、parser golden/fuzz、真实 fingerprint 测试；
- 暂未完成 remote runtime 时 API 明确报告 tool/runtime capability，不暴露半成品 connect。

### Phase 4：SSH runtime 与 transport

原子提交建议：`feat(connection): add OpenSSH connections and transport reuse`

- remote instance、ControlMaster registry、显式 reuse、no-fallback guard、drain/revision watch；
- 完成真实 sshd fixture 的 transport 计数、prompt、drift、shutdown/race 测试；
- 本阶段结束时 local 和 remote 的后端契约完整可用。

### Phase 5：Key generation

原子提交建议：`feat(ssh): add interactive key generation`

- 专用 keygen instance、PTY prompt、staging、RENAME_NOREPLACE promotion、失败保留；
- 完成真实 Ed25519/RSA/passphrase/interrupt/no-overwrite 测试。

### Phase 6：完整 Connection manager

原子提交建议：`feat(frontend): add the connection manager`

- Connections/Keys tabs、结构化 editor、capability/warning、所有创建入口集中；
- workspace connection 术语、菜单、changed/deleted、reuse mode、history/reconnect/relaunch；
- 删除自动初始 terminal 与全部直接创建入口；
- 完成组件、accessibility 和响应式 Playwright 测试。

### Phase 7：部署和运维

建议拆为两个原子提交：

1. `build(kubernetes): add persistent SSH home`
2. `docs: document connection and SSH storage operations`

- 增加默认 SSH PVC/mount、容器运行示例、Secret/direct-file 矩阵和备份说明；
- 保持普通 YAML、readonly root、tini 和 direct Service 验证方式；
- 更新架构/API/运维文档，移除“仅持久 Bash terminal”的过时描述。
- 首次切换 develop 前验证 namespace、PVC 和 legacy schema，仅清理已确认的 develop
  pre-release `sessions` 内容，使新 backend 能按 fail-fast 规则启动；这是一项部署操作，不得
  变成 product migration，也不得扩展到其他 namespace/PVC。

### Phase 8：端到端收口

原子提交建议：`test(ssh): cover deployed connection workflows`

- 对不可变 image 跑完整 ForgeKit、真实 OpenSSH、develop namespace 和 Playwright gates；
- 修复所有发现的问题，按所属模块追加小型原子提交，不把多个无关修复 squash 成一包；
- 检查进程、socket、PVC、Secret、console、日志和敏感数据。

### Phase 9：版本发布

原子提交建议：`release: bump Roaminal to 0.2.0`

- 这是 session terminal 向 connection platform 的 breaking minor 里程碑，目标版本 `0.2.0`；
- 必须使用 ForgeKit version 命令生成版本变更，绝不手工编辑 `container/VERSION`；
- 版本提交前后都执行仓库规定的 ForgeKit lint/version 校验。

执行过程中可以根据真实依赖把一个阶段拆成更多原子提交，但不得改变阶段顺序、合并安全门槛
或把测试推迟到末尾。无需为每个提交请求确认。

## 16. 每阶段质量门槛

每个涉及源码、build 或工具配置的提交至少执行：

1. 对应 Go/TypeScript/worker 单元和 integration tests；
2. backend、frontend、terminal-worker 构建；
3. 仓库 `AGENTS.md` 要求的 ForgeKit lint；
4. `git diff --check` 和生成物/敏感数据检查。

跨模块阶段还要执行全量测试。Phase 4 之后必须跑真实 OpenSSH fixture；Phase 6 之后必须跑
Playwright；Phase 7 之后必须构建 image 并在 develop namespace 验证。测试命令以仓库当时
ForgeKit task 为准，不在本文硬编码可能变化的 wrapper。

## 17. 停止条件

开始实施后，仅以下情况允许停止并请求用户处理：

1. 用户在同一核心文件留下无法安全合并、且会改变已确认产品决策的并发修改；
2. 连续三次独立诊断后，当前系统 OpenSSH 的真实 fixture 仍无法证明 reuse 不 fallback，或
   其行为与本文安全保证冲突；
3. 支持的 Linux/runtime 无法提供安全目录固定、no-follow 和 no-replace 原语，导致 key 或
   config 可能越界/覆盖；
4. 基线基础设施持续失败，且失败不在本计划范围、会使关键测试结果不可判定；
5. develop namespace、直接 Service、registry 或测试 SSH fixture 所需权限/credential 缺失，
   在完成本地与静态验证并做三轮诊断后仍无法继续部署验收；
6. 新事实揭示需求文档内部矛盾，继续实现必然需要改变已确认决策。

普通编译错误、测试失败、实现复杂、需要补依赖、需要重构、可恢复的集群 rollout 或首次方案
不工作都不是停止条件；agent 应自行修复并继续。

## 18. Definition of Done

以下条件必须全部满足，MVP 后续 Connection feature 才算实现完成：

- local/SSH definition 与 instance 模型、API、UI、state 完全分离，无旧 session 产品契约；
- 唯一 local launcher 和一次性 cwd 行为符合需求，所有创建入口只通过 manager；
- main SSH config 能 lossless 读取/结构化编辑，Include 永不被产品读取，未知语法原样保留；
- ETag 冲突、外部编辑/删除/同名重建、Secret 轮换都得到确定 source state 和 transport drain；
- key inventory、public 显式读取、Ed25519/RSA 交互生成和无覆盖提交通过真实工具测试；
- password/passphrase/host trust 完全在 OpenSSH terminal 中处理，没有 secret form/storage/log；
- 独立 SSH transport 与显式 reuse 都通过真实 sshd 计数验证，复用失败绝不 fallback；
- exit、网络失败、关闭、owner 先退、最后 channel、backend shutdown 都释放资源且 UI 不冻结；
- restart 只恢复只读历史，旧 sessions 非空时 fail-fast，不存在 compatibility decoder；
- writable PVC、whole Secret、direct config/key mount 的读取/修改矩阵均通过测试；
- Connection manager、Keys、workspace、desktop/mobile、keyboard/accessibility 和 hover preview
  通过 Playwright，canvas 非空、无重叠、无 console error；
- API/log/state/browser storage 敏感数据审计通过；
- 所有源码、构建、ForgeKit lint、真实 OpenSSH integration、image 和 develop deployment gates
  通过，直接 Service 地址可访问；
- 文档与实现一致，普通 Kubernetes YAML 保持，过时 terminal/session 说明已删除；
- `container/VERSION` 由 ForgeKit 更新为 `0.2.0`，工作树只含预期内容，所有变更形成清晰原子
  commits 并按用户届时指令 push。

任一项未满足都不能以“主体功能完成”宣告结束。

## 19. 需求追踪矩阵

| 需求 | 主要设计章节 | 主要验收 |
| --- | --- | --- |
| `CON-001..008` | 3、4、9、10 | model/API/历史/隔离测试 |
| `LOC-001..009` | 4.1、4.2、10.3、11 | local launcher、cwd、manager Playwright |
| `REM-001..013` | 4.3、6.6、8、10.3 | SSH lifecycle、source drift、真实 sshd |
| `SSHRT-001..008` | 3、8 | 系统 OpenSSH、PTY prompt、reuse fixture |
| `CFG-001..048` | 5、6、10.2 | parser golden/fuzz、lossless、ETag、mount |
| `KEY-001..025` | 5、7、10.4 | inventory、真实 ssh-keygen、敏感数据审计 |
| `UI-001..016` | 11 | component、accessibility、responsive Playwright |
| `AUTH-001..018` | 7、8、10.1、14.4 | 原生 prompt、无 secret flow/storage/log |
| `STATE-001..013` | 4.2、9、10.5 | restart、history、旧数据 fail-fast |
| `NAME-001..010` | 3、9、10、15 | 静态搜索、契约测试、worker v2 |
| `DEP-001..010` | 5、12、14.3 | image、PVC/Secret/direct mount、develop |
| `COPY-001..030` | 4.3、6.4、8、11.3 | duplicate、reuse、drain、no-fallback |
| `DATA-001..009` | 5、6.5、13、14.4 | race、atomic write、资源与敏感数据审计 |
| 必需用户流程 1..18 | 10、11、12、14.3 | develop namespace 的完整 Playwright suite |
| 验收基线 | 14、16、18 | 全量 gates 与 Definition of Done |

## 20. 关键技术依据

- OpenSSH client、multiplexing 和 `-O` control command：
  <https://man.openbsd.org/ssh>
- OpenSSH config 的 first-obtained-value、Host/Match/Include 和 token 语义：
  <https://man.openbsd.org/ssh_config>
- `ssh-keygen` 生成和 fingerprint 行为：
  <https://man.openbsd.org/ssh-keygen>
- OpenSSH 客户端源码中的 mux-before-connect 行为：
  <https://github.com/openssh/openssh-portable/blob/master/ssh.c>
- Go 目录范围文件访问设计：
  <https://go.dev/blog/osroot>
- Kubernetes volume、Secret readonly 与 `subPath` 更新限制：
  <https://kubernetes.io/docs/concepts/storage/volumes/>

这些依据用于锁定机制和测试目标，不能替代本仓库需求。实现时若当前依赖版本或操作系统行为
与本文不一致，必须通过真实测试确认，并按停止条件处理安全语义差异。
