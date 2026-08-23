# Mornlea Authoritative Hunger HUD Main Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 PR #64 的 Pixel Perfection HUD 与 `origin/main` 的服务端权威饥饿系统做一次原子语义合并，交付三列生存状态 HUD、完整的 15 场景视觉基线和最终 benchmark scenario v19 契约。

**Architecture:** `origin/main` 继续独占饥饿、协议、存档、模拟、预测与版本推进；当前分支继续独占已经验收的心形/气泡、HUD 响应式布局、背包、聊天、采掘和 capture 行为。合并只在现有 `internal/render/hud` 纹理图集、CPU 布局和既有 hotbar pass 内组合两侧能力：三列共享一个 531 design-pixel 的居中几何，隐藏满氧气只释放实例、不改变列位置。

**Tech Stack:** Go 1.26、Rust 1.97.1、现有 `mornlea_engine`/`mornlea_client` ABI、OpenSpec、Go `testing`、无窗口 capture/golden、现有 SDD review-package helper。

**Spec:** `openspec/changes/bedrock-survival-hud/proposal.md`、`openspec/changes/bedrock-survival-hud/specs/hud/spec.md`、`openspec/changes/bedrock-survival-hud/specs/capture/spec.md`、`openspec/changes/bedrock-survival-hud/design.md`、`openspec/changes/bedrock-survival-hud/tasks.md`

## Global Constraints

- 后续对已核对合并问题的代码改动，MUST 在单一未提交 merge 中原子解决，并在完整合并结果测试通过后才创建 merge commit。
- 计划 MUST NOT 采用 rebase、force-push、整文件 `ours`/`theirs` 源码或文档覆盖，也 MUST NOT 对冲突 PNG 选择任一侧旧文件。
- 最终协议版本为 v24、玩家 schema 为 v7、benchmark scenario 为 v19；engine ABI 保持 v6、client ABI 保持 v7、chunk schema 保持 v9、world metadata 保持 v2、`companions.ai` schema 保持 v4。
- HUD 图集 MUST 在 item 区之前按固定顺序包含七个 16×16 UI cell：空心、半心、满心、空气泡、实气泡、空鸡腿、满鸡腿。
- 生存状态组 MUST 按 health / depleted oxygen / hunger 排列；每列 169 design px，列间距 12 design px，总宽 531 design px，整体水平居中并共用 `hudScale`。
- 满氧气隐藏实例但保留中间列几何。背包关闭时状态行在 hotbar 上方、采掘反馈在状态行上方；背包打开时状态行在 hotbar 下方，且不得改变或覆盖 36 个 `InventorySlotAt` 交互格。
- 未确认饥饿值隐藏；已确认值钳制到 `0..core.MaxHunger`，绘制 10 个空槽及 `ceil(Hunger/2)` 个填充，奇数值末格只填右半边，不新增背景面板，饥饿最多 20 quad。
- health 和 depleted oxygen 各自使用已决议的 10-quad 上限；满氧气时绘制 0 个气泡 quad。
- scenario v19 固定为 `maxHotbarQuads = 267`、glyph capacity 700、glyph offset 13312 bytes、总容量 46912 bytes、instance size 48 bytes；所有固定 buffer offset 保持 256-byte 对齐。
- 最终关闭/打开布局最大值分别是 96/265，即现有 76/245 加饥饿 20；聊天仍锚定关闭态状态行，并保留 `1/256` framebuffer-pixel slack。
- 不新增 shader、render pass、上传格式、API、ABI、配置、依赖、PNG 资源、材质注册表、主题系统、产品态测试接口或动态 GPU 资源。
- capture 场景顺序固定为 15 项：`terrain-noon`、`hud-hotbar-health`、`hud-survival-feedback`、`avatar-nametag`、`inventory-crafting`、`debug-panel`、`skylight-tunnel`、`block-light-room`、`materials-showcase`、`target-block-feedback`、`oak-grove`、`ai-companion`、`water-surface-slope`、`far-horizon`、`water-underwater`。
- `hud-hotbar-health` 固定为 10 个满心、满饥饿、满氧气隐藏；`hud-survival-feedback` 固定为生命 5、氧气 `core.MaxOxygenTicks / 3`、饥饿 9、磨损工具和受阻采掘 4/9；`inventory-crafting` 固定为生命 5、氧气三分之一、饥饿 9，并保证状态行避开 36 个背包格与 10 行配方。
- capture fixture MUST 在成功、错误和重复 restore 路径中一起固定并恢复 predictor、mining 与 hunger 状态。
- 所有 15 张 golden MUST 由最终合并二进制重新生成并逐张人工检查；不得放宽阈值，必须保留 LOD near-band 控制、`water-surface-slope`、倒数第二的 `far-horizon` 和最后的 `water-underwater`。
- 任务 ledger 只在证据已经存在时更新 checkbox；每个阶段都要追加 implementer、reviewer、进度、findings、repairs、rulings 与 visual 证据。
- 本计划只有一个 independently reviewable core implementation task；review、repair、whole-branch final review 与 push 属于 SDD closeout，不拆成新的产品实现任务。
- 不归档此 OpenSpec change；归档需要用户另行明确批准。

