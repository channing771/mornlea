# rust-client-render-entities Specification

## Purpose

把客户端一帧的实体、文本与 HUD 呈现补齐到 Rust 离屏渲染器,以完整帧的双后端图像一致性锁定平行实现与 Go 渲染器的等价性;生产渲染路径本期保持不变。
## Requirements
### Requirement: 完整帧双后端图像一致

对同一份完整帧输入(地形 sections、avatar/掉落物 instance 流、目标方块
轮廓、名牌/HUD/调试面板顶点流、伤害红边强度、字形与 HUD 图集字节),Rust
离屏渲染器输出 MUST 与 Go 渲染器整帧输出在既有 `diffThreshold` 双阈值内
一致;对照 MUST 覆盖含实体与文本的既有 capture 场景,阈值 MUST NOT 放宽。

#### Scenario: 含实体与名牌的场景双后端对照通过

- GIVEN 含远端玩家/伙伴 avatar 与 Unicode 名牌的 capture 场景帧数据
- WHEN 同帧数据分别驱动 Go 渲染器与 Rust 渲染器并回读图像
- THEN 两图差异落在既有 diffThreshold 双阈值内

#### Scenario: 含 HUD 与调试面板的场景双后端对照通过

- GIVEN 含快捷栏 HUD 与调试面板文本的帧数据
- WHEN 双后端分别渲染整帧
- THEN 两图差异落在既有 diffThreshold 双阈值内

### Requirement: frame v2 单次 FFI 与 pass 段完整性

每帧的全部可变呈现数据(可见 sections、instance 流、顶点流、uniform 标量)
MUST 经一次 frame v2 调用过境;帧内 MUST NOT 产生逐 pass 渲染 FFI。pass 段
MUST 按 tag+length 编码,未知 tag 或长度越界 MUST 被拒绝且不产生部分渲染。

#### Scenario: 帧循环调用计数

- GIVEN 已装配完整场景的 Rust 渲染器
- WHEN 连续渲染多帧且资源无变化
- THEN 每帧恰好一次渲染 FFI,无图集或 section 上传调用

#### Scenario: 非法 pass 段被拒绝

- GIVEN 构造的含未知 tag 或长度越界 pass 段的 frame v2 输入
- WHEN 调用渲染入口
- THEN 返回输入错误状态,离屏 target 保持上一帧内容

### Requirement: 字形与 HUD 图集字节同源

Rust 侧字形图集内容 MUST 来自 Go 字形光栅化 worker 的增量矩形上传,与 Go
渲染器写入自身图集的字节一致;HUD 图集 MUST 来自 Go 构建的同一份像素。
Rust MUST NOT 内置字体光栅化或 HUD 贴图生成。

#### Scenario: 相同文本的名牌字形一致

- GIVEN 同一段 Unicode 名牌文本经 Go 光栅化并同步上传两个后端
- WHEN 双后端渲染名牌
- THEN 字形区域像素差异落在既有阈值内

### Requirement: 生产渲染路径保持不变

本变更交付后,客户端与无窗口 capture 的生产渲染 MUST 仍由 Go 渲染器执行;
golden 基线字节 MUST 不变,`internal/gfx` 与 Go WebGPU 绑定 MUST 保持在位。

#### Scenario: 既有视觉门禁零改动通过

- GIVEN 既有 capture golden 测试与视觉比对门禁
- WHEN 在本变更合并后的主线运行
- THEN 全部断言与 golden 文件不修改而通过

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

### Requirement: client ABI 升至 v10 且旧动态库被早期拒绝

客户端动态库 ABI SHALL 提升至 v10；低于 v10 版本的动态库在装载或首帧边界 MUST 被稳定拒绝且不产生半启动。容量与 ABI 常量 MUST 不晚于本版本的实现落地。

#### Scenario: 旧 ABI 动态库被拒绝

- **GIVEN** 装载一个 ABI v9 的 `mornlea_client` 动态库
- **WHEN** 客户端启动并校验 ABI
- **THEN** 启动 MUST 被拒绝并报告版本不匹配，MUST NOT 进入渲染循环

