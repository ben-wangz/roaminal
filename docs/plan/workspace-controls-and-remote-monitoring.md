# Roaminal 场景虚拟键盘与远端监控实施计划

> 状态：实施中（已完成代码落地与自动化验证，待部署验收）。
>
> 本文是交给实施 agent 的连续执行计划。获得明确实施授权前，只允许修改本文，
> 不得改动业务代码、部署资源或运行中的 connection。

## 1. 目标

本功能同时完成三项界面调整：

1. 删除左侧栏顶部重复的 `+ Connections` 按钮。Connection manager 继续只从
   workspace 右上角的 `Connections` 入口打开。
2. 在左侧栏增加固定的“虚拟键盘”区域，为当前 active connection 提供场景化
   输入按钮。首批场景为 `Tmux` 和 `Codex`。
3. 保留 Roaminal 自身监控，并为当前 active SSH connection 增加目标执行环境的
   基础监控。现有 Roaminal 监控继续保持在第一行且布局不变；REMOTE 使用独立的
   第二行 band，不能挤占或重排 `Connected ... / Sign out` 的现有排版。

虚拟键盘只是现有 terminal input 的快捷输入面板，不创建新连接、不执行后端
业务命令、不保存 terminal input，也不改变 OpenSSH/tmux/Codex 的生命周期。
远端监控是尽力而为的只读观测能力；它不安装 agent、不成为 SSH connection 的
健康前置条件，也不把宿主机指标伪装成容器或 Pod 指标。

## 2. 当前实现基线

- `frontend/src/ui/sidebar.tsx` 在 sidebar header 下方渲染一个
  `.sidebar-actions` 区域，其中只有重复的 `+ Connections` 按钮。
- workspace 右上角已经有 `Connections` 跳转入口，因此删除左侧入口不会使
  connection manager 失去可达性。
- `frontend/src/input/touch-keyboard.tsx` 已在宽度不超过 `800px` 时提供通用
  `ESC/TAB/SHIFT/CTRL/ALT/SYM/方向键`，但它位于 terminal 底部，不支持场景。
- `TerminalRuntime` 已通过 WebSocket 的 `input` message 向当前 PTY 发送字节，
  但公开的 `send()` 不会主动 claim terminal control；新的输入入口必须统一这一
  行为。
- connection instance summary 已有 `tmuxEnabled` 和 `tmuxSessionName`，可以
  准确判断当前实例是否以 tmux 模式启动，不能根据标题或 terminal output 猜测。
- Roaminal 容器内的 `$HOME/.tmux.conf` 不是 SSH 目标主机的配置，不能用它决定
  remote tmux prefix。
- `backend/internal/monitor/` 已按 Roaminal 进程自己的 cgroup v2 读取
  `cpu.stat`、`cpu.max`、`memory.current`、`memory.stat` 和 `memory.max`；heartbeat
  每秒把快照发给 frontend。该数据源必须保留。
- `frontend/src/status/system-status.tsx` 当前把 `.status-monitor` 绝对居中放在
  `.topbar` 第一行。该 Roaminal monitor 的组件、位置和指标保持现状；REMOTE 必须
  使用 topbar 之后的独立第二行，不能向 `.status-monitor` 追加字段。
- 一个 SSH transport 可以承载 owner、derived terminal 与短生命周期辅助 channel；
  远端监控必须以 transport 为采样和合并单位，不能按浏览器或 terminal instance
  重复启动探测。

## 3. 需求基线

### 3.1 Sidebar

- `VK-SIDE-001`：删除 sidebar 内的 `+ Connections` 按钮、`onCreate` prop、
  `Plus` import 和仅供它使用的 CSS。
- `VK-SIDE-002`：右上角 `Connections` 入口保持现有行为、焦点语义和响应式布局。
- `VK-SIDE-003`：删除按钮后 session list 应自然向上填充，不留下空白占位。
- `VK-SIDE-004`：虚拟键盘位于 session list 下方、sidebar footer 上方；session
  较多时只滚动 session list，不能把虚拟键盘挤出 viewport。

### 3.2 场景选择

- `VK-MODE-001`：虚拟键盘提供 `Tmux` 和 `Codex` 两个 segmented mode。
- `VK-MODE-002`：active instance 的 `tmuxEnabled=true` 时，首次选中该实例默认
  使用 `Tmux` mode。
- `VK-MODE-003`：其他 active instance 默认使用 `Codex` mode；产品不通过
  process、title、PWD 或 terminal output 自动判断 Codex 是否正在运行。
- `VK-MODE-004`：用户可以在两个 mode 间手动切换。选择仅按 connection instance
  保存在当前页面内存中，不写 localStorage、不写后端；刷新后重新使用默认规则。
- `VK-MODE-005`：切换 active instance 时立即切换到该实例自己的 mode，不把一个
  connection 的临时选择错误应用到另一个 connection。
- `VK-MODE-006`：pending tmux launch、无 active runtime、WebSocket 未连接或
  connection 已结束时，所有虚拟按键禁用，不能把输入发给旧 runtime。

### 3.3 Tmux mode

Tmux mode 的按键为：

| 显示 | 发送字节 | 说明 |
| --- | --- | --- |
| 动态 `Ctrl+<Prefix>` | 一个经过验证的 Ctrl control byte | 只发送 prefix |
| 动态 `Ctrl+<Prefix> [` | prefix control byte 后紧跟 ASCII `[` | 进入 tmux copy mode |
| `PageUp` | `ESC [ 5 ~`，即 `\x1b[5~` | 普通 terminal PageUp sequence |
| `PageDown` | `ESC [ 6 ~`，即 `\x1b[6~` | 普通 terminal PageDown sequence |
| `q` | ASCII `q` | 退出 copy mode 等原生用途 |

- `VK-TMUX-001`：prefix label 和发送字节必须来自同一个规范化模型，禁止 UI 显示
  `Ctrl+K` 却发送其他字节。
- `VK-TMUX-002`：例如远端 effective prefix 为 `C-k` 时，两个动态按钮显示
  `Ctrl+K` 与 `Ctrl+K [`，分别发送 `\x0b` 与 `\x0b[`。
- `VK-TMUX-003`：prefix 查询失败时按本需求使用 `Ctrl+A` 作为 Roaminal fallback，
  并把 source 标记为 `fallback`。这是产品 fallback，不声称它是 tmux 原生默认值。
- `VK-TMUX-004`：tmux 原生默认 prefix 实际是 `Ctrl+B`；当远端 tmux server
  正常返回 `C-b` 时必须展示并发送 `Ctrl+B`，不能被 fallback 覆盖。
- `VK-TMUX-005`：若远端明确返回当前版本不支持的 prefix（例如多键 table、
  function key 或无法安全映射的值），禁用两个 prefix-dependent 按钮并显示简短
  tooltip；不得静默发送错误的 `Ctrl+A`。PageUp、PageDown 和 `q` 仍可使用。
