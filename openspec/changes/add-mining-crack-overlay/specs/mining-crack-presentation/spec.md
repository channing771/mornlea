# mining-crack-presentation Delta Spec

## Purpose

为图形客户端定义玩家采掘方块时的世界空间裂纹反馈：以最后确认的权威采掘状态为
唯一进度来源，在目标方块六面呈现 10 级离散递进的透明裂纹 overlay，并在采掘
停止、目标切换或方块破坏时立即清理，不残留、不闪烁。

## ADDED Requirements

### Requirement: 采掘裂纹以离散阶段呈现权威采集进度

图形客户端 SHALL 以最后确认的权威采掘状态（active、目标方块位置、
progressTicks、requiredTicks）作为裂纹呈现的唯一输入：进度比例 MUST 为
`clamp(progressTicks/requiredTicks, 0, 1)`，裂纹阶段 MUST 为
`min(9, floor(比例 × 10))` 的 10 个离散阶段之一；阶段切换 MUST 只由权威状态
更新驱动，客户端 MUST NOT 自建与权威进度脱节的动画计时器或本地预测推进。
权威采掘非 active、requiredTicks 为 0 或无有效目标时，客户端 MUST NOT 呈现
任何裂纹。进度比例饱和到 1 时阶段 MUST 呈现为第 9 阶段（最重裂纹）。

#### Scenario: 阶段随权威进度离散递进

- GIVEN 玩家正在采掘一个 requiredTicks 为 30 的方块
- WHEN 权威状态先后报告 progressTicks 为 1、9、15、30
- THEN 呈现的裂纹阶段 MUST 依次为第 0、3、5、9 阶段，且同一阶段内裂纹图案
  MUST 保持不变（阶段之间离散切换，不做纹理渐变）

#### Scenario: 非 active 或无目标时不呈现裂纹

- GIVEN 客户端持有上一次采掘的裂纹状态
- WHEN 权威采掘状态变为非 active、requiredTicks 为 0 或不含有效目标
- THEN 本帧起 MUST NOT 呈现任何裂纹，且 MUST NOT 沿用上一目标的位置或阶段

#### Scenario: 进度驱动而非计时器驱动

- GIVEN 权威状态长时间停留在同一 progressTicks（如网络暂停）
- WHEN 客户端连续渲染多帧
- THEN 裂纹阶段 MUST 保持不变，MUST NOT 随帧数自行加深

### Requirement: 单一可复用 overlay 随状态切换立即清理

裂纹呈现 SHALL 使用单一可复用的 overlay 资源（固定容量、常驻、以可见性与
实例数据更新表达状态），MUST NOT 在开始采掘、阶段切换或目标切换时创建/销毁
渲染对象。权威目标在两个方块之间切换时，裂纹 MUST 在目标切换后的呈现中只
出现在新目标上；采掘停止、方块被破坏、连接断开或 reset、打开背包/容器或进入
菜单相位时，裂纹 MUST 在下一次呈现中消失。方块破坏的权威方块变更与采掘结束
同帧到达时，MUST NOT 在已变为空气的位置呈现最后一帧裂纹。

#### Scenario: 目标切换立即迁移

- GIVEN 玩家正在采掘方块 A 且裂纹已可见
- WHEN 权威状态的目标变为方块 B（进度按规则重新开始）
- THEN 下一帧 MUST 只在 B 上呈现新进度的裂纹，A 上 MUST NOT 残留裂纹

#### Scenario: 停止采掘立即消失

- GIVEN 裂纹正在呈现
- WHEN 玩家松开采集键使权威采掘变为非 active
- THEN 下一帧起 MUST NOT 呈现裂纹

#### Scenario: 破坏同帧无残留

- GIVEN 玩家采掘的最后一批权威消息同时包含目标方块变为空气与采掘结束
- WHEN 客户端呈现该批消息之后的一帧
- THEN 该位置 MUST NOT 出现裂纹，MUST NOT 出现悬浮于空气中的裂纹

#### Scenario: UI 与断连状态不呈现裂纹

- GIVEN 裂纹正在呈现
- WHEN 打开背包/容器、进入主菜单/暂停相位，或连接断开/reset
- THEN 裂纹 MUST 立即消失，相位恢复后 MUST NOT 恢复显示陈旧裂纹

### Requirement: 裂纹以透明 cutout 覆盖整个方块且深度正确

裂纹 SHALL 以单位立方体形状的 overlay 覆盖当前目标方块的完整包围盒（六面
全部呈现同一阶段裂纹），MUST NOT 只在命中点呈现小贴花。裂纹纹理 MUST 为
透明背景的像素风裂纹图案（深棕/深灰系、清晰轮廓），经 alpha cutout 呈现为
原方块材质之上的叠加层，MUST NOT 替换或修改原方块材质。overlay 几何 MUST
相对单位方块各向外扩不超过 `0.005` 个世界单位以避免深度冲突；深度测试 MUST
沿用 `CompareLessEqual` 且 MUST NOT 写入深度附件，被地形遮挡的裂纹部分
MUST NOT 穿透显示。裂纹呈现路径初始化后 MUST 使用固定容量实例（容量恰为
1）、复用方块材质 atlas，稳定状态下每帧更新 MUST 不产生堆分配，且
MUST NOT 引入每帧动态 GPU 资源创建。

#### Scenario: 六面呈现同一阶段裂纹

- GIVEN 玩家正在采掘一个方块
- WHEN 从任意方向观察该方块
- THEN 该方块可见的每个面 MUST 呈现同一阶段的裂纹图案，且裂纹贴合方块表面
  不随相机方向漂移

#### Scenario: 被遮挡的裂纹不穿透

- GIVEN 正在采掘的方块部分表面被其他地形遮挡
- WHEN 渲染该帧
- THEN 被遮挡部分的裂纹 MUST NOT 可见，裂纹 MUST NOT 写入深度附件

#### Scenario: 稳定呈现固定有界

- GIVEN 渲染器已完成一次预热且裂纹持续可见
- WHEN 连续多帧更新裂纹可见性、实例数据并绘制
- THEN 每帧实例数 MUST 不超过 1，MUST NOT 产生堆分配或每帧动态资源创建，
  MUST NOT 出现第二个裂纹 overlay 或残留实例
