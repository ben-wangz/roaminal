# Roaminal ForgeKit 版本管理与 GitHub Actions 发布实施方案

> 状态：设计完成，待确认后实施。
>
> 本文是交给实施 agent 的连续执行方案。本轮只新增本文，不修改版本文件、
> GitHub Actions、ForgeKit 配置、skill、Chart、镜像，也不创建 tag 或发布制品。

## 1. 目标

Roaminal 只有一个主应用，但会发布两个紧密关联的制品：一个 runtime container
image 和一个 Helm Chart。本方案在现有 ForgeKit 基础上补齐统一版本注册、版本提升、
tag 驱动发布、远端验证和 agent release skill，形成以下闭环：

1. `version-control.yaml` 只登记唯一主应用 `roaminal`。
2. `roaminal-runtime` 不是第二个主应用，而是由 Chart annotation 声明的 linked
   container。
3. 所有受管理版本只通过 ForgeKit 读取、提升和同步，不手工改写。
4. `main` push 只验证；`roaminal-v<chart-semver>` tag 才触发正式发布。
5. GitHub Actions 分别以 `release-container` 和 `release-chart` 发布镜像与 Chart。
6. `.agents/skills/release-version/SKILL.md` 固化 Roaminal 专用 release 操作，负责
   版本判断、原子 commit、远端 lint gate、tag、workflow 监视和发布结果报告。

这里的“单一应用”不等于“所有制品共用一个版本号”。Chart package version 和
runtime version 具有不同含义，必须允许独立演进；一次对外发布则由唯一的 Roaminal
tag 将该提交上的两个版本绑定起来。

## 2. 调研结论

### 2.1 当前 Roaminal 基线

仓库已经具备部分能力：

- `setup/forgekit.sh` 固定 ForgeKit `v0.6.1`，并把校验后的二进制放到忽略的
  `build/bin/forgekit`；
- `lint.yaml` 和 `.github/workflows/lint.yaml` 已使用 ForgeKit 执行统一质量门禁；
- `container/VERSION` 是 runtime 版本；
- `chart/Chart.yaml` 已通过 `roaminal/images` annotation 关联
  `roaminal-runtime`，`appVersion` 和 `chart/values.yaml` 的 `image.tag` 已可由
  `forgekit version bump chart roaminal ... --sync` 同步；
- `release-chart` 已响应 `roaminal-v*` tag 并发布 Chart；
- `docs/releasing.md` 已描述部分手工流程。

当前缺口和不一致为：

- `version-control.yaml` 同时把 `roaminal` Chart 和它的 linked runtime 登记成两个
  顶层 app，不符合“一个主应用”的模型，也与参考仓库的 Chart ownership 规则重复；
- 缺少 `release-container`，因此正式 tag 只发布 Chart，不保证默认引用的 runtime
  image 已被发布；
- `release-chart` 内联解析 JSON 和校验 tag，没有可测试、可复用的 release plan；
- Chart 默认 image 已指向 `ghcr.io/ben-wangz/roaminal`，但尚无 container workflow
  实际向该契约地址发布；
- 没有仓库专用 `release-version` skill，agent 容易漏掉 bump、sync、远端 lint、tag
  和 workflow 验证中的某一步；
- 当前发布文档仍把 image push 视为 release owner 的额外手工动作，与目标自动发布
  流程冲突。

### 2.2 从 `/root/code/github/k8s-at-home` 采用的做法

- `version-control.yaml` 是唯一 app registry；
- Chart 是主 app，container 由 `Chart.yaml` annotation 关联和解析；
- 正式发布采用 `<app-name>-v<semver>` tag；
- `main` push 和 pull request 运行 `lint`，tag 才运行 release；
- workflow 只负责 orchestration，版本解析和制品构建交给 ForgeKit；
- workflow 名称使用 `lint`、`release-container`、`release-chart`；
- container build context 使用仓库根目录；
- release skill 使用 `release-version` 名称，并在正文固定适配的 ForgeKit 版本；
- tag 前等待该 release commit 在远端 `lint` 成功，tag 后监控所有 release runs。