---

### Task 1: 原子语义合并 authoritative hunger 与 Pixel Perfection HUD

**Files:**

- Modify: `AGENTS.md`
- Modify: `CLAUDE.md`
- Modify: `cmd/mornlea/app_frame.go`
- Modify: `cmd/mornlea/capture.go`
- Modify: `cmd/mornlea/capture_hud_test.go`
- Modify: `cmd/mornlea/chat_test.go`
- Modify: `cmd/mornlea/health_hud_test.go`
- Modify: `cmd/mornlea/testdata/golden/*.png`（固定 15 张，全部重生成）
- Modify: `docs/notes/progress.md`
- Modify: `internal/render/hud/atlas.go`
- Modify: `internal/render/hud/atlas_test.go`
- Modify: `internal/render/hud/health.go`
- Modify: `internal/render/hud/health_test.go`
- Modify: `internal/render/hud/hotbar_test.go`
- Modify: `internal/render/hud/layout.go`
- Modify: `internal/render/hud/layout_test.go`
- Modify: `internal/render/hud/oxygen.go`
- Modify: `internal/render/hud/oxygen_test.go`
- Modify: `internal/render/hud/renderer.go`
- Modify: `internal/render/hud/renderer_test.go`
- Add from main, then modify: `internal/render/hud/hunger.go`
- Add from main, then modify: `internal/render/hud/hunger_test.go`
- Modify: `openspec/changes/bedrock-survival-hud/tasks.md`
- Modify: `openspec/changes/bedrock-survival-hud/ledger.md`

#### Step 1: 固定 linked worktree、基线身份与唯一任务身份

- [ ] 进入现有 linked worktree，并确认不是主工作目录：

```bash
cd /Users/chen/chenwork/minecraft-go/.worktrees/bedrock-survival-hud
test "$(pwd -P)" = "/Users/chen/chenwork/minecraft-go/.worktrees/bedrock-survival-hud"
git worktree list --porcelain
git status --short
git diff --quiet
git diff --cached --quiet
```

Expected: 状态为空；若不为空，停止并把现有改动报告给 controller，不清理、不覆盖。

- [ ] 固定并核对三条已审核 ref：

```bash
planning_base=dbb9e811d9bf3d87aafd56bd1fdb18f0f1107c33
main_commit=f9808f4c0c07e17b084a1a5a91ab97f2f925e407
common_base=08932d9240274299cd0b0b1288900120540c636d
git merge-base --is-ancestor "$planning_base" HEAD
test "$(git rev-parse origin/main)" = "$main_commit"
test "$(git merge-base HEAD "$main_commit")" = "$common_base"
integration_base=$(git rev-parse HEAD)
printf 'planning_base=%s\nmain_commit=%s\ncommon_base=%s\nintegration_base=%s\n' \
  "$planning_base" "$main_commit" "$common_base" "$integration_base"
```

Expected: `planning_base` 是已审核规划基线；`integration_base` 是执行开始时包含本计划提交的实际 BASE，后续 review package 必须使用它。

- [ ] 在 `openspec/changes/bedrock-survival-hud/ledger.md` 追加一行执行开始记录，任务身份固定为 `6.2–6.13 main authoritative-hunger integration`，写入 controller 派发的 fresh implementer 名称、上述四个 ref、开始时间和“未提交 merge 前置检查通过”。不要提前勾选任何实现 checkbox。

#### Step 2: 先构建 Rust，再确认两侧合并前基线

- [ ] 遵守仓库 clean-checkout 顺序，先运行 canonical Rust target：

```bash
make rust
```

Expected: Rust 1.97.1 locked release build 成功。

- [ ] 运行合并前 focused Go 基线：

```bash
go test ./internal/render/hud ./cmd/mornlea ./cmd/perfcheck -count=1
go test ./internal/archcheck -count=1
```

Expected: 全部通过。把命令与结果追加到 ledger；失败时停止，不能把基线失败归因于合并。

#### Step 3: 创建唯一未提交 merge 并核对精确冲突集

- [ ] 执行正常、非 rebase 的未提交 merge，并验证 `MERGE_HEAD`：

```bash
set +e
git merge --no-ff --no-commit "$main_commit"
merge_exit=$?
set -e
test "$merge_exit" -eq 1
test "$(git rev-parse MERGE_HEAD)" = "$main_commit"
git status --short
git diff --name-only --diff-filter=U
```

