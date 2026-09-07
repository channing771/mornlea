## Why

`weather-camera-tree-diversity` 落地了天气呈现与三态视角，但视觉基线只更新了树形漂移的 7 张旧图：雨/雪/雷暴没有稳定态场景，第三人称没有机位场景，天气切换没有过程演示。呈现类能力没有像素/GIF 证据，回归只能靠人眼偶然发现。

## What Changes

- 新增世界静态场景 `rain-noon`（正午雨天固定夹具：雨粒子、灰天空、压暗亮度同框）。
- 新增世界静态场景 `camera-third-back` 与 `camera-third-front`（同一机位第三人称背面/正面：自身身体可见、无 viewmodel、手臂摆动中性）。
- 新增过程演示 GIF `weather-cycle.gif`（晴→雨→雷暴→晴全过程：粒子起落、天空灰度、雷暴闪光，跨 tick 步进抓帧，只供人眼审查，不进比对）。
- 为新场景配 golden PNG（`--update-golden` 显式生成并逐图人工复核），GIF 入 `motion/`（复用既有 motion 演示入口形态）。
- 非目标：不改变 24 个既有场景的定义与顺序（新场景插到 `far-horizon` 之前，尾序与全部既有相对顺序不变）；不给 GIF 设比对阈值；不做雪天独立场景（降水形态由雪线派生，`rain-noon` 取雪线下机位锁定雨形）。

## Capabilities

### New Capabilities

- `weather-camera-showcase`: 雨天与第三人称的固定场景抓帧基线及天气过程演示 GIF。

### Modified Capabilities

- `visual-verification`: 正式场景清单由 24 项扩展为 27 项（新增上列三景，位置、顺序约束与双阈值规则同步更新）。

## Impact

- `packages/client/cmd/mornlea/capture`：新增三场景构造 + 天气注入（headless 固定天气夹具）+ 第三人称机位装配 + `weather-cycle` motion 演示入口；场景清单常量与顺序测试同步。
- `testdata/visual-golden/`：新增 3 张 `world/` PNG golden 与 1 个 `motion/` GIF；既有 24 张字节不变。
- 性能：只影响抓帧模式；游戏与基准路径零新增开销。
- 合规：`make visual-update` 显式生成、新图逐图人工复核后入库；不放宽双阈值；GIF 不进比对门禁。