### 2.3 针对 Roaminal 的调整

- 不引入参考仓库面向任意 app 的通用矩阵，也不引入 `release-binary`；
- release planner 固定只接受 `roaminal` 和唯一 linked target
  `roaminal-runtime`，遇到其他 app、重复 target 或路径漂移立即失败；
- container package 使用单应用名称 `ghcr.io/<owner>/roaminal`，不生成冗余的
  `roaminal-roaminal-runtime` package；`roaminal-runtime` 只作为 ForgeKit module
  name；
- Chart-only release 不重新构建并覆盖相同 runtime SemVer；planner 会与前一个
  Roaminal release tag 比较 `container/VERSION`，只在首次发布或版本变化时发布镜像；
- `release-chart` 在发布前确认精确 runtime image tag 已可从 GHCR 读取。runtime
  发生变化时它会等待并行的 `release-container`，避免先发布一个引用不存在镜像的
  Chart；
- 保留 Roaminal 当前“本地 ForgeKit gate + 远端 lint gate”双重要求。远端 gate 是
  tag 的最终依据，本地 gate 用于在 push 前尽早发现问题；
- skill 使用系统 `gh` CLI 并做认证预检，不照搬参考仓库未在 Roaminal 中提供的
  `build/bin/gh` 路径。

### 2.4 明确不采用

- 不使用手工 `workflow_dispatch` 发布；
- 不根据目录名自动发现 app；
- 不建立多应用 `*-v*` 路由或 container/binary matrix；
- 不发布独立 frontend、terminal-worker、backend 或 plugin 版本；
- 不让 npm `package.json` 的私有 `0.0.0` 参与 release；
- 不在本阶段增加 GitHub Release 页面、二进制附件、签名、SBOM、provenance、
  multi-architecture image 或自动部署；
- 不把 develop namespace rollout 混入正式 release workflow。

## 3. 版本模型

### 3.1 唯一应用注册

目标 `version-control.yaml` 只保留：

```yaml
apps:
  - name: roaminal
    type: chart
    path: chart
```

`roaminal-runtime` 继续由 `chart/Chart.yaml` 声明：

```yaml
annotations:
  roaminal/images: |
    - name: roaminal-runtime
      path: container
      valuesKey: image.tag
```

ForgeKit 必须能从主应用 metadata 解析 Chart 和 linked runtime，并能按 linked
container name 执行版本提升：

```sh
forgekit version get roaminal --output json
forgekit version bump container roaminal-runtime <part>
```

ForgeKit `v0.6.1` 的 `version get <name>` 只直接查询顶层 registry app，因此删除重复
登记后，`version get roaminal-runtime` 返回 not found 是预期行为。所有只读检查必须
从 `version get roaminal` 的 `linked` 数组获取 runtime metadata；不得为了保留这个
直接查询命令而重新登记第二个 app。

不得把 `roaminal-runtime` 再次放回顶层 `apps:`。将来出现真正独立发布的应用时，
应另行设计，不能借此方案扩展成隐式多应用 registry。

### 3.2 各版本字段职责

| 字段 | 含义 | ForgeKit 操作 |
| --- | --- | --- |
| `container/VERSION` | Roaminal runtime image SemVer，也是 `/api/version` 和 OCI image version | `version bump container roaminal-runtime <part>` |
| `chart/Chart.yaml.version` | Helm package SemVer，也是 release tag 中的版本 | `version bump chart roaminal <part> --sync` |
| `chart/Chart.yaml.appVersion` | 默认部署的 runtime 版本展示值 | 由 `--sync` 从 linked container 同步 |
| `chart/values.yaml.image.tag` | Chart 默认引用的 runtime image tag | 由 `--sync` 从 linked container 同步 |
| frontend/worker `package.json.version` | 私有模块占位值 `0.0.0` | 永不参与 release |

约束：

