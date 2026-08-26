# Mornlea Agent 指南与文档分层实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 Mornlea 的根级巨型双份指南改造成 `AGENTS.md` 单一内容源、`CLAUDE.md` 薄导入、核心子系统局部指南和可导航的当前文档体系。

**Architecture:** 根 `AGENTS.md` 只保留全仓方向和稳定红线，局部 `AGENTS.md` 沿目录祖先链补充具体约束。根及三个主要模块的 `CLAUDE.md` 使用相同的 `@AGENTS.md` shim；OpenSpec、当前架构、进度和历史设计各自承担不同权威职责。

**Tech Stack:** Markdown、YAML、Go 1.26 `testing`、现有 `internal/archcheck`。

**Spec:** `docs/superpowers/specs/2026-08-26-agent-guidance-hierarchy-design.md`

## Global Constraints

- `AGENTS.md` 是 Agent 规则的唯一内容源；`CLAUDE.md` 不复制版本、能力或工作流正文。
- 当前契约保持协议 v26、玩家 schema v7、区块 schema v9、世界 metadata v2、`companions.ai` schema v4、engine ABI v7、client ABI v9、benchmark scenario v19。
- 不修改生产代码、Hook、gate、构建、发布、CI、协议、schema、ABI、scenario 或视觉 golden。
- 仅允许修改两个非文档文件：`internal/archcheck/baseline_test.go` 与 `internal/archcheck/identity_test.go`。
- 不批量修订 `docs/superpowers/`（本设计和计划除外）、归档 OpenSpec 或历史证据。
- 不增加指南 manifest、字节预算、祖先链预算、链接扫描器或新测试文件。
- 只运行计划明确列出的 focused archcheck；不运行 `make dev-check`、全仓 Go/Rust 测试、benchmark、capture 或图形客户端。
- 当前本地 `main` 与 `origin/main` 分叉；不得 reset、rebase、merge 或以其他方式改写本地 `main`。
- 不提交 Git commit，除非用户另行明确要求。

---

### Task 1: 根方向层与薄导入契约

**Files:**
- Modify: `internal/archcheck/baseline_test.go`
- Modify: `internal/archcheck/identity_test.go`
- Modify: `AGENTS.md`
- Modify: `CLAUDE.md`

**Interfaces:**
- Consumes: 现有 `baselineVersionMappings`、`assertBaselineVersions`、`readBaselineDoc` 和 `TestNativeEngineLibraryIdentity`。
- Produces: 根方向层；`baselineDocName = "AGENTS.md"`；`claudeAgentImport`；初始只校验根 shim 的 `TestClaudeImportsAgentGuidance`；原生引擎身份门禁以根 `AGENTS.md` 为指南来源。

- [ ] **Step 1: 将版本映射注释切换到 `AGENTS.md` 单一来源**

在 `internal/archcheck/baseline_test.go` 中把 `baselineVersionMapping`、字段、映射表和测试注释里的 `CLAUDE.md` 改为 `AGENTS.md`。保留全部八条 `docPattern` 和代码权威来源不变，避免借文档重组放宽版本门禁。

- [ ] **Step 2: 写出薄导入的失败断言**

删除 `baselineDocNames` 和 `TestBaselineDocsAreIdentical`，用以下形状替换：

```go
const baselineDocName = "AGENTS.md"

const claudeAgentImport = `# CLAUDE.md

本作用域的 Agent 指南位于 [AGENTS.md](AGENTS.md)，供 Claude Code、Codex、OpenCode 及其他编码代理共享。Claude Code 在下方导入该指南。

@AGENTS.md
`

var claudeImportDocs = []string{"CLAUDE.md"}

func TestBaselineVersionsMatchCode(t *testing.T) {
	assertBaselineVersions(t, moduleRoot(t), baselineDocName)
}

func TestClaudeImportsAgentGuidance(t *testing.T) {
	root := moduleRoot(t)
	for _, name := range claudeImportDocs {
		t.Run(name, func(t *testing.T) {
			if text := readBaselineDoc(t, root, name); text != claudeAgentImport {
				t.Errorf("%s 不是规范的 AGENTS.md 薄导入（want %d bytes, got %d bytes）：\n%s", name, len(claudeAgentImport), len(text), text)
			}
		})
	}
}
```

