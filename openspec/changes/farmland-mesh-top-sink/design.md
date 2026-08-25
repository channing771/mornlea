# Design: farmland-mesh-top-sink

## D1 数据所有权与通道选择

registry 是方块呈现属性的唯一数据通道（先例：`emission`、`light_attenuation`、`fluid_height` 全走 registry，engine crate 零硬编码游戏 ID）。本变更沿同一通道追加 `block_top_raw`：

```
| id: u16 | opaque: u8 | emission: u8 | material[6]: u16 | fluid_height: u8 |
| light_attenuation: u8 | block_top_raw: u8 |   ← 共 19 字节（原 18）
```

- **所有权**：`internal/assets` 构造值、`internal/mesh/native_input.go` 编码、Rust `input.rs` 只读消费。三方域规则一致：`block_top_raw ∈ 0..=14`，`15` 拒绝；与 `fluid_height` 互斥（流体格 MUST 填 0）。
- **哨兵取 0 的理由**：满格方块占绝对多数，0 让现有全部条目零改动；与 `fluid_height` 的「0=非流体」同构。

被否替代一：mesher 硬编码 `FarmlandDryID/WetID`。零契约变更，但把游戏 ID 常量引进 mesher 破坏「engine 无游戏 ID」纯度（连 worldgen 注水的 water ID 都是经 input 传入的），且不为 B-16 薄雪层等未来半高方块留扩展点。

被否替代二：复用 `fluid_height` 字段表达耕地。语义冲突致命——`cell_height` 有「上方也是流体则取满格 15」与邻域平均规则，水下的耕地上缘会被拉平成满格；且耕地会被误判进流体的 1×1 出面分支之外的所有流体特判。

## D2 mesher 几何路径

复用水面角高度的既有位通道（quad bits 12..19 角 0/1、55..62 角 2/3，quad 仍 8 字节）：

- `MaskCell` 追加 `short: bool` 与常量角集合；`block_top_raw(id) > 0` 时置位。
- 角赋值完全仿照 `fluid_corners` 的形状：**只有顶点位于该格顶层（`y == p[1]+1`）的角取 `block_top_raw`，其余角为 0**。于是顶面四角全下沉、四个侧面上缘两角下沉、底面不动——与耕地碰撞盒（底面整格、顶面 15/16）逐面一致。
- 与流体的差异：不走 `corner_height` 邻域平均，直接用 registry 常量——耕地是刚体方块，相邻格高度恒等，不存在需要插值的斜面；也天然规避 D1 所述「上方为流体取满格」污染。
- 不贪心合并：合并条件从 `if !cell.fluid` 扩为 `if !(cell.fluid || cell.short)`。理由同流体注释——合并后 `w/h` 位无法同时表达尺寸与角高度。
- 光照/AO：沿用普通方块路径不变（耕地仍按不透明方块遮挡与采样，只改几何上缘）。

**回归边界**：不含非零 `block_top_raw` 条目的输入 MUST 产出与 v6 逐位一致的 packed quads——由跨语言 oracle parity 测试钉住（喂既有夹具 + 新增耕地夹具）。

## D3 ABI 升版

- Rust `ffi.rs` `ABI_VERSION: u32 = 6 → 7`；`include/mornlea_engine.h` 的 `MORNLEA_ENGINE_ABI_VERSION` 同步（Go 经 cgo 读 header，天然同源）；`internal/mesh/native_abi.go` 引用的是 `nativeabi.ABIVersion` 常量，无需手改。
- 版本互斥检查：当前无其他持有者持有 engine ABI 升版行（backlog 全表核对过）。
- 双侧手工同步常量（ENTRY_BYTES 18→19、ABI 6→7、上限 48）必须在同一 Task 内改齐，由「喂满 registry 的容量测试」与 ABI 握手测试兜底——仓库既有纪律。

## D4 场景与 golden

- `materials-showcase` 夹具追加干/湿耕地各一个 2×1 列（与既有材料列同规格），放在既有列网格的空位上，不移动任何既有方块——保证其余 17 景逐字节不变的前提是本场景独占其 golden。
- golden 再生遵循「基线更新必须显式」：仅 `materials-showcase.png` 一景再生，逐图人工复核下沉顶面可见后入库；其余场景 visual-check 必须零 diff。
- 本机对 main 的既有机器偏差清单先行归因（D-05 先例），避免把固有偏差误记为本变更引入。

## D5 风险与回退

| 风险 | 缓解 | 回退 |
|---|---|---|
| ENTRY_BYTES 变更破坏旧 dylib 兼容 | ABI 握手在执行任何网格化前拒绝（既有机制，spec 场景覆盖） | 无需回退——混装本就不支持 |
| 耕地侧面临界 Z-fighting（上缘与相邻满格方块侧面共面） | 上缘 15/16 与邻居侧面 0..15/16 区间重叠属正常遮挡；parity 测试 + 场景目检 | 几何路径独立，可单独 revert mesher 分支 |
| 角高度位与植物正背位冲突（同用 bit 12..19） | 二者按 material 区间互斥（植物 material ∈ 31..38，耕地材质不在其中），解包端既有判定顺序不变 | 位布局一字未动 |

## D6 验证方法

```bash
cargo test -p mornlea_engine --locked          # input 校验 / mesher short 路径 / parity
go test ./internal/assets ./internal/mesh -count=1
go test ./cmd/mornlea -run TestCapture         # materials-showcase 夹具
make visual-check                              # 18 景：仅 showcase 有意 diff
scripts/agents/gates.sh                        # 收尾全量
```
