# Roaminal SSH tmux 连接需求

状态：需求已确认。本文档记录本轮新增产品需求及与现有连接管理基线的关系。
全部产品决策已经完成，可以据此编写实施设计；本文档本身不是实施计划，也不
授权直接修改产品代码。

整理日期：2026-08-10。

## 1. 背景与目标

Roaminal 当前可以从 connection manager 启动普通 SSH connection，并通过系统
OpenSSH 建立独立 transport 或显式复用已经认证的 transport。本轮需求在这个
模型上增加 tmux mode：Remote SSH connection definition 可以选择启用 tmux
连接。启用后，Roaminal 连接远端已有的指定 tmux session；该 session 不存在时，
在远端创建后进入。

目标是让需要长时间保留远端工作状态的用户获得连贯的 tmux attach/create 体验，
同时保持以下既有原则：

- 系统 OpenSSH 仍是唯一 SSH runtime；
- Roaminal 不保存密码或 private-key passphrase；
- tmux 是远端用户环境中的可选能力，Roaminal 不安装、托管或复制远端 tmux
  状态；
- 本功能不能演变为任意远程命令、任意 shell template 或远程软件管理器。

## 2. 范围与术语

| 术语 | 本文档中的含义 |
| --- | --- |
| tmux mode | 某个 remote connection definition 上可选的启动行为。启用后，新 connection instance 进入指定远端 tmux session，而不是直接进入普通远端 login shell。 |
| tmux session name | 用户为某个 remote definition 配置的远端 tmux session 名称，例如 `t`。它作为数据参数用于 session 探测、attach 或 create。 |
| pending connection launch | 用户点击 tmux connection 后、connection instance 正式发布前的短暂启动过程。它可以在 workspace 显示 OpenSSH 原生 terminal、执行认证和远端能力检查，但不是 live/history connection instance。 |
| tmux probe | OpenSSH 已连接目标后，以同一个远端用户和运行环境检查 `tmux` 命令及目标 session 是否存在的受约束步骤。 |

本功能只适用于来自 OpenSSH `Host` 条目的 remote connection definition。内置
local connection 不增加 tmux 配置；用户仍可在 local connection 中自行运行
tmux，但该行为不由 Roaminal 管理。

## 3. 高级配置界面需求

### 3.1 展示与启用

- `TMUX-UI-001`：Remote connection 的 create/edit 界面必须提供一个默认收起的
  “高级选项”区域。tmux 配置只能出现在该区域，不能挤占基础 SSH 字段的主要
  操作路径。
- `TMUX-UI-002`：高级区域必须提供明确的“启用 tmux 连接”checkbox 或 toggle；
  新建 remote definition 时默认关闭。
- `TMUX-UI-003`：只有用户显式启用 tmux mode 后，tmux session name 控件才可
  编辑。未启用时该控件必须 disabled，不能只依靠视觉变灰阻止输入。
- `TMUX-UI-004`：启用 tmux mode 后，tmux session name 是必填项；空值或不符合
  `^[A-Za-z][A-Za-z0-9_-]{0,63}$` 的值必须在客户端和服务端得到一致拒绝，
  不能保存一个连接时才静默补默认值。
- `TMUX-UI-005`：界面可以用 `t` 作为示例或 placeholder，但不得把示例误保存
  为用户选择。本文档不要求一个隐式默认 session name。
- `TMUX-UI-006`：关闭 tmux mode 后，保存结果不得继续影响后续连接。表单是否
  在本次未保存编辑期间保留已输入名称可以由设计决定，但关闭状态不能把名称
  发送为有效启动配置。
- `TMUX-UI-007`：编辑已有普通 SSH definition 时，tmux mode 默认显示为关闭；
  升级不能让现有 Host 自动进入 tmux。
- `TMUX-UI-008`：启用了 tmux mode 的 definition 必须在连接列表中显示简洁、
  非纯颜色的状态及 session name，使用户在点击 Connect 前能区分普通 SSH 和
  tmux SSH。
- `TMUX-UI-009`：SSH config write capability 与 tmux add-on write capability
  必须分别展示。SSH config 只读但 `~/.roaminal` 可写时，用户不能修改 Host
  字段，但仍可启用、关闭或修改 tmux 附加选项；两者都只读时才禁用 tmux 控件。
