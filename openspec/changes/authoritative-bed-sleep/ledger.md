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

- **Task 1（基线验证）**：完成，提交 `c710d542`。`make rust` 与 9 包 `-race` 全绿；编号核对：床 8 形态取 76..83（`BlockIDMax`→84）、`ItemBed`=45、`RecipeBed`=16、metadata v2、玩家 schema v7（`player_codec.go:13`）、协议 29、`PlayerState`=3。`DisplayDayPhase` 缺位→按并行裁决记入本行自带清单。控制会话抽查通过。
- **Task 2（床方块/物品/配方/碰撞/纹理/模型）**：实现提交 `f8e4d938`→评审 QUALITY FAIL（床面专属层 60..67 在 mesh 输出不可达：`emit_bed` 全 quad 读 face 0 材质而床面层只挂 FacePosY——生产链缺陷， SPEC PASS）→R1 修复（方案 A：平顶读 face 3、侧板各读自身面、非同质 Rust 夹具、生产链穿透测试 `TestBedSurfaceLayerReachesMesherThroughProductionRegistry` 红→绿、修正 face 序注释、ABI header 注释同步）→amend `dc43e746`→R1 复核 SPEC PASS + QUALITY PASS + 容量专项维持成立（80→96 有原注释/门火把同批先例/两侧同步双守卫/+16 步距四点依据，不属 ABI 版本契约）。修复轮：R1（1 轮，原实现者完成）。
- **Task 4（入睡/跳夜/重生点）**：实现提交 `451b0efb`；评审 SPEC PASS + QUALITY PASS，五项自报裁决全部核实成立（offset 基准取 tick 完成后绝对时间；「未验证 ≠ 失效」经 spec 措辞+Requirement 标题+禁停摆三重佐证成立并以 `TestDeathWithUnverifiedRespawnKeepsRecord` 钉定；`CommandInteractDoor` 先例确无 wire 映射、本任务零碰 network；`SetWorldTimeForTest` 无生产调用；敌怪对拍无可接线点、判夜入口已收敛 `Engine.displayDayPhase()`）。评审指出的两处产物-代码不一致（design D1 公式缺 +1 基准、delta spec 缺「区块未就绪」场景）已由控制会话修订（`c7f3d76a`），另修一处注释空格。修复轮：0。
- **Task 3（放置/采掘/支撑）**：实现提交 `5eb7b581`；评审 SPEC PASS + QUALITY PASS，三项自报裁决经核实全部成立（支撑扫除比照火把系 spec 明文要求且门无先例可抄；伙伴采掘床双清分支防半床残留；现行命令集确无命中床的 use 路径）。非阻塞建议四条留归档期参考（`clearBedPair` 防御纵深不对称、`bedHalfPositions` 第二份坐标拷贝、级联举例、火把触发床失效当 tick 覆盖缺直测）。修复轮：0。实现者裁决：支撑扫除比照火把先例（门无运行时扫除而 spec 明文要求整床移除掉落）、伙伴采掘床双清分支（避免通用单清残留半床）、右键交互留待 Task 4。

## 最终验证输出摘要（收尾补）

- （待整分支终审后补：make rust、focused -race、archcheck、vet、gofmt、openspec strict、visual-check 的数值摘要；benchmark 数值只记录）
