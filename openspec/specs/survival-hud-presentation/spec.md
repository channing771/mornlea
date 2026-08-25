# survival-hud-presentation Specification

## Purpose

为玩家提供紧凑、颜色无关且在窄窗口与容器界面中仍可读的生存 HUD，同时保持所有状态的既有权威来源和固定渲染资源契约。
## Requirements
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

系统 SHALL 只消费已确认的权威生命值，并 MUST 先把该值钳制到 `0..core.MaxHealth`，再让固定十个槽位各自直接解析为空心、半心或满心。每个槽位 MUST 恰好产生一个完整 cell，生命行 MUST NOT 绘制背景面板；未确认生命值 MUST 完全隐藏。

#### Scenario: 奇数生命显示半心

- **GIVEN** 已确认生命值为一个落在 `0..core.MaxHealth` 内的奇数
- **WHEN** 系统布局生命状态行
- **THEN** 系统 MUST 显示十个 resolved 心形槽位，其中对应数量的槽位直接使用满心 cell，并有恰好一个槽位直接使用半心 cell
- **AND** 未填充的其余槽位 MUST 直接使用空心 cell，每个槽位 MUST 只有一个实例
- **AND** 生命行 MUST NOT 产生任何背景面板实例

#### Scenario: 越界和未确认生命安全处理

- **GIVEN** 生命值未确认或高于 `core.MaxHealth`
- **WHEN** 系统布局生命状态行
- **THEN** 未确认值 MUST 不产生任何生命实例
- **AND** 高于上限的已确认值 MUST 按 `core.MaxHealth` 呈现且不得超过十个满心

### Requirement: 氧气以耗损时可见的十段气泡呈现

系统 SHALL 只消费已确认的权威氧气值并将其钳制到 `0..core.MaxOxygenTicks`。满氧与未确认氧气 MUST 完全隐藏；氧气耗损时 MUST 把十个槽位中的 `ceil(value * 10 / core.MaxOxygenTicks)` 个直接解析为满气泡，其余槽位直接解析为空气泡。每个槽位 MUST 恰好产生一个完整 cell，零值 MUST 产生十个空气泡。

#### Scenario: 满氧完全不占用呈现实例

- **GIVEN** 氧气未确认或已确认值等于 `core.MaxOxygenTicks`
- **WHEN** 系统布局氧气状态行
- **THEN** 氧气 MUST 不产生任何呈现实例

#### Scenario: 耗损氧气按向上取整分段

- **GIVEN** 已确认氧气值小于 `core.MaxOxygenTicks`
- **WHEN** 系统布局氧气状态行
- **THEN** 系统 MUST 显示十个 resolved 气泡槽位，每个槽位 MUST 只有一个实例
- **AND** 直接使用满气泡 cell 的槽位数量 MUST 等于 `ceil(value * 10 / core.MaxOxygenTicks)`，其余槽位 MUST 直接使用空气泡 cell，其中零值 MUST 产生十个空气泡

### Requirement: 饥饿以十个常驻空槽和权威填充呈现

系统 SHALL 只消费已确认的权威饥饿值，并 MUST 先把该值钳制到 `0..core.MaxHunger`。未确认饥饿 MUST 完全隐藏；已确认饥饿无论是否满值都 MUST 显示十个空鸡腿槽，并在其上追加 `ceil(Hunger / 2)` 个填充鸡腿。奇数值的最后一个填充 MUST 只覆盖对应鸡腿的右半边，饥饿条 MUST NOT 绘制背景面板。

#### Scenario: 已确认饥饿始终显示十格刻度

- **GIVEN** 已确认饥饿值为 `0`、任一中间值、`core.MaxHunger` 或高于 `core.MaxHunger`
- **WHEN** 系统布局饥饿状态行
- **THEN** 系统 MUST 恰好显示十个空鸡腿槽
- **AND** 填充鸡腿数量 MUST 等于钳制后饥饿值的 `ceil(Hunger / 2)`
- **AND** 饥饿条 MUST NOT 产生背景面板实例

#### Scenario: 奇数饥饿使用右半格且未确认值隐藏

- **GIVEN** 一份奇数的已确认饥饿值和一份未确认饥饿值
- **WHEN** 系统分别布局两份状态
- **THEN** 已确认值的最后一个填充 MUST 只显示对应鸡腿的右半边
- **AND** 未确认值 MUST 不产生任何鸡腿实例

### Requirement: 主状态行锚定快捷栏边缘且氧气向外堆叠

生命与饥饿 SHALL 组成稳定的主状态行。两条状态各自的设计宽度 MUST 为十个 16 design-pixel 图标加九个 1 design-pixel 间隔，即 `169` design pixels；生命起点 MUST 精确等于 `hotbarRowBounds` 的快捷栏左边缘，饥饿终点 MUST 精确等于同一边界的快捷栏右边缘，两者之间的空间 MUST 由快捷栏实际宽度自然得出而不得引入新的栏间宽度或主状态组宽度常量。生命、饥饿、氧气、快捷栏和采掘反馈 MUST 使用完全相同的 `hudScale`。