- `TMUX-UI-010`：高级区域、enable 控件、session name、校验错误和帮助文本必须
  满足现有 desktop/tablet/mobile 布局及键盘、焦点、label、screen reader 要求，
  不得重现 select 或 dialog 打开后焦点被意外转移的问题。

### 3.2 Definition 操作

- `TMUX-UI-011`：Create、edit、duplicate 和 refresh 后必须一致展示 tmux 配置；
  页面 reload 或 Pod 内应用重启不能让已保存配置静默恢复为普通 SSH。
- `TMUX-UI-012`：复制 remote definition 时必须复制其 tmux enable 状态和 session
  name；用户仍需提供新的唯一 Host alias。
- `TMUX-UI-013`：删除 remote definition 时，与它绑定的 tmux 启动偏好也必须
  删除，但不得连接远端或删除远端 tmux session。
- `TMUX-UI-014`：启用、关闭或修改 tmux session name 只影响之后创建的
  connection instance。已经 live 的普通 shell 或 tmux client 不得被重启、
  转换或强制 detach。
- `TMUX-UI-015`：保存 definition 时不要求也不得自动连接远端检查 tmux。
  `tmux` 是否可用只能在用户发起连接时按实际目标、用户、PATH 和认证上下文判断。

## 4. tmux 配置事实与数据边界

- `TMUX-CFG-001`：tmux enable 状态和 session name 属于 remote connection
  definition 的启动配置，不属于 connection instance title、terminal history
  或一次性 connect form 状态。
- `TMUX-CFG-002`：SSH config 仍是 remote connection definition 的主事实源。
  tmux 配置只是以 Host alias 为外键的 Roaminal 附加启动选项；没有对应的当前
  SSH Host definition 时，tmux 条目没有独立存在或启动 connection 的能力。
- `TMUX-CFG-003`：tmux 附加选项的唯一持久化事实源固定为
  `~/.roaminal/ssh-connection-options.yaml`。不得同时在 SSH config、其他 state
  文件、browser storage 或 profile database 保存副本。
- `TMUX-CFG-004`：YAML 必须具有显式 `formatVersion`，并且每个条目只能保存
  Host alias、tmux enabled 状态和 session name。不得复制 `HostName`、`User`、
  `Port`、`IdentityFile`、Host block、effective SSH config、key 内容或 credential。
- `TMUX-CFG-005`：关闭 tmux mode 时应删除对应 alias 的 tmux 条目；absence 表示
  disabled。YAML 中没有有效条目时可以删除整个文件，不能依赖保存大量
  `enabled: false` 条目表达默认状态。
- `TMUX-CFG-006`：YAML 必须通过固定 schema 的安全 parser 读取；unknown field、
  duplicate mapping key、不受支持的 format version 或无效值必须报告明确配置
  错误，不能执行 YAML tag、任意类型构造或静默采用最后一个重复值。
- `TMUX-CFG-007`：OpenSSH 不认识 Roaminal 自定义 directive。实现不得向
  `$HOME/.ssh/config` 写入会导致 OpenSSH 报错的 `TmuxSession`、`RemoteCommand`
  或其他伪造 SSH option。
- `TMUX-CFG-008`：tmux 配置不能改变既有 SSH 字段的含义，也不能把显式
  `Host` alias 展平为一份 derived effective config。实际 destination、认证、
  host trust 和 transport 仍由 OpenSSH 按原始 alias 计算。
- `TMUX-CFG-009`：tmux session name 必须作为不透明的字面值处理，不做 shell、
  environment variable、OpenSSH token、路径、模板或命令替换。
- `TMUX-CFG-010`：Session name 必须匹配
  `^[A-Za-z][A-Za-z0-9_-]{0,63}$`：总长度为 1 至 64 个 ASCII 字符，首字符只能
  是字母，后续只能是字母、数字、连字符或下划线。系统必须保留大小写，不能
  trim、大小写转换或做其他 normalization；空白、句点、冒号、斜杠、Unicode、
  控制字符和 shell metacharacter 一律拒绝。
- `TMUX-CFG-011`：前端校验不是安全边界。backend 在保存和启动时都必须验证
  session name，且只能把它作为独立 argv/data 参数交给受约束命令流程，不能
  拼接进可执行的 shell command string。
