# container-ui-presentation Delta

## MODIFIED Requirements

### Requirement: 三类容器使用统一的原创像素表面

系统 SHALL 在既有 HUD pass 内为背包/合成、熔炉和箱子呈现统一的原创程序化像素框、栏位凹槽、标题和来源选择轮廓。凹槽、三个标题、熔炉火焰与进度箭头 MUST 来自既有 HUD atlas 的固定程序化 cell，不得来自二进制 UI 资产或 Mojang 像素；每个打开的 overlay MUST 恰好追加一个标题 quad，标题 MUST NOT 进入固定 700 glyph 流。背包/合成 overlay 的合成区 MUST 呈现个人 2×2 或工作台 3×3 网格与一个产物格，替代既有十条固定配方列表；网格格与产物格 MUST 使用与全部栏位相同的凹槽 cell。既有 slot、bar 与 hit-test 几何中不属于配方列表的部分 MUST 保持不变；panel 只可向上扩出恰好 20px header，其他三条边 MUST 保持不变，且标题与 header MUST NOT 与任何命中区域相交。

#### Scenario: 背包与合成使用像素框、程序化标题与格子

- **GIVEN** 玩家打开普通背包且没有打开熔炉或箱子
- **WHEN** HUD 绘制背包、快捷栏与合成区
- **THEN** 外框、36 个栏位凹槽、2×2 网格格、产物格和来源轮廓 MUST 使用同一套原创像素风格
- **AND** 背包/合成 overlay MUST 追加恰好一个 atlas 标题 quad 且不增加 glyph

#### Scenario: 工作台界面呈现 3×3 网格

- **GIVEN** 玩家已打开工作台并收到尺寸 3 的权威网格状态
- **WHEN** HUD 绘制同一 overlay
- **THEN** 合成区 MUST 呈现恰好 3×3 网格格与一个产物格，凹槽与标题风格与普通背包一致

#### Scenario: 熔炉使用像素流程图示

- **GIVEN** 玩家打开一个包含已确认输入、燃料、输出、燃烧进度和熔炼进度的熔炉
- **WHEN** HUD 绘制熔炉 overlay
- **THEN** 三个栏位 MUST 使用与背包相同的原创凹槽，overlay MUST 追加恰好一个程序化标题 quad
- **AND** 燃烧和熔炼进度 MUST 分别以原创火焰与箭头像素图示表达，空、部分和完成状态 MUST 可区分
- **AND** 图示 MUST 复用既有进度 bar quad 的位置与数量，燃烧填充以火焰 cell 自下向上裁剪，熔炼填充以箭头 cell 自左向右裁剪
- **AND** 填充实例尺寸与 UV 端点 MUST 按同一进度比例收缩，不得压缩完整图标，也不得增加新的进度 quad 或 glyph

#### Scenario: 箱子使用同一像素语言

- **GIVEN** 玩家打开一个包含 27 个权威容器格的箱子
- **WHEN** HUD 绘制箱子与玩家背包
- **THEN** 箱子外框、27 个箱子栏位、36 个玩家栏位和来源轮廓 MUST 使用同一套原创像素风格
- **AND** 箱子 overlay MUST 追加恰好一个程序化标题 quad 且不增加 glyph

### Requirement: 容器换肤不改变统一栏位和权威交互

系统 SHALL 保持背包打开态的统一视图命中：合成网格格 `0..8`、36 个背包栏位与一个产物格，产物格 MUST NOT 作为普通移动目标命中。熔炉 `0..38` 的 39 个统一栏位和箱子 `0..62` 的 63 个统一栏位 MUST 保持。每个栏位与格子的左上闭、右下开命中结果 MUST 确定；标题、框线、凹槽、来源轮廓和熔炉图示 MUST NOT 遮挡或扩张任何命中区域，网格格与产物格的命中矩形 MUST 与各自绘制矩形一致且互不相交。