同时在 `internal/archcheck/identity_test.go` 的当前文档身份列表中只删除根 `CLAUDE.md`：它改为 import-only adapter，不再承载 `mornlea_engine` 正文。根 `AGENTS.md` 继续作为指南身份来源；crate、产物、Makefile、cgo、README、OpenSpec 配置、进度文档检查和对 `libmornlea_mesh` 的拒绝全部保留。

- [ ] **Step 3: 运行定点测试确认旧全文镜像失败**

Run:

```bash
go test ./internal/archcheck \
  -run 'TestNativeEngineLibraryIdentity|TestBaselineVersionsMatchCode|TestClaudeImportsAgentGuidance' \
  -count=1
```

Expected: `TestNativeEngineLibraryIdentity` 与 `TestBaselineVersionsMatchCode` PASS；`TestClaudeImportsAgentGuidance/CLAUDE.md` FAIL，诊断显示当前 `CLAUDE.md` 不是规范薄导入。

- [ ] **Step 4: 把根 `AGENTS.md` 改写为方向层**

根文件按以下固定结构编写：

1. `# Mornlea 项目指南`
2. `## 指南作用域`：说明祖先链叠加、最近指南补充父级、局部规则不得降低全局安全和正确性门禁。
3. `## 项目与契约`：用一段短文保留项目身份，明确 Rust `mornlea_engine` 与 Rust `mornlea_client`，并包含能被现有正则匹配的精确事实：

```text
当前基线已经包含协议 v26；玩家 schema v7、区块 schema v9、世界 metadata v2、独立 `companions.ai` schema v4、engine ABI v7、client ABI v9，benchmark scenario 为 v19。
```

4. `## 真相优先级`：代码与测试 → `openspec/specs/` → `docs/architecture.md` → `docs/notes/progress.md` → `docs/superpowers/`。
5. `## 仓库与局部指南`：路由到 `internal/AGENTS.md`、`engine/AGENTS.md`、`cmd/mornlea/AGENTS.md`、`docs/AGENTS.md`、`scripts/AGENTS.md` 和 `openspec/config.yaml`。
6. `## 开始工作前`：读 `openspec/config.yaml`；有 change 时依次读 proposal/delta specs/design/tasks；检查 `git status`；clean checkout 涉及 Rust 时先 `make rust`。
7. `## 全局架构边界`：服务端唯一权威、Memory/TCP 共路、Go 不导入 WebGPU、Go 仅经既定 ABI bridge、消息发送后不可变、热路径不阻塞、无未经授权资产。
8. `## 工程纪律`：聚焦修改、测试先行、中文注释和 GoDoc/Rust doc comment、标识符反引号、保护用户改动、禁止破坏性 Git。
9. `## 开发流程`：只摘要 OpenSpec 和 subagent-driven-development 硬门禁，详细链接 `docs/development-process.md`、`docs/openspec.md`、`docs/test-organization.md`。
10. `## 验证`：给出风险递增的短命令入口并链接 `docs/notes/test-quickstart.md`；保留性能数值 record-only、报告完整性/overflow/数据丢失硬失败和不启动前台窗口。
11. `## Hook`：准确说明两端共用 guard 实现，但 Codex 有 `Stop`，Claude 当前只有 `PreToolUse`/`PostToolUse`。

不得把原“项目定位”中的逐功能里程碑、伙伴状态机、农业公式、HUD 容量或 capture 场景清单搬入新根文件。

- [ ] **Step 5: 将根 `CLAUDE.md` 替换为规范 shim**

文件内容必须与 `claudeAgentImport` 常量逐字节相同。

- [ ] **Step 6: 运行定点测试确认根迁移通过**

Run:

```bash
go test ./internal/archcheck \
  -run 'TestNativeEngineLibraryIdentity|TestBaselineVersionsMatchCode|TestClaudeImportsAgentGuidance' \
  -count=1
```

Expected: 三项 PASS；八条版本映射都从根 `AGENTS.md` 成功解析，原生引擎身份检查也只把根 `AGENTS.md` 作为指南身份来源。

- [ ] **Step 7: 任务级自检**

Run:

```bash
git diff --check -- AGENTS.md CLAUDE.md internal/archcheck/baseline_test.go internal/archcheck/identity_test.go
```

Expected: exit 0，无输出。