- `TMUX-CFG-012`：Backend 必须在应用启动、SSH config refresh/watch 更新、
  connection manager 读取，以及 edit/duplicate/delete/connect 前执行附加选项
  reconciliation；每次使用条目前都必须重新确认主 SSH config 中仍有对应的
  当前可选择具体 Host alias。
- `TMUX-CFG-013`：只有成功读取并解析主 SSH config 后，才允许进行破坏性清理。
  确认 alias 已删除或不再是可选择的具体 Host 时，必须从 YAML 原子删除对应
  条目；确认主 config 文件不存在时，必须清空全部 tmux 条目。
- `TMUX-CFG-014`：主 SSH config 临时不可读、解析失败、mount 轮换中断或状态
  未知时，不得删除 YAML 条目。UI 应报告主配置不可用，禁止使用无法验证的
  tmux 条目，并在下一次成功读取后重试 reconciliation。
- `TMUX-CFG-015`：清理写入失败时，孤立条目在当前内存模型中仍必须视为无效，
  不能据此启动 connection；系统必须展示清理失败并在后续 reconciliation 重试，
  不能覆盖整个 YAML 或影响仍有效的其他条目。
- `TMUX-CFG-016`：通过 Roaminal rename Host 时，必须在同一受控 mutation 中把
  tmux 条目移动到新 alias；duplicate 必须复制条目到新 alias；delete 必须删除
  条目。具体跨文件 rollback/并发协议由设计定义，不能把半完成状态报告成功。
- `TMUX-CFG-017`：外部 rename 按“旧 alias 删除、新 alias 新建”处理：旧条目在
  reconciliation 时清理，新 alias 默认 tmux disabled，不根据相似 Host 内容
  猜测迁移。外部删除后同名重建若曾被成功观察为 absent，也不得恢复已清理条目。
- `TMUX-CFG-018`：SSH config read-only 不影响读取和使用有效 tmux 条目；tmux
  是否可编辑仅由 YAML state 的 write capability 决定。YAML 只读时仍可使用
  有效条目，但 create/update/delete/cleanup 必须禁用或报告明确 capability。
- `TMUX-CFG-019`：YAML 文件必须沿用 `~/.roaminal` state 的安全 ownership、
  permission、atomic write、durability、symlink 和并发保护，不得使用临时文件
  覆盖其他 state，也不得通过 YAML alias 引用 SSH config 之外的 Host。
- `TMUX-CFG-020`：YAML 无文件表示全部 disabled；YAML 内容损坏时必须保留原文件
  并报告错误，不能自动重置为空、部分采用或阻止普通 SSH connection 使用。

## 5. 远端 tmux 启动行为

### 5.1 固定流程

- `TMUX-RT-001`：未启用 tmux mode 的 remote definition 必须保持现有普通 SSH
  connection 行为，不运行 tmux probe，也不改变远端 login shell。
- `TMUX-RT-002`：启用 tmux mode 后，Roaminal 必须先用系统 OpenSSH 连接用户
  选择的 Host alias。不得由 Go、Node、浏览器 SSH library 或 tmux library
  自行建立 transport。
- `TMUX-RT-003`：tmux probe 和最终 attach/create 必须在目标 Host alias 的同一
  SSH 用户、同一已认证 transport 上执行，并采用该远端 SSH command 环境实际
  可见的 PATH。不得在 Roaminal 容器本地执行 `tmux ls` 后推断远端能力。
- `TMUX-RT-004`：认证成功后必须先探测远端 PATH 中是否存在可执行的 `tmux`
  命令。命令不存在时必须返回明确错误，终止 pending launch，且不能创建、
  发布或保留 connection instance。
- `TMUX-RT-005`：Roaminal 不得在远端自动安装 tmux、猜测非 PATH 绝对路径、
  上传 binary、修改 shell rc，或在 tmux 缺失时静默 fallback 到普通 shell。
- `TMUX-RT-006`：确认命令存在后，系统必须先通过 `tmux ls`/`tmux list-sessions`
  探测 session 列表，并按完整、精确的 session name 判断目标是否存在。不能用
  substring、前缀或 locale-dependent human message 猜测匹配。
