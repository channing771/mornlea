# Design: eating-progress-hud

## 内容确认结论（阶段 1，2026-08-26 批准）

- **分类**：bounded——纯呈现层，无 wire/schema/ABI/scenario 变更。
- **批准的设计**：进食进度条在两行状态栈上方与采掘条同锚点、复用其呈现形状；进度为客户端预测（20 ms 累积、分母 32）；采掘激活时互斥优先采掘；中断镜像覆盖输入归零/切格/换物，受伤/死亡不镜像（已知简化）。

## 实现决策

### D1：客户端预测而非权威进度字段

权威侧没有进食进度的 wire 字段（`PlayerState` 只有 `Hunger`），加字段须升协议版本——版本号互斥当前由第一夜批次（A-07 目标 v27）持有。进食是「按住即知」的本地输入状态，预测无歧义；分母用与权威 `EatingTicks` 默认值同源的呈现层常量（32），远程改配置的偏差只影响条速不影响结算。被否决方案：`PlayerState` 追加 `EatingProgressTicks`（版本互斥冲突，且为呈现层数据污染权威快照）。

### D2：与采掘条互斥复用同锚点，容量红线零变化

`appendMiningBar`（`layout.go`）锚定 `statusBarBounds` 上方、动态追加 quad（激活时最多 3 个：轨道/填充/帽）。进食条同锚点、自身最多 2 quad（轨道/填充，无帽——没有「可采/不可采」的二元状态），且与采掘条互斥：布局的最坏情况 quad 数不变，`renderer_test.go` 钉死的 `maxHotbarQuads=267`、`hotbarUploadBytes=46912` 与 benchmark scenario v19 全部不动。互斥方向为采掘优先（采掘有瞄准语义、信息密度更高）。被否决方案：进食条独立锚点（新增最坏 quad 数→容量升版→撞 scenario 互斥）。

### D3：预测状态机落 `internal/client`，按 elapsed 累积

`eating_progress.go` 提供小状态机（连续满足的输入时长以 `20ms/tick` 累积，浮点或整毫秒均可但必须钳制与确定性无涉——纯呈现）；复位条件在状态机入参里显式传入（输入位、选中格、格内物品），不复用 predictor 内部时钟——20 TPS 与显示帧率的差异对进度条不可感知，且不动 `predictor*.go` 的分层。被否决方案：在 `predictor_advance.go` 内计数（呈现状态渗入物理预测层）。

### D4：中断镜像的边界

客户端天然镜像：`interactive.go` 的 `allowActions = captured && !justCaptured && !a.inventoryOpen` 已使 `Eating` 位在松手/开箱/菜单时归零（含 B-31 的容器中断），状态机按输入位复位即得。显式追加：选中格变化、格内物品变化（服务端「切格/换物中断」的镜像）。**不镜像**：受伤/死亡中断——伤害经权威镜像到达时输入位通常仍为真，客户端计数会超前最多 `EatingTicks`（1.6 秒），权威结算与饥饿条仍准确；记入 proposal「延期与放弃」。

### D5：hud 包按关注点一文件

`EatingOverlay` 类型与 `appendEatingBar` 落新建 `eating.go`（同包引用 `layout.go` 的既有几何常量），配套 `eating_test.go`——沿用 `health.go`/`hunger.go`/`oxygen.go` 的关注点文件先例；`renderer.go` 的 `Prepare` 与 `layout.go` 的布局函数签名追加 overlay 参数（既有 overlay 同形传递）。

## 测试策略

- **client 侧**：状态机单测——累积速率（连续 N tick 的进度值）、复位三源（输入位/切格/换物）、满格钳制、零时长不激活；断言精确值。
- **hud 侧**：`eating_test.go`——几何复用断言（与 `appendMiningBar` 同锚点同轨道尺寸）、互斥（采掘激活时调用侧不追加进食 quad——互斥判定在布局函数内：进食布局函数收到 `mining.Active` 时不追加）、进度≤1 钳制、非激活零实例；**容量红线**：进食激活帧的 quad 数 ≤ 采掘激活帧（复用 `renderer_test.go` 的容量常量断言不改）。
- **cmd/mornlea 定点**：新增测试文件覆盖三点位（overlay 构造从 tracker 取值、lifecycle 复位清零 tracker），用既有 app 测试范式；不触碰 A-01/A-04 的既有测试文件。
- **变异验证**：注释互斥判定（进食条与采掘条同时出现）→ 容量/互斥用例红；去掉复位条件 → 复位用例红；分母写错 → 速率用例红。
- 既有 golden 与 capture 不跑不变断言以外的改动；`gates.sh` 收尾。

## 并发与性能

- 每帧一次常量级累积与至多 2 quad 追加，零分配热路径之外的新增；固定上传布局零变化；不触碰权威 tick。

## 风险与回退

- 呈现风险：进度条与权威结算的时刻差（预测 vs 权威 tick 边界）≤1 tick，不可感知；受伤漂移见 D4。
- 回退：revert 单 commit，无数据/wire/存档耦合。

## 受影响文件（冻结集）

| 文件 | 改动 |
|---|---|
| `internal/client/eating_progress.go` + 测试 | 新建预测状态机 |
| `internal/render/hud/eating.go` + `eating_test.go` | 新建 `EatingOverlay` 与 `appendEatingBar` |
| `internal/render/hud/renderer.go`、`layout.go` | `Prepare`/布局签名追加 overlay 参数（最小行） |
| `cmd/mornlea/app.go` | 裁决点位：字段声明行 |
| `cmd/mornlea/app_frame.go` | 裁决点位：构造与 `Prepare` 实参行 |
| `cmd/mornlea/app_lifecycle.go` | 裁决点位：复位行 |
| `cmd/mornlea/eating_overlay_test.go`（新建） | 三点位定点测试 |
| `openspec/changes/eating-progress-hud/*` | 本 change 产物 |

刻意不触碰：`hud/container.go`（A-01）、`capture_scene*.go` 与 golden（E-12/A 批次）、`internal/audio` 与 `app_audio.go`/`app_messages.go`（音频非目标）、`combat.go`/`hunger.go`（B-13）、`engine_step.go`/`drop.go`/`tunables.go`（A-04）、`internal/sim` 全部（含 `eating.go`——B-31 已合入归档）、`internal/core` 编号段、`predictor*.go`。