---

### Task 2: 核心子系统局部指南

**Files:**
- Create: `internal/AGENTS.md`
- Create: `internal/CLAUDE.md`
- Create: `internal/client/AGENTS.md`
- Create: `internal/nativeabi/AGENTS.md`
- Create: `internal/network/AGENTS.md`
- Create: `internal/sim/AGENTS.md`
- Create: `internal/storage/AGENTS.md`
- Create: `engine/AGENTS.md`
- Create: `engine/CLAUDE.md`
- Create: `engine/crates/mornlea_client/AGENTS.md`
- Create: `engine/crates/mornlea_engine/AGENTS.md`
- Create: `cmd/mornlea/AGENTS.md`
- Create: `cmd/mornlea/CLAUDE.md`
- Modify: `internal/archcheck/baseline_test.go`

**Interfaces:**
- Consumes: Task 1 的祖先链说明和 `claudeAgentImport`。
- Produces: 三个主要模块 shim；Go、Rust、图形客户端装配的局部约束。

- [ ] **Step 1: 扩展 shim 断言并确认缺失文件失败**

把 `claudeImportDocs` 改为：

```go
var claudeImportDocs = []string{
	"CLAUDE.md",
	filepath.Join("internal", "CLAUDE.md"),
	filepath.Join("engine", "CLAUDE.md"),
	filepath.Join("cmd", "mornlea", "CLAUDE.md"),
}
```

Run:

```bash
go test ./internal/archcheck -run TestClaudeImportsAgentGuidance -count=1
```

Expected: 根子测试 PASS；另外三个子测试因文件不存在而 FAIL。

- [ ] **Step 2: 编写 `internal/AGENTS.md` 与 shim**

`internal/AGENTS.md` 包含：

- 本文件补充根规则，并列出五个下级局部指南；
- 依赖真相位于 `internal/archcheck/dependency_test.go`，不复制 `allowed` 表；
- `sim`、`world`、`network`、`storage`、`server`、`client` 的高层所有权；
- engine ABI 仅由 `internal/nativeabi` 接触，client ABI 由 `internal/client` 接触；
- 测试与代码同目录、按关注点组织，共享 helper 遵循 `docs/test-organization.md`；
- focused 命令示例 `go test ./internal/sim -race -count=1`，其他包替换为对应真实目录；依赖边界入口为 `go test ./internal/archcheck -count=1`。

`internal/CLAUDE.md` 与 `claudeAgentImport` 完全一致。

- [ ] **Step 3: 编写五个 Go 子系统指南**

每个文件直接从局部主题开始，不复制根级“开始工作前”、完整 OpenSpec 流程或版本矩阵：

- `internal/sim/AGENTS.md`：服务端唯一权威；`Engine.Step` 阶段组合；成功路径的状态变更与库存/耐久/方块原子结算；`recordChange`/批次广播汇聚；有界 tick 工作；不得依赖 client/render/network transport。
- `internal/network/AGENTS.md`：输入验证和长度上限；发送后不可变；Memory/TCP 共用 codec 与登录/模拟路径；协议升级同步 packet/registry/codec/fuzz/golden；LAN 无认证和加密的既有边界不被误写成公网安全。
- `internal/storage/AGENTS.md`：当前 schema 只在代码权威常量中维护；旧版本只读迁移；原子替换和关闭安全；容量上限、故障注入、重启和零数据丢失；不让存储包依赖 network。
- `internal/nativeabi/AGENTS.md`：engine C ABI 唯一 Go 入口；无生产 Go fallback；FFI 输入先校验；指针和 slice 生命周期只覆盖同步调用；固定容量和跨语言容量测试；ABI 变更同步 header/Rust/Go/version。
- `internal/client/AGENTS.md`：客户端只读镜像、预测可被权威快照纠正、不得复制服务端玩法规则；client ABI 桥接、窗口/GPU/音频资源生命周期；capture/benchmark 路径不得打开交互窗口或请求音频设备。

每个文件末尾只列本包 focused test 命令及最相关的当前文档入口。

- [ ] **Step 4: 编写 Rust workspace 与 crate 指南**

`engine/AGENTS.md` 包含：