- `appVersion`、默认 `image.tag` 和 `container/VERSION` 必须完全相同；
- Chart version 与 runtime version 可以不同，例如 Chart `0.1.1` 引用 runtime
  `0.2.3`；
- 正式 tag 中的版本始终等于 Chart version，而不是 runtime version；
- 一次 runtime release 必须同时提升 Chart version 并执行 `--sync`，否则不能打 tag；
- Chart-only release 只提升 Chart version，但仍执行 `--sync` 以验证关联字段；
- 文档或测试用例变化若不需要发布制品，不提升任何版本；
- 版本只允许 ForgeKit 支持的 `major`、`minor`、`patch` 变更，不用字符串替换或手工
  编辑补救失败的 bump。

### 3.3 变更到 bump 的映射

| 变更类型 | runtime bump | Chart bump | 结果 |
| --- | --- | --- | --- |
| backend/frontend/worker/container/runtime 依赖 | 必须 | 必须，带 `--sync` | 新 image + 新 Chart |
| Chart template/values/schema/Chart 文档 | 不需要 | 必须，带 `--sync` | 只发布新 Chart，复用旧 image |
| 同时修改 runtime 与 Chart | 必须 | 必须，带 `--sync` | 新 image + 新 Chart |
| 纯 docs/tests/tooling 且不改变发布物 | 不需要 | 不需要 | 无 release tag |

默认使用 `patch`。只有用户明确要求不兼容变化或新功能版本策略时才使用 `major` 或
`minor`；skill 不从 commit message 猜测 major/minor。

## 4. 命名与发布地址

命名遵循参考仓库风格，并针对单应用缩短 package 名：

- app：`roaminal`；
- linked container module：`roaminal-runtime`；
- release tag：`roaminal-v<chart-semver>`，例如 `roaminal-v0.1.1`；
- GitHub Actions workflow name：`lint`、`release-container`、`release-chart`；
- container image：`ghcr.io/<owner>/roaminal:<runtime-semver>`；
- Helm OCI repository：`oci://ghcr.io/<owner>/roaminal-charts`；
- Helm artifact：`oci://ghcr.io/<owner>/roaminal-charts/roaminal:<chart-semver>`；
- release commit：`chore(release): bump Roaminal release versions`；
- skill 目录：`.agents/skills/release-version/`，front matter name 为
  `release-version`。

tag parser 接受稳定 `X.Y.Z` 和合法 prerelease `X.Y.Z-...`，拒绝 app 名漂移、缺少
`v`、非 SemVer 和包含 build metadata `+...` 的 tag。ForgeKit 的 OCI multi-tag
策略必须原样记录：当前 `0.x` 和 prerelease 只发布完整版本 tag；到稳定 `1.x` 后才
同时发布 `latest`、major、minor 和完整版本 tag。

Chart 当前默认 image reference 已是 `ghcr.io/ben-wangz/roaminal`。实施必须保持并
自动验证它与 workflow 的 `IMAGE_NAME` 一致；用户通过 values 覆盖私有 mirror 的
能力保持不变。

## 5. Release planner

新增 `.github/scripts/release-plan.sh`，名称沿用参考项目，但逻辑固定服务 Roaminal。
脚本是 workflow 和本地测试共享的唯一 release 元数据校验入口。

输入：

- 位置参数：`chart` 或 `container`；
- `TAG_NAME`：Git tag；
- `FORGEKIT_BIN`：已 bootstrap 的 ForgeKit；
- `GITHUB_OUTPUT`：Actions output 文件；
- Git checkout 必须包含完整 tag 历史。

校验：

1. tag 严格符合 `roaminal-v<semver>`；
2. `forgekit version get roaminal --output json` 成功；
3. app name/type/path 分别为 `roaminal`、`chart`、`chart`；
4. tag version 等于 Chart version；
5. linked targets 中恰好有一个 `roaminal-runtime` container，路径为 `container`；
6. linked runtime version、Chart `appVersion`、`values.yaml image.tag` 一致；
7. tag commit 是 `origin/main` 的祖先，拒绝从未合并分支发布；
8. 当前 checkout commit 与 tag ref 指向同一 commit；远端 tag 不存在的检查由 push
   前的 release skill 执行，planner 不访问 registry 或改写 tag；
