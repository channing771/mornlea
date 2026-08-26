# Basic Gameplay Backlog Replan Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把任务池改成以首夜生存和自给家园为主线的串行发布列车，并让自动领取、Discussion 镜像和本地状态看板只把 `就绪` 视为可领取状态。

**Architecture:** `docs/feature-backlog.md` 继续是唯一规划真相源；状态值直接表达可领取性，不引入依赖求解器。`refresh-discussion.py` 和 `relay.sh` 消费同一状态契约，planner 负责把串行队首晋升为 `就绪`，implementer 只领取 `就绪`；agent board 只展示这些状态，不参与调度。

**Tech Stack:** Markdown、Python 3 标准库、Bash、Go 1.26、React 19 / TypeScript 5 / Vitest。

**Spec:** `docs/superpowers/specs/2026-08-26-basic-gameplay-backlog-replan-design.md`

## Global Constraints

- 不修改任何游戏行为、协议、存档、ABI、benchmark、capture 场景或 golden。
- 不删除、清理、rebase 或改写 A-02、E-12 及其它现有 worktree；A-02 和 E-12 的未提交文件必须原样保留。
- `docs/feature-backlog.md` 是唯一规划来源；Discussion #71 只由脚本生成镜像。
- `就绪` 是唯一可领取状态；`排队` 和 `设计候选` 不得被自动实现者领取。
- 核心玩法版本化任务串行推进；每一行未来自行完成版本、golden、基线、归档和 PR。
- 不新增第三方依赖，不创建自动依赖求解器，不写排期日期或工时估算。
- 历史归档、旧设计和 Discussion 历史评论不批量现代化。
- 代码与脚本注释使用中文；本计划涉及的 Go 文件必须经过 `gofmt`。

## Execution Protocol

- 控制会话开始 Task 1 前创建 `docs/superpowers/plans/2026-08-26-basic-gameplay-backlog-replan-ledger.md`，记录每个任务的 implementer、规格评审、质量评审、修复轮次和裁决。
- Task 1..4 各派一个 fresh implementer；任务 brief 是该子代理唯一需求来源，implementer 不得派生其它代理。
- 每个任务实现后依次派独立规格评审与质量评审；发现问题回到同一 implementer 修复，单任务最多 5 轮。
- Task 1..4 全部通过后派 fresh final reviewer 审查 `origin/main...HEAD`；控制会话只协调、验证和维护 ledger，不直接实现任务文件。
- Ledger 的最终评审结论作为单独文档提交；这是流程记录，不改变规划状态或产品行为。

## File Map

- `docs/feature-backlog.md`：当前状态模型、首夜/家园串行队列和全部规划行状态。
- `scripts/agents/refresh-discussion.py`：解析 backlog、严格验证状态并生成 Discussion 正文。
- `scripts/agents/refresh_discussion_test.py`：Discussion 生成器的标准库回归测试。
- `scripts/agents/relay.sh`：没有 `就绪` 行时终止接力。
- `docs/development-process.md`：仓库开发流程中的领取语义。
- `docs/agents/{README.md,planner.md,planner-prompt.md,implementer.md,setup-new-machine.md}`：常驻工作者对新状态的唯一解释。
- `cmd/mornlea-agent-board/parse.go`、`parse_backlog_test.go`：看板后端状态枚举与解析测试。
- `web/agent-board/src/lib/{fmt.ts,fmt.test.ts}`：前端状态类型和徽章样式。
- `web/agent-board/src/components/{TaskBoard.tsx,StatCards.tsx}`：状态分组顺序和 `就绪` 统计。
- `docs/notes/agent-dashboard.md`：看板当前状态契约说明。
- `docs/superpowers/plans/2026-08-26-basic-gameplay-backlog-replan-ledger.md`：SDD 实现、评审和裁决记录。

---

### Task 1: 迁移规划真相源与基础玩法队列

**Files:**
- Modify: `docs/feature-backlog.md`

