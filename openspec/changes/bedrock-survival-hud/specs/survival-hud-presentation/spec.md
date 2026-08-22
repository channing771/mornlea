## Purpose

为玩家提供紧凑、颜色无关且在窄窗口与容器界面中仍可读的生存 HUD，同时保持所有状态的既有权威来源和固定渲染资源契约。

## ADDED Requirements

### Requirement: 快捷栏固定为居中九格并明确标识选中格

系统 SHALL 在关闭容器界面时显示固定九格、水平居中的快捷栏。选中格 MUST 同时具有外扩的高对比外边框和强调色内边框，使其在忽略颜色后仍可由双层轮廓及外扩几何与未选中格区分。

#### Scenario: 九格快捷栏和选中格可由几何判定

- **GIVEN** 已确认的有效快捷栏镜像和任一合法选中下标
- **WHEN** 系统在关闭容器界面时布局 HUD
- **THEN** 快捷栏 MUST 恰好包含九个等尺寸格并整体水平居中
- **AND** 只有选中格 MUST 具有外扩双层边框，且该边框 MUST NOT 仅靠颜色区别于其他格

### Requirement: 数量与耐久保持权威镜像语义

快捷栏物品数量和工具耐久 SHALL 继续只由已确认的权威物品镜像驱动。数量 MUST 继续只对多于一件的堆叠显示最多两位的阴影与前景数字；耐久 MUST 继续只对耐久上限存在且 `0 < Durability < MaxDurability` 的工具显示背景与剩余比例填充。

#### Scenario: 单件与满耐久不产生附加标记

- **GIVEN** 一个数量为一的物品堆叠和一个满耐久工具
- **WHEN** 系统布局快捷栏
- **THEN** 单件堆叠 MUST NOT 显示数量数字
- **AND** 满耐久工具 MUST NOT 显示耐久条

#### Scenario: 多件与磨损工具沿用确认值

- **GIVEN** 已确认镜像包含数量大于一的堆叠和一个部分磨损工具
- **WHEN** 系统布局快捷栏
- **THEN** 数量 MUST 以阴影和前景两层显示，且最多显示两位
- **AND** 耐久填充比例 MUST 由该镜像中的当前耐久和耐久上限决定

### Requirement: 生命以十个空半满心呈现

系统 SHALL 只消费已确认的权威生命值，并 MUST 先把该值钳制到 `0..core.MaxHealth`，再以固定十个槽位的空心、半心和满心呈现。生命行 MUST NOT 绘制背景面板；未确认生命值 MUST 完全隐藏。

#### Scenario: 奇数生命显示半心

- **GIVEN** 已确认生命值为一个落在 `0..core.MaxHealth` 内的奇数
- **WHEN** 系统布局生命状态行
- **THEN** 系统 MUST 显示十个空心槽
- **AND** 覆盖层 MUST 包含对应数量的满心和恰好一个半心
- **AND** 生命行 MUST NOT 产生任何背景面板实例

#### Scenario: 越界和未确认生命安全处理

- **GIVEN** 生命值未确认或高于 `core.MaxHealth`
- **WHEN** 系统布局生命状态行
- **THEN** 未确认值 MUST 不产生任何生命实例
- **AND** 高于上限的已确认值 MUST 按 `core.MaxHealth` 呈现且不得超过十个满心

### Requirement: 氧气以耗损时可见的十段气泡呈现

系统 SHALL 只消费已确认的权威氧气值并将其钳制到 `0..core.MaxOxygenTicks`。满氧与未确认氧气 MUST 完全隐藏；氧气耗损时 MUST 先显示十个空气泡槽，再以 `ceil(value * 10 / core.MaxOxygenTicks)` 个满气泡覆盖，零值 MUST 没有满气泡覆盖。

#### Scenario: 满氧完全不占用呈现实例

- **GIVEN** 氧气未确认或已确认值等于 `core.MaxOxygenTicks`
- **WHEN** 系统布局氧气状态行
- **THEN** 氧气 MUST 不产生任何呈现实例

#### Scenario: 耗损氧气按向上取整分段

- **GIVEN** 已确认氧气值小于 `core.MaxOxygenTicks`
- **WHEN** 系统布局氧气状态行
- **THEN** 系统 MUST 显示十个空气泡槽
- **AND** 满气泡数量 MUST 等于 `ceil(value * 10 / core.MaxOxygenTicks)`，其中零值 MUST 产生零个满气泡

### Requirement: 采掘进度同时使用颜色和形状反馈