9. 对 container 计划，比较前一个可达 `roaminal-v*` tag 中的
   `container/VERSION`；首次 release 或版本变化时 `should_run=true`，否则为 false；
10. runtime version 不得相对前一个 release 倒退。

输出至少包括：

- `should_run`；
- `app_name=roaminal`；
- `tag_version`；
- `chart_dir=chart`；
- `runtime_name=roaminal-runtime`；
- `runtime_dir=container`；
- `runtime_version`；
- `commit_sha`；
- `previous_tag`（首次发布为空）。

脚本不得发布、登录 registry、修改文件或创建 tag。JSON 使用结构化解析，不能依赖
`grep`/`sed` 抽取 ForgeKit 输出。错误必须指出实际值和预期值，但不得输出 token。

## 6. GitHub Actions 设计

### 6.1 `lint`

保留 `.github/workflows/lint.yaml` 的触发规则：

- `pull_request`；
- push 到 `main`。

继续通过 `setup/forgekit.sh` 获取固定版本，并运行根 `lint.yaml`。实施时增加 release
planner tests 和 workflow 静态校验到 ForgeKit gate；不能另建一套与本地不同的 CI
测试清单。

权限使用只读默认值：

```yaml
permissions:
  contents: read
```

lint 不登录 GHCR、不创建 tag、不上传正式制品。

### 6.2 `release-container`

新增 `.github/workflows/release-container.yaml`：

- workflow name 为 `release-container`；
- 只响应 `roaminal-v*` tag push；
- checkout 使用 `fetch-depth: 0`；
- plan job 调用 `release-plan.sh container`；
- `should_run=false` 时清晰显示为 skipped，不重推旧 runtime SemVer；
- publish job 使用 `packages: write`、`contents: read` 和 tag 级 concurrency；
- 安装 Podman，使用 `setup/forgekit.sh`，不维护第二套 build shell；
- `CONTAINER_REGISTRY=ghcr.io`；
- `IMAGE_NAME=${GITHUB_REPOSITORY}`，结果为
  `ghcr.io/ben-wangz/roaminal:<runtime-version>`；
- module 固定使用 planner 返回的 `roaminal-runtime`；
- container dir 固定由 planner 验证为 `container`；
- build context 为仓库根目录；
- 设置 `BUILD_ARG_ROAMINAL_VERSION=<runtime-version>`，确保二进制
  `/api/version`、Containerfile label 和 image tag 一致；
- 使用 `--semver --multi-tag`；
- 追加 OCI `version`、`revision`、`source`、`licenses` labels，其中 revision 使用
  checkout 后的 commit SHA，不使用可能指向 annotated tag object 的不明确值；
- 通过 `GITHUB_TOKEN` 登录，不把密码拼入参数或日志。

workflow 成功后必须在 job summary 写出精确 image reference；不得自动部署 develop
或生产 namespace。

### 6.3 `release-chart`

重构现有 `.github/workflows/release-chart.yaml`：

- workflow name 保持 `release-chart`；
- 只响应 `roaminal-v*` tag push；
- checkout 使用 `fetch-depth: 0`；
- plan job 调用 `release-plan.sh chart`，移除当前内联 Node JSON 解析；
- publish job 使用 `packages: write`、`contents: read` 和 tag 级 concurrency；
- 发布前执行 `helm lint chart --strict` 和关键 values render；
- 发布前轮询 GHCR 中
  `ghcr.io/<owner>/roaminal:<runtime-version>`，只有精确 tag 可读取才继续；
- 轮询必须有明确总超时和短间隔。超时后失败，不得发布引用缺失 image 的 Chart；
- 使用 `CHART_REGISTRY=ghcr.io/${GITHUB_REPOSITORY}-charts`；
- 使用 ForgeKit `publish chart build --semver --multi-tag`；
- job summary 写出精确 OCI Chart reference 和默认 runtime image reference。

