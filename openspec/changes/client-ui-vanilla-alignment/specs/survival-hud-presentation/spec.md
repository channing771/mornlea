# survival-hud-presentation Delta

## MODIFIED Requirements

### Requirement: 生存 HUD 在窄 framebuffer 内整体等比缩小

系统 SHALL 把快捷栏、生命、氧气、饥饿、采掘反馈、准星与物品名弹条作为同一布局整体响应 framebuffer 尺寸。空间不足时所有元素 MUST 使用同一比例等比缩小；宽度约束 MUST 继续来自既有快捷栏或打开态浮动面板内容，因为生命与饥饿锚定在快捷栏边缘以内，不得再加入独立的主状态组宽度常量。打开态与关闭态的高度约束 MUST 都容纳主状态行和向外堆叠的氧气次行，即比单行状态布局多恰好一个 `healthHeartSize + statusBarGap` design row；关闭态高度约束还 MUST 容纳物品名弹条行。宽或高为零时 MUST 不产生任何实例；任一正尺寸 framebuffer 中包括两条状态行、准星与可见弹条在内的所有 HUD 矩形 MUST 完全位于 framebuffer 内。

#### Scenario: 窄窗口保持同一比例并不越界

- **GIVEN** 一个不足以按设计尺寸容纳完整生存 HUD 的正尺寸 framebuffer
- **WHEN** 系统布局快捷栏、耗损氧气、生命、饥饿、活动采掘反馈、准星与可见物品名弹条
- **THEN** 全部元素 MUST 使用同一缩放比例
- **AND** 每个矩形 MUST 完全位于 framebuffer 内

#### Scenario: 零尺寸不生成实例

- **GIVEN** framebuffer 的宽或高为零
- **WHEN** 系统准备生存 HUD
- **THEN** 系统 MUST 不生成任何 HUD 矩形或字形实例

### Requirement: 生存 HUD 保持固定资源和实例兼容性

生存 HUD SHALL 继续使用 main 的 benchmark scenario v20 已锁定的固定容量预分配资源、同一个既有 HUD pass 和相同的 48-byte instance 编码。`maxHotbarQuads` MUST 保持 320，`maxHotbarGlyphs` MUST 保持 768，glyph offset MUST 保持 15616 bytes，固定上传总容量 MUST 保持 52480 bytes，所有固定区间 offset MUST 保持 256-byte 对齐。合法最坏组合 MUST 不超过这些固定上限；稳定态准备和呈现 MUST 保持零每帧动态 GPU 资源，且不得新增 HUD shader、GPU pass、上传格式、API、ABI、配置项或依赖。

#### Scenario: 关闭和打开界面的合法最坏组合均有界

- **GIVEN** 分别构造关闭界面的最坏快捷栏、状态、准星与采掘组合，以及打开界面的最大浮动面板、背包、容器、状态、聊天与悬停 tooltip 组合
- **WHEN** 系统准备两种 HUD
- **THEN** 加入最多 20 个饥饿 quad 后，关闭合法最坏组合的 quad 数量 MUST 恰好为 100，打开合法最坏组合 MUST 恰好为 274，且都 MUST 不超过固定上限 320
- **AND** 合法最大 glyph 组合（含 tooltip 与物品名弹条）MUST 不超过固定上限 768
- **AND** 两种组合 MUST 继续通过同一 HUD pass 以相同 48-byte instance 编码输出
- **AND** glyph offset 与固定上传总容量 MUST 分别保持 15616 与 52480 bytes，固定区间 MUST 保持 256-byte 对齐

#### Scenario: 稳定态不创建每帧动态 GPU 资源

- **GIVEN** HUD atlas 与固定上传资源已经预热完成
- **WHEN** 系统连续准备和呈现多帧生存 HUD
- **THEN** 每帧 MUST 只更新固定资源中的实际实例前缀
- **AND** MUST NOT 创建新的每帧动态 GPU 资源、HUD pass 或 shader

## ADDED Requirements

### Requirement: 准星以屏幕中心十字呈现

系统 SHALL 在生存 HUD 可见时于 viewport 中心呈现原创十字准星：横向与纵向两条等宽臂组成十字，MUST 以深色投影与亮色前景双层矩形呈现，使其在任意世界背景上可辨认；准星 MUST 随 `hudScale` 等比缩放且中心 MUST 与 viewport 几何中心重合；主菜单、设置页与暂停覆盖层等菜单相位 MUST 不绘制准星。准星 MUST NOT 进入 glyph 流，MUST NOT 使用新 shader、GPU pass 或固定容量之外的资源。

#### Scenario: 游戏相位常显且居中

- **GIVEN** 已登录且 HUD 可见的游戏相位
- **WHEN** 系统布局生存 HUD
- **THEN** viewport 中心 MUST 出现由横竖两臂组成的十字准星，其中心 MUST 与 viewport 几何中心重合
- **AND** 准星 MUST 由深色投影与亮色前景两组矩形构成，且不产生任何字形实例

#### Scenario: 菜单相位不绘制

- **GIVEN** 主菜单、设置页或暂停覆盖层可见
- **WHEN** 系统布局该帧
- **THEN** MUST 不产生任何准星实例

#### Scenario: 容器面板覆盖准星

- **GIVEN** 任一容器浮动面板打开且准星位于面板矩形内
- **WHEN** 系统按 HUD 实例顺序呈现
- **THEN** 面板实例 MUST 绘制在准星之后并遮挡二者重叠区域，准星与面板的命中与绘制几何 MUST 互不干扰

### Requirement: 选中栏位变化以物品名弹条呈现

系统 SHALL 在已确认权威镜像的选中栏位下标发生变化时，于两行状态栈上方呈现所选物品的中文名弹条：文本 MUST 来自既有注册表显示名（物品级缺名时回退其方块显示名，均缺省则不显示），以阴影加前景双层字形居中于快捷栏宽度；弹条自确认变化所属权威 tick 起持续恰好 40 tick 后消失；容器界面打开或菜单相位 MUST 抑制弹条。弹条 MUST NOT 使用任何新协议字段，MUST NOT 产生 quad 实例，MUST 始终在固定 glyph 预算内呈现，且 MUST 与采掘/进食进度条同帧共存时互不遮挡。

#### Scenario: 确认切换显示物品名

- **GIVEN** 已确认镜像中选中栏位变化为含已知中文名物品的格
- **WHEN** 系统布局弹条
- **THEN** 弹条 MUST 居中显示该物品中文名，且文字带阴影层

#### Scenario: 40 tick 后消失

- **GIVEN** 弹条自确认变化所属 tick 起持续呈现
- **WHEN** 权威 tick 前进超过 40 tick
- **THEN** 弹条 MUST 不再产生任何字形实例

#### Scenario: 未确认变化不触发

- **GIVEN** 客户端发出的选中请求尚未获得服务端确认
- **WHEN** 系统布局弹条
- **THEN** 弹条 MUST 保持上一确认状态，MUST NOT 因未确认变化显示或刷新

#### Scenario: 容器与菜单抑制

- **GIVEN** 任一容器浮动面板或菜单相位可见
- **WHEN** 选中栏位的确认值发生变化
- **THEN** 弹条 MUST 不产生任何字形实例
