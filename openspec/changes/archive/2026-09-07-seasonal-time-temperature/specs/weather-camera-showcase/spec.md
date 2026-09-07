# weather-camera-showcase Specification

## MODIFIED Requirements

### Requirement: 雨天固定场景抓帧

系统 SHALL 提供 `rain-noon` 场景：固定正午（季节化显示相位恰为 6000）、固定机位、固定雨天权威天气夹具，并把 capture 季节相位钉在夏至正午（`DayPhaseOffset` 按当季昼弧补偿，天空与日照和既有基线逐字节一致），经与交互客户端相同的完整呈现链路收敛后无窗口抓取。画面 MUST 同时显示雨粒子、灰化天空与压暗后的露天亮度；降水形态 MUST 由共享温度公式的局部温度决定（夏至正午低海拔为雨，降水柱顶部高于温度边界 y≈84.8 的极少量粒子呈雪尘，属温度梯度的真实表现）。抓帧 MUST NOT 创建或聚焦前台游戏窗口，既有双阈值 MUST 保持不变。

#### Scenario: 雨天三要素同框

- **GIVEN** `rain-noon` 的固定夹具已装入（夏至正午、雨天、固定机位、相位补偿后的显示相位 6000）
- **WHEN** 场景完成预热、网格收敛和上传并抓帧
- **THEN** 图像 MUST 显示雨线粒子（主体）、灰度高于晴天基线的天空与低于晴天正午的露天亮度，柱顶少量雪尘 MUST 由共享温度公式确定性派生
- **AND** 画面 MUST NOT 出现 HUD 像素

#### Scenario: 雨天只走无窗口完整链路

- **GIVEN** `rain-noon` 使用固定天气、季节相位与机位
- **WHEN** 生成或比对该场景
- **THEN** 抓帧 MUST 使用与交互客户端相同的完整呈现链路，且 MUST NOT 创建或聚焦前台游戏窗口