- `TMUX-RT-007`：最终交互命令固定使用
  `tmux new-session -A -s <session-name>`。`-A` 必须让远端 tmux server 原子决定：
  目标 session 已存在时语义等同于 `tmux attach -t <session-name>`，不存在时
  语义等同于 `tmux new -s <session-name>`。
- `TMUX-RT-008`：`tmux ls` 是连接前的显式能力与状态探测，但其结果不能作为
  最终 attach/create 的排他事实。最终 `new-session -A` 必须重新以远端 tmux
  server 的当前状态作出决定，避免使用已过期的 list 结果。
- `TMUX-RT-009`：`tmux ls` 在“tmux server 尚未启动/没有 session”时可能非零
  退出；该状态应视为目标 session 不存在并进入 create 分支。权限、socket、
  protocol、配置或其他异常不能被一概当作“没有 session”，必须明确失败。
- `TMUX-RT-010`：probe 与最终 `new-session -A` 之间仍可能发生远端并发删除或
  tmux server 重启。最终命令失败时最多允许一次使用相同参数的受控重试；再次
  失败必须明确回滚，不能无限循环、误连其他 session、运行普通 shell或留下
  冻结实例。
- `TMUX-RT-011`：建立新 transport 时，command probe、`tmux ls` 和最终交互
  channel 必须共用同一个由本次 pending launch 建立的 OpenSSH ControlMaster/
  ControlPath。Probe 完成后通过已认证 master 打开最终 PTY channel，不得建立
  第二条网络 transport 或再次认证。
- `TMUX-RT-012`：复用现有 transport 时，probe 和最终交互 channel 必须共用
  用户明确选中的现有 ControlPath，并继续遵守 no-fallback 校验。任一 mux
  channel 无法在该 transport 上创建时必须失败，不能退回独立 SSH transport。
- `TMUX-RT-013`：最终 tmux client 必须获得可交互的远端 PTY，支持现有 terminal
  resize、键盘输入、颜色、alternate screen、复制和 snapshot 渲染能力。
- `TMUX-RT-014`：Roaminal 只支持固定的 list/attach/new 行为，不提供任意
  `tmux` flags、custom socket、custom config file、command prefix、startup
  command 或 shell fragment 字段。

### 5.2 失败与发布语义

- `TMUX-RT-015`：Connection instance 必须采用 reserve -> authenticate/probe ->
  spawn -> publish/rollback 语义。只有 tmux 命令存在且正确的 attach/create
  client 已启动后，实例才可作为 live connection 对用户发布。
- `TMUX-RT-016`：在 publish 前失败不能生成伪造的 live/exited/history instance，
  不能占用 instance limit、留下 audit snapshot、PTY、OpenSSH child、ControlPath、
  transport reservation 或远端辅助进程。
- `TMUX-RT-017`：tmux 缺失错误必须在 connection manager 当前目标附近显示，
  至少明确目标 alias、tmux mode 已启用以及“远端 PATH 中未找到 tmux”；错误
  不能只写入后端日志或以通用 `connection failed` toast 代替。
- `TMUX-RT-018`：认证失败、tmux probe 失败、attach 失败和 create 失败必须可
  区分到足以指导用户操作的稳定错误类别；不得通过解析任意 locale-sensitive
  terminal 文本泄露远端输出或生成脆弱分类。
- `TMUX-RT-019`：pending launch 必须可由用户取消，并具有有界的资源清理行为。
  取消或浏览器离开不能留下不可见 connection 或永久占用 starting slot。
- `TMUX-RT-020`：tmux probe 的 stdout/stderr 只用于受约束判断和错误诊断，
  不进入 terminal history、connection snapshot 或普通审计材料；错误响应必须
  去除控制字符和敏感环境信息。

## 6. tmux 生命周期与 transport reuse

- `TMUX-LIFE-001`：tmux session 属于远端 tmux server 和远端用户，不属于
  Roaminal connection instance。关闭、删除、网络断开或退出 Roaminal
  connection 只能 detach/结束对应 tmux client，不得执行 `tmux kill-session`。
- `TMUX-LIFE-002`：用户在 tmux 内 detach、结束目标 session、退出最后一个 pane，
  或远端 tmux server 终止时，SSH client 应按真实 exit 状态结束；Roaminal 必须
  走现有 connection exited、审计复制、活动数据清理和自动切换流程，界面不能
  冻结。
