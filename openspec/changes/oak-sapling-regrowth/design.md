# B-33 树苗与橡树再生（design）

> 脑风暴输入：用户 2026-09-09 显式批准（bounded：树形几何经新增 engine ABI 函数由 Rust 单一真源提供，普通橡树家族、高 5–7、半径 2，不含稀有巨树）。分支 `feat/B-33-sapling-oak-regrowth`，基线 `main@329ba7ea`。

## 数据所有权与依赖方向

- 权威全部在 Go：`core` 只加编号、谓词、名称与掉落表；`sim/entity` 做种植校验、采掘与耐久豁免；`sim/realm` 做随机 tick 生长、支撑清理与流体冲毁；`fluid` 与 Rust `fluid_eval` 同步可替换表。
- 树形几何是生产数值计算，独占 Rust：新增 `mornlea_tree_blocks` 由 `worldgen.rs` 的树冠层形复用实现，Go 侧只有 `packages/shared/worldgen` 的 ABI 桥（与 `MGW1` 同包，保持「只有 nativeabi 接触 ABI、领域包经既有桥调用」的边界）。`packages/server/sim/realm` 因此新增一条到 `packages/shared/worldgen` 的依赖边，需同步 `packages/audit/dependency_test.go` 的三处 allowlist（`packages/server/sim/realm` 条目），并在该文件的边注释里写明理由。
- 客户端只做呈现：`assets` 追加原创程序化层与植物材质集合成员，`mesh` 同步 Go 侧植物材质常量，Rust `quad.rs` 同步跨语言常量；shader 以 `face >= 6` 判别植物，不复制层号。

## 关键决策

- D1 编号与语义：`SaplingID = 89`（`BlockIDMax = 90`）、`ItemSapling = 57`（`ItemIDMax = 58`）、堆叠 64、无耐久；`IsSapling` 是唯一判定入口，`IsPlant = IsCrop || IsWildGrass || IsSapling`。零碰撞、非不透明、发光 0、衰减 0、可被权威射线命中，全部经既有 `IsPlant` 通道自动成立。
- D2 树叶掉落：树叶沿用 `5` tick 与自身掉落；新增独立冻结 salt 的 `hash & 7 == 0` 判定额外掉 1 个树苗。与短草种子判定同族但 salt 不同，只依赖世界种子/维度/坐标；命中时走既有 `PrepareDropBatch` 原子预演（树叶物品与树苗作为同一次结算），容量不足整体拒绝且重试仍命中。
- D3 种植支撑：目标格必须 `AirID`，正下方必须 `DirtID` 或 `GrassID`；不引入耕地、砂砾等其它支撑。走 `executePlacement` 的既有分叉，成功消耗 1 个物品，拒绝零副作用。
- D4 生长触发：复用既有随机 tick 阶段（`realm.AdvanceCrops` 的分派），新增 `advanceSaplingCell`：被考察格是树苗、露天（`position.Y >= chunk.HighestOpaque(localX, localZ)`，与作物同式）、下方仍是泥土/草、`saplingGrowthRoll` 的 `hash & 7 == 0` 命中才尝试。期望生长时间约 9 分钟（每区段每 tick 3 个样本、4096 格/区段、`1/8` 判定），与作物抽样成本同源。判定用固定常量而非新 tunable：本 change 的契约是「有界确定性」，把速率做成 tunable 会同时扩 config 面与版本面，留待确有调参需求时独立提案。
- D5 树形来源：新增 engine ABI `mornlea_tree_blocks`（ABI v10→v11）。输入 28 字节：`MTB1` magic(4) + layout u32(4) + seed i64(8) + x/y/z i32(12)；输出 `count u32` + 每条 8 字节 `[dx i8][dy i8][dz i8][reserved u8][block u16 LE][reserved u16]`，记录上限 `128`（最坏普通橡树 68 条）。几何只取普通橡树家族：高度 `5 + hash%3`、蓬松位由哈希给出、树冠沿用既有普通层形（`-2/-1` 去角 5×5、`0` 3×3、`1` 十字、`2` 蓬松十字），半径 ≤ 2、无分杈、无珍异巨树。参数由独立冻结 salt 从 (世界种子, 根坐标) 派生，与世界生成的 8×8 候选格网格无关，因此 `GenerateChunk`/`BaseBlockAt` 逐格不变、worldgen golden 字节不变。
- D6 空间校验：树形几何的每一格必须在世界高度范围内且当前为 `AirID` 或 `ShortGrassID`；任一格不满足即放弃本次生长（树苗保留）。覆盖短草零掉落（与短草被流体清除同口径）。
- D7 跨区块原子：写入前要求全部目标区块 `ChunkReady`（任一未就绪即放弃，后续 tick 可重试）；写入采用「保存旧值 → 逐格 `SetBlock` → 任一失败即恢复全部旧值」的全有或全无；成功后每格经 `Mutation.Record` 登记，受影响区块 revision 各推进一次。沿用门/床的 save-old/restore-old 先例，但不新建通用事务抽象（B-21 仍是独立候选）。
- D8 环境移除：支撑清理沿用 `SweepUnsupportedWildPlants` 的相位位置新增树苗分支，掉落 1 个树苗且容量不足原子拒绝；流体冲毁复用作物冲毁的结算形状（掉落 1 个树苗，容量不足原子拒绝并保留待更新重试）。Rust `fluid_eval` 的 `is_plant` 镜像与 `SHORT_GRASS` 常量旁追加 `SAPLING = 89`，由既有 `block_id_constants_match_go_core` 钉住。
- D9 客户端呈现：新增 `LayerSapling = LayerHumanSageHead + 50`（追加在雪层之后，`layerCount` 随之 +1），原创程序化 16×16 二值 alpha 纹理；`isCutoutLayer` 纳入该层；Go `mesh.PlantMaterial` 与 Rust `quad.rs::plant_material` 同步为 `[31..54] ∪ {68, 164}`。该层位于 `LayerItemCoal` 之后的内部区，按既有材质包契约不开放文件覆盖（与雪层、人物层同口径），如需开放由材质包 v2（D-06）统一裁决。树苗物品图标复用其方块材质层（`blockItemTexture` 回退路径），不新增物品图标层。
- D10 伙伴边界：树苗加入 `planPlaceExempt`（理由：伙伴植树属未裁决的农业/植被语义，与作物种植同口径），采掘沿用「单一 `BlockDrop` 的非容器、非农业、非流体方块」通用规则（树苗恰有单一掉落，因此允许），不为它新增显式拒绝。
- D11 版本矩阵：唯一升版是 engine ABI v10→v11，须同步 `packages/engine/include/mornlea_engine.h`、`ffi.rs` 自检、`packages/shared/nativeabi` 版本断言、根 `AGENTS.md` 与 `openspec/config.yaml` 的版本矩阵（`packages/audit` 基线测试兜底）。
- D12 视觉基线：新增 `sapling-growth` 场景，固定夹具同时呈现一株树苗与一棵由本 change 树形路径长成的橡树；在场景清单中紧随 `oak-grove`、先于 `ai-companion`，场景总数 29→30；既有双阈值与其它 golden 不变。

