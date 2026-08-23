# 容器 UI 视觉对齐实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变背包、合成、箱子和熔炉行为的前提下，用原创程序化像素元素把四类打开态界面统一为高对比框体、凹槽栏位、中文标题和明确的熔炉火焰/进度构图，并新增箱子与熔炉正式视觉门禁。

**Architecture:** 继续只在 `internal/render/hud` 的 Go CPU 半部生成固定容量实例；用现有 HUD atlas 的程序化 cell 承载凹槽、三个双字标题和熔炉图标，避免动态 glyph、PNG、shader、pass 或 ABI 变化。绘制与命中继续共用现有 origin 函数，`cmd/mornlea` 只增加确定性 capture 夹具。

**Tech Stack:** Go 1.26、现有 HUD quad/glyph 编码、程序化 RGBA atlas、Rust `mornlea_client` 既有 HUD pass、OpenSpec、PNG golden。

**Spec:** `docs/superpowers/specs/2026-08-22-five-way-parallel-wave-design.md` §4、§3、§9。

## Global Constraints

- [ ] 仅在 `bedrock-survival-hud` 已归档后的 main 上执行；记录 `BASE=$(git rev-parse main)`，从该提交创建独立 worktree、分支 `codex/container-ui-visual-alignment` 和同名 OpenSpec change。
- [ ] 产品文件领地限于 `internal/render/hud/container.go`、`atlas.go` 及其测试、`cmd/mornlea/capture*.go`、capture 测试与 17 张正式 golden；不改网络、模拟、存档、Rust、长期基线文档或其他并行 change 的文件。
- [ ] 保持协议 v24、玩家 schema v7、benchmark scenario v19、engine ABI v6、client ABI v7；保持 267 quad、700 glyph、glyph offset 13312、总上传 46912 bytes、48-byte instance 与 256-byte 对齐。
- [ ] 所有视觉均为代码生成的原创像素；不得复制或提交 Mojang 像素、PNG UI 源资产或新依赖。
- [ ] 每个任务使用全新 implementer；候选提交后由独立 reviewer 同时做 SPEC 与 QUALITY 裁决，finding 仅以追加提交修复，最多 5 轮，并把证据写入 `openspec/changes/container-ui-visual-alignment/ledger.md`。

---

## Task 1: 建立独立 OpenSpec change 和当前行为红线

**Files:**
- Create: `openspec/changes/container-ui-visual-alignment/.openspec.yaml`
- Create: `openspec/changes/container-ui-visual-alignment/proposal.md`
- Create: `openspec/changes/container-ui-visual-alignment/design.md`
- Create: `openspec/changes/container-ui-visual-alignment/tasks.md`
- Create: `openspec/changes/container-ui-visual-alignment/ledger.md`
- Create: `openspec/changes/container-ui-visual-alignment/specs/container-ui-presentation/spec.md`
- Create: `openspec/changes/container-ui-visual-alignment/specs/visual-verification/spec.md`

- [ ] proposal 只包含视觉换肤与两场景；明确 36/39/63 统一栏位、10 配方、两次点击整堆移动、权威边界和所有版本不变。
- [ ] delta specs 覆盖原创像素框、凹槽、标题、来源轮廓、熔炉图示、命中不变和固定资源；把视觉场景数改为 17，并锁新增场景位置和尾序。
- [ ] design 记录最小实现：凹槽和标题进入程序化 HUD atlas；每个 overlay 只增加一个标题 quad，标题不进入 700 glyph 流；现有 panel/slot/bar quad 原位换 UV/颜色。
- [ ] tasks 映射本计划，ledger 记录共享基线 SHA、implementer/reviewer、裁决和验证。
- [ ] 运行并提交：

```bash
openspec validate container-ui-visual-alignment --strict --no-interactive
git diff --check
git add openspec/changes/container-ui-visual-alignment
git commit -m "docs(openspec): plan container UI alignment"
```

## Task 2: 用程序化 atlas 建立原创容器像素基元

**Files:**
- Modify: `internal/render/hud/atlas.go`
- Modify: `internal/render/hud/atlas_test.go`
- Modify: `internal/render/hud/container.go`
- Modify: `internal/render/hud/container_test.go`