- `TMUX-LIFE-003`：再次启动同一 tmux-enabled definition 时，必须重新探测：
  session 仍存在则 attach，不存在则创建。Roaminal 不得以本地缓存宣称远端
  session 仍然存在。
- `TMUX-LIFE-004`：多个 connection instance 可以同时 attach 同一个远端 tmux
  session；tmux 原生支持多个 client 共享同一 session/window/pane 状态。每个
  Roaminal 实例仍拥有独立 connection ID、SSH channel、PTY、title、scrollback
  和关闭生命周期，关闭其中一个只 detach 对应 client。
- `TMUX-LIFE-005`：tmux mode 必须兼容现有独立 SSH transport 与用户显式
  authenticated transport reuse。复用时仍须在所选 transport 上执行 probe 和
  attach/create，但不得重新要求 SSH authentication。
- `TMUX-LIFE-006`：tmux probe 或 attach/create 在 reuse channel 上失败时，
  只回滚本次 pending launch，不得 drain、关闭或破坏其他正在使用共享 transport
  的 connection instance。
- `TMUX-LIFE-007`：关闭首个 transport owner 后，只要已有派生 channel 仍存活且
  transport 未进入真正 draining，后续从有效派生 connection 发起的 tmux reuse
  必须继续可用；不能把 `owner closed` 本身等同于 `transport draining`。
- `TMUX-LIFE-008`：修改 tmux enable 状态或 session name 不影响现有 live
  instance；之后的新独立连接和 reuse channel 必须使用当前 tmux 配置。
- `TMUX-LIFE-009`：系统必须分别维护 SSH transport/source revision 与 tmux
  launch revision。前者只由影响 SSH 路由、身份、host trust 或 transport
  compatibility 的 SSH 主配置决定；后者只由 YAML 中当前 alias 的 enabled/
  session name 决定，不能合并为一个 revision。
- `TMUX-LIFE-010`：Host block 中会影响 SSH transport 的字段发生变化时，仍按
  现有 source changed/draining/no-fallback 规则处理；tmux 功能不得削弱该规则。
- `TMUX-LIFE-011`：Roaminal restart 不恢复 remote tmux client 或 OpenSSH
  transport。原 instance 仍按现有规则成为 interrupted history；用户重新连接
  时通过远端实时探测 attach 到幸存的 tmux session。
- `TMUX-LIFE-012`：Live tmux-enabled remote connection 必须继续显示并支持
  “Open over existing transport”操作。来源 tmux 配置未改变时，新派生 connection
  使用同一 ControlPath 打开新的 SSH channel，并通过同一 session name attach
  为另一个 tmux client；不得因来源是 tmux client 而禁用 transport reuse。
- `TMUX-LIFE-013`：多个 client 的 viewport 尺寸可能触发 tmux 自身的 window-size
  策略。Roaminal 必须分别发送每个 PTY 的真实 resize，但不得尝试覆盖远端 tmux
  配置、伪造统一尺寸或把共享 session 的正常 resize 行为误报为 connection 故障。
- `TMUX-LIFE-014`：正式 connection instance 必须记录其启动时实际使用的 tmux
  enabled 状态、session name 和 launch revision，以便 workspace 如实展示运行
  事实；这些是 instance metadata，不是 SSH definition 或 YAML 的持久化副本。
- `TMUX-LIFE-015`：仅 tmux launch revision 变化时，现有 instance 继续显示其
  实际旧 session，不标记 `source changed`，现有 authenticated transport 继续
  current/reusable。新独立连接和 reuse channel 必须使用当前 YAML 选项。
- `TMUX-LIFE-016`：从使用旧 tmux 选项的 live instance 发起 reuse 时，connection
  manager 必须在确认动作中显示当前 YAML 将进入的 session；不能假装新 channel
  一定进入来源 instance 的旧 session，也不能静默使用旧选项。
- `TMUX-LIFE-017`：Pending launch 在 reserve 时必须锁定当前 tmux launch revision，
  并在最终 `new-session -A` 前重新比较。期间 enabled/session name 发生变化时，
  必须以稳定的 `tmux_settings_changed` 类别回滚并要求重新连接；不能自动改用新
  session，也不能继续使用过期选项。

## 7. OpenSSH 原生认证、tmux probe 与发布顺序