- `VK-TMUX-006`：prefix 是 connection 启动时的 effective snapshot。本迭代不
  监听远端 `source-file` 或运行时 `set-option`；修改 prefix 后新建 connection
  即可重新探测。

### 3.4 Codex mode

Codex mode 的按键为：

| 显示 | 发送字节 | 说明 |
| --- | --- | --- |
| `Ctrl+T` | `\x14` | 单个 Ctrl control byte |
| `PageUp` | `\x1b[5~` | PageUp sequence |
| `PageDown` | `\x1b[6~` | PageDown sequence |
| `Esc` | `\x1b` | UI 使用正确拼写 `Esc`，不是 `ECS` |
| `q` | ASCII `q` | 普通文本输入 |
| `commit and push` | ASCII `commit and push` | 只输入文本，不追加空格、换行或 Enter |

- `VK-CODEX-001`：`commit and push` 必须是普通 terminal input，不调用 Git、
  GitHub API、shell command endpoint 或任何后端自动化。
- `VK-CODEX-002`：所有按键严格发送表中序列，不做平台键位转换，不根据浏览器
  `metaKey` 修改。

### 3.5 SSH 目标监控

- `MON-001`：Roaminal 自身监控不得被远端监控替代、迁移或删减；第一行
  `Connected <instance-name>`、现有 local monitor 与 workspace actions 保持当前布局。
- `MON-002`：active SSH connection 时，仅在第一行下方增加独立的
  `REMOTE <host-alias>` monitor band；不得把任何 REMOTE 指标塞入第一行。
- `MON-003`：REMOTE 提供 CPU、memory、resource scope、freshness、probe RTT、
  uptime、load average 和 root filesystem disk capacity。各指标必须标明自己的事实
  scope，不能用 CPU/memory 的 cgroup scope 概括所有数据。
- `MON-004`：容器目标优先显示它自身可见 cgroup 的 CPU 与 memory；只有正向判定
  当前视图代表 host 时才允许使用 `/proc/stat` 和 `/proc/meminfo`，且 UI 必须明确
  标记 `HOST`。无法判定归属时返回 unknown/unavailable，不能猜成 Pod/container/host。
- `MON-005`：支持 cgroup v2、常见 cgroup v1 controller 布局和明确标识的 host
  fallback。缺少 `sh`、`/proc`、cgroup 文件或读取权限时显示 `Unavailable`，terminal
  仍保持完全可用。
- `MON-006`：只有当前 active、live SSH instance 才轮询。local connection、manager、
  无 active instance、页面 hidden、instance exited 或 transport draining 时停止轮询，
  不为监控建立或复活 SSH transport。
- `MON-007`：同一 transport 的多个 derived connection、浏览器 tab 或并发 HTTP
  请求共享缓存和 singleflight；监控不得随 connection instance 数量线性放大远端
  process 数量。
- `MON-008`：首个 CPU 样本只建立 counter baseline，可显示 `Warming up`；从第二个
  样本开始基于 counter delta 计算。不得用一次累计值伪造 CPU 百分比。
- `MON-009`：保留最后一个成功样本时必须展示 freshness；超过阈值标为 `Stale`，
  连续失败达到阈值后标为 `Unavailable`，不能无限展示旧值冒充实时数据。
- `MON-010`：远端指标只存在于进程内短期缓存，不写 connection metadata、session
  history、audit、日志或浏览器持久存储。
- `MON-011`：指标允许部分可用。CPU/memory scope 无法判定时按已确认策略显示
  unavailable，但不能因此隐藏仍可可靠取得的 uptime、load、disk、freshness 或 RTT。
- `MON-012`：uptime 表示目标当前 PID namespace 可见 PID 1 的存活时间；load 表示
  `/proc/loadavg` 的 system view；disk 表示目标可见根文件系统 `/` 的容量。三者不得
  分别冒充 Pod lifetime、cgroup load 或 Kubernetes ephemeral-storage quota。

## 4. Tmux prefix 的事实源

### 4.1 为什么不直接解析文件

对于 SSH tmux connection，需要的是远端 tmux server 当前生效的 prefix，而不是
Roaminal Pod 内的 `~/.tmux.conf`。直接下载和解析远端文件还有以下问题：

- tmux config 支持 `source-file`、条件命令、命令别名及运行时 reload；文本中的
  最后一条 `set -g prefix` 不一定等于 effective value；
- 读取 raw config 会扩大 API 和敏感数据边界；
- 自行实现 tmux config parser 会形成第二套不完整语义。

因此本计划把用户需求“从 `~/.tmux.conf` 读取”实现为：让远端 tmux 自己加载其
配置，再读取它的 effective global option。固定命令为：

```text
tmux show-options -gv prefix
```

对于示例配置，结果应为 `C-k`：

```tmux
unbind C-b
set -g prefix C-k
bind C-k send-prefix
```

### 4.2 查询时序

1. 保持现有 `command -v tmux`、`tmux ls` 和 `new-session -A` 启动语义。
2. 原始 tmux PTY 通过 `tmux-ready` marker 进入 publish 回调后，使用同一个已认证
   OpenSSH ControlMaster 打开一个短生命周期的辅助 exec channel。
3. 辅助 channel 只运行固定的 `tmux show-options -gv prefix`，不得接受任何 UI、
   HTTP 或 config 提供的 command fragment。
4. 因 `tmux-ready` marker 与远端 server 实际完成启动之间可能有毫秒级竞态，使用
   有上限的短退避重试。建议总上限不超过 2 秒、单次输出上限 64 bytes。
5. 查询成功且值通过严格规范化后，把 prefix 写入即将 publish 的 instance meta；
   然后执行现有 `PromotePending`。
6. 查询超时、transport 已关闭或输出不支持时，不使已成功的 tmux connection
   失败；使用 fallback 或 unsupported 状态继续 publish。

辅助 SSH channel 必须沿用 transport reuse 的 no-fallback 约束：

- `ControlMaster=no`
- `ControlPersist=no`
- 指定现有 `ControlPath`
- `CanonicalizeHostname=no`
- `ProxyCommand=/bin/false`
- `-- <host-alias>`

它不得重新认证、建立独立 TCP transport、读取 private key、使用 ssh-agent，或在
ControlPath 失效时静默 fallback。

### 4.3 规范化

后端只接受并输出有限的 key model，不把命令原始 stdout 交给 frontend：

```text
tmuxPrefixKey: "k"
tmuxPrefixSource: "runtime" | "fallback" | "unsupported"
```

首版至少支持 `C-a` 至 `C-z`，大小写输入统一为小写 key，展示统一为大写。输出
必须 trim 单个行尾，拒绝多行、控制字符、额外 token 和超过上限的结果。

fallback 使用 `tmuxPrefixKey="a"`、`tmuxPrefixSource="fallback"`。明确但不支持
的值使用空 key 和 `unsupported`，从而避免错误按键。

## 5. SSH 目标监控技术方案

### 5.1 可行性结论与精度边界

