# visual-verification Specification

## ADDED Requirements

### Requirement: `bed-night` 无窗口夜景场景

无窗口 capture MUST 提供 `bed-night` 场景（位于 `torch-night` 之后、`ai-companion` 之前；`far-horizon` MUST 仍为倒数第二、`water-underwater` MUST 仍为唯一末场景）：固定夜间卧室内同时呈现至少两种朝向的床形态，并用像素差证明床的原创配色与半高轮廓在夜间光照下可辨认。场景 MUST 经与交互客户端相同的完整呈现链路收敛后抓取，MUST NOT 创建或聚焦任何前台窗口；其 golden 由本变更经显式基线更新写入并逐图人工复核，MUST NOT 通过放宽双阈值接受差异。

#### Scenario: 场景可构造且包含多朝向

- **GIVEN** `bed-night` 场景构造代码
- **WHEN** 无窗口运行该场景
- **THEN** 场景 MUST 至少包含两个不同朝向的完整床形态（床头与床尾同框）
- **AND** 运行不得创建或聚焦前台窗口

#### Scenario: 夜间配色可辨认且不污染后续场景

- **GIVEN** `bed-night` 场景完成预热、收敛与抓帧
- **WHEN** 与其 golden 按既有双阈值比对
- **THEN** 比对 MUST 通过，且床的配色与半高轮廓 MUST 可从图像辨认
- **AND** 场景结束后临时床与时间夹具 MUST 一并恢复，使后续场景不继承任何夹具值