- Rust 1.97.1 固定工具链；
- `mornlea_engine` 和 `mornlea_client` 职责及独立 ABI；
- FFI 不 unwind、指针/长度先校验、导出项中文 doc comment；
- Rust 测试按 `docs/test-organization.md` 的主题模块规则组织；
- `make rust`、`make rust-check` 和 crate focused Cargo 命令。

`engine/crates/mornlea_engine/AGENTS.md` 只记录 mesh/light、collision、raycast、physics、worldgen 的唯一生产实现，engine ABI 同步面，以及无窗口/无业务规则边界。

`engine/crates/mornlea_client/AGENTS.md` 只记录窗口、事件、GPU、egui、Darwin 音频的所有权，client ABI 同步面，GPU pass/固定容量/无头路径约束，以及专服不得依赖本 crate。

`engine/CLAUDE.md` 与 `claudeAgentImport` 完全一致。

- [ ] **Step 5: 编写图形客户端装配指南与 shim**

`cmd/mornlea/AGENTS.md` 包含：

- 这里只做应用装配，不复制服务端玩法规则；
- 普通本地游戏也必须经过 transport/login 边界；
- `-connect`、普通菜单启动、capture、benchmark 的入口差异；
- capture/benchmark 忽略用户材质覆盖，benchmark 固定工作负载，二者不请求音频设备且不启动交互窗口；
- golden 只在预期视觉变化经人工确认后更新；性能数值 record-only，但 overflow、数据丢失、报告身份仍失败；
- 场景顺序和容量引用代码/测试，不在指南复制数字；
- focused `go test ./cmd/mornlea -race -count=1`、相关无窗口视觉命令。

`cmd/mornlea/CLAUDE.md` 与 `claudeAgentImport` 完全一致。

- [ ] **Step 6: 运行唯一的局部指南定点测试**

Run:

```bash
go test ./internal/archcheck -run TestClaudeImportsAgentGuidance -count=1
```

Expected: 根及三个模块 shim 子测试全部 PASS。

- [ ] **Step 7: 任务级自检**

Run:

```bash
git diff --check -- internal engine cmd/mornlea internal/archcheck/baseline_test.go
```

Expected: exit 0，无输出。

---

### Task 3: 当前文档入口与基线同步

**Files:**
- Create: `docs/AGENTS.md`
- Create: `docs/README.md`
- Create: `docs/architecture.md`
- Create: `scripts/AGENTS.md`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `openspec/config.yaml`

**Interfaces:**
- Consumes: Task 1 的真相优先级和 Task 2 的局部职责。
- Produces: 当前文档地图、当前架构入口、文档与脚本局部规则、准确的用户/生成上下文版本摘要。

- [ ] **Step 1: 编写文档作用域指南**

`docs/AGENTS.md` 明确：

- 当前可观察行为由代码、测试和 `openspec/specs/` 决定；
- `docs/architecture.md` 描述当前架构，`docs/notes/progress.md` 是编年史；
- `docs/superpowers/` 和归档 change 是历史证据，不做批量现代化；
- 长期基线只陈述当前事实，不写单个 change 的非目标；
- 文档、测试说明和开发者文本优先中文，外部 API/标识符保留英文；
- 修改链接或命令时核对真实目标，不在多个入口复制正文。

- [ ] **Step 2: 编写 `docs/README.md` 文档地图**

用表格列出并解释以下入口：

- 用户入口：`README.md`、`README.en.md`；
- 当前架构：`docs/architecture.md`；
- 行为主规格：`openspec/specs/`；
- active change：`openspec/changes/`；
- OpenSpec 流程：`docs/openspec.md`；
- 开发流程：`docs/development-process.md`；
- 测试组织与快检：`docs/test-organization.md`、`docs/notes/test-quickstart.md`；
- 进度与 backlog：`docs/notes/progress.md`、`docs/feature-backlog.md`；
- Agent 自动化：`docs/agents/README.md`；
- 历史背景：`docs/superpowers/`、`openspec/changes/archive/`。

开头写明“索引只负责导航，不复述各文档内容”。

- [ ] **Step 3: 编写 `docs/architecture.md`**

固定章节为：

1. 系统总览；
2. 服务端权威与客户端镜像；
3. Memory/TCP 共用路径；
4. Go 包职责与 archcheck 依赖边界；
5. `mornlea_engine`/engine ABI v7；
6. `mornlea_client`/client ABI v9；
7. 图形客户端与无图形专服 release unit；
8. 并发、数据和热路径约束；
9. 行为规格、实现进度和历史设计入口。