本功能在“不安装远端 agent、不要求 root、只依赖现有 OpenSSH transport”的约束下
可行。远端固定 collector 读取的是执行该 SSH channel 的进程可见的 `/proc` 与
cgroup filesystem，因此它能准确描述该执行环境可见的资源视图，但不能凭空获得
Kubernetes Metrics API、Pod 名称、request/limit metadata 或宿主机外部视角。

这里必须区分“能读取”与“能正确命名”：

- 有限 cgroup quota/limit 或明确的隔离 cgroup root 时，使用 cgroup 数据并标为
  `CGROUP V2` 或 `CGROUP V1`；这适合有资源约束的 Pod/container。
- 普通 VM/物理机上的 sshd/session cgroup 如果没有独立资源约束，不把该 user slice
  的 `memory.current` 冒充整机 memory；改用 `/proc/stat`、`/proc/meminfo` 并标为
  `HOST`。
- 无 limit 的 container 与无约束 host 在某些 namespace/mount 组合下无法用可移植
  shell 百分之百区分。collector 只能按 cgroup mount root、self/PID 1 cgroup 关系和
  controller 约束作保守判定；仍有歧义时返回 `scope=unknown`，UI 显示 unavailable，
  不猜测数据归属。
- uptime 不直接展示 `/proc/uptime`：它是 system uptime。使用当前 PID namespace
  可见的 `/proc/1/stat` starttime 与 `/proc/uptime` 计算 PID 1 age，scope 标为 `PID1`。
- `/proc/loadavg` 是 system load view，不是 cgroup load。产品仍按需求展示 1/5/15
  分钟值，但单独标为 `SYSTEM`；即使 CPU/memory 属于 cgroup，也不能改标成 cgroup。
- disk 使用目标 mount namespace 内 `/` 的 filesystem capacity，单独标为 `ROOTFS`。
  overlay/rootfs 可能反映 backing filesystem，且不等于 Kubernetes ephemeral-storage
  quota；只要如实标识，它仍是对目标用户有用且可重复验证的容量视图。

因此产品文案使用 `REMOTE <host-alias>` 表示 SSH 目标，并按 metric family 使用
`CGROUP V1/V2`、`HOST`、`PID1`、`SYSTEM`、`ROOTFS` scope；不得用一个总 badge
掩盖混合 scope，也不得出现未经证实的 `Pod` 或 `Container` 标签。

### 5.2 固定 collector

后端通过现有 ControlMaster 打开无 PTY、短生命周期的辅助 channel，运行固定的
POSIX `sh` collector。不得上传二进制、写远端文件、执行 `free`/`top`/`docker`/
`kubectl`、依赖 Python/jq，或接受来自 HTTP/UI/config 的 command fragment。uptime 与
disk 可以尝试 POSIX `getconf`、`awk`、`df`；缺少某个可选 utility 时只把对应 metric
标为 unavailable，不能使其他 metric 或 SSH connection 失败。

collector 按以下顺序工作：

1. 从 `/proc/self/cgroup` 与 `/proc/self/mountinfo` 映射当前 process namespace 中
   controller 的安全 mount path；正确处理 mount root、mountpoint 与 proc octal
   escape，拒绝任何逃出已识别 mountpoint 的路径。
2. cgroup v2 读取 `cpu.stat` 的 `usage_usec`、`cpu.max`、
   `cpuset.cpus.effective`、`memory.current`、`memory.stat` 的 `inactive_file` 以及
   `memory.max`。
3. cgroup v1 分别定位 `cpu,cpuacct`、`cpuset` 与 `memory` controller，读取
   `cpuacct.usage`、`cpu.cfs_quota_us`、`cpu.cfs_period_us`、有效 CPU set、
   `memory.usage_in_bytes`、`memory.stat` 的 `total_inactive_file`/`inactive_file` 和
   `memory.limit_in_bytes`；识别 kernel 的 unlimited sentinel，不显示为巨大 limit。
4. 只有 scope 判定为 host 时才读取 `/proc/stat` 的 aggregate CPU counters 与
   `/proc/meminfo` 的 `MemTotal`/`MemAvailable`。
5. uptime 读取 `/proc/uptime` 的 system elapsed time、`/proc/1/stat` field 22 的
   PID 1 starttime 以及 `getconf CLK_TCK`。解析 `stat` 时必须从最后一个 `) ` 后定位
   fields，不能因 process `comm` 含空格或括号而错位。Go 后端计算
   `system_uptime - start_ticks / clk_tck`，只接受有限、非负结果，scope 固定为 `pid1`。
6. load 读取 `/proc/loadavg` 前三个字段，严格验证 1/5/15 分钟非负十进制值，scope
   固定为 `system`。不读取 runnable/total/PID 字段，也不将其重新解释为 cgroup load。
7. disk 只执行固定的 `LC_ALL=C df -k -P /`，按 POSIX format 从最右侧字段解析 `/`
   所在 filesystem 的 total/used/available KiB 与 capacity percent，Go 后端完成安全的
   bytes conversion。mount 固定为 `/`、scope 固定为 `rootfs`；不接受 UI 路径。
8. 输出 raw counters、limit、scope 与能力位，不在 shell 中做浮点计算。Go 后端用
   连续样本计算百分比并执行所有 bounds/overflow validation。

每个 controller 必须先选定一个 observation cgroup，再从同一 cgroup directory 读取
usage 与 limit，禁止把 leaf usage 与 ancestor limit 拼成一个百分比。self 已映射为
隔离 mount root 时使用该 root；否则选择 self 到可见 mount root 间最近且明确受约束的
cgroup。只有共享祖先约束、无法把容量可靠归属给当前执行环境时，capacity/percent
返回 null，不制造“精确份额”。

cgroup CPU 使用率定义为采样区间内 CPU time delta 除以 wall-time 和可归属的
effective CPU capacity；host CPU 使用率按 `/proc/stat` 的 aggregate total/idle delta
计算。`100%` 表示已用满当前标示 scope 的容量，而不是固定表示宿主机单核。同时返回
capacity cores，避免 `100% of 0.5 core` 与 `100% of 8 cores` 失去语境。memory 同时
保留 current 与 working set；UI 主值使用
`max(current - inactive_file, 0)`，百分比只在有限且合法的 limit/total 存在时显示。

### 5.3 SSH 调用与输出协议

辅助调用至少显式设置：

```text
ssh -T \
  -o ControlMaster=no \
  -o ControlPersist=no \
  -o ControlPath=<existing-path> \
  -o CanonicalizeHostname=no \
  -o ProxyCommand=/bin/false \
  -o BatchMode=yes \
  -o ClearAllForwardings=yes \
  -o PermitLocalCommand=no \
  -o RemoteCommand=none \
  -- <host-alias> sh -s -- <server-generated-nonce>
```

固定 script 从 stdin 传入。`ProxyCommand=/bin/false` 保持现有 no-fallback 约束：
ControlPath 失效时必须快速失败，不得新建 TCP connection 或触发认证。`BatchMode`
禁止 password/host-key prompt，`RemoteCommand=none` 防止用户 SSH config 中的
`RemoteCommand` 替换 collector，`ClearAllForwardings` 防止辅助 channel 意外继承
forwarding。实现必须用 argv 而非拼 shell command string。

