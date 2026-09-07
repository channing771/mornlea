# weather-camera-showcase Specification

## Purpose

为天气呈现与第三人称视角提供像素级与过程级视觉证据，使雨天稳定态、两种第三人称机位与天气切换全过程可审查、可回归。

## Requirements

### Requirement: 雨天固定场景抓帧

系统 SHALL 提供 `rain-noon` 场景：固定正午、固定雪线下机位、固定雨天权威天气夹具，经与交互客户端相同的完整呈现链路收敛后无窗口抓取。画面 MUST 同时显示雨粒子、灰化天空与压暗后的露天亮度，且 MUST NOT 出现雪粒子（机位在雪线以下）。抓帧 MUST NOT 创建或聚焦前台游戏窗口，既有双阈值 MUST 保持不变。

#### Scenario: 雨天三要素同框

- **GIVEN** `rain-noon` 的固定夹具已装入（正午、雨天、雪线下机位）
- **WHEN** 场景完成预热、网格收敛和上传并抓帧
- **THEN** 图像 MUST 显示雨线粒子、灰度高于晴天基线的天空与低于晴天正午的露天亮度
- **AND** 画面 MUST NOT 出现雪粒子或 HUD 像素

#### Scenario: 雨天只走无窗口完整链路

- **GIVEN** `rain-noon` 使用固定天气与固定机位
- **WHEN** 生成或比对该场景
- **THEN** 抓帧 MUST 使用与交互客户端相同的完整呈现链路，且 MUST NOT 创建或聚焦前台游戏窗口

### Requirement: 第三人称双机位固定场景抓帧

系统 SHALL 提供 `camera-third-back` 与 `camera-third-front` 场景：同一世界位置、同一朝向，仅机位模式不同，经完整链路收敛后无窗口抓取。两图 MUST 都显示自身身体且 MUST NOT 含 viewmodel 像素；正面图 MUST 可辨认脸部，背面图 MUST 可辨认背后。两场景 MUST 在清单中相邻且位于 `far-horizon` 之前。

#### Scenario: 双机位互斥与可辨认

- **GIVEN** 两场景的固定夹具已装入（同位置同朝向，仅视角模式不同）
- **WHEN** 分别完成收敛并抓帧
- **THEN** 两图 MUST 都含自身身体像素且都不含 viewmodel 像素
- **AND** 正面图的脸部区域与背面图的背部区域 MUST 各自可辨认

### Requirement: 天气切换过程演示 GIF

系统 SHALL 提供 `weather-cycle` motion 演示：按 tick 步进抓帧覆盖晴转雨、雨转雷暴、雷暴转晴全过程（含粒子起落、天空灰度变化、雷暴闪光帧），以标准库 `image/gif` 编码存入 `motion/`。GIF 只验呈现、不进任何比对；单基线帧预算 MUST 有界（建议不超过 8fps 乘 12 秒共 96 帧，天气段按压缩 tick 推进，不得按真实分钟时长录制）。

#### Scenario: 全过程覆盖

- **GIVEN** `weather-cycle` 演示入口
- **WHEN** 生成 GIF 并逐帧解码审查
- **THEN** 序列 MUST 包含晴天帧、降雨帧、雷暴闪光帧与回晴帧
- **AND** 编码 MUST 确定可复现（同输入逐字节一致）

#### Scenario: 不进比对门禁

- **GIVEN** `weather-cycle.gif` 已入库
- **WHEN** 运行 `make visual-check` 或全量门禁
- **THEN** 该 GIF MUST NOT 参与任何阈值比对，且 MUST NOT 改变既有 PNG 的比对结论
