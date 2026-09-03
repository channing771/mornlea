## Why

视觉基线 PNG 分散在两处：Go 世界场景图在 `cmd/mornlea/capture/testdata/golden/`（24 张），前端 UI 部件图在 `engine/crates/mornlea_client/frontend/visual/golden/`（19 张）。找某场景的图要在跨语言目录里翻，没有一篇索引说明哪张图对应哪个场景。

## What Changes

- 新建统一目录 `testdata/visual-golden/`，下分 `world/`、`ui/`，配中文 `README.md` 索引（文件名→场景/fixture→一句话说明）。
- `git mv` 搬迁 43 张 PNG（像素逐字节不变，不重生成基线）。
- 同步路径引用：Go `captureGoldenDir` 常量与测试硬编码、前端 `visual.mjs` 的 `goldenDir`、三处 `AGENTS.md`。
- 非目标：不改比对阈值、不改场景表与 fixture 名、不重拍任何基线。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

无。本 change 是纯资产搬迁加文档，行为零变化（`skip_specs: true`）。

## Impact

- 受影响：`cmd/mornlea/capture` 视觉管线路径、`frontend/visual` 基线管线路径、三处 `AGENTS.md`。
- 不适用：协议、存档 schema、engine/client ABI、benchmark scenario 均不变；golden 像素逐字节不变，只是换目录。
- 回退：整批 `git mv` 反向移回并恢复四个路径引用即回退。