不得写逐玩法清单、协议字段、capture 场景数量或 HUD 固定容量。

- [ ] **Step 4: 编写脚本作用域指南**

`scripts/AGENTS.md` 记录：

- Hook、gate、发布和 agent 自动化脚本改变的是仓库开发契约；
- 失败应修根因，不得删步骤、吞错误、放宽 overflow/数据丢失或使用无 spec 绕过变量；
- `.codex/hooks.json` 与 `.claude/settings.json` 共用 guard 实现但生命周期不同；
- `scripts/agents/gates.sh` 当前实际运行 gofmt/vet/archcheck/OpenSpec/`make rust`/可选 full race，不宣称包含 `make rust-check`；
- 修改脚本时使用对应 shell/Node focused check；本次只新增指南，不修改脚本。

- [ ] **Step 5: 修正 README 当前版本和架构入口**

在 `README.md` 和 `README.en.md` 中：

- 把所有当前式 `engine ABI v6` 改为 `engine ABI v7`；
- 把“整体架构与技术选型/Architecture & technology choices”主链接改到 `docs/architecture.md`；
- 将旧 `docs/superpowers/specs/2026-07-26-minecraft-go-design.md` 放到历史设计说明中，不再称为当前架构；
- 在文档导览加入 `docs/README.md`。

不得改动其他产品说明或版本号。

- [ ] **Step 6: 收敛 `openspec/config.yaml` 当前上下文**

保留 `schema`、`rules` 和 `operations` 结构。把 `context` 中的巨型能力编年史替换为以下信息：

- Mornlea 身份和 Go module；
- 当前八项版本矩阵；
- 服务端权威、Memory/TCP 共路；
- Go 持有 app/world/sim/network/storage/CPU render，Rust 两 crate 持有生产数值计算与 GPU/窗口；
- engine ABI 仅 `internal/nativeabi`，client ABI 仅 `internal/client`；
- archcheck、消息不可变、热路径、资源版权边界；
- 文档和 OpenSpec 权威入口。

把 archive guidance 中“同步更新 `CLAUDE.md` 项目定位”改为“必要时更新根 `AGENTS.md` 当前版本矩阵，并更新对应主规格或 `docs/notes/progress.md`”；`CLAUDE.md` 只保留 shim。

- [ ] **Step 7: 任务级自检**

Run:

```bash
git diff --check -- docs/AGENTS.md docs/README.md docs/architecture.md scripts/AGENTS.md README.md README.en.md openspec/config.yaml
```

Expected: exit 0，无输出。

---

### Task 4: 现行文档漂移清偿与最终定点验证

**Files:**
- Modify: `docs/development-process.md`
- Modify: `docs/feature-backlog.md`
- Modify: `docs/openspec.md`
- Modify: `docs/agents/README.md`
- Modify: `docs/agents/confirmation-channel.md`
- Modify: `docs/agents/implementer.md`
- Modify: `docs/agents/planner.md`
- Modify: `docs/agents/planner-prompt.md`
- Modify: `docs/notes/go-rust-division.md`
- Modify: `docs/notes/test-quickstart.md`

**Interfaces:**
- Consumes: `AGENTS.md` 单一来源、实际 Hook 配置、实际 `gates.sh` 步骤和当前 ABI bridge。
- Produces: 不再引用全文镜像、失效命令、旧本机路径或已删除包的当前工作文档。

- [ ] **Step 1: 更新当前流程中的 Agent 指南所有权**

在 `docs/development-process.md`、`docs/feature-backlog.md`、`docs/agents/implementer.md`、`docs/agents/planner.md` 和 `docs/agents/planner-prompt.md` 中：

- 把“AGENTS/CLAUDE 逐字节相同”改为“规则只更新对应作用域的 `AGENTS.md`，`CLAUDE.md` 保持薄导入”；
- 把 `TestBaselineDocsAreIdentical`/`cmp -s` 改为 `TestClaudeImportsAgentGuidance` focused 门禁；
- 并行所有权从“独占两份巨型基线”改为“独占根 `AGENTS.md` 版本矩阵和相关局部指南”；
- 历史记录和已归档计划不修改。