第一次点击有效格 MUST 只记录当前来源并显示来源轮廓；第二次点击有效目标 MUST 发送恰好一个移动请求（统一视图内的网格移动或既有容器移动）并清除来源；点击产物格 MUST 发送恰好一次产物取出请求。合成网格的格 `0` MUST 位于最上一行最左一列，行序与形状表的行主序（顶排在先）一致，使工具类配方以直立形态呈现。客户端 MUST NOT 因任何点击而本地扣除、增加或移动物品；服务端继续是 inventory、grid、furnace 和 chest 状态的唯一权威。

#### Scenario: 普通背包保持网格、36 格与产物格命中

- **GIVEN** 普通背包与合成区已经打开
- **WHEN** 穷举每个网格格、栏位和产物格的边界内外样本
- **THEN** 背包打开态的命中 MUST 恰好覆盖统一视图格（网格 `0..8`、背包 `9..44`）与产物格，全部边界结果确定且与绘制矩形一致
- **AND** 产物格 MUST NOT 作为普通移动目标出现

#### Scenario: 熔炉保持 39 格两次点击整堆移动

- **GIVEN** 玩家背包和熔炉三个格都来自最后确认镜像
- **WHEN** 玩家先点击统一来源栏位再点击不同的统一目标栏位
- **THEN** 熔炉打开态 MUST 仍只命中 `0..38` 的 39 个栏位
- **AND** 客户端 MUST 只在第二次点击后发送一个引用现有容器、来源和目标的整堆移动请求
- **AND** 请求确认前 inventory 与 furnace 镜像 MUST 保持逐格不变

#### Scenario: 箱子保持 63 格两次点击整堆移动

- **GIVEN** 玩家背包和箱子 27 格都来自最后确认镜像
- **WHEN** 玩家先点击统一来源栏位再点击不同的统一目标栏位
- **THEN** 箱子打开态 MUST 仍只命中 `0..62` 的 63 个栏位
- **AND** 客户端 MUST 只在第二次点击后发送一个引用现有容器、来源和目标的整堆移动请求
- **AND** 请求确认前 inventory 与 chest 镜像 MUST 保持逐格不变

### Requirement: 容器像素换肤保持固定 HUD 资源契约

系统 SHALL 复用既有 hotbar atlas、layout、48-byte instance 编码和 HUD GPU pass。三类 overlay 各自只比当前组成增加一个标题 quad 且零 glyph；合成区以网格格与产物格取代既有十条配方行后，最大打开态的精确 quad 数量 MUST 由布局边界测试锁定且 MUST NOT 超过 scenario v19 固定的 267 quad。固定 glyph 上限 MUST 仍为 700，glyph offset MUST 仍为 13312 bytes，总容量 MUST 仍为 46912 bytes，所有固定区间 MUST 继续按 256 bytes 对齐。稳定态 MUST 只更新既有固定上传资源的实际实例前缀，不得创建每帧动态 GPU 资源。

#### Scenario: 最大打开态装入 scenario v19 固定缓冲

- **GIVEN** 打开态同时取合法 overlay（含 3×3 工作台合成区）、物品数量、来源轮廓和生存状态的最坏互斥组合
- **WHEN** HUD 准备 quad 与 glyph 实例
- **THEN** quad 数量 MUST 等于布局边界测试锁定的固定值且不超过固定上限 267
- **AND** glyph 上限、glyph offset、总容量、instance 大小和对齐 MUST 分别保持 700、13312 bytes、46912 bytes、48 bytes 和 256 bytes
- **AND** 标题 MUST 只占一个 quad 且不占 glyph

#### Scenario: 零尺寸与窄窗口不引入资源或边界例外

- **GIVEN** framebuffer 为零尺寸或现有支持的窄正尺寸
- **WHEN** 任一容器 overlay 准备布局
- **THEN** 零尺寸 MUST 不发出实例，正尺寸的 `openHUDHeight` MUST 包含同一 20px header 高度，全部实例 MUST 有限且严格位于 framebuffer 内
- **AND** 极窄或极矮正尺寸 MUST 继续由统一 HUD 缩放比缩放绘制与命中，标题、header 和所有命中矩形 MUST 保持不相交
- **AND** 系统 MUST NOT 为容器换肤分配动态 GPU 资源或放宽固定容量