耗损氧气 SHALL 作为沿饥饿右边缘对齐的次状态行。关闭容器时，氧气 MUST 位于饥饿上方恰好一个 `healthHeartSize + statusBarGap` design row；打开任一容器时，氧气 MUST 位于饥饿下方恰好一个相同 design row，使次状态行总是从快捷栏向外堆叠。满氧或未确认氧气不产生实例，但这条次状态行 MUST 继续计入共享垂直缩放、关闭态采掘/聊天锚点和打开态底部预留，因此生命、饥饿及周边 overlay 不得随氧气显隐跳动。

#### Scenario: 生命和饥饿精确对齐快捷栏两端

- **GIVEN** 已确认生命与饥饿状态
- **WHEN** 系统布局关闭或打开容器的主状态行
- **THEN** 生命起点 MUST 与快捷栏左边缘完全相同
- **AND** 饥饿终点 MUST 与快捷栏右边缘完全相同，且饥饿仍 MUST 从右向左排列
- **AND** 两条状态各自的设计宽度 MUST 为 `169`，并与快捷栏、氧气和采掘反馈使用相同的缩放比例

#### Scenario: 耗损氧气沿饥饿右边缘向外堆叠

- **GIVEN** 已确认的耗损氧气、生命与饥饿状态
- **WHEN** 系统分别布局关闭态与打开态 HUD
- **THEN** 两种状态下氧气终点 MUST 与饥饿终点完全相同
- **AND** 关闭态氧气 MUST 位于饥饿上方一个 `healthHeartSize + statusBarGap` design row
- **AND** 打开态氧气 MUST 位于饥饿下方一个 `healthHeartSize + statusBarGap` design row

#### Scenario: 满氧隐藏不改变主行或周边锚点

- **GIVEN** 两帧具有相同的生命与饥饿值，其中一帧氧气耗损、另一帧氧气为满值
- **WHEN** 系统布局两帧状态栈、采掘反馈与聊天
- **THEN** 满氧帧 MUST 不产生气泡实例
- **AND** 两帧的生命与饥饿 X/Y 坐标 MUST 逐实例完全相同
- **AND** 两帧的共享垂直缩放、关闭态采掘与聊天锚点 MUST 完全相同

### Requirement: 采掘进度同时使用颜色和形状反馈

活动且所需 tick 非零的权威采掘进度 SHALL 位于关闭态完整两行状态栈上方，进度比例 MUST 钳制到 `0..100%`。可采目标和不可采目标 MUST 同时具有颜色差异与形状差异；可采进度 MUST 具有随填充末端移动的亮标记，不可采进度 MUST 具有固定数量和位置的警示缺口，且不得使用固定警示缺口冒充可采末端标记。

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

系统 SHALL 把快捷栏、生命、氧气、饥饿和采掘反馈作为同一布局整体响应 framebuffer 尺寸。空间不足时所有元素 MUST 使用同一比例等比缩小；宽度约束 MUST 继续来自既有快捷栏或打开态面板内容，因为生命与饥饿锚定在快捷栏边缘以内，不得再加入独立的主状态组宽度常量。打开态与关闭态的高度约束 MUST 都容纳主状态行和向外堆叠的氧气次行，即比单行状态布局多恰好一个 `healthHeartSize + statusBarGap` design row；宽或高为零时 MUST 不产生任何实例；任一正尺寸 framebuffer 中包括两条状态行在内的所有 HUD 矩形 MUST 完全位于 framebuffer 内。

#### Scenario: 窄窗口保持同一比例并不越界

- **GIVEN** 一个不足以按设计尺寸容纳完整生存 HUD 的正尺寸 framebuffer
- **WHEN** 系统布局快捷栏、耗损氧气、生命、饥饿和活动采掘反馈
- **THEN** 全部元素 MUST 使用同一缩放比例
- **AND** 每个矩形 MUST 完全位于 framebuffer 内

#### Scenario: 零尺寸不生成实例

- **GIVEN** framebuffer 的宽或高为零
- **WHEN** 系统准备生存 HUD
- **THEN** 系统 MUST 不生成任何 HUD 矩形或字形实例

### Requirement: 两行状态栈按容器状态向快捷栏外侧避让

关闭背包、合成、箱子或熔炉界面时，生命/饥饿主状态行 SHALL 位于快捷栏正上方一行，耗损氧气 SHALL 沿饥饿右边缘堆叠在主状态行上方，采掘反馈 SHALL 位于完整两行状态栈上方。任一容器界面打开时，同一个主状态行 MUST 保持可见并移至快捷栏正下方一行，耗损氧气 MUST 沿饥饿右边缘继续向下堆叠；打开态底部预留与高度约束 MUST 比单行布局增加恰好一个 `healthHeartSize + statusBarGap` design row。两条状态行 MUST 完全位于 framebuffer 内，不得覆盖、相交或改变任何既有可交互物品格或十行配方的区域。

