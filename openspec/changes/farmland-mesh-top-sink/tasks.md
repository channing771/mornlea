# Tasks: farmland-mesh-top-sink

## Task 1: registry 高度通道（Go + Rust 原子落地）

> 双侧手工同步常量（ENTRY_BYTES、ABI 版本）必须在本任务内一次改齐，避免「Go 编 19 字节 vs Rust 解 18 字节」的半升级窗口；中间态不允许提交为绿。

- [ ] 1.1 Go 红侧：`internal/assets` registry 条目追加 `BlockTopRaw`（GoDoc 说明哨兵与域），干/湿耕地填 `14`、其余填 `0`；域校验失败测试（>14 拒绝、流体格非零拒绝）
- [ ] 1.2 Rust 红侧：`input.rs` `REGISTRY_ENTRY_BYTES` 18→19、解析 `block_top_raw`、域校验测试（15 拒绝、fluid 互斥拒绝）、`RegistryView::block_top_raw(id)` 查询
- [ ] 1.3 `internal/mesh/native_input.go` 编码扩到 19 字节 + 同规则域校验；`engine/include/mornlea_engine.h` 与 `ffi.rs` 的 ABI 常量 6→7 同步，握手测试断言新值
- [ ] 1.4 mesher short 路径：`greedy/mod.rs` 追加 `MaskCell.short` 分支、常量角赋值（顶层角取 h、其余仿 `fluid_corners` 形状为 0）、不合并条件扩展
- [ ] 1.5 测试：`water_corner_tests.rs` 同目录新增耕地几何主题文件（顶面四角、四侧上缘两角、底面不动、相邻不合并）；oracle parity——既有夹具 v7 输出逐位等于 v6 期望 + 新增干/湿耕地 neighborhood 跨语言 parity
- [ ] 1.6 验证：`cargo test -p mornlea_engine --locked && cargo clippy --workspace --all-targets -- -D warnings`；`go test ./internal/assets ./internal/mesh -count=1`（喂满 48 条的容量测试全绿）；`go test ./internal/nativeabi -count=1`

## Task 1b: 客户端解码半边（terrain/cull shader 分支）

> Task 1 实现期发现（见 ledger）：角高度解码器只存在于 `water.wgsl`，`terrain.wgsl:63-64` 把 bit 12..19 当 w/h 尺寸读，耕地 quad 不补此半边会渲染成巨型石板。design D2a 是本任务依据。

- [ ] 1b.1 Rust 客户端侧新增耕地区间常量（复述 Go `LayerFarmlandDry/Wet` 层号，仿 `PLANT_MATERIAL_FIRST/LAST` 手工同步纪律），跨语言一致性测试钉住
- [ ] 1b.2 `terrain.wgsl`：material 落入耕地区间走角高度路径（`(raw+1)/16` 上移公式与 water.wgsl 同源），否则保持既有 w/h 尺寸路径
- [ ] 1b.3 `cull.wgsl`：区间内 quad 按满格 AABB 处理（保守正确），不误读尺寸位
- [ ] 1b.4 client ABI 不变（无新导出）；验证：`cargo test -p mornlea_client --locked && cargo clippy --workspace --all-targets -- -D warnings`，`make visual-check` 确认既有 18 景零回归（耕地尚未入景）

## Task 2: materials-showcase 扩展与 golden 再生

- [ ] 2.1 `cmd/mornlea/capture_scene.go`：夹具追加干/湿耕地各一个 2×1 可见列（不移动既有方块）；场景清单顺序不变
- [ ] 2.2 golden：先跑 `make visual-check` 归因基线（记录既有机器偏差清单），确认仅 `materials-showcase` 有意 diff 后按显式再生流程更新该单景 golden；其余 17 景 MUST 零 diff
- [ ] 2.3 验证：`go test ./cmd/mornlea -run TestCapture -count=1` 与 `make visual-check`

## Task 3: 收尾门禁与基线同步

- [ ] 3.1 `AGENTS.md` 与 `CLAUDE.md` 同步 engine ABI v6→v7 表述（含「v6 新增 lod_shell」演进句改为 v7 新增本行），逐字节相同；`docs/notes/progress.md` 待归档时补段（归档阶段执行）
- [ ] 3.2 全量门禁：`scripts/agents/gates.sh` 全绿；`openspec validate --all --strict --no-interactive`
- [ ] 3.3 ledger 记录全部评审结论与裁决；未决项誊入 proposal「延期与放弃」
