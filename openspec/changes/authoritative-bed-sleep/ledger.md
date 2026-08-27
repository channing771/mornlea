# Ledger：authoritative-bed-sleep

> 记录：控制会话裁决（ruling）、每 Task 评审结论与修复循环、最终验证输出摘要。

## 内容确认记录（brainstorming 硬门禁，2026-08-28）

- **分类**：architectural（新方块子系统 + 玩家 schema v8 + 世界 metadata v3 + 协议字段追加 + 显示相位语义扩展）。
- **探索**：backlog 行「床与睡眠」（原 `codex-implementer @ feat/A-05-authoritative-bed-sleep` 履历与共享契约 SHA `785ea07b` 均已丢失、本无实现损失）；现行主规格核对：`authoritative-daylight`（绝对时间 + metadata v2 + 客户端相位）、`authoritative-health`（死亡重生回出生锚点）、`internal/sim/door.go`（双格原子放置/交互/采掘先例）、`internal/sim/death.go`（`beginReset` 重生路径）、`internal/storage/metadata.go`（v2 定长布局 + v1 迁移先例）、`internal/storage/player_types.go`（v7 无重生点字段）、`internal/render/daylight.go`（`DayLengthTicks=24000`）。
- **Ruling: 并行互动（2026-08-28，用户裁决「解耦：无条件可睡」）** — 睡眠不检查夜行者；跳夜后白昼灼烧按夜行者既有规则自然结算；两行唯一共享契约为 `core.DisplayDayPhase(ticks, offset)`（A-04 交付、本行提供 offset 生产端）。为什么：契约面最小、两线真并行；靠近拒睡等耦合玩法留待后续行。
- **Ruling: A-05-approval（2026-08-28，approve）** — 按节呈现的设计（共享契约 / A-04 重定基线 / A-05 范围与配方 / 编排与门禁）经用户显式批准；床配方定为「顶排 3 小麦 + 下排 3 橡木木板 → 床 ×1」（麦秸床垫，材料可再生，与门 2×3 形状不冲突）。
- **Ruling: 合并序（2026-08-28）** — A-04 先合并（交付 `DisplayDayPhase` 与 S→C 22/23/24），本行 rebase 后合并；协议版本号由本行合并时基于届时 `main` 取下一空闲（A-04 取 v30 则本行 v31）；`bed-night` 场景插在 `torch-night` 之后、`ai-companion` 之前，与 A-04 的 `hostile-mob` 插入点互不冲突。

## 变更产物

- [x] `openspec/changes/authoritative-bed-sleep/`：proposal/5 delta specs/design/tasks/ledger 已建于本 worktree 功能分支。

## Task 1 基线验证（2026-08-28）

### 验证命令输出摘要（数值只记录）

- `git status --short`：worktree 干净（分支 `feat/A-05-authoritative-bed-sleep`，起始 HEAD `3e846534`）。
- `make rust`：通过（exit 0，release 构建 `mornlea_engine` 与 `mornlea_client`，约 1m 12s）。
- `go test ./internal/core ./internal/sim ./internal/storage ./internal/network ./internal/client ./internal/render ./internal/assets ./internal/mesh ./cmd/mornlea -race -count=1`：通过（exit 0，9 包全 ok）。耗时：`core` 2.155s、`sim` 68.709s、`storage` 35.083s、`network` 7.309s、`client` 8.544s、`render` 4.960s、`assets` 3.326s、`mesh` 36.844s、`cmd/mornlea` 337.818s。
- `openspec validate --all --strict --no-interactive`：通过（72 passed, 0 failed）。
- `git diff --check`：通过（无输出，exit 0）。

### 前置检查：`core.DisplayDayPhase` 状态

- `core.DisplayDayPhase` 当前在本分支不存在（`internal/`、`cmd/` 全量 grep 无匹配），与预期一致（夜行者行尚未合并交付）。
- 按本 change tasks 头部前置检查，记入**本行自带清单**：钉定签名 `DisplayDayPhase(worldTime uint64, offset uint16) uint16`；语义为先对 `worldTime` 做 `%24000`、再与 `offset` 相加后取模 24000；随实现交付边界测试；rebase 合并时与夜行者行交付的同一函数去重（保留一份）。

### 契约与编号核对（本行将取用的编号）

- `internal/core/block_properties.go`：`BlockEmission` 已存在（发光方块 15、火把五形态 14、其余 0）；`BlockLightAttenuation` 已存在（八个流体编号 1、其余 0）；`BlockOpaque` 不存在（按预期由夜行者行新增，rebase 后若已存在则直接消费、不重复定义）。
- 方块编号：`internal/core/block.go` 现方块段末为 `TorchWallNegZID` = 75，哨兵 `BlockIDMax` = 76。本行床 8 形态（床尾/床头 × 4 水平朝向）将取 76..83，`BlockIDMax` 顺延 8 至 84。
- 物品编号：`internal/core/item.go` 现物品段末为 `ItemTorch` = 44，哨兵 `ItemIDMax` = 45。本行 `ItemBed` 将取 45，`ItemIDMax` 顺延至 46。
- 配方编号：`internal/core/recipe.go` 现配方段末为 `RecipeTorch` = 15（`iota+1` 起始共 15 条，无哨兵常量）。本行 `RecipeBed` 将取 16。
- 世界 metadata：`internal/storage/metadata.go` 的 `currentMetadataVersion = 2`（符合预期）；本行将升至 3。
- 玩家存档 schema：`internal/storage/player_codec.go` 第 13 行 `currentPlayerSchema uint32 = 7`（v7 唯一定义点，`player_migration.go` 仅消费）；本行将升至 8。
- 协议版本：`internal/network/packet.go` 的 `ProtocolVersion uint32 = 29`（符合预期）；终值仍按合并序在合并期基于届时 `main` 重订。
- S→C 消息编号：`internal/network/registry.go` 的 `PlayerState` = 3（符合预期）；现 S→C 段末为 `CraftingState` = 21，夜行者行的 22..24 尚未占用。本行不新增 S→C 消息（`DayPhaseOffset` 尾部追加进 `PlayerState`）。

## 评审记录（Task 1 起，逐 Task 追加）

- （待逐 Task 填：SPEC 合规结论 / QUALITY 结论 / 修复轮 R1..Rn / 对应 Ruling）

## 最终验证输出摘要（收尾补）

- （待整分支终审后补：make rust、focused -race、archcheck、vet、gofmt、openspec strict、visual-check 的数值摘要；benchmark 数值只记录）