Expected: merge 因已审核的冲突返回 1；从此直到完整 gate 通过都不得提交。

- [ ] 将实际 unmerged 路径与下面精确的 22 项逐项核对；数量、路径或类型任一不符都停止并追加 ledger finding，交 controller 裁决。

Text/source conflicts（8）:

```text
cmd/mornlea/chat_test.go
cmd/mornlea/health_hud_test.go
docs/notes/progress.md
internal/render/hud/atlas.go
internal/render/hud/layout.go
internal/render/hud/layout_test.go
internal/render/hud/renderer.go
internal/render/hud/renderer_test.go
```

Binary conflicts（14）:

```text
cmd/mornlea/testdata/golden/ai-companion.png
cmd/mornlea/testdata/golden/avatar-nametag.png
cmd/mornlea/testdata/golden/block-light-room.png
cmd/mornlea/testdata/golden/debug-panel.png
cmd/mornlea/testdata/golden/far-horizon.png
cmd/mornlea/testdata/golden/hud-hotbar-health.png
cmd/mornlea/testdata/golden/inventory-crafting.png
cmd/mornlea/testdata/golden/materials-showcase.png
cmd/mornlea/testdata/golden/oak-grove.png
cmd/mornlea/testdata/golden/skylight-tunnel.png
cmd/mornlea/testdata/golden/target-block-feedback.png
cmd/mornlea/testdata/golden/terrain-noon.png
cmd/mornlea/testdata/golden/water-surface-slope.png
cmd/mornlea/testdata/golden/water-underwater.png
```

`hud-survival-feedback.png` 是当前分支已增加的第 15 张，不在 binary conflict 列表中，但必须与其余 14 张一起由最终二进制重生成。

- [ ] 在 ledger 记录实际冲突清单、`MERGE_HEAD` 与以下 ownership ruling：

| 区域 | 权威来源 | 合并结果 |
|---|---|---|
| 饥饿规则、协议 v24、玩家 schema v7、network/sim/storage/predictor/app wiring、benchmark scenario v19 | `origin/main` | 保留 main 行为和版本推进 |
| 心形/气泡、状态行响应式定位、背包、聊天、采掘、capture、远环/水景视觉 | 当前分支 | 保留已验收行为 |
| atlas、三列状态布局、容量、capture fixture/goldens、长期基线文档 | 双方语义组合 | 按本计划测试先行重写冲突块 |

#### Step 4: 手工消除文本 marker，建立可编译但尚未满足最终断言的 RED 基线

- [ ] 手工编辑全部 8 个文本冲突；不得对整个文件使用 `ours` 或 `theirs`。同时语义检查自动合并的 `AGENTS.md`、`CLAUDE.md`、`cmd/mornlea/app_frame.go`、`cmd/mornlea/capture.go`、`cmd/mornlea/capture_hud_test.go`、`internal/render/hud/atlas_test.go`、`internal/render/hud/health*.go`、`internal/render/hud/oxygen*.go`、`internal/render/hud/hotbar_test.go` 和 main 新增的 `hunger*.go`。

- [ ] 先把工作树整理成能由 Go parser 读取的临时 RED 状态：删除所有 conflict marker；保留 main 的 `HungerOverlay` 数据流和当前分支的 health/oxygen/open-layout 数据流；不提交、不暂存 PNG。运行：

```bash
rg -n '^(<<<<<<<|=======|>>>>>>>)' \
  AGENTS.md CLAUDE.md cmd/mornlea docs/notes internal/render/hud
```

Expected: 无输出。此步只消除语法级冲突，不宣称布局已经正确。

#### Step 5: 先写七列 atlas 的失败测试

- [ ] 在 `internal/render/hud/atlas_test.go` 与 `internal/render/hud/hunger_test.go` 写最终断言，覆盖：七个 UI cell 的固定顺序、每列确有非透明像素、item 起始 offset 从 5 变为 7、block 顶面复制仍正确、每个 UV 都严格留在本列、奇数饥饿只使用满鸡腿列的右半 UV。

测试形状必须直接枚举稳定列，而不是复刻 painter 实现：

```go
wantColumns := [...]int{
	0, // empty heart
	1, // half heart
	2, // full heart
	3, // empty bubble
	4, // full bubble
	5, // empty drumstick
	6, // full drumstick
}
```

并断言第一个 item layer 的列是 `len(wantColumns)`，而不是保留旧的 4 或 5。

- [ ] 运行 RED：

```bash
go test ./internal/render/hud -run 'TestHotbar(TextureAtlas|ColumnUV)|TestHungerBar' -count=1
```

Expected: 因缺失鸡腿 cell、列顺序/offset 不正确或奇数半格 UV 不正确而失败。把失败测试名与核心错误追加到 ledger。

#### Step 6: 最小实现七列程序化图集

