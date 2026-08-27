# Tasks

## 1. core 登记、单一光源表与火把配方

- 目标文件/包：`internal/core`（`block.go`、`block_name.go`、`item.go`、`item_test.go`、`block_test.go`、`recipe.go`、`recipe_test.go`、新建 `block_properties.go`、`block_properties_test.go`）；`internal/assets/blocks.go`、`blocks_test.go`。
- 验证命令：`gofmt -w internal/core internal/assets`；`go test ./internal/core ./internal/assets -race -count=1`。
- [x] 契约产物已按 2026-08-27 裁决修订（proposal/design/tasks/ledger 与五份 delta；控制会话完成，docs-only 提交随审后进行）。
- [x] RED：穷举 0..<`BlockIDMax` 的 `BlockEmission`（发光方块 15、五种火把 14、其余 0、越界 0）与 `BlockLightAttenuation`（八流体 1、其余 0）；五种火把非 opaque/零碰撞/可被射线瞄准；`ItemTorch`=43 堆叠 64 且不进食物/工具表；`BlockDrop` 五形态均掉回一个火把；`PlaceableBlockAtFace` 逐面映射与立方体物品不随面变化；`RecipeTorch`=14 形状为「煤炭位于木棍正上方」产出 4 火把、recipe 1..13 语义不变、查询 15 及以上稳定拒绝。
- [x] GREEN：core 追加火把枚举 62..66、`ItemTorch`=43、两张光源表、`PlaceableBlockAtFace`、`BlockDrop` 映射与 `RecipeTorch`；assets 的 `Emission`/`LightAttenuation` 转调 core 并删除重复 switch。
- [x] `go test ./internal/core ./internal/assets -race -count=1` 与 `go test ./internal/archcheck -count=1` 通过后，双评审（规格合规 + 代码质量）并提交 `feat: register torch blocks, items and recipe`；结论记入 ledger。

## 2. sim 支撑放置、六邻居失效与掉落

- 目标文件/包：`internal/sim`（`engine_placement.go`、`engine_changes.go`、`block.go`、`engine_step.go`、`drop.go`、`drop_test.go`、`placement_success_test.go`、新建 `torch.go`、`torch_test.go`）；`internal/world/chunk.go`（如需支撑格查询辅助）；`internal/physics/types.go`（`BlockCollisionBoxes` 五形态火把零碰撞——Task 1 交付的 `core.IsTorch` 谓词在此接线，含对应 physics 定点测试）。
- 验证命令：`gofmt -w internal/sim internal/world internal/physics`；`go test ./internal/sim ./internal/world ./internal/physics -race -count=1`；`go test ./internal/server -race -count=1`（physics 零碰撞的一致性消费者：伙伴寻路可通过表由 collision oracle 测试逐块守护）。
- [x] RED：五向放置成功与拒绝路径（底面/无支撑/非实心/流体/未加载/玩家占位）逐条断言拒绝不扣料；成功路径同 tick 原子扣物品并写方块、广播走既有变更路径；采掘/流体替换/作物替换等任何权威变化撤销支撑后，仅移除依赖火把并掉落一个火把、共享 revision/broadcast、非支撑邻居不变；火把经 A-01 网格（2×2 摆放「煤上棍下」）合成一次恰得 4 个火把；五形态火把零碰撞（`BlockCollisionBoxes` 恒空）。
- [x] GREEN：`torchSupport` 唯一形态→支撑映射；`executePlacement` 火把分支（世界写入前校验）；`finishChanges` 前对本 tick 已变位置排序去重、精确六邻居复核、`recordChange` 写空气 + 既有掉落 append；火把合成零新分支（由 task 1 的配方经既有匹配/取出路径生效，此处只加验收测试）。
- [x] `go test ./internal/sim ./internal/world ./internal/physics -race -count=1` 通过后，双评审并提交 `feat: enforce torch support`；结论记入 ledger。

## 3. Rust 有限模型与 engine ABI v8