每次 probe 使用 server 生成的 hex nonce，stdout 仅解析 nonce 包围的版本化记录：

```text
ROAMINAL_MONITOR_V1_BEGIN_<nonce>
scope=cgroup-v2
cpu_usage_ns=...
cpu_capacity_milli=...
memory_current_bytes=...
memory_inactive_file_bytes=...
memory_limit_bytes=...
pid1_start_ticks=...
clock_ticks_per_second=...
system_uptime_seconds=...
load_1=...
load_5=...
load_15=...
rootfs_total_kib=...
rootfs_used_kib=...
rootfs_available_kib=...
rootfs_capacity_percent=...
ROAMINAL_MONITOR_V1_END_<nonce>
```

登录 banner、motd 与 marker 外内容全部忽略；字段采用 allowlist、每个 key 只允许一次、
按字段只接受 bounded integer、fixed decimal 与受控 enum；禁止 NaN、Inf、指数表示和
locale decimal comma。可选 metric 通过受控 capability/status 表达，不能伪造 `0`。
单次 timeout 为 2 秒，stdout/stderr 各限制 8 KiB，
超限、缺 marker、重复/未知字段、数值 overflow 或非零退出均作为 probe failure。
不得把 raw stdout/stderr 写日志或返回 frontend。

### 5.4 采样、缓存与 API

新增受认证 endpoint：

```text
GET /api/connection-instances/:id/remote-monitor
```

endpoint 先验证 instance 为 live SSH instance，再在 manager 内解析其 transport。
缓存和 singleflight key 使用内部 transport identity，不使用可被用户控制的 alias、
instance title 或 ControlPath 字符串。建议时序：

- 未认证沿用现有 `401`；未知 instance 返回 `404`；已知但属于 local、exited 或没有
  live reusable transport 的 instance 返回稳定的 `409 no_remote_transport`，不启动
  probe；live transport 的 collector failure 使用 `200` 加下述 status 表达；
- frontend active poll interval：5 秒；`document.hidden` 时暂停并在 visible 后立即刷新；
- backend fresh cache TTL：4 秒；同一 transport 同时最多一个 probe；
- frontend poll 加小幅 jitter，manager 使用全局最多 4 个 collector process 的 semaphore；
  达到上限时优先返回尚可用的 cache/stale 状态，不建立无界等待队列；
- probe hard timeout：2 秒；任何请求都不能覆盖 manager 的 timeout；
- 最后成功样本超过 15 秒标为 `stale`，同时返回 age；
- 连续 3 次失败后标为 `unavailable`，仍可保留最后成功值但 UI 不再把它显示为 live；
- transport cleanup 时立即删除 counter baseline、snapshot 和 failure state。

建议响应模型使用显式 typed metric，便于 REMOTE 独立扩展而不修改第一行 local
monitor contract：

```text
status: "warming" | "available" | "partial" | "stale" | "unavailable"
sampledAt: RFC3339 timestamp | null
ageMs: non-negative integer | null
metrics: {
  cpu: { status, scope: "cgroup-v2" | "cgroup-v1" | "host" | "unknown",
         percent, usageCores, capacityCores },
  memory: { status, scope: "cgroup-v2" | "cgroup-v1" | "host" | "unknown",
            workingSetBytes, currentBytes, limitBytes, percent },
  uptime: { status, scope: "pid1", seconds },
  load: { status, scope: "system", one, five, fifteen },
  disk: { status, scope: "rootfs", mount: "/", totalBytes, usedBytes,
          availableBytes, percent }
}
probeRttMs: non-negative integer | null
```

每个 metric status 独立。第一份 CPU cumulative counter 只能返回 `warming`，但同一
snapshot 的 memory/uptime/load/disk 可以 available；只要部分字段成功，top-level
status 使用 `partial` 而不是清空整组。整个 collector snapshot 过期时 top-level 使用
`stale`，各 metric 保留自己的 capability 状态。CPU counter rollback、scope/capacity
改变或间隔异常时丢弃 delta 并重新建立 baseline。所有 percentage/byte/seconds 在后端
完成 validation 和合理 bound，frontend 不根据 raw counter 自行计算。

### 5.5 Transport 生命周期

monitor probe 是 transport 的 auxiliary channel，不是 terminal connection，也不能
增加 login session/history。`Transport` 需要明确的 probe reservation 或等价引用计数：

- 仅在 transport 可复用且非 draining 时取得 reservation；
- owner terminal 退出但 derived channel 仍存活时可以继续采样；
- 最后 terminal channel 退出后禁止新 probe，并 cancel 最多 2 秒内的在途 probe；
- cleanup 必须等待/终止 reservation 后才能 `ssh -O stop/exit` 和删除 ControlPath；
- probe 完成只释放 auxiliary reservation，不改变 owner、Channels、session title 或
  transport draining 语义；
- 多浏览器请求共享 singleflight，request cancel 不得遗留 goroutine/process；也不能
  因单个浏览器断开破坏其他 client 正在等待的同一份结果。

远端监控失败永远不能关闭 transport、改变 session 状态、触发自动重连或阻止用户
输入。它是 transport 的只读消费者，不是其 owner。

### 5.6 技术依据

- Linux kernel cgroup v2 文档定义了 `cpu.stat/usage_usec`、`cpu.max`、
  `memory.current` 与 `memory.max`：
  <https://docs.kernel.org/admin-guide/cgroup-v2.html>
- Linux kernel cgroup v1 文档定义了 `cpuacct.usage`、cpuset 和 memory controller：
  <https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v1/cpuacct.html>、
  <https://docs.kernel.org/admin-guide/cgroup-v1/cpusets.html>、
  <https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v1/memory.html>
- Linux kernel CFS bandwidth 文档定义了 v1 `cpu.cfs_quota_us`、
  `cpu.cfs_period_us` 与 `-1` unlimited 语义：
  <https://docs.kernel.org/scheduler/sched-bwc.html>
- `/proc/<pid>/cgroup` 与 `/proc/<pid>/mountinfo` 是 namespace 内定位 controller
  路径的事实源：<https://man7.org/linux/man-pages/man5/proc_pid_cgroup.5.html>、
  <https://man7.org/linux/man-pages/man5/proc_pid_mountinfo.5.html>
- `/proc/uptime` 描述 system uptime，因此不能默认解释为 container uptime：
  <https://man7.org/linux/man-pages/man5/proc_uptime.5.html>
- `/proc/<pid>/stat` field 22 是 process starttime，可与 clock ticks/system uptime 计算
  当前 PID namespace 可见 PID 1 age：
  <https://man7.org/linux/man-pages/man5/proc_pid_stat.5.html>
- `/proc/loadavg` 的前三个字段是 system run-queue/I/O wait 的 1/5/15 分钟平均值，
  因而必须标为 system 而非 cgroup：
  <https://man7.org/linux/man-pages/man5/proc_loadavg.5.html>