- [ ] 修改 `internal/render/hud/atlas.go`，只扩展现有 UI icon 区和 painter；固定常量形状为：

```go
const (
	hotbarEmptyHeartColumn = iota
	hotbarHalfHeartColumn
	hotbarFullHeartColumn
	hotbarEmptyBubbleColumn
	hotbarFullBubbleColumn
	hotbarEmptyDrumstickColumn
	hotbarFullDrumstickColumn
	hotbarItemColumnOffset
)
```

鸡腿继续程序化绘制到现有 HUD atlas；不得添加 PNG、注册表项或新的 atlas/pass。保留 Pixel Perfection 方块 layer 的复制和未映射 layer 程序化 fallback。

- [ ] 运行 GREEN：

```bash
gofmt -w internal/render/hud/atlas.go internal/render/hud/atlas_test.go internal/render/hud/hunger_test.go
go test ./internal/render/hud -run 'TestHotbar(TextureAtlas|ColumnUV)|TestHungerBar' -count=1
```

Expected: 全部通过，且 deterministic atlas 测试连续构建结果逐字节一致。

#### Step 7: 先写三列共享几何与饥饿呈现的失败测试

- [ ] 在 `internal/render/hud/health_test.go`、`oxygen_test.go`、`hunger_test.go` 和 `layout_test.go` 写最终布局测试，至少包含这些可观察断言：

  - 状态组宽度严格为 `(169*3 + 12*2) * hudScale`，整体水平居中。
  - health、oxygen、hunger 分别占左、中、右列；列内不重叠。
  - `oxygen.Confirmed && oxygen.Value == core.MaxOxygenTicks` 时 oxygen quad 数为 0，但 health/hunger 的 X 坐标与 depleted oxygen 情况逐项相同。
  - closed 状态行在 hotbar 上方，mining 在状态行上方；open 状态行在 hotbar 下方。
  - open 状态行不与 36 个 `InventorySlotAt` 命中矩形或 10 行配方重叠。
  - `HungerOverlay{Confirmed:false}` 为 0 quad；已确认值钳制到 `core.MaxHunger`。
  - 饥饿值 `0, 1, 2, 9, 19, 20, 255` 分别得到 10 个空槽加 `ceil(value/2)` 个 fill，奇数末格只填右半；绘制顺序从右向左。
  - health/oxygen 各自不超过 10 quad，hunger 不超过 20 quad。

建议新增精确测试名：

```text
TestStatusColumnsStayCenteredWhenFullOxygenHides
TestStatusColumnsUseExactDesignGeometry
TestOpenStatusRowAvoidsInventoryHitCells
TestHungerBarUsesConfirmedClampedTwoPointSegments
```

- [ ] 运行 RED：

```bash
go test ./internal/render/hud -run 'Test(Health|Oxygen|Hunger|Status|Responsive|InventorySlotAt)' -count=1
```

Expected: 旧的独立左右锚点、旧 hunger mirror 或 open 状态缺失导致坐标、quad 数或半格断言失败。

#### Step 8: 最小实现三列 531 design-pixel 状态组

- [ ] 在 `internal/render/hud/layout.go` 增加一个共享几何 helper，不增加布局对象层级：

```go
const (
	statusColumnWidth = healthSegmentCount*healthHeartSize +
		(healthSegmentCount-1)*healthHeartGap // 169
	statusColumnGap = float32(12)
	statusGroupWidth = statusColumnWidth*3 + statusColumnGap*2 // 531
)

func statusColumnOrigin(column int, open bool, width, height float32) (x, y, scale float32)
```

`statusColumnOrigin` 必须复用 `hotbarRowBounds`/现有 `hudScale`，只计算三列组的居中 X 与 open/closed 共用 Y；不得创建第二套 scale 或复制 hotbar 几何公式。`column` 只允许 0、1、2；错误值应沿用本包内部不产生布局的安全行为。

- [ ] 修改 `appendHealthBar`、`appendOxygenBar` 使用 column 0/1；修改 main 带入的 hunger 接口为：

```go
func appendHungerBar(
	dst *hotbarLayout,
	hunger HungerOverlay,
	open bool,
	width, height float32,
)
```

并使用 column 2。饥饿在右列从右向左排 10 格；empty 先画，fill 后画；奇数末格只改变 U 范围和屏幕宽度，不引入新纹理 cell。

- [ ] 保持 `HotbarRenderer.Prepare` 的 main 参数顺序：

```go
func (r *HotbarRenderer) Prepare(
	state State,
	mining MiningOverlay,
	health HealthOverlay,
	oxygen OxygenOverlay,
	hunger HungerOverlay,
	chat ChatOverlay,
	width, height float32,
) ([]byte, uint32)
```

在 `cmd/mornlea/app_frame.go` 保留 main 的权威读取：