- [ ] **Step 2: 修正视觉命令说明**

在 `docs/development-process.md` 和 `docs/agents/implementer.md` 中把不存在的 `make visual` 改为明确流程：

```text
预期视觉不变时运行 `make visual-check`；预期视觉变化时先逐图确认，再运行 `make visual-update`，随后重新运行 `make visual-check`。
```

不得把 `visual-update` 描述成无需人工确认即可执行。

- [ ] **Step 3: 去除 Agent 自动化文档中的旧绝对路径**

在 `docs/agents/README.md`：

- 删除硬编码 `/Users/chen/chenwork/minecraft-go` 的 cron 和 plist 等价展开；
- 以 `scripts/agents/install-cron.sh planner` 和 `scripts/agents/install-launchd.sh` 为 canonical 安装入口；
- 说明安装脚本会解析当前仓库根并写入绝对路径，可用 `crontab -l` 或生成的 plist 检查结果。

在 `docs/agents/confirmation-channel.md`：

- 删除含旧仓库路径的手工 plist heredoc；
- 保留 `scripts/agents/confirm/install-listener.sh`；
- 说明脚本通过当前仓库根和 `which node` 生成 `ProgramArguments`、`WorkingDirectory` 与日志路径。

- [ ] **Step 4: 修正当前 Go/Rust 边界说明**

在 `docs/notes/go-rust-division.md` 第 4 节把：

```text
绕过 `internal/nativeabi` / window 绑定直接调 engine；绕过 `internal/gfx` 直接导入 WebGPU 绑定。
```

改为当前边界：

```text
绕过 `internal/nativeabi` 直接调用 engine ABI；绕过 `internal/client` 直接调用 client ABI 或导入 GPU 绑定。
```

同时删除“oracle 测试副本除外”的当前式表述，因为生产 Go fallback 已删除，历史 oracle 不再作为新实现许可。

- [ ] **Step 5: 让 Hook 文档匹配真实配置**

在 `docs/openspec.md` 和 `docs/agents/README.md` 中明确：

- Claude Code 与 Codex 共用 `scripts/agent-hooks/guard.mjs`；
- 两者都配置 `PreToolUse` 与 `PostToolUse`；
- 只有 `.codex/hooks.json` 当前配置 `Stop`；
- Claude Code 检查 Project 来源的两类 Hook，不声称三类；
- 本次不修改 Hook 配置。

- [ ] **Step 6: 让 gate 文档匹配真实脚本**

在 `docs/development-process.md` 和 `docs/notes/test-quickstart.md` 中明确：

- `scripts/agents/gates.sh` 当前执行 gofmt、vet、archcheck、OpenSpec、`make rust` 和未跳过时的 full race；
- 完整提交前 Rust 门禁 `make rust-check` 仍需单独运行；
- 不把脚本描述为已经包含 `make rust-check`；
- 本次不修改脚本。

- [ ] **Step 7: 搜索现行文档中的旧约定**

Run:

```bash
rg -n \
  'TestBaselineDocsAreIdentical|AGENTS\.md.*CLAUDE\.md.*逐字节相同|make visual`|chenwork/minecraft-go|绕过 `internal/gfx`' \
  AGENTS.md openspec/config.yaml docs/development-process.md docs/feature-backlog.md docs/openspec.md docs/agents docs/notes/go-rust-division.md docs/notes/test-quickstart.md
```

Expected: 无当前式匹配。若 `docs/agents/` 中某段明确引用历史事件，保留并在验证记录中说明；不得据此改写 `docs/superpowers/` 或归档 change。

- [ ] **Step 8: 运行最终 focused archcheck**

Run:

```bash
go test ./internal/archcheck \
  -run 'TestNativeEngineLibraryIdentity|TestBaselineVersionsMatchCode|TestClaudeImportsAgentGuidance' \
  -count=1
```

Expected: PASS。不得扩大到其他 Go/Rust 测试。

- [ ] **Step 9: 运行最终非测试检查**

Run:

```bash
git diff --check
git status --short
```

Expected: `git diff --check` exit 0；`git status --short` 只显示本计划列出的文档、四个 `CLAUDE.md` shim、`internal/archcheck/baseline_test.go` 和 `internal/archcheck/identity_test.go`，以及开始前已经存在且未被修改的用户内容。