- POSIX `df -k -P /` 提供目标 mount namespace 可见 `/` filesystem 的稳定字段格式：
  <https://pubs.opengroup.org/onlinepubs/7908799/xcu/df.html>
- OpenSSH 官方手册说明 ControlMaster multiplexing、BatchMode、
  ClearAllForwardings 与 RemoteCommand 行为：<https://man.openbsd.org/ssh_config>。

## 6. 后端设计

### 6.1 Instance metadata

在以下结构中增加可选的 `TmuxPrefixKey` 与 `TmuxPrefixSource`：

- `persistence.SessionMeta`
- connection persistence JSON v1 对应结构
- `terminal.Summary`
- heartbeat、GET instance 和 `launch_published` summary

字段只对 `tmuxEnabled=true` 的 instance 有意义。历史数据缺少字段时按 fallback
规则解释；不需要 migration 或复制远端配置。Persisted value 只是该 instance 的
启动时快照，允许进入 audit metadata，因为它不含 secret 或 raw config。

持久化 validation 必须确保：

- key 为空或单个 ASCII `a..z`；
- source 为空或受控枚举；
- 非 tmux instance 不接受非空 prefix metadata；
- raw command output、ControlPath 和 remote config text 不进入 metadata。

### 6.2 Tmux probe helper

在 `backend/internal/connection/` 中建立小型、可单测的 tmux prefix helper，职责
限定为：

- 构造固定 no-fallback SSH argv；
- 在 timeout/output limit 内执行；
- 规范化 `tmux show-options` 输出；
- 区分 runtime、fallback、unsupported；
- 不记录 stdout、stderr、alias 之外的远端信息。

独立 transport 和 derived/reuse tmux connection 必须复用同一 helper。不要在
两个 launch 分支复制探测逻辑。

### 6.3 Tmux publish race 与 cleanup

- Probe 在 pending launch 的 publish callback 内运行，仍受现有 cancel、disconnect、
  five-minute idle timeout 和 shutdown cleanup 管理。
- 用户取消时 context 必须终止 probe，不得延迟 process/ControlPath 回收。
- 原始 PTY 在 probe 期间退出时停止重试，并按现有失败语义清理。
- Probe 的额外 channel 要计入 transport 的临时使用，或在锁内证明其生命周期不会
  与最后 channel cleanup 竞态；不能被 owner 退出提前删除 ControlPath。
- 同一 connection 只能 publish 一次；probe、cancel 和 process exit 并发时保持现有
  幂等保证。

### 6.4 Remote monitor 模块边界

实现按职责拆分，避免把 shell parsing、SSH lifecycle 和 HTTP 混在 handler：

- `backend/internal/monitor/`：复用/扩展 CPU set、counter delta、memory working-set、
  cgroup scope 与 strict collector protocol 的纯解析/计算；不能依赖 connection manager。
- `backend/internal/connection/`：固定 collector、ControlMaster auxiliary executor、
  transport reservation、singleflight/cache 与 instance-to-transport authorization。
- `backend/internal/server/`：只做认证、ID/path validation、status mapping 与 JSON DTO；
  不直接运行 `ssh` 或持有 raw counters。

Tmux prefix probe 与 remote monitor 可以共享 no-fallback argv builder、受限 output
capture 和 transport reservation，但不能抽象成接受 HTTP command string 的通用
remote exec API。monitor state 只挂在 live transport/manager memory 上，不进入任何
persistence struct。

## 7. 前端设计

### 7.1 输入 API 收口

为 `TerminalRuntime` 增加语义明确的 `input(data: string)` 方法：

1. runtime disposed/closed/未连接时 no-op；
2. 先发送 `claim_terminal_control`；
3. 再发送现有 `{type:"input", data}` message。

xterm `onData`、现有 `TouchKeyboard` 和新虚拟键盘全部使用该方法。保留通用
`send()` 供 resize/ping 等协议消息使用，但 UI 不再直接拼装 input message。

把 control-key 与 terminal-key sequence 提取到纯函数模块，例如：

```text
frontend/src/input/terminal-input.ts
```

纯函数负责 `Ctrl+A..Z`、PageUp、PageDown、Esc 和 literal text 的确定性编码，
组件中不得散落 magic string。

### 7.2 组件边界

新增：

```text
frontend/src/input/contextual-keyboard.tsx
frontend/src/input/contextual-keyboard-model.ts
```

组件输入至少包括：

- active connection instance 或 `null`；
- 当前 runtime 是否为 pending；
- runtime connected/closed 状态；
- `onInput(data)` callback。

`AppShell` 负责把当前 active instance 与 active runtime 对齐后传入 `Sidebar`。
`Sidebar` 只负责布局，不直接访问 WebSocket、heartbeat 或 localStorage。

### 7.3 Sidebar 布局

- 删除 `.sidebar-actions` 后给 `.session-list` 提供紧凑的顶部 padding。
- 虚拟键盘 section 使用 sidebar 全宽 band，不嵌套 card。
- section header 显示 `Virtual keyboard` 与 `Tmux/Codex` segmented control。
- key grid 使用稳定列宽与最小高度；`commit and push` 占满一整行。
- 普通 key 使用清晰的 `<kbd>` 风格文本按钮；不使用不必要的图标。
- panel 固定在 session list 下方；session list 设置 `min-height: 0` 并独立滚动。
- sidebar footer 可保留，但不得与 panel 重叠。

### 7.4 焦点与可访问性

- 每个按键有准确 accessible name，例如 `Send Ctrl+K then [`、`Type commit and push`。
- segmented control 使用 button group/`aria-pressed`，不能伪装成 tab 而缺少 tab
  keyboard semantics。
- 鼠标或触摸按键后应把焦点恢复到 active terminal；键盘用户通过 Tab 聚焦按键
  并按 Enter/Space 时仍能得到可预测焦点。
- disabled 状态使用原生 `disabled`，tooltip 只补充 unavailable 原因。
- 不在 UI 中展示 remote config 路径、probe 命令或实现说明。

### 7.5 REMOTE 独立监控带

保留当前 `SystemStatus`、`.status-monitor` 与第一行布局；新增独立
`RemoteMonitorBand`。DOM 顺序固定为：

```text
topbar: Connected <instance-name>  <existing ROAMINAL monitor>  workspace actions
remote-monitor-band: REMOTE <alias> <remote metrics with per-metric scope>
optional search bar
terminal stage
```

- `remote-monitor-band` 是主内容区全宽 band，不放入 sidebar、不嵌套 card，也不绝对
  定位到 topbar。不得修改第一行 `ROAMINAL` 的 CPU/MEM/UP/CONN/RTT 内容、顺序、
  居中位置或响应式行为。
- 只在 active live SSH instance 时显示 REMOTE band；local instance、manager 或无
  live instance 时隐藏并清空 remote state，不显示上一个目标数据。