活动且所需 tick 非零的权威采掘进度 SHALL 位于生命与氧气状态行上方，进度比例 MUST 钳制到 `0..100%`。可采目标和不可采目标 MUST 同时具有颜色差异与形状差异；可采进度 MUST 具有随填充末端移动的亮标记，不可采进度 MUST 具有固定数量和位置的警示缺口，且不得使用固定警示缺口冒充可采末端标记。

#### Scenario: 超额进度钳制在轨道内

- **GIVEN** 活动采掘的已确认进度大于所需 tick
- **WHEN** 系统布局采掘反馈
- **THEN** 填充 MUST 钳制到轨道宽度且所有标记 MUST 留在轨道边界内

#### Scenario: 忽略颜色仍可区分采掘状态

- **GIVEN** 两个相同中段比例的活动采掘状态，其中一个目标可采、另一个不可采
- **WHEN** 系统布局两种采掘反馈并忽略全部颜色值
- **THEN** 可采状态 MUST 具有填充末端亮标记
- **AND** 不可采状态 MUST 具有固定警示缺口，且两者的矩形几何序列 MUST 不同

#### Scenario: 无有效进度时不显示采掘反馈

- **GIVEN** 采掘未活动或所需 tick 为零
- **WHEN** 系统布局采掘反馈
- **THEN** 系统 MUST 不产生采掘轨道、填充或状态标记

### Requirement: 生存 HUD 在窄 framebuffer 内整体等比缩小

系统 SHALL 把快捷栏、生命、氧气和采掘反馈作为同一布局整体响应 framebuffer 尺寸。空间不足时所有元素 MUST 使用同一比例等比缩小；宽或高为零时 MUST 不产生任何实例；任一正尺寸 framebuffer 中的所有 HUD 矩形 MUST 完全位于 framebuffer 内。

#### Scenario: 窄窗口保持同一比例并不越界

- **GIVEN** 一个不足以按设计尺寸容纳完整生存 HUD 的正尺寸 framebuffer
- **WHEN** 系统布局快捷栏、耗损氧气、生命和活动采掘反馈
- **THEN** 全部元素 MUST 使用同一缩放比例
- **AND** 每个矩形 MUST 完全位于 framebuffer 内

#### Scenario: 零尺寸不生成实例

- **GIVEN** framebuffer 的宽或高为零
- **WHEN** 系统准备生存 HUD
- **THEN** 系统 MUST 不生成任何 HUD 矩形或字形实例

### Requirement: 容器打开时状态行避让既有交互格子

关闭背包、合成、箱子或熔炉界面时，生命状态 SHALL 位于快捷栏上方并与其左边沿对齐，耗损氧气 SHALL 位于快捷栏上方并与其右边沿对齐。任一容器界面打开时，生命和耗损氧气 MUST 保持可见并移至快捷栏下方的既有底部留白，且 MUST NOT 覆盖或改变任何既有可交互物品格的命中区域。

#### Scenario: 打开容器后状态行保持可见且不拦截格子

- **GIVEN** 已打开任一既有容器界面、已确认低生命和已确认耗损氧气
- **WHEN** 系统布局 HUD 并对所有可交互物品格执行既有命中测试
- **THEN** 生命和氧气 MUST 位于快捷栏下方且完全可见
- **AND** 它们 MUST 不与任何可交互物品格矩形相交，既有命中结果 MUST 保持不变

### Requirement: 生存 HUD 保持固定资源和实例兼容性

生存 HUD SHALL 继续使用固定容量的预分配资源、同一个既有 HUD pass 和相同的 48-byte instance 编码。合法最坏组合 MUST 不超过固定 quad 与 glyph 上限；稳定态准备和呈现 MUST 保持零每帧动态 GPU 资源，且不得新增 HUD shader、GPU pass 或上传格式。

#### Scenario: 关闭和打开界面的合法最坏组合均有界

- **GIVEN** 分别构造关闭界面的最坏快捷栏、状态与采掘组合，以及打开界面的最大背包、容器、状态与聊天组合
- **WHEN** 系统准备两种 HUD
- **THEN** 两种组合的 quad 与 glyph 数量 MUST 均不超过各自固定上限
- **AND** 两种组合 MUST 继续通过同一 HUD pass 以相同 48-byte instance 编码输出

#### Scenario: 稳定态不创建每帧动态 GPU 资源

- **GIVEN** HUD atlas 与固定上传资源已经预热完成
- **WHEN** 系统连续准备和呈现多帧生存 HUD
- **THEN** 每帧 MUST 只更新固定资源中的实际实例前缀
- **AND** MUST NOT 创建新的每帧动态 GPU 资源、HUD pass 或 shader
