## Context

`internal/render/hud/atlas.go` 的 `hotbarTextureUV(column)` 把列的纹素边界除以当前图集宽度得到归一化 UV：

```go
left  := float32(column*hotbarTextureSize) / float32(hotbarTextureWidth)
right := float32((column+1)*hotbarTextureSize) / float32(hotbarTextureWidth)
```

图集宽度 `hotbarTextureWidth = (hotbarBlockColumnOffset + core.ItemIDMax) × 16`（当前 50 × 16 = 800 纹素）随物品追加自动扩列。Rust `mornlea_client` 的 HUD pass 以 Nearest 过滤采样该图集（`engine/crates/mornlea_client/src/render/mod.rs` 中 `hud_pass` 使用 `nearest_sampler: true`），采样坐标为归一化 UV 与纹理宽度的乘积。宽度变化时全部既有列的 f32 UV 重归一化，解码回纹素空间的误差量级为 `W · ulp(u) ≈ W · 2^-24`；非整数 HUD 缩放下部分采样点恰好落在纹素边界 ±ε 内而翻转归属。`authoritative-farming` 扩列时在 `hud-hotbar-health` golden 实测 0.115% 像素漂移（遗留清单 18），本变更清偿该遗留。

数据所有权与依赖方向不变：UV 计算属于 `internal/render/hud` 的 CPU 布局半部，GPU 半部仍在 Rust，Go 侧零 WebGPU 接触；无跨 goroutine 新边界。

## Goals / Non-Goals

**Goals**

- 使「图集扩列」对既有 cell 的采样纹素集合零影响，并以机械属性测试钉死。
- 消除「采样点恰落在列边界上」这一实现定义行为（tie-break），使采样归属完全确定。

**Non-Goals**

- 不动 glyph 图集、quad 容量常量、benchmark scenario、协议/存档/ABI 版本号。
- 不改任何 UV 消费方文件；`hotbarTextureUV` 对外包内可见签名不变。

## Decisions

### D1: 对称亚纹素收进（micro-inset），余量取 1/256 纹素

每列左右 UV 界向列内收进固定余量 `δ = 1/256` 纹素后再归一化：

```go
// hotbarUVInsetTexels 是每列左右界向列内收进的亚纹素余量……
const hotbarUVInsetTexels = 1.0 / 256.0

left  := (float32(column*16) + hotbarUVInsetTexels) / W
right := (float32((column+1)*16) - hotbarUVInsetTexels) / W
```

- **为什么能钉住稳定性**：收进后解码界距列边界 ≥ δ − 重归一化噪声；噪声上界 `W·2^-24` 在适用域 `W ≤ 2^15` 纹素（2048 列、两千余种物品）时 ≤ 1/512 纹素，即裕度恒满足 delta spec 要求的最小值；当前真实宽度 800 纹素下噪声 ≤ 4.8e-5 纹素，实际裕度比 >80×。物品表规模一旦可能超出该适用域，必须重审余量而不是沿用本常量。
- **为什么是 1/256 而不是更大**：收进会把每个采样点移动 ≤δ 纹素，δ 越大对当前渲染的扰动越大；1/256 在上述适用域内已保证裕度达标，再小会侵蚀裕度，再大会增加贴边界样本翻转风险。（修订记录：初稿误写「`W < 2^16` 内上界 ≤ 1/2048、裕度 >8×」——`W·2^-24` 在 `W = 2^16` 处等于 `2^-8` 即 δ 本身，故适用域收缩为 `W ≤ 2^15`；初稿还把纹素数当物品种数换算，一并更正。）
- **v 轴不动**：图集单行 16 纹素高，v ∈ [0,1] 无内部列界问题，上下边缘由 ClampToEdge 兜住。

被否决的替代方案：

- **半纹素中心对齐**（`left = (col*16+0.5)/W, right = (col*16+15.5)/W`）：裁掉每列两侧各半纹素并把剩余 15 纹素拉伸到 quad，视觉放大 ~6.7%，golden 大面积重生成且图标语义（像素级原创掩码）失真 → 否决。
- **固定图集宽度上限**：违背「按 `ItemIDMax` 自动扩列、与枚举末项守护同一穷举界」的既有设计，且预分配浪费显存 → 否决。
- **shader 侧归一化 / quad 编码携带纹素坐标**：需改 client ABI 与 Go↔Rust 固定上传布局，版本号由批次集成任务独占 → 否决。
- **不修（维持现状）**：每次追加物品都要接受一次不可解释的 golden 亚像素噪声并人工甄别「变得对不对」→ 与遗留 18 的承接方向相悖 → 否决。

### D2: 稳定性以「宽度参数化的内部函数 + 属性测试」机械化

把纯计算提取为包内私有 `hotbarColumnUV(column, width int) [4]float32`，`hotbarTextureUV` 变成钉死当前宽度的薄包装（签名不变）。测试据此可以扫描「当前宽度 + 一组模拟未来扩列的宽度」（含 2 的幂与非 2 幂），断言 delta spec 的四条可观察性质：

1. 解码左右界在本列内且距边界 ≥ 1/512 纹素；
2. 相邻列区间无重叠；
3. 同一列在不同宽度下、以列内均匀探针位置解码得到相同纹素下标集合；
4. 解码左右界与精确列边界的偏差 < 1/64 纹素（delta spec 第三条 Scenario 的上界，钳住收进量不被放大或随宽度缩放）。

既有 `TestHotbarColumnUVStaysInsideItsOwnColumn`（容差 0.01 纹素）保持原样通过，作为回归底线。

### D3: golden 零触碰与差异升级路径

capture golden 由批次集成任务独占，本分支不修改任何 PNG。验证改为：改动前后本地运行相关 capture 场景并与仓库内 golden 逐字节对比。预期整数缩放场景逐位不变；若出现字节差（非整数缩放下贴边界样本翻转），停下向控制会话呈报实测差值清单并请求裁决——裁决通过才允许 golden 再生，且再生只发生在集成侧。绝不静默再生成。

## Risks / Trade-offs

- [收进扰动使非整数缩放下极少数贴边界样本翻转] → 余量取最小充分值 1/256；本地 capture 对比前置；有差异即升级裁决而非静默处理。
- [`float32` 除法在极端未来宽度下裕度不足] → 属性测试以「距边界 ≥ 1/512」与「偏差 < 1/64」两条判据经验性强制噪声模型的推论（不直接断言 `W·2^-24` 公式本身）；注释钉死「`W ≤ 2^15` 纹素」的适用域，超出时必须重审余量。
- [消费方误用内部函数绕过包装] → `hotbarColumnUV` 不导出且仅测试可见；`hotbarTextureUV` 保持唯一生产入口。

## Migration Plan

单 commit 可回退；无存档/协议/配置迁移。回滚即 revert 该 commit，UV 回到精确边界计算，属性测试一并移除。

## Verification

- `go test ./internal/render/hud -race -count=1`
- `make dev-check`
- 本地 capture 场景对比（hud-hotbar-health、hud-survival-feedback、inventory-crafting、chest-container、furnace-container、debug-panel）：与仓库内 golden 逐字节比较，零差或按 D3 升级裁决。
- 收尾门禁：`gofmt -l .` 无输出、`go vet ./...`、`go test ./... -race`、`openspec validate --all --strict --no-interactive`。