- band header 使用 definition host alias：`REMOTE <host-alias>`，不得用 instance 数字
  后缀替代 alias。指标 grid 至少容纳 CPU、MEM、UP、LOAD、DISK、AGE 与 RTT；这里
  `AGE` 是 freshness，不能和目标 `UP` 混为一项。
- CPU/MEM 各自显示 `CGROUP V1/V2`、`HOST` 或 unavailable；UP 标 `PID1`；LOAD 标
  `SYSTEM` 并显示 1/5/15；DISK 标 `ROOTFS /` 并显示 used/total 与 percent。scope
  使用紧凑 badge/tooltip，不增加误导性的 Pod/container 文案。
- warming、available、partial、stale、unavailable 使用相同稳定 grid dimensions，
  单项 unavailable 只影响自己的 cell。状态文本变化不能推动 terminal 或造成水平
  跳动；tooltip 只说明 scope/不可用原因，不展示 collector 命令。
- `REMOTE` probe 的网络状态与 terminal WebSocket 状态分离；probe error 不能触发
  connection toast 风暴。状态改变时最多一条去重通知，普通短暂失败只在 band 内表示。
- frontend 用带 AbortController 的 hook/service 管理 active ID、visibility 和 poll；
  请求必须走现有 authenticated `api()`/refresh 流程，不能另建裸 fetch auth 状态；
  response 返回后再次核对 active ID，防止切换 connection 后写入旧目标快照。
- monitor band 高度变化、显示/隐藏和 responsive reflow 后，现有 terminal
  ResizeObserver 必须触发 xterm fit/resize，tmux pane 尺寸随实际 terminal stage 更新。

### 7.6 响应式

- Desktop 1440x900 和 1024x768：sidebar 300px 内 key label 不截断，session list
  仍有可用高度。
- Tablet/mobile sidebar overlay：panel 位于 overlay 内且可以点击；不能扩宽页面或
  遮挡 sidebar close button。
- 390x844 与 320x568：两行 key grid 和 segmented control 不重叠，最长的
  `commit and push` 自动占整行。
- 现有 mobile `TouchKeyboard` 保留；它提供通用 modifier/方向键，新 sidebar
  panel 提供场景快捷键，两者共享编码与 runtime input API。
- REMOTE band 使用可扩展 CSS grid：desktop 一行容纳核心指标，空间不足时在 band
  内换为两行；窄屏使用紧凑两列/三列 grid。无论 viewport 多窄都不能把 REMOTE
  指标塞回第一行。LOAD 三个值、ROOTFS used/total 和最长 scope/status 不得溢出或
  覆盖 terminal。

## 8. 不变量与安全边界

- 不新增 arbitrary remote command API。
- 不返回或保存 `~/.tmux.conf` raw content。
- 不根据 terminal output 解析 prefix、password、passphrase 或 Codex 状态。
- 不把虚拟键盘输入写入结构化日志、heartbeat、connection metadata 或浏览器存储。
- `commit and push` 不自动追加 Enter，避免点击即执行命令或发送 Codex prompt。
- 不为 probe 建立新的 SSH transport，不允许 OpenSSH fallback。
- remote monitor 只执行版本控制中的固定只读 collector；无 arbitrary command、raw
  stdout/stderr、remote environment dump 或新 credential prompt。
- monitor cache 以内部 transport identity 隔离，受现有 auth 保护；用户不能通过 ID
  枚举其他 instance，也不能用 alias/title 污染 cache key。
- 不把 remote metrics、counter、failure detail 或 collector output 写 audit/session。
- 不改变现有 tmux session attach/create、transport reuse、pending launch、audit 或
  connection exit 语义。
- 多浏览器 client 场景下，虚拟键盘与物理键盘遵守同一 terminal control ownership。

## 9. 测试计划

### 9.1 前端单元测试

- `Ctrl+A`、`Ctrl+K`、`Ctrl+T` 映射到准确 control byte；
- PageUp、PageDown、Esc、`q` 和 literal text 映射准确；
- `commit and push` 不包含换行；
- tmux runtime/fallback/unsupported model 生成正确 label、sequence 和 disabled 状态；
- tmux instance 默认 Tmux，普通 instance 默认 Codex，per-instance 临时选择隔离；
- pending/disconnected/closed runtime 不发送输入；
- remote monitor hook 只轮询 active live SSH ID，切换 ID/manager/local/hidden 后 abort
  或停止，迟到 response 不污染新 connection；
- warming/available/partial/stale/unavailable 与每个 metric 独立状态准确；
- CPU/MEM、PID1 uptime、SYSTEM 1/5/15 load、ROOTFS used/total/percent 的 scope、
  nullable 值和格式化准确；
- remote failure 不清空 local heartbeat monitor，也不改变 terminal runtime state。

### 9.2 后端单元与 integration

- 接受 `C-a`、`C-k`、大小写与单个合法行尾；
- 拒绝多行、额外参数、控制字符、过长输出和不支持的 key；
- 固定 SSH argv 包含 ControlPath/no-fallback guard，且不存在用户 command fragment；
- probe timeout、cancel、transport exit 和 retry 不泄漏 goroutine/process/channel；
- persistence round-trip 保留合法 prefix metadata，拒绝非法字段；
- 使用真实 sshd/tmux fixture，让远端 home 的 `.tmux.conf` 设置 `prefix C-k`，确认
  `tmux show-options -gv prefix` 返回并发布 `k`；
- 无自定义配置时确认实际 server value `C-b`；
- owner、derived reuse 两条 tmux launch 路径都覆盖；owner 先退出后 probe/reuse
  仍遵守 transport lifecycle；
- cgroup v2 fixture 覆盖 mount root mapping、nested path、quota/cpuset capacity、
  unlimited limit、working set floor、counter delta/reset，以及不能混合 leaf usage 与
  ancestor shared limit；
- cgroup v1 fixture 覆盖分离 controller mount、`cpuacct.usage`、quota sentinel、
  `total_inactive_file` 与 memory unlimited sentinel；
- host fixture 覆盖 `/proc/stat` delta 和 `MemAvailable`，ambiguous scope 必须返回
  unknown 而不是猜成 container/host；
- uptime fixture 覆盖 PID 1 `comm` 含空格/右括号、不同 CLK_TCK、container-like PID
  namespace、host PID 1、counter overflow/starttime 大于 system uptime；`getconf` 缺失
  时只让 uptime unavailable；
- load fixture 覆盖合法 1/5/15 fixed decimal、locale 隔离、NaN/Inf/负数/指数拒绝，并
  始终输出 `system` scope；
- disk fixture 覆盖 `LC_ALL=C df -k -P /`、filesystem source 含空格、large blocks、
  reserved/negative available、overflow、命令缺失和 malformed output，并始终输出
  mount `/` 与 `rootfs` scope；
- 任一 optional metric 失败只产生 top-level partial，不清空其他 metric；整个 framed
  protocol/SSH 失败才进入 snapshot stale/unavailable；
