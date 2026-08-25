# Tasks: farmland-mesh-top-sink

## Task 1: Go 侧 registry 高度通道

- [ ] 1.1 `internal/assets`：registry 条目结构追加 `BlockTopRaw`（GoDoc 说明哨兵与域），干/湿耕地填 `14`，其余全部填 `0`；域校验（>14 拒绝、流体格非零拒绝）带失败测试
- [ ] 1.2 `internal/mesh/native_input.go`：条目编码扩到 19 字节，追加同规则域校验；`nativeMaxRegistryEntries`/容量常量复核不变
- [ ] 1.3 验证：`go test ./internal/assets ./internal/mesh -count=1`；喂满 48 条 registry 的既有容量测试保持绿（Rust 未升级前此为纯 Go 半边红绿循环的红侧来源——本任务先落 Go 编码与测试，Task 2 升 Rust 后全绿）

## Task 2: Rust 解析、mesher 路径与 ABI v7（双侧常量同一任务内改齐）

- [ ] 2.1 `input.rs`：`REGISTRY_ENTRY_BYTES` 18→19、解析 `block_top_raw`、域校验（15 拒绝、与 fluid 互斥拒绝）+ 失败测试；`RegistryView::block_top_raw(id)` 查询
- [ ] 2.2 `ffi.rs` `ABI_VERSION` 6→7 + `include/mornlea_engine.h` 常量同步；ABI 握手测试断言新值
- [ ] 2.3 `greedy/mod.rs`：`MaskCell.short` 分支、常量角赋值（顶层角取 h、其余 0）、不合并条件扩展；`water_corner_tests.rs` 同目录新增主题测试文件覆盖耕地几何（顶面四角、四侧上缘两角、底面不动、相邻不合并）
- [ ] 2.4 oracle parity：既有夹具逐位回归（v7 输入 vs v6 期望）+ 新增干/湿耕地 neighborhood 的跨语言 parity 测试
- [ ] 2.5 验证：`cargo test -p mornlea_engine --locked && cargo clippy --workspace --all-targets -- -D warnings`；随后 Task 1 的 Go 容量/编码测试全绿

## Task 3: materials-showcase 扩展与 golden 再生

- [ ] 3.1 `cmd/mornlea/capture_scene.go`：夹具追加干/湿耕地各一个 2×1 可见列（不移动既有方块）；场景清单顺序不变
- [ ] 3.2 golden：先跑 `make visual-check` 归因基线（记录既有机器偏差清单），确认仅 `materials-showcase` 有意 diff 后按显式再生流程更新该单景 golden；其余 17 景 MUST 零 diff
- [ ] 3.3 验证：`go test ./cmd/mornlea -run TestCapture -count=1` 与 `make visual-check`

## Task 4: 收尾门禁与基线同步

- [ ] 4.1 `AGENTS.md` 与 `CLAUDE.md` 同步 engine ABI v6→v7 表述（含「v6 新增 lod_shell」演进句改为 v7 新增本行），逐字节相同；`docs/notes/progress.md` 待归档时补段（归档阶段执行）
- [ ] 4.2 全量门禁：`scripts/agents/gates.sh` 全绿；`openspec validate --all --strict --no-interactive`
- [ ] 4.3 ledger 记录全部评审结论与裁决；未决项誊入 proposal「延期与放弃」