### 7.1 可观察启动顺序

启用 tmux mode 的完整可观察顺序必须是：

1. 用户在 connection manager 点击 Connect；
2. 系统保留未发布的 pending launch，进入 workspace 并显示由系统 OpenSSH
   PTY 支撑的原生 terminal；
3. Password、keyboard-interactive、private-key passphrase 和 host-key
   confirmation 全部由用户在该 terminal 中按 OpenSSH 原有流程交互；
4. OpenSSH 完成实际 authentication 和 host verification；
5. 在同一远端上下文确认 `tmux` command 存在；
6. 通过 `tmux ls` 精确探测目标 session；
7. 在同一 ControlMaster 上通过 `tmux new-session -A -s <name>` 原子 attach 或
   create；
8. 交互 client 成功启动后才把 pending launch 发布为正式 connection instance，
   并在当前 workspace 中选中新实例；
9. 任一步失败都按其真实类别回滚，不生成 live/history connection 或 audit，
   workspace 显示错误并提供返回 connection manager 的明确动作。

这组顺序是产品行为要求，不代表必须启动多个 SSH 网络连接。设计必须尽可能在
同一 OpenSSH transport 上完成，并保持现有 ControlMaster/no-fallback 保证。

认证 terminal 必须复用现有 workspace 的 xterm/PTY 输入输出路径，不增加
password/passphrase modal、credential form、askpass broker、secret endpoint 或
prompt text parser。Roaminal 不缓存、结构化采集、保存、记录或重放任何认证输入。
Pending launch 可以在 workspace 可见，但在 tmux probe 成功前不得出现在正式
connection list、history、heartbeat 或 audit 中。普通非 tmux SSH connection
继续使用现有流程，不因本功能增加额外 provisional state。

### 7.2 PendingConnectionLaunch 生命周期

- `TMUX-PEND-001`：Tmux-enabled connection 必须使用独立、仅存在于当前 backend
  runtime 内存中的 `PendingConnectionLaunch`。它不是 connection instance，不能
  写入 connection metadata、snapshot、history、audit 或 browser persistent
  storage。
- `TMUX-PEND-002`：每个 pending launch 必须拥有不可预测的 launch ID，并绑定
  创建它的当前 Roaminal authentication identity。其他用户、过期认证或未知
  browser context 不能 attach、输入或取消该 launch。
- `TMUX-PEND-003`：状态机固定为
  `reserved -> authenticating -> probing_tmux -> starting_tmux -> published`；
  任一未发布状态只能单调进入 `failed` 或 `cancelled`，不能从终态恢复或把失败
  process 重新标记为 live。
- `TMUX-PEND-004`：Pending launch 从 reserve 起计入全局 connection instance
  limit。成功 publish 时必须把同一 reservation 原子转移给正式 instance，失败
  时释放；不能在 pending 与 instance 之间重复计数或留下配额泄漏。
- `TMUX-PEND-005`：Workspace 必须通过仅属于该 launch 的临时 terminal stream
  显示 OpenSSH PTY。该 stream 沿用现有 origin/auth/input 安全约束，但不进入
  terminal worker snapshot、scrollback persistence、heartbeat connection list
  或审计材料。
- `TMUX-PEND-006`：只有最终 tmux PTY client 已成功启动后，backend 才能在同一
  受控状态转换中创建正式 `connectionInstanceId`、注册 instance/transport 关系、
  启用普通 snapshot/history，并向 frontend 发布成功事件。
- `TMUX-PEND-007`：用户显式 Cancel、authentication identity 失效或 backend
  shutdown 必须立即取消 launch，终止 provisional OpenSSH process group、PTY、
  probe/final channel、ControlMaster 和 runtime socket，并释放全部 reservation。
- `TMUX-PEND-008`：最后一个 frontend 离开 pending terminal 后保留固定 15 秒
  reconnect grace period；同一认证身份在期限内恢复可以继续，期限结束仍无
  client 时必须取消。不得让不可见认证 process 无限存活。
- `TMUX-PEND-009`：连续 5 分钟没有 terminal input、terminal output 或经过认证
  的 launch heartbeat 时必须按 idle timeout 取消；存在真实交互时不设置额外
  总时长上限，不能让慢速人工认证因固定 wall-clock deadline 失败。