**Interfaces:**
- Consumes: 已批准设计中的八个状态和固定首夜/家园顺序。
- Produces: 后续脚本消费的状态集合 `就绪|已认领|开发中|待集成|排队|设计候选|已完成|已取消`，以及新增行 `B-33..B-37`。

- [ ] **Step 1: 记录迁移前任务集合**

Run:

```bash
git show HEAD:docs/feature-backlog.md > /tmp/mornlea-backlog-before.md
```

Expected: `/tmp/mornlea-backlog-before.md` 存在，供 Task 5 核对既有 ID 没有静默丢失。

- [ ] **Step 2: 改写状态图例和领取规则**

把状态表替换为：

```markdown
| 状态 | 含义 | 谁可继续 |
|---|---|---|
| 就绪 | 需求、依赖、版本槽和文件冲突均已清空 | 任意 agent 可认领 |
| 已认领 | 正在内容确认或 OpenSpec 设计，尚未改功能代码 | 只有认领人 |
| 开发中 | 已进入实现或验证 | 只有认领人 |
| 待集成 | 实现与任务评审完成，等待本行自己的归档、PR 或合入 | 只有认领人 |
| 排队 | 范围已明确但前序未完成，不占版本号和文件 | 无人；由 planner 晋升 |
| 设计候选 | umbrella、推测性或远期能力，尚不能形成独立实现闭环 | 无人；先重新设计 |
| 已完成 | 已合入 `main` 并归档 | 无人 |
| 已取消 | 已被其它流程取代或不再需要，保留历史原因 | 无人 |
```

把领取步骤改为只允许选择 `就绪` 行；说明 `排队`/`设计候选` 不得预占文件，planner 只在前序完成后晋升队首。

- [ ] **Step 3: 增加近期产品目标与串行队列表**

在并行规则之前增加以下当前队列，不复制实现细节：

```markdown
## 当前基础玩法发布列车

近期产品边界是「首夜生存 + 自给家园」。核心玩法按下表串行推进；同一时刻最多一行进入版本化实现，绝对版本号在各行基于届时 `main` 确定。

| 顺序 | 行 | 结果 | 解锁条件 |
|---|---|---|---|
| 1 | A-01 | 可真实使用的 2×2/3×3 格子合成与最小单件拆分 | 当前在途 |
| 2 | A-02 | 可放置、可支撑、会发光的火把 | A-01 已完成 |
| 3 | A-03 | 三级剑与统一近战结算 | A-02 已完成 |
| 4 | A-04 | 可生成、追击、攻击、掉落和持久化的夜行者 | A-03 已完成 |
| 5 | A-05 | 床、跳夜、敌怪阻睡和个人重生点 | A-04 已完成 |
| 6 | B-04 | 草丛自然掉种子并取消固定种子赠送 | A-05 已完成 |
| 7 | B-02 | 可搬运水源 | B-04 已完成 |
| 8 | B-33 | 树苗与橡树再生 | B-02 已完成 |
| 9 | B-27 | 单一被动生物、原肉与熟肉 | B-33 已完成 |
| 10 | B-34 | 移除十四组整栈初始材料包 | B-27 已完成 |
```

把原“同批并行共享契约”规则替换为“核心玩法串行、每行自行收尾”；保留版本号互斥、范围冻结和保护无关改动规则。

- [ ] **Step 4: 精确迁移 A 组状态和备注**

按下表修改，不删除原 branch SHA、丢失分支历史或来源：