```go
hunger, hungerReady := a.predictor.Hunger()
// Prepare 参数中的 hunger：
hud.HungerOverlay{Confirmed: hungerReady, Value: hunger}
```

注释必须说明 UI 只画权威确认值；不得让预测器之外的客户端逻辑推导饥饿。

- [ ] 运行 GREEN：

```bash
gofmt -w internal/render/hud/layout.go internal/render/hud/health.go \
  internal/render/hud/oxygen.go internal/render/hud/hunger.go \
  internal/render/hud/health_test.go internal/render/hud/oxygen_test.go \
  internal/render/hud/hunger_test.go internal/render/hud/layout_test.go \
  internal/render/hud/renderer.go cmd/mornlea/app_frame.go
go test ./internal/render/hud -run 'Test(Health|Oxygen|Hunger|Status|Responsive|InventorySlotAt)' -count=1
```

Expected: 全部通过；满氧气隐藏前后 health/hunger 坐标完全一致。

#### Step 9: 先写容量、聊天和 app 权威数据流的失败测试

- [ ] 手工语义合并并扩充 `internal/render/hud/renderer_test.go`、`hotbar_test.go`、`cmd/mornlea/health_hud_test.go` 与 `cmd/mornlea/chat_test.go`。保留双方现有测试，新增/调整以下精确契约：

```go
const (
	maxHotbarQuads       = 267
	hotbarGlyphCapacity  = 700
	hotbarGlyphOffset    = 13312
	hotbarUploadCapacity = 46912
)
```

  - 固定 instance size 为 48 bytes，所有固定 offset `% 256 == 0`。
  - `TestHotbarFixedUploadLayoutMatchesScenarioVersion` 同时钉死 scenario v19 与上述布局。
  - closed 最大实例数严格 96，open 最大实例数严格 265；不是只验证“小于容量”。
  - `TestHotbarPrepareReusesLayoutAndUploadStorage` 保持零热路径 allocation/资源复用契约。
  - `TestHotbarMaximumBranchesAndEncodingContract` 同时覆盖 health 10、depleted oxygen 10、hunger 20 和现有最大分支。
  - `TestChatOverlayStaysInFramebufferAndAboveClosedSurvivalStatus` 把三列状态组作为 anchor，仍要求 `1/256` framebuffer-pixel slack。
  - app 测试让权威 hunger 9 产生 10 health + 15 hunger quad；unconfirmed hunger 只保留 health 10，不能画 20 个空鸡腿。
  - depleted oxygen 的增量固定为 10 quad，不沿用 main 的旧气泡计数。
  - 保留 `TestFormatChatEventUnknownKindFallsBackToNeutralLine`。

- [ ] 运行 RED：

```bash
go test ./internal/render/hud ./cmd/mornlea \
  -run 'Test(Hotbar|ChatOverlay|HUDHunger|ApplicationRenders)' -count=1
```

Expected: 旧 247/12288/45888 容量、旧 closed/open 最大值、旧 main status 计数或缺少 hunger 参数的调用点导致失败。

#### Step 10: 最小合并 renderer、容量、调用点和长期基线文档

- [ ] 在 `internal/render/hud/renderer.go` 把 hunger 作为现有 layout append 的第五种状态输入；只扩大静态 CPU/GPU 上传布局到 scenario v19 常量，不增加 pass、binding、buffer、shader 或每帧资源。

- [ ] 更新所有 `HotbarRenderer.Prepare` 调用点和测试 helper；使用命名的测试常量表达 health/hunger/oxygen instance 数，避免裸数把 10-quad 与 20-quad 契约混在一起。

- [ ] 手工语义合并 `docs/notes/progress.md`，同时保留 Pixel Perfection HUD 完成记录和 authoritative hunger 的协议 v24/schema v7/scenario v19 记录。

- [ ] 手工更新 `AGENTS.md` 后逐字节同步到 `CLAUDE.md`。长期基线必须同时陈述最终 v24/v7/v19、七 cell atlas、三列 HUD 和 15 场景顺序，不保留已经失真的否定式能力描述。

- [ ] 运行 GREEN 与架构门禁：

```bash
gofmt -w internal/render/hud/renderer.go internal/render/hud/renderer_test.go \
  internal/render/hud/hotbar_test.go cmd/mornlea/health_hud_test.go \
  cmd/mornlea/chat_test.go cmd/mornlea/app_frame.go
go test ./internal/render/hud ./cmd/mornlea \
  -run 'Test(Hotbar|ChatOverlay|HUDHunger|ApplicationRenders)' -count=1
cmp -s AGENTS.md CLAUDE.md
go test ./internal/archcheck -count=1
```

Expected: 全部通过；`cmp` 返回 0。

#### Step 11: 先写 capture fixture 和 15 场景的失败测试