## 被否决的替代方案

- Go 侧实现树形几何：不升 ABI，但把生产数值计算搬进 Go，违反「生产数值在 Rust、无 Go fallback」边界，并与世界生成树形形成两份真源。否决。
- 生长速率做成 tunable：扩 config 面与版本面，且本 change 的契约是固定确定性速率。否决（留待独立提案）。
- 种植走新命令：`PlaceBlock` 已能表达全部语义，新增命令会升协议版本而无收益。否决。
- 树苗生长阶段方块（多编号渐进）：需要额外编号与状态，且本 change 不建设通用植被系统。否决。
- 允许珍异巨树：其几何依赖世界生成的 8×8 候选格网格，无法从任意根坐标确定性派生。否决。
- 树叶衰减/腐烂：需要跨区块的叶子支撑判定与新的持久状态，超出「木材可再生」闭环。否决。

## 风险与回退

- 生长在区块边界跨区块写入：已由「全部 Ready 才写、全有或全无、失败零副作用」覆盖；若实现发现 `ChunkReady` 前置条件在活动兴趣边界不可满足，代价是生长被推迟到区块就绪（不产生部分树）。
- 新 ABI 版本与 Rust/Go 常量漂移：由 `nativeabi` 版本断言、`ffi.rs` 自检、`packages/audit` 基线矩阵与跨语言常量 pin 测试共同兜底。
- 生长速率体感偏差（约 9 分钟）：代价是改一处常量并重跑分布测试，无契约面影响。
- 回退即 revert 分支；编号与 ABI 版本号追加式，无迁移。

## 验证方法

- 定点：`go test ./packages/shared/core ./packages/shared/worldgen ./packages/shared/nativeabi ./packages/shared/companion ./packages/server/sim/... ./packages/server/fluid ./packages/client/... -race -count=1`、`cargo test -p mornlea_engine --locked`、`go test ./packages/audit -count=1`。
- 门禁：`make rust`、`make test-race`、六模块 `go vet`、`gofmt`、`openspec validate --all --strict --no-interactive`、`make visual-check`（新增 `sapling-growth`，既有 golden 零漂）。
- 视觉变化只允许新增场景；比对失败时先逐图人工确认再 `make visual-update`，禁止放宽阈值。