| ID | 新状态 | 认领人列 | 备注必须追加的当前裁决 |
|---|---|---|---|
| A-01 | 开发中 | 保留 `zcode-implementer @ feat/A-01-authoritative-grid-crafting` | 分支头 `8c2ea638`；因整堆移动令多格同材配方不可正常制作而重开最小单件拆分；本行自行完成协议/golden/归档/合入 |
| A-02 | 排队 | `—` | 保留原认领人和分支履历；worktree 有未提交 `core/assets/physics` 改动，A-01 合入前不得清理或 rebase |
| A-03 | 排队 | `—` | 依赖 A-02；版本和编号不再由 A-06 预分配 |
| A-04 | 排队 | `—` | 保留原批准设计和分支履历；依赖 A-03；排队期不占文件 |
| A-05 | 排队 | `—` | 保留原 claim 履历；依赖 A-04；排队期不占文件 |
| A-06 | 已取消 | `—` | 合流与接线职责回收到 A-01..A-05 |
| A-07 | 已取消 | `—` | 版本、golden 和基线职责回收到各功能行 |
| A-08 | 已取消 | `—` | 终审、归档和 PR 职责回收到各功能行 |

删除 A 组正文中固定的未来绝对版本目标；改为引用本次设计并说明各行基于届时 `main` 使用 next version。

- [ ] **Step 5: 精确迁移 B..F 组状态**

保持全部 `已完成` 行不变，其余按以下集合迁移：

```text
排队：
B-02 B-03 B-04 B-06 B-11 B-17 B-23 B-24 B-27 B-30
B-33 B-34 B-35 B-36 B-37 D-02

设计候选：
B-01 B-08 B-12 B-15 B-16 B-18 B-19 B-20 B-21 B-22 B-25 B-26 B-28 B-29
C-02 C-03 C-04 C-05 C-06 C-07 C-08 C-09 C-10 C-11
D-03 D-06 D-08 D-09
E-01 E-02 E-03 E-05 E-06

已取消：
D-04

开发中：
E-12
```

C-08 的认领人列改为 `—`，备注保留原认领履历并写明“未发现对应 worktree、branch 或开放 PR，若出现未同步证据先恢复证据再裁决”。E-12 保留原认领人和分支，备注明确 worktree 有未提交实现且允许原认领者收尾。

- [ ] **Step 6: 新增 B-33..B-37 五行**

在 B-32 后按以下内容连续追加：

```markdown
| B-33 | 树苗与橡树再生 | 一种树苗掉落、种植与有界确定性生长，让木材可再生 | 方块/物品编号追加；worldgen/随机 tick 规则扩展 | 排队 | — | `2026-08-26-basic-gameplay-backlog-replan-design.md` §7；依赖 B-02；不建设通用植被系统 |
| B-34 | 生存初始背包 | 在关键资源已有自然取得路径后移除十四组整栈材料包，并锁定新世界可达性 | 玩家首次初始化语义；无 wire 结构变更 | 排队 | — | 同上 §7；依赖 B-27；必须证明工作台、照明、工具、食物均可从空背包取得 |
| B-35 | 完整分堆与快捷搬运 | 在 A-01 最小合成拆分之外补齐容器半组/单件与快捷搬运 | 协议命令与容器 UI 扩展 | 排队 | — | 同上 §8；依赖 B-23；不做拖拽铺放或自动整理 |
| B-36 | 斧与铲 | 木/石/铁斧铲的配方、采掘速度、收获等级与耐久 | 物品/配方与采掘规则追加 | 排队 | — | 同上 §8；依赖 B-35；不建设通用工具组件系统 |
| B-37 | 基础洞穴生成 | 单一确定性洞穴 carve 竖切，暴露地下矿物并保持旧区块不改写 | worldgen 与 engine ABI 可能升版 | 排队 | — | 同上 §8；依赖 B-36；不含结构、生物群系装饰或地下城 |
```

把 B-04 备注补为依赖 A-05 且同一行取消固定 64 种子赠送；B-02 依赖 B-04；B-27 依赖 B-33 并把范围收紧为一种被动生物、原肉、熟肉和一条熔炉配方。B-17→B-30→B-11→B-24→B-23→B-35→B-36→B-37 的备注写成显式串行依赖。B-01 改名或备注收敛为“更多作物”，肉类不再重复。