- [ ] 先写失败测试，固定以下新 cell 排在物品列之前，且每格 16×16、alpha 只取 0/255、重复构建逐字节一致、互相不相同：凹槽、`合成`、`箱子`、`熔炉`、火焰、进度箭头。
- [ ] 同一测试遍历全部 `core.ItemID`，证明 `hotbarBlockColumnOffset` 后移后每个可放置物品 cell 仍逐像素等于 registry 顶面。
- [ ] 运行红测：

```bash
make rust
go test ./internal/render/hud -run 'TestHotbarAtlas.*(Container|Item)|TestContainerPixelCells' -count=1
```

预期：新列常量和 painter 尚不存在，测试编译失败。

- [ ] 在 `atlas.go` 只追加固定列和小型 painter，保持既有七个生存图标原顺序：

```go
const (
	hotbarEmptyHeartColumn = iota
	hotbarHalfHeartColumn
	hotbarFullHeartColumn
	hotbarEmptyBubbleColumn
	hotbarFullBubbleColumn
	hotbarEmptyDrumstickColumn
	hotbarFullDrumstickColumn
	hotbarContainerSlotColumn
	hotbarCraftingTitleColumn
	hotbarChestTitleColumn
	hotbarFurnaceTitleColumn
	hotbarFurnaceFlameColumn
	hotbarFurnaceArrowColumn
	hotbarBlockColumnOffset
)
```

- [ ] 用 `[7]string` 掩码和一个 `paintPixelMask` helper 画三个双字标题；每字 7×7、两字并排放入一个 16×16 cell。用浅边、面色、深边三层像素画凹槽；火焰与箭头用固定整数像素，不建通用 sprite registry。
- [ ] 在 `container.go` 将 inventory/recipe/chest/furnace slot 表面 quad 改用同一凹槽 UV；保留 item tile、数量、耐久、栏位 origin 和所有 hit-test 原样。
- [ ] 格式化、复绿并提交：

```bash
gofmt -w internal/render/hud/atlas.go internal/render/hud/atlas_test.go internal/render/hud/container.go internal/render/hud/container_test.go
go test ./internal/render/hud -run 'TestHotbarAtlas|TestContainer' -race -count=1
git diff --check
git commit -am "feat: add original container pixel primitives"
```

- [ ] 完成 task 双裁决和 ledger 记录。

## Task 3: 重绘合成、箱子和熔炉而不改变交互几何

**Files:**
- Modify: `internal/render/hud/container.go`
- Modify: `internal/render/hud/container_test.go`
- Modify: `internal/render/hud/layout.go`
- Modify: `internal/render/hud/layout_test.go`
- Modify: `internal/render/hud/renderer_test.go`

- [ ] 先写布局失败测试：三种 overlay 各恰好一个标题 quad；标题使用对应 atlas cell；slot 共用凹槽 cell；来源轮廓仍包住对应 origin；熔炉两个填充 quad 分别使用火焰/箭头 cell。
- [ ] 新增表驱动命中回归，遍历 36/39/63 个栏位中心和边界外 1px；绘制 origin 必须由 `InventorySlotAt`、`FurnaceSlotAt`、`ChestSlotAt` 命中同一统一索引。
- [ ] 保留并扩充配方测试：10 个 `RecipeButtonAt` 中心仍返回原 `RecipeID`，不可合成按钮、空白区域和零 framebuffer 不产生交互。
- [ ] 运行红测：

```bash
go test ./internal/render/hud -run 'Test(ContainerTitles|ContainerSlotGeometry|Furnace.*Composition|RecipeButton)' -count=1
```

- [ ] 复用每类 overlay 现有面板 quad，改为高对比框体配色；每个 `append*` 末尾追加一个标题 quad。不要引入主题或组件树。
- [ ] 新增唯一共享几何 `containerTitleSize=16`、`containerTitleGap=4`、`containerHeaderHeight=20`：标题放在各 overlay 面板左上，位于首行内容上方；面板只向上扩出 header，slot origin 不另写偏移。`openHUDHeight` 增加同一 header 高度，极窄/极矮 framebuffer 继续由统一 `hudScale` 缩放，绘制与命中仍共用该 scale。
- [ ] 只把三项容量各加一个标题 quad：

```go
recipeQuads  = 1 + len(inventoryRecipeIDs)*9 + 1
furnaceQuads = 1 + 3 + 3*2 + 4 + 1
chestQuads   = 1 + core.ChestSlots + core.ChestSlots*2 + 1
```

