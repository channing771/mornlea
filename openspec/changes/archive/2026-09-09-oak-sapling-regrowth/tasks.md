# B-33 树苗与橡树再生（tasks，如实勾选）

> 按 `subagent-driven-development` 执行：一任务一实现 + SPEC/QUALITY 双评审，ledger 见工作区 `progress.md`。任务 brief 是唯一需求来源，本文件、proposal、design 与 specs 是 brief 的输入。

## Task 1: 树苗编号与语义

- [x] `packages/shared/core/block.go`：`SnowLayer4BlockID` 之后追加 `SaplingID = 89`，哨兵 `BlockIDMax = 90`
- [x] `packages/shared/core/item.go`：`ItemWaterBucket` 之后追加 `ItemSapling = 57`，哨兵 `ItemIDMax = 58`；`ItemPlacement(ItemSapling) = SaplingID`；堆叠上限 `64`、无耐久
- [x] `packages/shared/core/farming.go`：新增 `IsSapling`，`IsPlant` 覆盖树苗
- [x] `packages/shared/core/block_name.go`、`canonical_name.go`、`item_name.go`：树苗中文名与 canonical 名
- [x] `packages/shared/core/item.go`：`BlockDrop(SaplingID) = (ItemSapling, true)`（树叶的树苗掉落是概率判定，不进 `BlockDrop`）
- [x] 测试：编号哨兵、`IsPlant`/`IsSapling` 穷举、名称覆盖、`BlockDrop` 往返、`ItemPlacement` 双射
- [x] 验证：`go test ./packages/shared/core -race -count=1`

## Task 2: Rust 树形 ABI 与版本矩阵

- [x] `packages/engine/crates/mornlea_engine/src/worldgen.rs`：新增 `tree_blocks(seed, x, y, z)` 纯函数（普通橡树家族：高度 `5 + hash%3`、蓬松位、既有普通/蓬松树冠层形、半径 ≤ 2、无分杈/珍异），独立冻结 salt，`#[cfg(test)]` 主题测试
- [x] `packages/engine/crates/mornlea_engine/src/ffi.rs`：新增 `mornlea_tree_blocks` 导出，`ABI_VERSION` 升 `11`，自检断言同步；输入 `MTB1`+layout u32+seed i64+x/y/z i32（28 字节），输出 `count u32` + 每条 8 字节记录，上限 `128`，坏输入不改写输出
- [x] `packages/engine/include/mornlea_engine.h`：宏 `MORNLEA_ENGINE_ABI_VERSION 11u` 与函数声明
- [x] `packages/shared/worldgen/tree.go`：ABI 桥（编码/解码/错误映射），导出 `TreeBlock` 与 `TreeBlocks`
- [x] `packages/shared/nativeabi/native.go`：`TreeBlocks` 包装与版本断言更新
- [x] 版本矩阵：根 `AGENTS.md` 与 `openspec/config.yaml` 的 `engine ABI v10` 改 `v11`
- [x] 验证：`make rust`、`cargo test -p mornlea_engine --locked`、`go test ./packages/shared/nativeabi ./packages/shared/worldgen -race -count=1`、`go test ./packages/audit -count=1`

## Task 3: 种植、采掘与掉落

- [x] `packages/server/sim/entity/placement.go`：树苗放置分叉——目标格必须 `AirID`，下方必须 `DirtID`/`GrassID`，成功消耗 `1`，拒绝零副作用
- [x] `packages/server/sim/entity/mining.go`：树苗 `1` tick + 掉落自身 + 耐久豁免；树叶树苗判定与「树叶自身掉落 + 树苗」同一次原子预演、容量不足整次拒绝
- [x] `packages/server/sim/entity/yield.go`：新增独立冻结 salt 的 `hash & 7 == 0` 树苗判定（只依赖世界种子/维度/坐标）
- [x] `packages/server/fluid/rules.go` 与 `packages/engine/crates/mornlea_engine/src/fluid_eval.rs`：可替换表加入树苗并同步常量/pin 测试
- [x] 验证：`go test ./packages/server/sim/entity ./packages/server/fluid -race -count=1`、`cargo test -p mornlea_engine --locked`

## Task 4: 随机 tick 生长与环境移除

- [x] `packages/server/sim/realm/environment.go`：`advanceSaplingCell`（露天 + 支撑 + `hash & 7 == 0` 判定）、树形空间校验（`AirID`/`ShortGrassID` + 世界边界）、跨区块 `ChunkReady` 前置与全有或全无写入回滚、`Mutation.Record` 登记
- [x] `packages/server/sim/realm/environment.go`：支撑清理分支（掉落树苗、容量不足原子拒绝）、流体冲毁树苗掉落分支
- [x] `packages/audit/dependency_test.go`：`packages/server/sim/realm` 增加 `packages/shared/worldgen` 依赖边并写明理由
- [x] 验证：`go test ./packages/server/sim/... -race -count=1`、`go test ./packages/audit -count=1`

## Task 5: 客户端呈现

- [x] `packages/client/assets/blocks.go`：追加 `LayerSapling`（层号在雪层之后，`layerCount` +1）、树苗材质映射、`isCutoutLayer` 纳入
- [x] `packages/client/assets/procedural.go`：原创二值 alpha 树苗纹理
- [x] `packages/client/mesh/quad.go` 与 `packages/engine/crates/mornlea_engine/src/quad.rs`：植物材质集合同步为 `[31..54] ∪ {68, 164}`
- [x] 图标回退与掉落薄片覆盖测试：树苗物品复用方块材质层
- [x] 验证：`go test ./packages/client/... -race -count=1`、`cargo test -p mornlea_engine --locked`

## Task 6: capture 场景与 golden

- [x] `packages/client/cmd/mornlea/capture/`：新增 `sapling-growth` 场景与确定性夹具（树苗 + 运行时树形长成的橡树）
- [x] 场景顺序与数量断言：紧随 `oak-grove`、先于 `ai-companion`，总数 `29 → 30`
- [x] `packages/client/cmd/mornlea/capture/testdata/visual-golden/README.md` 同步
- [x] 验证：`go test ./packages/client/cmd/mornlea/capture -count=1`、`make visual-check`

## Task 7: 伙伴边界

- [x] `packages/shared/companion/plan_types.go`：树苗加入 `planPlaceExempt` 并写明理由；采掘沿用通用规则
- [x] 测试：注册表锁定与豁免守卫、伙伴采掘树苗通用规则
- [x] 验证：`go test ./packages/shared/companion -race -count=1`

## Task 8: 收尾门禁与基线文档

- [x] `docs/notes/progress.md` 基线段、`docs/feature-backlog.md` 行内履历
- [x] 验证：`test -z "$(gofmt -l .)"`、六模块 `go vet`、`make test-race`、`go test ./packages/audit -count=1`、`openspec validate --all --strict --no-interactive`、`make visual-check`
- [x] 整分支终审（含 deferred minor triage）与一次修复波（如需要）