- [ ] 修改 `cmd/mornlea/capture_hud_test.go`，先把 fixture 形状固定为：

```go
type captureHUDFixture struct {
	Health uint8
	Oxygen uint16
	Hunger uint8
	Mining hud.MiningOverlay
}
```

`applyCaptureHUDFixture` 必须通过 `network.PlayerState` 同时灌入 health/oxygen/hunger，保存并恢复整个 predictor snapshot 与 mining overlay；测试必须覆盖成功 defer、错误返回和对同一 app 重复 apply/restore 三条路径。

- [ ] 固定三个 HUD capture 的值与断言：

```text
hud-hotbar-health:       Health=core.MaxHealth, Oxygen=core.MaxOxygenTicks, Hunger=core.MaxHunger
hud-survival-feedback:  Health=5, Oxygen=core.MaxOxygenTicks/3, Hunger=9, worn tool, blocked mining=4/9
inventory-crafting:     Health=5, Oxygen=core.MaxOxygenTicks/3, Hunger=9, inventory open
```

`inventory-crafting` 必须继续断言 36 个 `InventorySlotAt` cell 与 10 行配方；不得添加产品态注入接口。

- [ ] 在场景顺序测试中逐项断言 15 项精确顺序，并保证 `water-underwater` 是最后一项、`far-horizon` 是倒数第二项、`water-surface-slope` 紧邻其前。运行 RED：

```bash
go test ./cmd/mornlea \
  -run 'Test(HUDCapture|CaptureSceneOrder|WaterUnderwater|FarHorizon)' -count=1
```

Expected: fixture 未持久/恢复 hunger、HUD 场景值不正确或场景清单不完整时失败。

#### Step 12: 最小实现 capture fixture，保持最终场景顺序

- [ ] 修改 `cmd/mornlea/capture.go`，只扩展现有测试态 capture fixture 调用：hotbar/full、survival/5-third-9、inventory/5-third-9。沿用 predictor 的权威 `network.PlayerState` 输入和已有 restore closure；错误路径必须 defer 同一个 restore。

- [ ] 检查 `hud-survival-feedback` 仍包含 worn tool 与 blocked mining 4/9；检查 `ai-companion` 的 predictor fixture 也不会把 hunger 泄漏到后续场景。

- [ ] 运行 GREEN：

```bash
gofmt -w cmd/mornlea/capture.go cmd/mornlea/capture_hud_test.go
go test ./cmd/mornlea \
  -run 'Test(HUDCapture|CaptureSceneOrder|WaterUnderwater|FarHorizon)' -count=1
```

Expected: 全部通过；重复 restore 是幂等的，失败路径后 predictor/mining/hunger 与进入 fixture 前逐项一致。

#### Step 13: 完成 focused precommit 验证

- [ ] 运行所有受影响包 race 测试：

```bash
go test ./internal/render/hud ./cmd/mornlea -race -count=1
go test ./internal/archcheck -count=1
```

- [ ] 运行 scenario v19 与 perfcheck 的精确契约组：

```bash
go test ./internal/render/hud ./cmd/mornlea ./cmd/perfcheck \
  -run 'Test(HotbarFixedUploadLayoutMatchesScenarioVersion|HotbarLayoutStaysWithinFixedCapacity|ScenarioV19ContainsSevenSortedUnicodeRemotePlayers|BenchmarkScenarioV19AccountsForCompanionRendererUploadLayout|BenchmarkScenarioVersionIncludesStaticBlockLightWorkload|PerfcheckOnlyAuthorizesScenarioV18ToV19|PerfcheckV19PerformanceRegressionIsRecordOnly)' \
  -count=1
```

- [ ] 运行 capture focused 组和机械门禁：

```bash
go test ./cmd/mornlea \
  -run 'Test(HUDCapture|CaptureSceneOrder|WaterUnderwater|FarHorizon)' -count=1
gofmt -l .
cmp -s AGENTS.md CLAUDE.md
openspec validate --all --strict --no-interactive
git diff --check
```

Expected: 全部成功；`gofmt -l .` 无输出。把每条命令、结果和时间追加到 ledger；此时仍不提交。

#### Step 14: 用最终合并二进制重生成全部 15 张 golden

- [ ] 建立输出目录并一次性重生成正式 golden：

```bash
mkdir -p build
make visual-update \
  VISUAL_OUT=build/visual-bedrock-survival-hud-main-integration-update
```

Expected: 命令从最终未提交 merge 构建 capture，并更新 15 张正式 PNG；不得从任一 parent 复制冲突 PNG。

- [ ] 核对输出与正式 golden 均恰好 15 张，并与固定场景集合一一对应：