- [ ] 标题不走动态 glyph；更新容量见证为打开态合法最坏 266、上限 267，`maxHotbarGlyphs` 仍为 700，offset 和总字节不变。
- [ ] 熔炉只重组已有四个 bar quad：两块底轨保持纯色；燃烧填充用火焰 UV 自下向上裁剪，熔炼填充用箭头 UV 自左向右裁剪。实例尺寸与 UV 端点按同一 fraction 同步变化，禁止把整张图标压缩到剩余宽高；权威分数和钳制不变。
- [ ] 格式化、复绿并提交：

```bash
gofmt -w internal/render/hud/container.go internal/render/hud/container_test.go internal/render/hud/layout.go internal/render/hud/layout_test.go internal/render/hud/renderer_test.go
go test ./internal/render/hud -race -count=1
go test ./internal/archcheck -count=1
git diff --check
git commit -am "feat: align container UI presentation"
```

- [ ] 完成 task 双裁决和 ledger 记录。

## Task 4: 新增箱子/熔炉 capture 并更新 17 张 golden

**Files:**
- Modify: `cmd/mornlea/capture.go`
- Modify: `cmd/mornlea/capture_ai_companion_test.go`
- Modify: `cmd/mornlea/capture_hud_test.go`
- Modify: `cmd/mornlea/capture_scene_test.go`
- Create: `cmd/mornlea/testdata/golden/chest-container.png`
- Create: `cmd/mornlea/testdata/golden/furnace-container.png`
- Modify: `cmd/mornlea/testdata/golden/*.png`

- [ ] 先写失败测试，精确锁定 17 个场景：

```go
wantNames := []string{
	"terrain-noon", "hud-hotbar-health", "hud-survival-feedback", "avatar-nametag",
	"inventory-crafting", "chest-container", "furnace-container", "debug-panel",
	"skylight-tunnel", "block-light-room", "materials-showcase", "target-block-feedback",
	"oak-grove", "ai-companion", "water-surface-slope", "far-horizon", "water-underwater",
}
```

- [ ] `chest-container` 构造已确认空/满混合的 27 格 `network.ChestState`、36 格背包和 `inventorySource=36`；`furnace-container` 构造 raw iron/coal/ingot、`ProgressTicks=73`、`BurnTicks=911` 和 `inventorySource=37`。
- [ ] 两场景显式 reset 另一容器、远端实体、面板、相机和世界时间；测试镜像种类、统一来源索引、状态值和互斥 reset。
- [ ] 运行红测：

```bash
go test ./cmd/mornlea -run 'Test(CaptureSceneOrder|ContainerCapture|WaterUnderwater|FarHorizon)' -count=1
```

- [ ] 最小注册两个场景并复绿；不增加产品测试 API，不修改视觉阈值。
- [ ] 在 Metal 环境生成和验证：

```bash
make visual-update VISUAL_OUT=build/visual-container-ui-update
make visual-check VISUAL_OUT=build/visual-container-ui-check
```

- [ ] 人工逐张检查 17 张结果并写入 ledger，重点确认三类中文标题、栏位凹槽/来源框、熔炉图示、内容不重叠，以及世界/水/LOD 场景没有无关漂移。
- [ ] 提交 `git add cmd/mornlea && git commit -m "test: lock container UI visuals"`，完成双裁决。

## Task 5: 全量验证和整分支终审

**Files:**
- Modify: `openspec/changes/container-ui-visual-alignment/tasks.md`
- Modify: `openspec/changes/container-ui-visual-alignment/ledger.md`

- [ ] 对账勾选项与证据，确认产品 diff 未越出 UI/capture 领地，协议/存档/ABI/scenario 常量均未变化。
- [ ] 运行最终门禁：

```bash
make rust
make rust-check
go test ./internal/render/hud ./cmd/mornlea -race -count=1
go test ./internal/archcheck -count=1
go test ./... -race
go vet ./...
gofmt -l .
make visual-check VISUAL_OUT=build/visual-container-ui-final
openspec validate --all --strict --no-interactive
git diff --check
```

- [ ] `gofmt -l .` 必须无输出；17 张视觉检查必须全绿；记录耗时但不改阈值。
- [ ] 提交 ledger/tasks：`git commit -am "docs: record container UI verification"`。
- [ ] 生成 `BASE..HEAD` committed review package 和 SHA-256，交给全新 reviewer 做整分支 SPEC/QUALITY 终审；修复循环不超过 5 轮。
- [ ] 正常 push 并创建独立 PR；不得自行归档，等待五路合并后的串行归档。