- collector protocol 覆盖 banner、正确 nonce、缺失/重复/未知字段、
  overflow、8 KiB 上限、stderr/non-zero、2 秒 timeout 和 malicious-looking text；
- SSH argv 覆盖 `-T`、现有 ControlPath、BatchMode、ClearAllForwardings、
  RemoteCommand override 与 no-fallback，测试 ControlPath 缺失时绝不新认证；
- 同一 transport 的并发请求只产生一个 remote process，4 秒 cache 命中不启动 probe；
  不同 transport 不串 snapshot/baseline；
- owner exit + derived live、最后 channel exit、draining、cancel 与 shutdown race 覆盖
  probe reservation，且 endpoint/probe 不增加 login session 或 persistence 数据。

### 9.3 Playwright 与部署验证

在 `develop` namespace 构建不可变 image，直接使用 Service 地址，不使用
port-forward。至少覆盖：

1. sidebar 不再存在左侧 `+ Connections`，右上角入口仍可打开 manager；
2. active tmux connection 自动选中 Tmux mode；
3. 测试目标远端 prefix 为 `C-k` 时按钮显示 `Ctrl+K` 和 `Ctrl+K [`；
4. 点击 prefix sequence 后远端 tmux 收到正确按键，PageUp/PageDown/q 可用；
5. 切换 Codex mode 后 `Ctrl+T`、Esc、PageUp/PageDown/q 发送准确；
6. `commit and push` 只出现在 terminal input 中且没有自动执行；
7. 切换 connection 不把输入发给前一个 runtime；
8. pending launch 和断开的 WebSocket 禁用按键；
9. desktop/tablet/390x844/320x568 无溢出、重叠和 console error；
10. hover preview、sidebar close、现有 mobile TouchKeyboard 和 terminal resize 无回归；
11. 第一行的 ROAMINAL CPU/MEM/UP/CONN/RTT、居中位置和 actions 与基线一致，没有
    迁移、删减或 reflow；
12. active SSH connection 在独立第二行显示正确 alias 的 `REMOTE`，切换两个 transport
    时不闪回旧 snapshot；local/manager 状态不显示该 band；
13. cgroup v2 fixture 显示准确 CPU/MEM 与 `CGROUP V2`，host fixture 明确显示 `HOST`；
14. 同一 REMOTE band 显示 PID1 UP、SYSTEM LOAD 1/5/15、ROOTFS `/` disk
    used/total/percent、freshness AGE 与 probe RTT，scope 无混淆；
15. 让 CPU/MEM scope ambiguous、`getconf` 缺失或 `df` malformed，验证单项
    unavailable/partial 不隐藏其他可靠指标；
16. warming、partial、stale、unavailable 与 monitor endpoint 失败不影响输入、
    WebSocket、tmux resize、其他 connection 或本地监控；
17. 多 browser context 同时观察同一 transport 时，通过测试计数确认 probe 被合并；
18. 刷新、切 manager、切 local connection、退出最后 SSH channel 后无遗留 poll、
    remote process、console error 或新增 login session card。

部署验证只能修改项目自己创建的 SSH fixture/codespace Pod，不影响 namespace 中的
其他 Pod。测试结束后恢复 fixture 配置并清理测试 connection。

## 10. 文档更新

实施时同步更新：

- `docs/api.md`：connection summary 新增的规范化 tmux prefix metadata；
- `docs/api.md`：remote monitor endpoint、status/scope/nullable 字段与错误语义；
- `docs/security.md`：两类 probe 使用既有 ControlMaster、固定 command、no-fallback、
  output limit、cache/auth 隔离且无 raw config/API；
- 必要的用户文档：说明虚拟键盘输入不自动执行，tmux prefix 是启动时快照；
- 运维/用户文档：解释 `CGROUP V1/V2`、`HOST`、`PID1`、`SYSTEM`、`ROOTFS`、
  warming/partial/stale/unavailable 与 agentless 精度边界，明确 remote uptime/load/disk
  分别不等于 Pod lifetime、cgroup load、Kubernetes ephemeral-storage quota；
- E2E 文档/fixture 说明：remote `.tmux.conf` 测试边界。

不得把本文中的实施细节作为可见 UI 帮助文字堆入产品界面。

## 11. 连续实施阶段与原子提交

获得实施授权后，agent 按顺序连续执行，不等待人工确认；只有第 13 节停止条件可
中断。

### Phase 0：基线

- 记录工作树、ForgeKit gate、develop deployment 与直接 Service 状态；
- 跑现有 backend/frontend/worker tests 和 Playwright sidebar/tmux/monitor 基线；
- 确认测试 SSH fixture 的 tmux、`.tmux.conf`、cgroup/proc/PID namespace/rootfs scope
  与受控故障能力；
- 记录 topbar、local monitor 与 terminal stage 在四个目标 viewport 的截图和尺寸，作为
  第二行布局/resize 回归基线。

### Phase 1：统一 terminal input

建议提交：`refactor(frontend): centralize terminal input sequences`

- 新增纯输入编码函数；
- 增加 `TerminalRuntime.input()` 并迁移 xterm/TouchKeyboard；
- 完成输入字节与 control ownership 单测。

### Phase 2：探测 effective tmux prefix

建议提交：`feat(tmux): expose effective prefix per connection`

- 增加 prefix metadata、validation 和 persistence；
- 实现同 transport/no-fallback probe、规范化、retry/cancel；
- owner/reuse 共用 helper；
- 完成真实 sshd/tmux integration、race 与 cleanup 测试。

### Phase 3：远端监控 collector 与 transport orchestration

建议提交：`feat(backend): collect remote connection metrics`

- 实现 cgroup v2/v1/host/unknown CPU/MEM、PID1 uptime、SYSTEM load、ROOTFS disk、
  strict framed protocol 与 counter calculator；
- 实现 fixed auxiliary SSH runner、probe reservation、timeout/output cap；
- 增加 per-transport cache/singleflight、remote-monitor endpoint 和完整 race/integration；
- 确认 monitor 不新增 login session、persistence、authentication 或 fallback。

### Phase 4：Sidebar 虚拟键盘

建议提交：`feat(frontend): add contextual virtual keyboard`

- 删除重复的左侧 Connections 按钮与 dead CSS/props；
- 增加 mode model、组件、active runtime 对齐与 exact input；
- 完成组件/单元测试。

### Phase 5：REMOTE 独立监控带、响应式与可访问性

建议提交：`feat(frontend): show remote connection metrics`

- 保持第一行 local monitor 原样，新增可独立扩展的第二行 REMOTE band；
- 增加 active/visibility-aware polling、typed per-metric scope、partial/freshness model
  与 stale guard；
- 调整 sidebar/session list/panel 稳定尺寸；
- 完成 keyboard focus、screen reader name、mobile overlay、monitor band 和截图测试；
- 检查现有 local monitor、TouchKeyboard、preview、terminal/tmux resize 回归。

### Phase 6：文档、发布与部署

- 更新 API/security/user docs；
- 运行完整 ForgeKit lint、真实 OpenSSH/tmux/cgroup fixture、race test 与 frontend
  production build；