- [ ] **Step 7: 核对规划表结构**

Run:

```bash
git diff --check -- docs/feature-backlog.md
python3 scripts/agents/refresh-discussion.py >/tmp/mornlea-discussion-before-automation.txt
```

Expected: `git diff --check` 通过；第二条在 Task 2 严格状态支持尚未实现时失败或遗漏新状态，这是预期 RED 证据，必须记录实际输出，不能据此改回旧状态。

- [ ] **Step 8: Commit**

```bash
git add docs/feature-backlog.md
git commit -m "docs: replan backlog around basic gameplay"
```

---

### Task 2: 让 Discussion 镜像和接力调度 fail closed

**Files:**
- Modify: `scripts/agents/refresh-discussion.py`
- Create: `scripts/agents/refresh_discussion_test.py`
- Modify: `scripts/agents/relay.sh`

**Interfaces:**
- Consumes: Task 1 的八个精确状态值和 A..F 两种表格列布局。
- Produces: `parse_rows(path=BACKLOG) -> list[dict]`、`build_body(rows) -> str`；未知状态抛出 `ValueError`；relay 只在存在 `| 就绪 |` 任务行时接力。

- [ ] **Step 1: 写 Discussion 生成器失败测试**

创建 `scripts/agents/refresh_discussion_test.py`，使用标准库 `importlib.util` 加载带连字符的脚本：

```python
#!/usr/bin/env python3
import importlib.util
import pathlib
import unittest

SCRIPT = pathlib.Path(__file__).with_name("refresh-discussion.py")
SPEC = importlib.util.spec_from_file_location("refresh_discussion", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def row(task_id, status):
    return {"id": task_id, "name": "功能", "status": status, "who": "—", "pr": ""}


class RefreshDiscussionTest(unittest.TestCase):
    def test_current_statuses_each_render_exactly_once(self):
        statuses = ["就绪", "已认领", "开发中", "待集成", "排队", "设计候选", "已完成", "已取消"]
        rows = [row(f"B-{i:02d}", status) for i, status in enumerate(statuses, 1)]
        body = MODULE.build_body(rows)
        for item in rows:
            self.assertEqual(body.count(f"| {item['id']} |"), 1)
        self.assertIn("🟢 就绪", body)
        self.assertNotIn("🟢 未认领", body)

    def test_retired_or_empty_status_fails_closed(self):
        for status in ["未认领", "", "评审中"]:
            with self.subTest(status=status):
                with self.assertRaisesRegex(ValueError, "未知任务状态"):
                    MODULE.build_body([row("A-01", status)])


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: 运行测试确认 RED**

Run:

```bash
python3 scripts/agents/refresh_discussion_test.py
```

Expected: FAIL；`就绪` 等状态未被分组，且 `未认领` 仍被接受。

- [ ] **Step 3: 用精确状态表替换 substring predicates**

在 `refresh-discussion.py` 定义唯一分组表：

```python
GROUPS = [
    ("开发中", "🟡", False),
    ("已认领", "📋", False),
    ("待集成", "⏳", False),
    ("就绪", "🟢", True),
    ("排队", "🧭", True),
    ("设计候选", "🧩", True),
    ("已完成", "✅", False),
    ("已取消", "⚪", True),
]
KNOWN_STATUSES = {status for status, _, _ in GROUPS}
```

把 `parse_rows()` 改为 `parse_rows(path=BACKLOG)` 并打开 `path`。`build_body` 开头执行：

```python
unknown = sorted({row["status"] for row in rows if row["status"] not in KNOWN_STATUSES})
if unknown:
    raise ValueError("未知任务状态: " + ", ".join(repr(status) for status in unknown))
