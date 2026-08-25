# Proposal: eating-progress-hud

## Why

`authoritative-hunger` 交付进食状态机时把「进度 HUD」列为呈现层遗留（design.md 遗留 2）：玩家按住「使用」进食的 1.6 秒里没有任何反馈，唯一可观察的确认是 32 tick 后饥饿条跳一格——长按动作无进度感，玩家不知道自己是在进食还是输入失效。遗留条目写明了承接方式：**复用采掘进度条的呈现形状**。同 change 的音效半边已被 `darwin-local-audio-feedback` 交付（`CueEatingComplete` 在权威确认边界播放），动画半边依赖第三人称/手部呈现（D-09 域）。

功能积压表 B-14（hunger 遗留 2）要求补齐剩余半边：进食进度 HUD。

## What Changes

- 进食输入持续满足（手持食物、按住使用、未打开容器/菜单）时，生存 HUD 在两行状态栈上方——**与采掘进度条同一锚点**——呈现进食进度条：固定轨道 + 比例填充，几何常量与 `appendMiningBar` 同源（复用呈现形状）。
- 进度值为**客户端预测**：按连续输入时长以权威 tick 周期（`physics.FixedDelta`，50 ms / 20 TPS）累积，分母为与权威 `EatingTicks` 默认值同源的呈现层常量 32；不新增协议字段、不升版本。
- 与采掘反馈**互斥**：采掘条激活时优先采掘条，进食条不出现——进食条最多 2 quad（轨道+填充）不超过采掘条激活时的 3 quad，固定上传容量 267 quad / 46912 bytes 与 benchmark scenario v19 均不变。
- 预测进度的中断镜像：`Eating` 输入位归零（松手/开箱/菜单，经 `interactive.go` 的 `allowActions` 天然覆盖）、选中栏位变化、栏位物品变化 → 立即清零。
- 客户端 hud 包新增关注点文件 `eating.go`（`EatingOverlay` 类型 + `appendEatingBar`），`internal/client` 新建进食进度小状态机。

### 用户可观察结果

- 手持面包按住「使用」：状态栈上方出现进度条并在约 1.6 秒内填满，随后随权威结算消失（输入仍按住则重新从零开始下一件）。
- 中途松手、切格、换物或打开容器：进度条立即消失；再次开始时从零。
- 既有 18 个 capture 场景与 benchmark 输出逐字节不变（无既有场景进食）。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `survival-hud-presentation`: 新增 requirement「进食进度以客户端预测呈现并与采掘反馈互斥」——锚点/形状复用、预测语义、中断清零、互斥优先与容量不变契约。

## Impact

- **代码**：`internal/client`（新建 `eating_progress.go` 状态机与测试）、`internal/render/hud`（新建 `eating.go` 与 `eating_test.go`，`renderer.go`/`layout.go` 的 `Prepare`/布局签名追加 overlay 参数）、`cmd/mornlea` 三处裁决点位（`app.go` 字段行、`app_frame.go` 构造与实参行、`app_lifecycle.go` 复位行）+ 新增定点测试文件。
- **兼容性**：无协议、存档、编号、ABI、scenario 变更；golden 不变。
- **性能**：每帧一次常量级累积计算与最多 2 个 quad 的追加；固定上传布局零变化。
- **并行边界**：不触碰 `hud/container.go`（A-01）、`capture_scene*.go`（E-12）、`internal/audio` 与音频装配、`combat.go`/`hunger.go`（B-13）、`engine_step.go`/`drop.go`（A-04）、`internal/core` 编号段与 `internal/sim` 全部。

## 延期与放弃

- **受伤/死亡中断的客户端镜像**：客户端预测计数无本地注入点（伤害经权威镜像到达时输入位常仍为真），进度条在「受伤中断后重开」场景下最多超前 1.6 秒；权威结算与饥饿条仍准确。记为已知简化，如需清偿须在伤害确认边界加复位调用（届时按文件所有权另行裁决）。
- **分母跟随服务端 tunable**：远程服务端修改 `eatingTicks` 配置时进度条速率与实际不一致（呈现层偏差）；单机与默认配置下同源。
- **进食动画**：无第一/第三人称手部呈现，属 D-09 域。
- **新 capture 场景**：`capture_scene.go` 为 E-12 独占；进食场景留待其合流后由 D-08 类行补。