- 按仓库规则使用 ForgeKit version 命令决定并生成版本变更，不手工编辑
  `container/VERSION`；
- 构建、推送不可变 image，滚动部署到 `develop`；
- 用直接 Service 地址执行 Playwright 全 viewport 验收；
- 按用户既有要求清理本地未使用 Podman images，不删除 volume。

版本提交和部署修复必须保持原子；不得把发现的无关重构混入本 feature。

## 12. Definition of Done

以下条件全部满足才可宣告完成：

- 左侧 `+ Connections` 完全移除，右上角 manager 入口无回归；
- sidebar 虚拟键盘在所有目标 viewport 可用且不挤压/覆盖 session card；
- tmux instance 默认选择 Tmux mode，普通 instance 默认 Codex mode；
- `C-k` 示例配置得到 `Ctrl+K`/`Ctrl+K [` 和准确字节；
- 实际默认 tmux server 返回 `C-b` 时 UI 如实使用 `Ctrl+B`；
- tmux prefix probe 失败 fallback、unsupported prefix、pending/disconnected/closed 状态安全；
- Codex 六个按键严格符合字节表，`commit and push` 不追加 Enter；
- 所有 UI 输入统一 claim control 后发送，不会误投旧 runtime；
- 第一行现有 ROAMINAL monitor 的指标、顺序、位置与响应式行为保持不变；active SSH
  connection 的 REMOTE monitor 只出现在独立第二行；
- cgroup v2、cgroup v1 与 host fixture 的数值和 scope 准确；ambiguous/unreadable 目标
  的 CPU/MEM 安全显示 unavailable，不出现伪造的 Pod/container/host 数据；
- REMOTE 同时提供 PID1 uptime、SYSTEM load 1/5/15、ROOTFS `/` disk
  used/total/percent、freshness 与 RTT，metric scope 与已确认语义一致；
- 单项缺失产生 partial 且不清空其他指标；首样本 warming、15 秒 stale、连续 3 次失败
  unavailable 和恢复状态通过确定性测试，最后成功值不会被无限当作 live；
- active/hidden/manager/local/exited 切换停止正确，迟到 response 不污染其他 connection；
- 同 transport 并发请求只产生一次 probe，monitor 不产生 login session/history、
  credential prompt、新 TCP connection 或 persistence；
- owner/derived/last-channel/draining/shutdown 下的 auxiliary reservation 与 ControlPath
  cleanup 通过 race 和真实 OpenSSH 测试；
- remote monitor 失败不影响 local heartbeat、terminal input/WebSocket、tmux resize 或
  connection lifecycle；
- 不存在 raw tmux config、arbitrary command、secret 或 input persistence 扩张；
- owner/reuse、cancel/timeout/exit/shutdown 的 transport 与 process cleanup 通过 race
  和真实工具测试；
- ForgeKit、unit/integration/build、Playwright、console/log 和直接 Service 验收通过；
- develop 使用本次不可变 image 且 rollout healthy；
- 文档与实现一致，工作树干净，变更形成清晰原子 commits 并按用户要求 push。

## 13. 停止条件

只有以下情况允许实施 agent 停止并请求决策：

1. 用户同时修改相同核心文件，无法在不改变本计划行为的前提下安全合并；
2. 现有 ControlMaster 无法在不重新认证或 fallback 的前提下承载 tmux prefix 或
   remote monitor auxiliary channel，且三轮独立诊断仍无法建立安全方案；
3. 目标 tmux 返回的 prefix 语义无法映射为确定字节，而需求要求对该未知形式继续
   自动发送；
4. registry、develop namespace、Service 或项目自有 SSH fixture 连续不可用，使
   部署验收结果无法判定；
5. 项目自有 cgroup v1/v2 fixture 表明 fixed collector 会把 scope 或资源归属错误标记，
   且无法通过返回 `unknown` 安全降级；
6. 新事实与已确认的 OpenSSH/tmux transport 安全边界直接冲突。

普通编译错误、测试失败、布局问题、短暂 rollout、需要增加受控 helper 或需要修复
相邻回归都不是停止条件，agent 应自行诊断并继续。

## 14. 非目标

- 通用可编辑 macro/keybinding 系统；
- 自动检测 Codex 进程、版本、模式或当前 TUI state；
- 自动执行 `commit and push`、Git 命令或 Enter；
- 完整解析、编辑或展示 `.tmux.conf`；
- tmux key table browser、copy-mode command editor 或任意 command palette；
- 监听远端 config reload 并实时刷新 prefix；
- local connection 的 tmux launcher；
- 修改现有 SSH config/tmux session 配置；
- 取代系统软键盘或现有 mobile TouchKeyboard；
- 在远端安装/管理 Roaminal agent、exporter、systemd service 或 Kubernetes sidecar；
- 调用 Kubernetes Metrics API、cAdvisor、Docker socket、Prometheus 或 cloud monitoring；
- 识别/展示 Pod 名、namespace、node、request/limit 来源或宿主机外部视角；
- 远端 process list、network/disk I/O、GPU、非 `/` filesystem 聚合与 inode dashboard；
- 把 SYSTEM load 伪装成 cgroup load，或把 ROOTFS capacity 伪装成 Kubernetes
  ephemeral-storage request/limit；
- 存储 remote metrics、历史趋势、告警、审计报表或跨 connection 聚合 dashboard；
- 通过 monitor 自动重连、改变 transport lifecycle 或判断 terminal 是否健康。

## 15. 评审说明

本计划采用以下解释，评审时需要特别注意：

1. 用户示例中的 `Ctrl+K` 由远端 effective tmux option 得出，不解析 Roaminal Pod
   本地文件，也不下载 raw remote config。
2. tmux 原生默认是 `Ctrl+B`；正常探测结果始终优先。需求中的 `Ctrl+A` 仅作为
   无法取得 effective value 时的 Roaminal fallback。
3. `commit and push` 只输入 15 个 ASCII 字符（包含两个内部空格），不发送换行。
4. Codex mode 由用户手动选择或普通 connection 默认选择，不做不可靠的进程识别。
5. 第一行的现有 ROAMINAL monitor 保持原位和原样；独立第二行只属于 REMOTE，方便
   后续扩展远端指标而不耦合 local monitor。
6. 远端监控选择 agentless、existing-ControlMaster、fixed collector 方案。它能准确
   呈现目标进程可见的 cgroup/host scope，但不承诺 Kubernetes 对象级语义。
7. REMOTE 同时提供 uptime/load/disk，但三者使用独立事实 scope：PID namespace
   可见 PID 1 age、SYSTEM `/proc/loadavg`、ROOTFS `/` capacity。它们不继承 CPU/MEM
   的 cgroup scope，也不承诺 Pod/Kubernetes metadata 语义。
8. monitor 按 transport 而非 instance 采样；多个 derived connection 和浏览器共享
   snapshot，避免每 5 秒制造成倍远端 process。