```

按 `status == label` 精确分组。第三个布尔值表示紧凑表：`就绪`、`排队`、`设计候选`、`已取消` 只显示 ID/功能/备注，活动和完成态继续显示认领人。认领规则首句改成“只可选择 🟢 `就绪` 行”。

在 `main()` 捕获 `ValueError` 并以清晰错误文本非零退出；只有 `build_body` 成功后才允许 `--update` 调远程 mutation。

- [ ] **Step 4: 运行 Discussion 测试确认 GREEN**

Run:

```bash
python3 scripts/agents/refresh_discussion_test.py
python3 scripts/agents/refresh-discussion.py
```

Expected: unittest PASS；dry-run 正文包含八个状态分组，A-01..F-03 和新增 B-33..B-37 每行恰好出现一次，且没有“🟢 未认领”。

- [ ] **Step 5: 把 relay 终结条件改为 `就绪`**

在 `scripts/agents/relay.sh` 只修改终结判据与文案：

```bash
# 终结判据：规划表没有状态单元格为「就绪」的任务行。
if ! grep -E '^\| [A-F]-[0-9]{2} \|' "$ROOT/docs/feature-backlog.md" | grep -q '| 就绪 |'; then
  rm -f "$GUARD"
  log "规划表已无就绪任务，循环终结"
  exit 0
