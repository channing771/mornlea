# rust-client-render-entities Specification

## ADDED Requirements

### Requirement: avatar 容量扩大到 75 具身体且天然拒绝超额

实体渲染 pass SHALL 支持至多 75 具身体（450 个 80-byte instance）；第 76 具 MUST 被拒绝且 MUST NOT 产生部分渲染。容量 MUST 由 Go 与 Rust 两侧共享同一字节布局，上传与 FFI 长度 MUST 精确一致；容量错误 MUST 在帧边界稳定报告。

#### Scenario: 恰 75 体渲染成功

- **GIVEN** 渲染输入含 75 具 body 的 instance 流
- **WHEN** 客户端渲染一帧
- **THEN** 帧 MUST 正常完成且全部 75 具渲染

#### Scenario: 第 76 体被拒绝

- **GIVEN** 渲染输入含 76 具 body 的 instance 流
- **WHEN** 客户端渲染一帧
- **THEN** 帧 MUST 被拒绝或稳定降级，MUST 不产生超界写入或部分实例

### Requirement: 夜行者使用独立实体 kind 且绝不生成名标

夜行者 MUST 使用独立于玩家与伙伴的实体 kind，渲染为原创轮廓（相同 6-cuboid 结构但头身比例不同）与固定原创调色。夜行者 MUST NOT 进入名称标签集合；名称标签容量 MUST 保持既有上限且不因夜行者数量变化。

#### Scenario: 多只夜行者不产生名标

- **GIVEN** 视野内存在 8 只夜行者与若干玩家/伙伴
- **WHEN** 客户端渲染一帧
- **THEN** 夜行者 MUST 以敌怪调色呈现，MUST NOT 出现任何与夜行者相关的名称标签，玩家/伙伴的名标 MUST 不受影响

### Requirement: client ABI 升至 v9 且旧动态库被早期拒绝

客户端动态库 ABI SHALL 提升至 v9；低于 v9 版本的动态库在装载或首帧边界 MUST 被稳定拒绝且不产生半启动。容量与 ABI 常量 MUST 不晚于本版本的实现落地。

#### Scenario: 旧 ABI 动态库被拒绝

- **GIVEN** 装载一个 ABI v8 的 `mornlea_client` 动态库
- **WHEN** 客户端启动并校验 ABI
- **THEN** 启动 MUST 被拒绝并报告版本不匹配，MUST NOT 进入渲染循环