Chart-only release 的 runtime tag 应已由上一次 release 发布，检查会立即通过。runtime
有变化时，检查用于等待并行 `release-container`。若 container workflow 失败，Chart
workflow 最终也必须失败，从而不产生已知不可安装的 Chart version。

### 6.4 Actions 通用约束

- 不使用 `pull_request_target`，来自 PR 的代码不能接触 package write token；
- 为每个 publish job 设置合理 `timeout-minutes`；
- concurrency key 包含 workflow 和完整 tag，禁止同一制品同一 tag 并发发布，但不
  取消已经开始的 publish；
- workflow 中不得手工计算或改写版本；
- 不缓存 registry credential；
- 正式 SemVer tag 已发布后视为不可移动，不删除再创建，不覆盖成另一提交；
- 瞬时网络失败优先 rerun 原 workflow；需要代码修复时提升新版本并创建新 tag；
- package visibility、retention 和 production deployment 不由 release workflow
  隐式改变；
- Actions major version与 ForgeKit pin 变更必须作为可审查的 tooling commit。

## 7. `release-version` skill 设计

新增 `.agents/skills/release-version/`。结构和命名参考 k8s-at-home，但内容只允许
操作 Roaminal，不保留 `<app-name>`、binary、matrix 或其他应用示例。按当前
`skill-creator` 规范使用其 `init_skill.py` 初始化到仓库 `.agents/skills`，不手工拼出
一个缺少校验的目录。

目录只包含必要内容：

```text
.agents/skills/release-version/
├── SKILL.md
└── agents/
    └── openai.yaml
```

release planner 已属于仓库 CI/CD 实现，skill 直接调用它，不在 skill 内复制 scripts、
references、README 或 quick reference。

front matter：

```yaml
---
name: release-version
description: |
  Release Roaminal with ForgeKit by bumping the linked runtime and Helm Chart,
  validating the release commit, pushing roaminal-v<semver>, and monitoring
  the GitHub Actions release workflows. Use when preparing a runtime or Chart
  version bump, release commit, Roaminal tag, release verification, or release
  failure diagnosis.
---
```

Front matter 只允许 `name` 和 `description`。适配版本在正文开头写为强约束
`This skill targets ForgeKit v0.6.1`，以符合当前 skill schema。

`agents/openai.yaml` 只配置当前需要的 interface 字段：

```yaml
interface:
  display_name: "Release Roaminal"
  short_description: "Version and publish Roaminal releases"
  default_prompt: "Use $release-version to prepare and publish a Roaminal release with ForgeKit."
```

不虚构 icon、brand color 或 MCP dependency。生成后使用 `skill-creator` 的
`quick_validate.py` 校验目录、front matter 和命名。

skill 必须覆盖以下流程：

1. 找到 repository root，检查 `git status`、当前 branch、remote 和 `gh auth status`；
2. 使用 `setup/forgekit.sh`，确认实际版本等于 skill 正文声明的 `v0.6.1`；
3. 用 `forgekit version get roaminal --output json` 的 `linked` 数组确认 Chart 与
   runtime；不得调用 `version get roaminal-runtime`；
4. 检查自上一个 `roaminal-v*` tag 以来的变更，按第 3.3 节决定 runtime/Chart bump；
5. 只调用 `forgekit version bump`，随后再次 `version get` 和 review 精确 diff；
6. 运行仓库要求的本地 ForgeKit lint；
7. 创建一个只包含版本变化的原子 commit，默认消息为
   `chore(release): bump Roaminal release versions`；
8. push branch，定位与该 commit SHA 完全匹配的远端 `lint` run 并等待成功；
9. 确认 commit 位于 `main`、工作树干净、远端 main 未漂移且 tag 不存在；
10. 创建 annotated `roaminal-v<chart-version>` tag 并只 push 该 tag；
11. 找到该 tag 的 `release-container` 与 `release-chart` runs，逐个等待终态；
12. 读取 planner/job summary 或 ForgeKit metadata，报告 tag、commit、Chart version、
    runtime version、image、Chart OCI 地址和 run URLs。

