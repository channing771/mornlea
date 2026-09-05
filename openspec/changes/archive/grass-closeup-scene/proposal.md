## Why

自然短草在 `oak-grove` 里只有一处 21×24px、193 差分像素的可辨识足迹，肉眼几乎无法确认短草外观；仓库没有短草的近景图片基线，短草材质/几何回归时无可复核的视觉真值。

## What Changes

- 在 capture 正式场景清单中新增第 25 个场景 `grass-closeup`：确定性手工夹具（草地条上立短草列）+ 固定近景机位 + 固定正午，走既有完整呈现链路无窗口抓帧。
- 正式场景数 24 → 25，golden 基线 24 → 25 张；其余 24 张 golden 逐字节不变。
- 更新两处 24 数门禁与场景顺序断言以反映新清单。

## Capabilities

### New Capabilities

（无：新场景仍由 `visual-verification` 能力覆盖，不引入新能力。）

### Modified Capabilities

- `visual-verification`：正式场景清单 24 → 25 项（新增 `grass-closeup`，插在 `oak-grove` 之前）；golden 基线 24 → 25 张；场景数与顺序断言随之更新。

## Impact

- 影响包：`packages/client/cmd/mornlea/capture`（场景表、夹具、顺序/数量测试）与 `testdata/visual-golden/world/`（新增 `grass-closeup.png`）。
- 不影响：worldgen 分布与密度、渲染管线、材质层、协议、存档 schema、engine/client ABI、benchmark scenario、双阈值（不放宽）。
- 兼容性：capture-only 变更；旧 golden 全部保留；回退即删除场景行与 golden 并恢复 24 数断言。