fi
```

不得把 `排队`、`设计候选` 或备注中的“就绪”当成可领取事实。

- [ ] **Step 6: 验证脚本语法和当前无接力条件**

Run:

```bash
bash -n scripts/agents/relay.sh
python3 -m py_compile scripts/agents/refresh-discussion.py scripts/agents/refresh_discussion_test.py
if grep -E '^\| [A-F]-[0-9]{2} \|' docs/feature-backlog.md | grep -q '| 就绪 |'; then exit 1; fi
```

Expected: 全部退出 0；当前 A-01 为 `开发中`，所以规划表没有可被新链领取的 `就绪` 行。

- [ ] **Step 7: Commit**

```bash
git add scripts/agents/refresh-discussion.py scripts/agents/refresh_discussion_test.py scripts/agents/relay.sh
git commit -m "fix(agents): gate relay on ready backlog rows"
```

---

### Task 3: 对齐开发流程和工作者提示词

**Files:**
- Modify: `docs/development-process.md`
- Modify: `docs/agents/README.md`
- Modify: `docs/agents/planner.md`
- Modify: `docs/agents/planner-prompt.md`
- Modify: `docs/agents/implementer.md`
- Modify: `docs/agents/setup-new-machine.md`

**Interfaces:**
- Consumes: Task 1 的状态模型与 Task 2 的 relay/Discussion 行为。
- Produces: planner 晋升规则和 implementer 唯一领取规则；所有常驻工作者读取同一状态词。

- [ ] **Step 1: 修改开发流程阶段 0 与并行规则**

在 `docs/development-process.md` 明确：

```markdown
1. 读 `docs/feature-backlog.md` 与 `openspec/config.yaml`；只能选择一行 `就绪` 任务。
2. `排队` 与 `设计候选` 不得认领；依赖满足也必须先由 planner/控制会话晋升为 `就绪`。
3. 认领后把状态改为 `已认领`，完成内容确认后才进入 `开发中`。
```

把“同批并行先共享契约”改成当前规则：版本化核心玩法串行，每行自行完成版本/golden/基线/归档；只有无版本影响且文件集合确实不交叠的任务可并行。

- [ ] **Step 2: 修改 planner 角色卡和真实 prompt**

在 `planner.md` 与 `planner-prompt.md` 同步写入：

- 状态集合为八个新状态。
- 新缺口默认进入 `排队`；umbrella、推测性或尚无独立闭环的功能进入 `设计候选`。
- 只有在前序已完成、版本槽空闲、worktree 证据一致时才能把串行队首晋升为 `就绪`。
- planner 不得同时晋升两个会推进稳定编号、协议/schema、ABI 或 golden 的核心玩法行。
- Discussion 正文由八组状态生成，只有 `就绪` 是绿色可领取组。
- 新行仍按组内连续 ID，来源必须已提交；不排日期或工时。

删除“A 组版本契约冻结在并行批次”和“新行一律未认领”等失效措辞。

- [ ] **Step 3: 修改 implementer 与运行入口文档**

在 `implementer.md`、`README.md`、`setup-new-machine.md` 把所有“无未认领任务”“选择未认领行”改为“无就绪任务”“只选择就绪行”。保留异常中断续跑、确认门禁、SDD、PR 收尾和 guard 语义不变。

状态评论模板允许：

```text
就绪|已认领|开发中|待集成|排队|设计候选|已完成|已取消
```

普通实现者只能主动产生 `已认领`、`开发中`、`待集成`、`已完成`；`排队`、`设计候选`、`就绪` 和 `已取消` 由 planner/控制会话维护。

- [ ] **Step 4: 搜索当前规范中的旧领取语义**

Run:

```bash
git grep -n '未认领' -- docs/development-process.md docs/agents scripts/agents ':!scripts/agents/refresh_discussion_test.py'
```

Expected: 只允许设计迁移解释或测试里的拒绝样本；不得再出现“选择未认领”“无未认领任务”“新行一律未认领”等当前规则。

- [ ] **Step 5: 验证文档链接和脚本引用**

Run:

```bash
test -f docs/superpowers/specs/2026-08-26-basic-gameplay-backlog-replan-design.md
test -f scripts/agents/refresh-discussion.py
test -f scripts/agents/relay.sh
git diff --check -- docs/development-process.md docs/agents
```

Expected: 全部退出 0。

- [ ] **Step 6: Commit**

```bash
git add docs/development-process.md docs/agents/README.md docs/agents/planner.md docs/agents/planner-prompt.md docs/agents/implementer.md docs/agents/setup-new-machine.md
git commit -m "docs(agents): adopt ready-only task claims"
```

---

### Task 4: 让 Agent Board 展示新状态

**Files:**
- Modify: `cmd/mornlea-agent-board/parse.go`
- Modify: `cmd/mornlea-agent-board/parse_backlog_test.go`
- Modify: `web/agent-board/src/lib/fmt.ts`
- Modify: `web/agent-board/src/lib/fmt.test.ts`
- Modify: `web/agent-board/src/components/TaskBoard.tsx`
- Modify: `web/agent-board/src/components/StatCards.tsx`
- Modify: `docs/notes/agent-dashboard.md`

**Interfaces:**
- Consumes: backlog 的八个状态字符串。
- Produces: 后端 `BacklogTask.Status` 保留八种当前状态；前端按活动→就绪→排队→候选→终态分组，并统计 `就绪任务`。

- [ ] **Step 1: 写 Go 状态解析失败测试**

在 `parse_backlog_test.go` 增加表驱动测试：

```go
// TestParseBacklogRowCurrentStatuses 锁定当前规划状态不会降级为「其他」。
func TestParseBacklogRowCurrentStatuses(t *testing.T) {
	statuses := []string{"就绪", "已认领", "开发中", "待集成", "排队", "设计候选", "已完成", "已取消"}
	for _, status := range statuses {
		t.Run(status, func(t *testing.T) {
			row := "| B-33 | 功能 | 简述 | 无 | " + status + " | — | 备注 |"
			task, ok := parseBacklogRow(row)
			if !ok {
				t.Fatal("任务行未被识别")
			}
			if task.Status != status {
				t.Fatalf("Status = %q, want %q", task.Status, status)
			}
		})
	}
}
```

把 `TestParseBacklogRowEmptyClaimant` fixture 的状态从 `未认领` 改成 `就绪`；三列表头 fixture 同步改为 `| 就绪 | ... |`。未知状态测试继续要求回退 `其他`。

- [ ] **Step 2: 写前端状态样式失败测试**

在 `fmt.test.ts` 增加：

```typescript
it('当前规划状态都有稳定样式', () => {
  expect(statusClass('就绪')).toContain('text-status-unclaimed');
  for (const status of ['排队', '设计候选', '已取消']) {
    expect(statusClass(status)).toContain('text-status-other');
  }
});
```

- [ ] **Step 3: 运行测试确认 RED**

Run:

```bash
go test ./cmd/mornlea-agent-board -run 'TestParseBacklogRow(CurrentStatuses|EmptyClaimant|NonTableAndLegend)' -count=1
npm --prefix web/agent-board test -- src/lib/fmt.test.ts
```

Expected: Go 的新状态归一为 `其他`；TypeScript 的 `就绪` 仍回退 other，至少一条失败。

- [ ] **Step 4: 更新 Go 后端状态集合**

把 `parse.go` 的 GoDoc 和状态集合改为：

```go
// Status 为规范化状态（就绪|已认领|开发中|待集成|排队|设计候选|已完成|已取消|其他）。