核心规则：

- 没有明确 release 请求时不得 bump、commit、push 或 tag；
- 用户未指定 version part 时默认 `patch`，但检测到可能的 breaking change 时先停止
  并说明，不能自行选择 major；
- 不手工修改 `container/VERSION`、Chart `version`、`appVersion` 或 `image.tag`；
- runtime 变化时必须先 bump runtime，再 bump Chart `--sync`；
- Chart-only 变化只 bump Chart `--sync`；
- 不修改 frontend/worker `package.json` version；
- tag 前远端 `lint` 必须成功，不能用本地成功代替；
- 不把 feature commit 与 release bump 混成一个 commit；
- 不对已经存在的 tag 强制 push；
- workflow failure 必须报告具体 run URL 和失败 job，不把“已 push tag”误报为发布成功；
- 当 `setup/forgekit.sh` pin 变化时，必须同步 skill 正文的 ForgeKit version contract
  并复核命令行为。

skill 正文使用简洁、命令导向的英文，与参考项目保持一致；面向用户的结果按当前
会话语言报告。skill 中所有命令从任意仓库子目录执行都必须可靠解析 project root。

## 8. 文档与仓库规则调整

实施时同步修改：

- `AGENTS.md`：明确唯一主 app、linked runtime、ForgeKit-only mutation、local/remote
  gates、tag 命名和 npm version 排除规则；
- `README.md`：保留简短的版本入口，链接完整 release 文档；
- `docs/releasing.md`：改为与自动 container + Chart 发布事实一致，说明两套版本、
  bump 表、tag、制品地址、multi-tag 行为、失败恢复和 GitHub CLI 前置条件；
- `chart/README.md`：记录默认 GHCR image 和 OCI Chart 安装方式；
- 必要时更新 `docs/deployment.md` 的正式 image 示例，但不把发布与集群 rollout
  混为同一流程；
- `.gitignore`：继续忽略下载的 ForgeKit binary 和 release 临时输出，不忽略 skill、
  workflow 或 planner tests。

文档不得声称 Roaminal 已支持尚未实现的 multi-arch、签名、SBOM 或自动部署。

## 9. 测试方案

### 9.1 Version registry 与 ForgeKit

- `forgekit version get roaminal` 只显示一个顶层 Chart app 和一个 linked runtime；
- 在临时 fixture 中验证 `version get roaminal-runtime` 不可直接查询，但
  `version bump container roaminal-runtime patch` 能定位 `container/VERSION`；
- 在临时干净 Git fixture 中验证 container patch bump 只改 runtime version；
- 验证 Chart `--sync` 同时更新 Chart version、`appVersion` 和 `image.tag`；
- 验证 Chart-only bump 不改 `container/VERSION`；
- 验证 root ForgeKit lint 完整通过。

版本 mutation 测试必须在临时 fixture 中进行，不能为了测试改动真实工作树版本。

### 9.2 Planner tests

为 `.github/scripts/release-plan.sh` 增加自动化测试，至少覆盖：

- 首次合法 release；
- runtime 变化和未变化；
- 稳定版和 prerelease tag；
- 错误 app 名、非法 SemVer、build metadata、tag/Chart version 不一致；
- app type/path 漂移；
- linked target 缺失、重复、名称或路径错误；
- `appVersion`/runtime/image tag 不同步；
- tag commit 不在 main；
- runtime version 倒退；
- 缺少环境变量、ForgeKit JSON 错误和 `GITHUB_OUTPUT` 写入。

测试使用临时 Git repository 和 fixture，不访问真实 GHCR，不创建真实 tag。

### 9.3 Publish dry-run

在干净临时 checkout 中执行：

- container `publish ... --dry-run --no-push`，确认 module、context、version、build arg、
  image name 和 labels；
- Chart `publish ... --dry-run --no-push`，确认 chart path 和 package version；
- `helm lint --strict` 与默认/关键 values render；
- workflow YAML 静态检查和 shell syntax。