```bash
find build/visual-bedrock-survival-hud-main-integration-update \
  -maxdepth 1 -type f -name '*.png' -print | sort
find cmd/mornlea/testdata/golden \
  -maxdepth 1 -type f -name '*.png' -print | sort
```

- [ ] 用图片查看工具逐张人工检查，逐项把结论追加到 ledger：

| 场景 | 必须看到且必须保持的最终语义 |
|---|---|
| `terrain-noon` | 默认注水世界、天空、云、Pixel Perfection 已映射材质与程序化 fallback 均正常 |
| `hud-hotbar-health` | 10 个满心、20 饥饿、满氧气隐藏；三列组仍整体居中，hotbar/选中框正常 |
| `hud-survival-feedback` | 生命 5、氧气三分之一、饥饿 9；磨损工具和受阻采掘 4/9 都可辨识且不重叠 |
| `avatar-nametag` | 远端玩家 silhouette、name tag、遮挡与背景保持当前分支结果 |
| `inventory-crafting` | 三列状态在 hotbar 下方；36 格交互单元和 10 行配方无覆盖、无错位 |
| `debug-panel` | debug 字形、行距、背景和边缘留白完整，无裁切 |
| `skylight-tunnel` | 天空光传播、隧道明暗层次和方块边界保持稳定 |
| `block-light-room` | 静态方块光范围、强度层次与遮挡保持稳定 |
| `materials-showcase` | 全部自然/加工/农业/水材质和 alpha-cutout 植物正常，无 atlas 列偏移 |
| `target-block-feedback` | 目标轮廓、方块名与遮挡正确，inventory open 时不出现错误反馈 |
| `oak-grove` | 确定性橡树、树叶 cutout、地表和远景正常 |
| `ai-companion` | 伙伴 body/name tag、聊天输入与模型台词行保持分离，位置与遮挡正确 |
| `water-surface-slope` | 斜水面角高、透明排序、岸线和水下边界无退化 |
| `far-horizon` | LOD shell、海平面钳制、裙边和 near-band 控制均保留；场景倒数第二 |
| `water-underwater` | 水下 tint、可见半径、氧气 HUD 与水面边界正常；场景最后 |

任一视觉问题都必须回到未提交 merge 修复、重跑相应 focused 测试并重新生成全部 15 张，不能调阈值掩盖。

#### Step 15: 运行一次 final visual check 与 benchmark producer

- [ ] 运行最终 capture comparison：

```bash
make visual-check \
  VISUAL_OUT=build/visual-bedrock-survival-hud-main-integration-final
```

Expected: 15 场景全部通过既有阈值，报告完整，无 I/O/overflow/data-loss 错误。

- [ ] 运行 scenario v19 benchmark producer；性能数值只记录，不作为退出门禁：

```bash
go run ./cmd/mornlea --benchmark \
  --benchmark-transport memory \
  --perf-output build/perf-bedrock-survival-hud-main-integration-v19.json
```

Expected: 输出报告身份为 scenario v19，remote players 为七个排序后的 Unicode 名称，companion renderer 与静态 block light 工作量存在；把结果文件路径和记录值追加到 ledger。

#### Step 16: 运行整分支终审前唯一一次 full gate

- [ ] 严格按仓库顺序运行，整分支只跑这一次 `go test ./... -race`：

```bash
make rust
make rust-check
go test ./... -race
go vet ./...
mkdir -p build
go run ./cmd/mornlea --benchmark \
  --benchmark-transport memory \
  --perf-output build/perf-bedrock-survival-hud-main-integration-v19.json
make visual-check \
  VISUAL_OUT=build/visual-bedrock-survival-hud-main-integration-final
openspec validate --all --strict --no-interactive
gofmt -l .
git diff --check
```

Expected: 所有命令成功，`gofmt -l .` 无输出；performance 数值只记录，报告完整性、真实 overflow、数据丢失和 I/O 错误仍是门禁。任一失败都在同一未提交 merge 修复后重跑受影响 focused 命令；若修复触及 full-gate 覆盖的横向契约，再完整重跑本步并在 ledger 解释原因。

#### Step 17: 完成 OpenSpec 证据、精确暂存与原子 merge commit

- [ ] 只有对应证据已存在时，才在 `openspec/changes/bedrock-survival-hud/tasks.md` 勾选 6.2–6.12；6.13 在 SDD closeout 的 review、repair、whole-branch final review 与 normal push 全部有证据前保持未勾选。

- [ ] 在 ledger 追加：全部 RED/GREEN、冲突裁决、15 张视觉人工结论、focused/full gate、benchmark 报告、golden 重生成方式；记录没有 rebase、force-push、whole-file source/docs choice 或 parent PNG choice。

- [ ] 对最终文本再次排除 marker，验证基线文档一致，并确认没有未知 unmerged path：