var knownStatus = map[string]bool{
	"就绪": true, "已认领": true, "开发中": true, "待集成": true,
	"排队": true, "设计候选": true, "已完成": true, "已取消": true,
}
```

不改变六列/七列识别、认领人解析或未知状态的 `其他` 降级。

- [ ] **Step 5: 更新前端状态类型、分组和统计**

在 `fmt.ts` 使用：

```typescript
export type TaskStatus = '就绪' | '已认领' | '开发中' | '待集成' | '排队' | '设计候选' | '已完成' | '已取消' | '其他';
```

样式映射复用现有 token：`就绪` 使用 `status-unclaimed` 绿色；`排队`、`设计候选`、`已取消` 使用 `status-other`；其它状态保持原样。不新增 CSS token。

`TaskBoard.tsx` 分组顺序改为：

```typescript
const groupOrder = ['开发中', '已认领', '待集成', '就绪', '排队', '设计候选', '已完成', '已取消', '其他'];
```

`StatCards.tsx` 把“未认领任务”替换为“就绪任务”，计数 `count('就绪')`；八格布局和其它统计不变。

- [ ] **Step 6: 更新看板当前契约说明**

在 `docs/notes/agent-dashboard.md` 的当前“任务状态”说明中列出八个状态和 `其他`；历史交付记录不改写。说明 `就绪` 是绿色可领取状态，排队/候选只展示不调度。

- [ ] **Step 7: 运行后端与前端测试**

Run:

```bash
gofmt -w cmd/mornlea-agent-board/parse.go cmd/mornlea-agent-board/parse_backlog_test.go
go test ./cmd/mornlea-agent-board -race -count=1
npm --prefix web/agent-board test
npm --prefix web/agent-board run build
```

Expected: Go 测试全绿；Vitest 全绿；TypeScript/Vite build 成功。

- [ ] **Step 8: Commit**

```bash
git add cmd/mornlea-agent-board/parse.go cmd/mornlea-agent-board/parse_backlog_test.go web/agent-board/src/lib/fmt.ts web/agent-board/src/lib/fmt.test.ts web/agent-board/src/components/TaskBoard.tsx web/agent-board/src/components/StatCards.tsx docs/notes/agent-dashboard.md
git commit -m "feat(agent-board): display planned backlog states"
```

---

### Task 5: 集成验证与 Discussion 发布门禁

**Files:**
- Verify: all files from Tasks 1..4
- Create and maintain: `docs/superpowers/plans/2026-08-26-basic-gameplay-backlog-replan-ledger.md`
- Remote update after merge: GitHub Discussion #71

**Interfaces:**
- Consumes: 四个已提交任务的集成树。
- Produces: 可评审分支、完整 dry-run Discussion 正文，以及 main 合入后可安全执行的 `--update` 操作。

- [ ] **Step 1: 核对既有任务 ID 没有静默丢失**

Run:

```bash
python3 - <<'PY'
import re
from pathlib import Path