- `TMUX-PEND-010`：Authentication、tmux command、probe、final spawn 或一次受控
  race retry 失败后，只能保存短生命周期、无 secret 的错误结果供当前 UI 展示，
  随后回收 launch。错误不得生成 exited/interrupted history 或 audit snapshot。
- `TMUX-PEND-011`：用户 Retry 必须创建全新的 launch ID、PTY、process group、
  transport reservation 和必要的 ControlPath，不能复用失败进程、terminal input
  或认证状态。
- `TMUX-PEND-012`：Cleanup 必须幂等并覆盖部分初始化的每个阶段。Backend shutdown、
  WebSocket race、重复 Cancel 和 timeout 并发发生时，只能有一个最终回收结果，
  不能误杀其他 launch 或已发布 connection。

## 8. 必需用户流程

后续设计和验收测试至少必须覆盖：

1. 新建普通 SSH definition，不展开高级选项，保存并连接；行为与当前版本一致，
   远端不安装 tmux 也不受影响。
2. 展开高级选项但不启用 tmux；session name 不可编辑，保存后仍创建普通 SSH
   shell。
3. 启用 tmux，输入 `t`，连接到已存在 session；通过 `tmux ls` 发现它并 attach，
   原有 pane 内容和状态可见。
4. 启用 tmux，连接到没有目标 session 但已安装 tmux 的主机；创建 `t` 并进入，
   断开后远端 session 仍存在，再次连接时走 attach。
5. 连接到 PATH 中不存在 tmux 的主机；manager 显示明确错误，无 live/history
   connection、audit snapshot 或残留 transport/PTY。
6. `tmux ls` 因权限或其他异常失败；系统不把异常误判为“session 不存在”，
   不执行 create。
7. 在探测和 attach/create 之间并发创建或删除同名 session；最终
   `new-session -A` 按远端当前状态原子 attach/create，极端失败只重试一次，
   不冻结、不误连、不 fallback 到普通 shell。
8. 同一 tmux definition 启动两个独立 connection instance；二者可同时 attach，
   关闭一个不关闭远端 session或另一个实例。
9. 从 live tmux connection 显式复用 authenticated transport；不重新认证，
   仍执行 tmux 探测并以新的 tmux client 进入同一个配置 session；两个
   connection 均可继续交互，关闭任一个不影响另一个或远端 session。
10. 关闭原始 transport owner 后，从仍存活的派生 connection 再次复用；只要
    transport 仍有效就能创建新的 tmux connection，不出现错误 draining。
11. 修改 tmux session name；旧实例保持原 session，新实例使用新名称，单纯的
    tmux 配置变化不终止仍兼容的 SSH transport。
12. 使用 encrypted key、password、keyboard-interactive 和 unknown host 分别
    启动 tmux connection；所有 prompt 都在 pending workspace terminal 中由
    OpenSSH 原生显示和读取，不出现结构化 credential 弹框。
13. 在 tmux-enabled Host 上完成 OpenSSH 认证后发现远端缺少 tmux；系统清理
    已认证但未发布的资源，不缓存认证输入，也不创建 history/audit connection。
14. 用户在认证或 host-key confirmation 期间取消 pending launch；系统关闭
    OpenSSH process、PTY 和 provisional ControlMaster，不影响其他 connection。
15. 在 desktop `1440x900`、tablet `768x1024`、mobile `390x844` 和 `320x568`
    验证高级区域、enable 控件、session name、pending terminal、错误状态、焦点
    与文本均可用且不重叠。

## 9. 非目标

本功能不包括：

- local connection 的结构化 tmux launcher；
- tmux session browser、pane/window 列表、预览、rename、kill、detach-all 或
  remote tmux server 管理；
- 自动安装/升级 tmux，或规定远端 tmux 版本；
- 自定义 tmux executable path、socket、config、flags、environment 或 startup
  command；
- 把 tmux session 内容、scrollback 或 server state 同步到 Roaminal；
- 任意 SSH `RemoteCommand` editor 或通用远程 command template；
- 保存、记住、同步、恢复或共享 private-key passphrase；
- SSH account password、private-key passphrase 或 keyboard-interactive response
  的结构化弹框、askpass broker、secret API 或 prompt parser；