```bash
rg -n '^(<<<<<<<|=======|>>>>>>>)' . \
  --glob '!build/**' --glob '!.git/**'
cmp -s AGENTS.md CLAUDE.md
git diff --name-only --diff-filter=U
```

Expected: 前两条成功且 `rg`/unmerged 列表无输出；若 binary paths 仍是 unmerged，必须确认 15 张刚由最终二进制生成后再 `git add`。

- [ ] 只暂存最终源码、文档、OpenSpec 证据和 15 张正式 golden；不得暂存 `build/` 或 `.superpowers/sdd/` 报告。用 `git status --short` 和 cached diff 逐项检查：

```bash
git add AGENTS.md CLAUDE.md cmd/mornlea docs/notes/progress.md \
  internal/render/hud openspec/changes/bedrock-survival-hud
git diff --name-only --diff-filter=U
git status --short
git diff --cached --check
git diff --cached --stat
```

Expected: 无 unmerged path；`build/` 与 `.superpowers/sdd/` 不在 cached diff；自动合入的 main 权威文件仍保留在 merge index 中。

- [ ] 再核对所有必需检查的最新 ledger 证据，然后创建唯一原子 merge commit：

```bash
git commit
```

Commit message subject:

```text
Merge origin/main into bedrock-survival-hud
```

Expected: `git rev-list --parents -n 1 HEAD` 显示两个 parent，第二个 parent 是 `f9808f4c0c07e17b084a1a5a91ab97f2f925e407`；从 merge 开始到此处没有中间 commit。

---

## SDD Closeout

本节是 controller 的 review/repair/final-review/push 编排，不是第二个产品实现任务。

- [ ] 记录 merge commit 并生成 task review package。BASE 必须是 Step 1 记录的 `integration_base`，HEAD 是 merge commit；复用现有 helper，不创建新脚本：

```bash
plan_file=docs/superpowers/plans/2026-08-22-bedrock-survival-hud-main-integration.md
review_out=.superpowers/sdd/bedrock-survival-hud-main-integration/implementation-review-package.diff
review_tmp=$(mktemp)
/Users/chen/.agents/skills/subagent-driven-development/scripts/review-package \
  "$plan_file" "$integration_base" HEAD "$review_tmp"
raw_diff_sha=$(git diff "$integration_base..HEAD" | shasum -a 256 | awk '{print $1}')
{
  sed -n '1p' "$review_tmp"
  printf '\nRaw diff SHA-256: %s\n' "$raw_diff_sha"
  sed -n '2,$p' "$review_tmp"
} > "$review_out"
rm "$review_tmp"
package_sha=$(shasum -a 256 "$review_out" | awk '{print $1}')
printf 'raw_diff_sha=%s\npackage_sha=%s\n' "$raw_diff_sha" "$package_sha"
```

Package 必须包含 BASE..HEAD commit list、stat、full diff、raw diff SHA-256；package SHA-256 记录在 task brief 与 ledger，不能把文件自身 hash 嵌入文件。

- [ ] controller 派发一个 fresh task reviewer，task brief 是唯一需求来源；reviewer 必须基于 package 和工作树分别给出 `SPEC PASS/FAIL` 与 `QUALITY PASS/FAIL`，并检查 15 张视觉人工证据、原子 merge、版本/容量、OpenSpec checkbox 与 ledger 一致性。

- [ ] 有 finding 时，把每项 finding 原文和裁决追加 ledger，派回同一 implementer 修复；修复形成 merge commit 之后的追加 commit，不改写 merge commit。每轮后重跑受影响 focused 验证、更新 ledger、重新生成同一 BASE..HEAD package/hash，并交同一 reviewer 复核；最多 5 轮，超限逐条交 controller 裁决。

- [ ] task reviewer 最终双 PASS 后，把 implementer、reviewer、轮次、最终 commit、验证和 ruling 写入 ledger。随后派一个与 task reviewer 不同的 fresh whole-branch reviewer，使用更新后的 BASE..HEAD package 做整分支终审。

- [ ] whole-branch reviewer 的 finding 仍进入同一有界 repair loop；最终双 PASS 后才正常 push PR #64 分支。禁止 rebase、force-push 或改写已审核历史。

- [ ] 6.13 只有在它描述的 task review、全部 repairs、whole-branch final review 和 normal push 都已有 ledger 证据时才能勾选。若 push 发生在证据提交之后，controller 必须用一个纯 bookkeeping commit 记录实际 push 结果、勾选 6.13、运行 `openspec validate --all --strict --no-interactive` 与 `git diff --check`，并正常 push；不得伪造未来证据。

- [ ] 最终向用户报告：merge/fix/bookkeeping commit、两级 reviewer 结论、repair 轮次、full gate、15 场景 visual check、benchmark v19 报告路径、raw/package SHA-256、PR #64 normal push 结果和仍存在的 concerns。不要归档 change。