#### Scenario: 关闭容器时状态与采掘按行堆叠

- **GIVEN** 容器界面关闭且两行状态栈和活动采掘反馈均可见
- **WHEN** 系统布局 HUD
- **THEN** 生命/饥饿主状态行 MUST 位于快捷栏正上方一行
- **AND** 氧气次状态行 MUST 位于饥饿上方一行并共享其右边缘
- **AND** 采掘反馈 MUST 位于完整两行状态栈正上方

#### Scenario: 打开容器后两行状态栈保持可见且不拦截格子

- **GIVEN** 已打开任一既有容器界面，并已确认低生命、耗损氧气与饥饿
- **WHEN** 系统布局 HUD 并对全部 36 个可交互物品格执行既有命中测试
- **THEN** 生命/饥饿主状态行 MUST 位于快捷栏正下方一行，氧气 MUST 位于饥饿下方一行，且两行都完全位于 framebuffer 内
- **AND** 两行状态 MUST 不与任何可交互物品格矩形或十行配方相交，既有命中结果 MUST 保持不变

### Requirement: 生存 HUD 保持固定资源和实例兼容性

生存 HUD SHALL 继续使用 main 的 benchmark scenario v19 已锁定的固定容量预分配资源、同一个既有 HUD pass 和相同的 48-byte instance 编码。`maxHotbarQuads` MUST 保持 267，`maxHotbarGlyphs` MUST 保持 700，glyph offset MUST 保持 13312 bytes，固定上传总容量 MUST 保持 46912 bytes，所有固定区间 offset MUST 保持 256-byte 对齐。合法最坏组合 MUST 不超过这些固定上限；稳定态准备和呈现 MUST 保持零每帧动态 GPU 资源，且不得新增 HUD shader、GPU pass、上传格式、API、ABI、配置项或依赖。

#### Scenario: 关闭和打开界面的合法最坏组合均有界

- **GIVEN** 分别构造关闭界面的最坏快捷栏、状态与采掘组合，以及打开界面的最大背包、容器、状态与聊天组合
- **WHEN** 系统准备两种 HUD
- **THEN** 加入最多 20 个饥饿 quad 后，关闭和打开合法最坏组合的 quad 数量 MUST 分别恰好为 96 和 265，且都 MUST 不超过固定上限 267
- **AND** 合法最大 glyph 组合 MUST 不超过固定上限 700
- **AND** 两种组合 MUST 继续通过同一 HUD pass 以相同 48-byte instance 编码输出
- **AND** glyph offset 与固定上传总容量 MUST 分别保持 13312 与 46912 bytes，固定区间 MUST 保持 256-byte 对齐

#### Scenario: 稳定态不创建每帧动态 GPU 资源

- **GIVEN** HUD atlas 与固定上传资源已经预热完成
- **WHEN** 系统连续准备和呈现多帧生存 HUD
- **THEN** 每帧 MUST 只更新固定资源中的实际实例前缀
- **AND** MUST NOT 创建新的每帧动态 GPU 资源、HUD pass 或 shader

### Requirement: HUD 图集列采样稳定性

HUD 图集的每个图标 cell SHALL 从其自己的图集列纹素范围内采样，且采样范围 MUST 与列边界保持一个不小于 1/512 纹素的安全裕度。当图集因物品表追加而扩列（总宽度增长）时，既有 cell 的采样 MUST NOT 改变其解析到的纹素集合——既不串入相邻列材质，也不产生可观察的亚像素漂移。

#### Scenario: 扩列后既有 cell 解码纹素集合不变

- **GIVEN** 当前图集宽度下任意一个合法 HUD cell 列
- **WHEN** 图集宽度按任意「固定列宽 16 纹素 × 更大列数」的方式增长并重新计算该列的归一化 UV
- **THEN** 该列 UV 解码回纹素空间后的左右界 MUST 仍严格落在原列的 16 个纹素内，且距两侧列边界各不少于 1/512 纹素
- **AND** 以该列内均匀分布的探针位置解码，扩列前后 MUST 得到完全相同的纹素下标集合

#### Scenario: 相邻列采样区间互不侵入

- **GIVEN** 任意图集宽度下的全部合法 HUD cell 列
- **WHEN** 计算每列的归一化 UV 并解码回纹素空间
- **THEN** 每列的采样区间 MUST 与相邻列的采样区间无重叠，且任意一列的区间 MUST NOT 覆盖相邻列的任何纹素

#### Scenario: 边界收进不破坏既有列界语义

- **GIVEN** 任意图集宽度与任一合法列
- **WHEN** 该列 UV 的左右界解码回纹素空间
- **THEN** 左右界的解码值与精确列边界（`列号 × 16` 与 `(列号 + 1) × 16`）的偏差 MUST 小于 1/64 纹素，保证收进只消除边界歧义而不改变列的语义位置

