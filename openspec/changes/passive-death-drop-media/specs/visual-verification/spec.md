## ADDED Requirements

### Requirement: GIF 动态基线覆盖牛行为剧本

系统 SHALL 为牛行为剧本提供 GIF 动态基线：吃草前后、持麦靠近、击杀与牛肉掉落按 tick 步进抓帧（禁用墙钟），以标准库 `image/gif` 编码并存入 `testdata/` 下 `.gif` 基线。单基线帧预算 MUST 有界（建议 ≤8fps×6s=48 帧，参照录制上限与 manifest 纪律）。比对时解码逐帧并沿用双阈值（最大通道差与差异像素占比）逐帧裁决；全部帧通过方为通过。只允许新增基线，既有 PNG 基线 MUST 逐字节不动。

#### Scenario: 剧本 GIF 可复现生成

- **GIVEN** 同一份代码与同一台机器
- **WHEN** 连续两次生成同一剧本 GIF 基线
- **THEN** 两次解码后的逐帧差异 MUST 落在既定双阈值以内

#### Scenario: 击杀剧本覆盖死亡与掉落

- **GIVEN** 击杀剧本的 GIF 基线
- **WHEN** 逐帧解码审查
- **THEN** 序列 MUST 包含红闪侧倒的死亡过渡帧与牛肉掉落小方块帧

#### Scenario: 超帧预算被拒绝

- **GIVEN** 一次请求超过帧预算上限的 GIF 录制
- **WHEN** 系统校验参数
- **THEN** 系统 MUST 在任何帧捕获之前拒绝该请求

#### Scenario: 旧 PNG 基线不受影响

- **GIVEN** 新增的 GIF 基线已入库
- **WHEN** 运行既有 PNG 视觉比对
- **THEN** 全部既有 PNG 基线的字节 MUST 与入库前一致，且比对 MUST 继续使用既有双阈值