正式 `--semver` dry-run 也必须使用干净 checkout，因为 ForgeKit 会拒绝 dirty tree。

### 9.4 GitHub Actions 验收

真正发布前先在非发布 commit 上完成所有本地验证。首个真实 tag 验收必须确认：

- tag 对应已通过远端 `lint` 的 main commit；
- `release-container` 和 `release-chart` 都由同一 tag 触发；
- image 中 `/api/version` 与 runtime SemVer 一致；
- GHCR 存在精确 runtime tag；
- OCI Chart 存在精确 Chart tag；
- Chart 默认 image reference 能 pull；
- 用 OCI Chart 执行 `helm template` 和隔离 namespace 安装后 rollout healthy；
- Chart-only 测试 release 会 skip container publish，并继续引用已存在 image；
- Actions log 和 job summary 不泄漏 token，也没有被忽略的 warning/error。

未经用户明确授权，实施阶段不得仅为测试而创建正式 release tag。若没有真实发布
授权，则完成到本地/CI dry-run，并把“首次真实 tag 验收”明确保留为待执行项，不能
虚报通过。

## 10. 连续实施阶段与原子提交

获得实施授权后，agent 按以下顺序连续执行；除第 12 节停止条件外不等待人工确认。

### Phase 0：基线与防冲突

- 记录 dirty worktree，并保护已有 Chart、docs 和 tests 变更；
- 读取当前 ForgeKit version metadata、Chart/runtime versions、Actions 和 remote tags；
- 运行现有 ForgeKit lint，区分基线失败与本次回归；
- 确认 GHCR package naming 和当前 GitHub repository owner；
- 不修改、清理或提交与本方案无关的既有变更。

### Phase 1：收敛单应用版本模型

建议提交：`refactor(release): model Roaminal as one chart application`

- 从 registry 删除重复的顶层 runtime entry；
- 保留并校验 Chart linked runtime annotation；
- 验证默认 GHCR image repository 与发布 package name 一致；
- 增加 registry、sync 和 dry-run tests；
- 更新 `AGENTS.md` 中的版本 ownership 规则。

### Phase 2：Release planner

建议提交：`feat(release): add Roaminal release planner`

- 实现结构化 tag/ForgeKit metadata 校验；
- 实现前一 release 和 runtime changed 判断；
- 实现 main ancestry、版本单调性和 outputs；
- 增加完整 planner fixture tests，并接入 `lint.yaml`。

### Phase 3：GitHub Actions 发布链路

建议提交：`feat(ci): publish Roaminal release artifacts`

- 新增 `release-container`；
- 重构 `release-chart` 使用 planner；
- 增加 image availability gate、least privilege、timeout、concurrency、summary；
- 对 workflow、ForgeKit dry-run、Helm render 进行静态和本地验收。

### Phase 4：Release skill 与文档

建议提交：`docs(release): add Roaminal release-version skill`

- 新增 `.agents/skills/release-version/SKILL.md`；
- 生成并核对 `.agents/skills/release-version/agents/openai.yaml`；
- 运行 `skill-creator` 的 `quick_validate.py`；
- 更新 README、release、Chart 和必要 deployment 文档；
- 检查 skill 的命令、版本 pin、workflow name 和 artifact address 与实现一致。

### Phase 5：完整验证

- 运行 root ForgeKit lint；
- 运行 planner tests、backend/frontend/worker tests、Helm lint/render；
- 在干净临时 checkout 执行 container/Chart ForgeKit publish dry-run；
- 检查 `git diff --check`、YAML、shell syntax 和文档链接；
- 若用户另行授权真实 release，按 skill 创建版本 commit、等待远端 lint、创建 tag、
  监控两个 workflows 并完成 OCI pull/install 验收；否则不创建 tag。

## 11. Definition of Done

以下条件全部满足才可宣告方案实施完成：