- 引入 ssh-agent、credential vault、浏览器 key vault 或 application-owned SSH
  implementation；
- 改变 `ssh-keygen` 创建 key 时现有的原生 passphrase 交互；
- 自动信任 unknown/changed host key，或弱化现有 host verification 规则。

## 10. 与现有认证基线的关系

`docs/plan/requirements.md` 中 `REM-004`、`SSHRT-007`、`AUTH-001..018`、
`COPY-024` 以及 key-generation passphrase 规则继续有效。本功能不对 password、
keyboard-interactive、private-key passphrase 或 host-key confirmation 增加任何
结构化 secret flow；唯一变化是 tmux-enabled launch 在认证期间以 provisional
状态显示于 workspace，probe 成功后才发布为正式 connection instance。

## 11. 验收基线

只有后续设计能够满足以下全部结果，才可以进入实施阶段：

- 普通 SSH definition 的默认行为完全不变，tmux mode 默认关闭；
- tmux 只作为 remote definition 的显式高级选项，启用后 session name 必填；
- tmux 附加选项只存储在 `~/.roaminal/ssh-connection-options.yaml`，不生成非法
  OpenSSH directive、不复制 SSH facts，并在每次使用前对主 SSH config 完成
  reconciliation；
- SSH config 成功读取后不存在的 Host 条目会被安全清理，临时不可读或解析失败
  不会误删 YAML；SSH config 与 tmux add-on write capability 分别如实展示；
- 远端命令缺失时明确失败且不创建 connection，不安装 tmux、不 fallback；
- 先检查 command，再以 `tmux ls` 精确探测，最终在同一 OpenSSH ControlMaster
  上使用 `tmux new-session -A -s <name>` 原子 attach/create；
- session name 严格匹配 `^[A-Za-z][A-Za-z0-9_-]{0,63}$`，无 normalization、
  命令注入、target 歧义或任意 remote command 能力；
- 远端 tmux session 生命周期独立于 Roaminal connection，关闭 connection 不
  kill session；
- 独立 transport、显式 reuse、owner 先关闭、config drift 和 restart 行为均有
  明确状态与资源回收；
- SSH transport/source revision 与 tmux launch revision 完全分离；仅 tmux 配置
  变化不产生 source changed 或 transport drain，新 launch/reuse 使用当前选项，
  pending 期间变化则明确回滚；
- Existing instance 持续显示实际 tmux session；从使用旧选项的 instance 发起
  reuse 时，manager 明确展示新 connection 将采用的当前 session；
- Pending launch 使用独立 runtime-only 状态机，计入限额，支持 15 秒断线重连和
  5 分钟无活动超时，成功原子发布、失败无 history/audit 且完整清理资源；
- password、keyboard-interactive、private-key passphrase 和 host trust 全部在
  pending workspace terminal 中由 OpenSSH 原生处理，没有 credential modal、
  askpass broker、secret API、prompt parser、cache 或新增持久化；
- 单元测试、真实 OpenSSH/tmux integration、race/resource cleanup、ForgeKit
  lint 和 Playwright 响应式/可访问性/console 验证全部通过；
- 文档、API 契约、部署安全前提和实际界面保持一致。

## 12. 需求决策完成状态

本轮需求中的产品决策已经全部确认：

1. Tmux 附加选项使用 `~/.roaminal/ssh-connection-options.yaml`，SSH config 保持
   主事实源，并对孤立 alias 安全 reconciliation。
2. Session name 匹配 `^[A-Za-z][A-Za-z0-9_-]{0,63}$`。
3. 保留 `tmux ls` 探测，最终使用 `tmux new-session -A -s <name>` 在同一
   OpenSSH transport 上原子 attach/create。
4. 所有 OpenSSH 认证和 host-key prompt 继续在 workspace terminal 原生交互，
   不增加 credential modal、cache 或 secret API。
5. Tmux launch 在 probe 成功前使用独立 runtime-only pending 状态，按第 7.2 节
   的 timeout、cancel、disconnect、publish 和 cleanup 规则处理。
6. SSH transport/source revision 与 tmux launch revision 分离；tmux 变化不
   drain transport，pending race 明确失败重试。

下一阶段可以据此编写完整设计和连续实施方案；在设计得到明确实施授权前不修改
业务代码。