before = set(re.findall(r'^\|\s*([A-F]-\d{2})\s*\|', Path('/tmp/mornlea-backlog-before.md').read_text(), re.M))
after = set(re.findall(r'^\|\s*([A-F]-\d{2})\s*\|', Path('docs/feature-backlog.md').read_text(), re.M))
missing = sorted(before - after)
added = sorted(after - before)
assert not missing, f"任务 ID 被删除: {missing}"
assert added == ['B-33', 'B-34', 'B-35', 'B-36', 'B-37'], added
print(f"保留 {len(before)} 个既有 ID，新增 {added}")
PY
```

Expected: 无缺失，只新增 B-33..B-37。

- [ ] **Step 2: 运行 focused 全套验证**

Run:

```bash
python3 scripts/agents/refresh_discussion_test.py
python3 scripts/agents/refresh-discussion.py
python3 -m py_compile scripts/agents/refresh-discussion.py scripts/agents/refresh_discussion_test.py
bash -n scripts/agents/relay.sh scripts/agents/run-agent.sh
go test ./cmd/mornlea-agent-board -race -count=1
go vet ./cmd/mornlea-agent-board
npm --prefix web/agent-board test
npm --prefix web/agent-board run build
go test ./internal/archcheck -count=1
openspec validate --all --strict --no-interactive
git diff --check origin/main...HEAD
```

Expected: 全部通过；Discussion 命令只打印正文，不执行远程更新。

- [ ] **Step 3: 核对受控当前文档不再使用旧领取规则**

Run:

```bash
git grep -n '未认领' -- docs/feature-backlog.md docs/development-process.md docs/agents scripts/agents ':!scripts/agents/refresh_discussion_test.py'
```

Expected: 只允许迁移历史说明；不得存在“选择未认领”“无未认领”“新行一律未认领”或 Discussion 绿色未认领组。

- [ ] **Step 4: 检查工作树和提交范围**

Run:

```bash
git status --short --branch
git log --oneline origin/main..HEAD
git diff --stat origin/main...HEAD
```

Expected: 没有未提交文件；提交只包含设计、计划、backlog、agent 自动化/文档和 agent board 状态消费者，不包含游戏代码、协议、ABI 或 golden。

- [ ] **Step 5: 完成整分支终审并提交 ledger**

派 fresh final reviewer 检查 `origin/main...HEAD` 的规格合规、状态一致性、脚本失败语义、看板兼容性和测试缺口。把 verdict、findings、修复轮次和所有控制会话裁决写入 ledger；存在 finding 时先回到对应 implementer 修复并重跑 Task 5 Step 2。

终审通过后提交 ledger：

```bash
git add docs/superpowers/plans/2026-08-26-basic-gameplay-backlog-replan-ledger.md
git commit -m "docs: record backlog replan execution"
git diff --check origin/main...HEAD
```

Expected: ledger 包含 Task 1..4 的双评审结果和整分支 PASS；最终 diff 检查通过。

- [ ] **Step 6: 在 main 合入前保持 Discussion 不变**

不得从 feature branch 运行：

```bash
python3 scripts/agents/refresh-discussion.py --update
```

理由：Discussion 声明仓库 `main` 的 backlog 是单一真相源；在分支先更新会制造远端镜像领先于真相源的窗口。

- [ ] **Step 7: main 合入后刷新 Discussion #71**

只有在用户明确批准并完成 branch merge、且本地已 fetch 到包含本计划实现提交的 `origin/main` 后执行：

```bash
git merge-base --is-ancestor HEAD origin/main
python3 scripts/agents/refresh-discussion.py --update
```

Expected: 第一条退出 0；GraphQL mutation 返回 Discussion #71 URL/number；正文只有 `就绪` 组可领取，当前因 A-01 为 `开发中` 而没有绿色可领取行。

若远程更新失败，保留已合入的仓库规划作为真相源，报告错误并重试脚本；不得回退规划提交或手工粘贴 Discussion 正文。
