## ADDED Requirements

### Requirement: 三类容器使用统一的原创像素表面

系统 SHALL 在既有 HUD pass 内为背包/合成、熔炉和箱子呈现统一的原创程序化像素框、栏位凹槽、标题和来源选择轮廓。凹槽与三个标题 MUST 来自既有 HUD atlas 的程序化 cell，不得来自二进制 UI 资产或 Mojang 像素；每个打开的 overlay MUST 恰好追加一个标题 quad，标题 MUST NOT 进入固定 700 glyph 流。既有 panel、slot 和 bar quad MUST 保持原几何，只可原位更换 UV 或颜色。

#### Scenario: 背包与合成使用像素框和程序化标题

- **GIVEN** 玩家打开普通背包且没有打开熔炉或箱子
- **WHEN** HUD 绘制背包、快捷栏和固定合成区域
- **THEN** 外框、36 个栏位凹槽和来源轮廓 MUST 使用同一套原创像素风格
- **AND** 背包/合成 overlay MUST 追加恰好一个 atlas 标题 quad 且不增加 glyph
- **AND** 十条固定配方的输入、输出、按钮与可用状态 MUST 全部保持可辨认

#### Scenario: 熔炉使用像素流程图示

- **GIVEN** 玩家打开一个包含已确认输入、燃料、输出、燃烧进度和熔炼进度的熔炉
- **WHEN** HUD 绘制熔炉 overlay
- **THEN** 三个栏位 MUST 使用与背包相同的原创凹槽，overlay MUST 追加恰好一个程序化标题 quad
- **AND** 燃烧和熔炼进度 MUST 分别以原创火焰与箭头像素图示表达，空、部分和完成状态 MUST 可区分
- **AND** 图示 MUST 复用既有进度 bar quad 的位置与数量，不得增加新的进度 quad 或 glyph

#### Scenario: 箱子使用同一像素语言

- **GIVEN** 玩家打开一个包含 27 个权威容器格的箱子
- **WHEN** HUD 绘制箱子与玩家背包
- **THEN** 箱子外框、27 个箱子栏位、36 个玩家栏位和来源轮廓 MUST 使用同一套原创像素风格
- **AND** 箱子 overlay MUST 追加恰好一个程序化标题 quad 且不增加 glyph

### Requirement: 容器换肤不改变统一栏位和权威交互

系统 SHALL 保持普通背包 `0..35` 的 36 个统一栏位、熔炉 `0..38` 的 39 个统一栏位和箱子 `0..62` 的 63 个统一栏位。换肤前后每个栏位与十条固定配方按钮的左上闭、右下开命中结果 MUST 相同；标题、框线、凹槽、来源轮廓和熔炉图示 MUST NOT 遮挡或扩张任何命中区域。

第一次点击有效栏位 MUST 只记录当前来源并显示来源轮廓；第二次点击有效目标 MUST 发送恰好一个现有整堆移动请求并清除来源。客户端 MUST NOT 因两次点击而本地扣除、增加或移动物品；服务端继续是 inventory、furnace 和 chest 状态的唯一权威。

#### Scenario: 普通背包保持 36 格与十条配方命中

- **GIVEN** 普通背包与固定合成区域已经打开
- **WHEN** 穷举每个栏位和配方按钮的边界内外样本
- **THEN** `InventorySlotAt` MUST 仍只返回 `0..35` 的 36 个栏位
- **AND** `RecipeButtonAt` MUST 仍只返回现有十条 UI 配方，所有边界结果 MUST 与换肤前一致

#### Scenario: 熔炉保持 39 格两次点击整堆移动

- **GIVEN** 玩家背包和熔炉三个格都来自最后确认镜像
- **WHEN** 玩家先点击统一来源栏位再点击不同的统一目标栏位
- **THEN** `FurnaceSlotAt` MUST 仍只返回 `0..38` 的 39 个栏位
- **AND** 客户端 MUST 只在第二次点击后发送一个引用现有容器、来源和目标的整堆移动请求
- **AND** 请求确认前 inventory 与 furnace 镜像 MUST 保持逐格不变

#### Scenario: 箱子保持 63 格两次点击整堆移动

- **GIVEN** 玩家背包和箱子 27 格都来自最后确认镜像
- **WHEN** 玩家先点击统一来源栏位再点击不同的统一目标栏位
- **THEN** `ChestSlotAt` MUST 仍只返回 `0..62` 的 63 个栏位
- **AND** 客户端 MUST 只在第二次点击后发送一个引用现有容器、来源和目标的整堆移动请求
- **AND** 请求确认前 inventory 与 chest 镜像 MUST 保持逐格不变

### Requirement: 容器像素换肤保持固定 HUD 资源契约

系统 SHALL 复用既有 hotbar atlas、layout、48-byte instance 编码和 HUD GPU pass。三类 overlay 各自只比当前组成增加一个标题 quad 且零 glyph；最大打开态 MUST 为 266 quad，不超过 scenario v19 固定的 267 quad。固定 glyph 上限 MUST 仍为 700，glyph offset MUST 仍为 13312 bytes，总容量 MUST 仍为 46912 bytes，所有固定区间 MUST 继续按 256 bytes 对齐。稳定态 MUST 只更新既有固定上传资源的实际实例前缀，不得创建每帧动态 GPU 资源。

#### Scenario: 最大打开态仍装入 scenario v19 固定缓冲

- **GIVEN** 打开态同时取合法 overlay、物品数量、来源轮廓和生存状态的最坏互斥组合
- **WHEN** HUD 准备 quad 与 glyph 实例
- **THEN** quad 数量 MUST 为 266 且不超过固定上限 267
- **AND** glyph 上限、glyph offset、总容量、instance 大小和对齐 MUST 分别保持 700、13312 bytes、46912 bytes、48 bytes 和 256 bytes
- **AND** 标题 MUST 只占一个 quad 且不占 glyph

#### Scenario: 零尺寸与窄窗口不引入资源或边界例外

- **GIVEN** framebuffer 为零尺寸或现有支持的窄正尺寸
- **WHEN** 任一容器 overlay 准备布局
- **THEN** 零尺寸 MUST 不发出实例，正尺寸的全部实例 MUST 有限且严格位于 framebuffer 内
- **AND** 系统 MUST NOT 为容器换肤分配动态 GPU 资源或放宽固定容量