- `version-control.yaml` 只有一个顶层 `roaminal` app；
- ForgeKit 可从 `version get roaminal` 的 linked metadata 解析唯一
  `roaminal-runtime`，并能按该名称 bump/publish container；
- runtime、Chart、appVersion 和 image tag ownership 清晰且只由 ForgeKit mutation；
- runtime/Chart/组合/无发布四类变更有确定 bump 规则；
- release tag 只使用 `roaminal-v<chart-semver>`，非法 tag 和非-main commit 被拒绝；
- `release-container` 发布 `ghcr.io/<owner>/roaminal:<runtime-semver>`；
- Chart-only release 不覆盖未变化的 runtime SemVer；
- `release-chart` 只在精确 runtime image 可读取后发布；
- OCI Chart 发布到 `ghcr.io/<owner>/roaminal-charts/roaminal:<chart-semver>`；
- build arg、二进制 `/api/version`、OCI label、runtime tag、Chart appVersion 和默认
  image tag 一致；
- Actions 使用 least privilege、tag concurrency、timeout 和无 secret 日志；
- `lint`、`release-container`、`release-chart` 名称与文档、skill、监控命令一致；
- `.agents/skills/release-version/SKILL.md` 是 Roaminal 专用，不含多应用或 binary
  死分支，正文 ForgeKit version contract 与 bootstrap pin 一致；
- skill front matter 只有 `name`/`description`，`agents/openai.yaml` 与 SKILL.md 一致，
  并通过 `quick_validate.py`；
- skill 强制本地 gate、远端 commit 对应的 lint gate、annotated tag 和两个 release
  workflow 终态验证；
- planner 正反测试、ForgeKit publish dry-run、Helm lint/render 和 root ForgeKit lint
  全部通过；
- README、release、Chart/deployment 文档与自动发布事实一致；
- 未经授权没有创建 tag、推送 package、部署集群或改写正式 release；
- 变更形成范围清晰的原子 commits，保留进入实施前已有的用户工作树变更。

## 12. 停止条件

只有以下情况允许实施 agent 中断并请求决策：

1. ForgeKit `v0.6.1` 在不重复注册 runtime 的情况下无法从
   `version get roaminal` 解析 linked metadata，或无法按 `roaminal-runtime` 执行
   container bump/publish，且参考仓库同版本行为无法复现；
2. GitHub repository/package 权限不允许 `GITHUB_TOKEN` 向预定 GHCR 地址发布，连续
   三次独立诊断后仍需新增 PAT、外部 registry 或组织级设置；
3. 用户已有未提交修改与本方案必须修改的 release/version 文件直接冲突，无法保留
   双方语义；
4. 现有公开制品或 tag 已使用与本文冲突的不可变命名，迁移会破坏真实用户拉取；
5. GitHub Actions 无法在不引入长期 credential 的情况下验证 runtime image 已存在；
6. 实施过程中发现 ForgeKit 实际 multi-tag、linked target 或 semver 语义与本文已
   验证的 `v0.6.1` 行为不一致。

普通脚本错误、workflow YAML 问题、测试失败、首次发布尚未授权、GHCR 短暂网络错误
或需要补充 fixture 都不是停止条件；agent 应自行修复并继续完成可验证范围。

## 13. 最终决策摘要

本方案已经固定以下设计，不再留给实施 agent 自行选择：

1. 一个主应用：`roaminal` Chart；一个 linked container：`roaminal-runtime`。
2. Chart version 与 runtime version 独立，tag 使用 Chart version。
3. tag 驱动，`main` push 不发布，release 不使用 manual dispatch。
4. GitHub Actions 是 CI/CD 核心，workflow 名沿用参考项目。
5. 单应用 image 名为 `ghcr.io/<owner>/roaminal`，不追加 runtime module 后缀。
6. Chart-only release 跳过重复 container publish；Chart 发布前必须确认 image 存在。
7. skill 名为 `release-version`，路径为 `.agents/skills/release-version/SKILL.md`。
8. 不做 binary、plugin、GitHub Release、签名、SBOM、multi-arch 和自动部署。