- 目标文件/包：`internal/mesh`（`native_input.go`、`native_abi.go`、`registry.go`、`quad.go`（交叉斜面编组通用化的单向断言，见 design）、`native_input_test.go`、`native_abi_test.go`、`native_capacity_test.go`、`native_parity_test.go`、`greedy_test.go`、`plant_test.go`（单向断言同步）、新建 `torch_test.go`）；`internal/nativeabi`（`native.go`、`native_test.go`）；`engine/crates/mornlea_engine`（`src/input.rs`、`src/greedy/mod.rs`、新建 `src/greedy/torch.rs`、`src/greedy/torch_tests.rs`、`src/ffi.rs`）；`engine/crates/mornlea_client`（`shaders/terrain.wgsl`、`shaders/cull.wgsl` 注释、`src/render/shaders.rs` 的 `TORCH_MATERIAL`、`src/render/farmland_tests.rs` pin 扩展——角高度解码半边，见 design）；`engine/include/mornlea_engine.h`；`AGENTS.md`（engine ABI v7→v8 版本表述最小同步，`TestBaselineVersionsMatchCode` 转绿为准；`CLAUDE.md` 为薄导入无需改动，`cmp` 命令作废以 `TestClaudeImportsAgentGuides` 为准）。
- 验证命令：`make rust`；`make rust-check`（含 cargo fmt 与 clippy `-D warnings`，Task 3 验证面曾漏此入口、由整分支门禁发现并回补）；`cd engine && cargo test --workspace --locked`；`go test ./internal/mesh ./internal/nativeabi -race -count=1`；`go test ./internal/archcheck -count=1`。
- [x] RED：registry entry 20 bytes 布局逐字节（id=0..1、opaque=2、emission=3、material=4..15、fluidHeight=16、lightAttenuation=17、blockTopRaw=18、model=19）、80 entries、未知 tag（≥7）与床 tag（6）失败、ABI 8；Go/Rust 同一 80 项容量夹具跨 FFI 成功、第 81 项 Go 侧失败；五形态火把 in-air 邻域固定数量双面 cutout quad、坐标全在本格、8-byte/bit63 结构、落地竖直居中/墙面贴面外倾、不参与 merge；无 model 覆写的既有方块（cube/短方块/流体/植物）输出逐位不变。
- [x] GREEN：model dispatcher（0=既有路径，1..5 → `emit_torch`，6 与未知回 invalid/unsupported 错误）；`emit_torch` 窄柱/墙斜几何；`REGISTRY_ENTRY_BYTES`=20、`MAX_REGISTRY_ENTRIES`=80、ABI 常数、C header 与 Go 常量 → 8；`nativeRegistryWords`/`maxNativeInputBytes` 与全部硬编码布局注释同步。
- [x] Go/Rust parity 核对（Go 解包对 model quad 的材料/光值）与双评审后提交 `feat: mesh finite torch models`；结论记入 ledger。

## 4. 火把纹理、`torch-night` 场景与 golden、整分支门禁

- 目标文件/包：`internal/assets/blocks.go`、`blocks_test.go`（程序化火把层与 `Model` tag 登记）；`cmd/mornlea/capture.go`、`capture_scene_test.go`、`capture_scene_order_test.go`、`capture_hud_test.go`（场景表插入与断言）；capture golden 目录（仅新增 `torch-night.png`）。
- 验证命令：`gofmt -w internal/assets cmd/mornlea`；`make rust`；`go test ./internal/assets ./internal/client ./cmd/mornlea -race -count=1`。
- [x] RED：火把层非空/alpha 仅 0/255/与既有层逐像素不同/无外部 PNG；`Model` 对五种形态返回冻结 tag（1..5）、其余块保持 0；场景表顺序锁定 `torch-night` 紧随 `block-light-room` 且先于 `materials-showcase`；场景内像素断言（落地 + 至少两种墙面、近亮远暗、透明边缘非实心矩形）。
- [x] GREEN：程序化像素（窄木柄 + 暖色火芯）；`torch-night` 场景构造（固定夜晚封闭暗室，不创建前台窗口）。
- [x] golden：经显式基线更新写入 `torch-night.png`（19 → 20 张），逐图人工复核；其余 19 张既有 golden 逐字节不变（`workbench-crafting` 缺口保持既有状态，不在本分支补）。提交 `feat: present placeable torches`。
- [x] 整分支终审与门禁：`make rust`；`make rust-check`；`go test ./internal/core ./internal/assets ./internal/mesh ./internal/nativeabi ./internal/sim ./internal/world ./internal/client ./cmd/mornlea -race -count=1`；`go test ./internal/archcheck -count=1`；`go test ./... -race`；`go vet ./...`；`gofmt -l .`；`cmp -s AGENTS.md CLAUDE.md`；`openspec validate --all --strict --no-interactive`；`git diff --check`；`scripts/agents/gates.sh` 全绿。终审要点（支撑原子性、六邻居上限、光属性唯一来源、20-byte/80-entry 双端一致、ABI v8、无新增 pass/资源、原创纹理、golden 20 张口径）与裁决记入 ledger；收尾按仓库惯例推分支开 PR，归档时同步 `docs/notes/progress.md` 与版本矩阵。
