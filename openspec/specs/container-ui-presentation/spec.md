# container-ui-presentation Specification

## Purpose

为背包/合成、箱子与熔炉提供统一的原创像素界面，同时保持既有权威交互、命中几何和固定 HUD 资源契约。
## Requirements
### Requirement: 三类容器使用统一的原创像素表面

系统 SHALL 在既有 HUD pass 内为背包/合成、熔炉和箱子呈现统一的原创像素浮动面板：每类容器打开时以一个屏幕居中的暖色半透明面板呈现（含 1 design px 深暖棕描边与投影），标题位于面板顶部，玩家背包 3×9 与快捷栏行收进同一面板下段。凹槽、三个标题、熔炉火焰与进度箭头 MUST 来自既有 HUD atlas 的六个固定程序化 cell，不得来自二进制 UI 资产或 Mojang 像素；每个打开的 overlay MUST 恰好追加一个标题 quad，标题 MUST NOT 进入固定 768 glyph 流。面板几何 MUST 由单一面板原点推导，绘制与全部命中测试共用同一组面板矩形；面板 MUST 布局在底部状态栈上方的剩余空间内，与两条状态行、快捷栏和采掘/进食反馈互不相交。熔炉面板 MUST 使用原版图式：输入栏位于上层、燃料栏位于下层、火焰图示居中于两栏之间，箭头图示自左向右指向右侧输出栏。

#### Scenario: 背包与合成使用像素框和程序化标题

- **GIVEN** 玩家打开普通背包且没有打开熔炉或箱子
- **WHEN** HUD 绘制个人合成浮动面板、底部快捷栏与状态栈
- **THEN** 面板、36 个栏位凹槽和来源轮廓 MUST 使用同一套原创像素风格
- **AND** 个人合成面板 MUST 在左上呈现 2×2 权威网格、箭头图示与产物格，既有十条固定配方入口 MUST 以右侧配方栏形式保留在同一面板内且命中下标不变
- **AND** 背包/合成 overlay MUST 追加恰好一个 atlas 标题 quad 且不增加 glyph
- **AND** 面板 MUST 与底部状态栈、快捷栏和进度反馈互不相交

#### Scenario: 熔炉使用像素流程图示

- **GIVEN** 玩家打开一个包含已确认输入、燃料、输出、燃烧进度和熔炼进度的熔炉
- **WHEN** HUD 绘制熔炉浮动面板
- **THEN** 三个栏位 MUST 使用与背包相同的原创凹槽，输入栏在上、燃料栏在下、火焰图示居中其间，箭头图示指向右侧输出栏
- **AND** overlay MUST 追加恰好一个程序化标题 quad
- **AND** 燃烧和熔炼进度 MUST 分别以原创火焰与箭头像素图示表达，空、部分和完成状态 MUST 可区分
- **AND** 图示 MUST 复用既有进度 bar quad 的位置与数量，燃烧填充以火焰 cell 自下向上裁剪，熔炼填充以箭头 cell 自左向右裁剪
- **AND** 填充实例尺寸与 UV 端点 MUST 按同一进度比例收缩，不得压缩完整图标，也不得增加新的进度 quad 或 glyph

#### Scenario: 箱子使用同一像素语言

- **GIVEN** 玩家打开一个包含 27 个权威容器格的箱子
- **WHEN** HUD 绘制箱子浮动面板与玩家背包段
- **THEN** 箱子面板、27 个箱子栏位、36 个玩家栏位和来源轮廓 MUST 使用同一套原创像素风格
- **AND** 箱子 overlay MUST 追加恰好一个程序化标题 quad 且不增加 glyph

### Requirement: 容器换肤不改变统一栏位和权威交互

系统 SHALL 保持普通背包 `0..35` 的 36 个统一栏位、熔炉 `0..38` 的 39 个统一栏位和箱子 `0..62` 的 63 个统一栏位。浮动面板重排后，每个栏位与十条固定配方按钮的左上闭、右下开命中 MUST 与其绘制矩形同源一致；标题、面板描边、凹槽、来源轮廓、熔炉图示、配方栏与 tooltip MUST NOT 遮挡或扩张任何命中区域；底部状态栈避让与既有命中区域互不相交的约束 MUST 保持不变。

第一次点击有效栏位 MUST 只记录当前来源并显示来源轮廓；第二次点击有效目标 MUST 发送恰好一个现有整堆移动请求并清除来源。客户端 MUST NOT 因两次点击而本地扣除、增加或移动物品；服务端继续是 inventory、furnace 和 chest 状态的唯一权威。

#### Scenario: 普通背包保持 36 格与十条配方命中

- **GIVEN** 普通背包浮动面板与固定合成区域已经打开
- **WHEN** 穷举每个栏位和配方按钮的边界内外样本
- **THEN** `InventorySlotAt` MUST 仍只返回 `0..35` 的 36 个栏位
- **AND** `RecipeButtonAt` MUST 仍只返回现有十条 UI 配方
- **AND** 每个命中的栏位下标 MUST 与绘制该格的凹槽矩形一一对应

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

系统 SHALL 复用既有 hotbar atlas、layout、48-byte instance 编码和 HUD GPU pass。三类 overlay 各自只比当前组成增加一个标题 quad 且零 glyph；不含 combat marker 的最大打开态（含浮动面板描边、准星与悬停 tooltip 背景）MUST 为 264 quad，显示 marker 时 MUST 只增加 4 个 quad 至 268，二者均不得超过 scenario v20 固定的 320 quad。固定 glyph 上限 MUST 为 768，glyph offset MUST 为 15616 bytes，总容量 MUST 为 52480 bytes，所有固定区间 MUST 继续按 256 bytes 对齐。稳定态 MUST 只更新既有固定上传资源的实际实例前缀，不得创建每帧动态 GPU 资源。

#### Scenario: 最大打开态仍装入 scenario v20 固定缓冲

- **GIVEN** 打开态同时取合法 overlay、物品数量、来源轮廓、生存状态、准星与悬停 tooltip 的最坏互斥组合，combat marker 不可见
- **WHEN** HUD 准备 quad 与 glyph 实例
- **THEN** quad 数量 MUST 为 264 且不超过固定上限 320
- **AND** glyph 上限、glyph offset、总容量、instance 大小和对齐 MUST 分别为 768、15616 bytes、52480 bytes、48 bytes 和 256 bytes
- **AND** 标题 MUST 只占一个 quad 且不占 glyph

#### Scenario: 最大打开态加入 marker 仍不扩容
- **GIVEN** 上述合法最大打开态收到新鲜 combat 确认
- **WHEN** HUD 同时准备 4-quad marker
- **THEN** quad 数量 MUST 恰好为 268，固定上限 MUST 仍为 320，所有 buffer offset 与总容量 MUST 不变化

#### Scenario: 零尺寸与窄窗口不引入资源或边界例外

- **GIVEN** framebuffer 为零尺寸或现有支持的窄正尺寸，且 marker 可能可见
- **WHEN** 任一容器浮动面板与可选 marker 准备布局
- **THEN** 零尺寸 MUST 不发出实例，正尺寸的打开态高度约束 MUST 包含浮动面板高度，全部实例 MUST 有限且严格位于 framebuffer 内
- **AND** 极窄或极矮正尺寸 MUST 继续由统一 HUD 缩放比缩放绘制与命中，面板、标题、tooltip 和所有命中矩形 MUST 保持不相交
- **AND** 系统 MUST NOT 为容器换肤分配动态 GPU 资源或放宽固定容量

### Requirement: 悬停栏位以 tooltip 呈现物品中文名

容器浮动面板打开时，系统 SHALL 在指针悬停于非空栏位（含产物格与配方产物）时呈现物品中文名 tooltip：tooltip MUST 位于指针右下侧（越出 framebuffer 时翻转到指针左上），由深色投影与表面双层 quad 加阴影加前景双层字形组成；悬停空栏位、位于面板外或容器未打开时 MUST 不产生 tooltip 实例。tooltip 名称来源 MUST 与物品名弹条同源（既有注册表显示名及其方块回退）。

#### Scenario: 悬停有物格显示名称

- **GIVEN** 容器浮动面板打开且指针悬停于含已知物品的栏位
- **WHEN** HUD 布局 tooltip
- **THEN** tooltip MUST 显示该物品中文名，整体 MUST 位于指针右下侧或翻转后的左上侧，且完全位于 framebuffer 内

#### Scenario: 空格与面板外无实例

- **GIVEN** 指针悬停空栏位、位于面板外或容器未打开
- **WHEN** HUD 布局 tooltip
- **THEN** MUST 不产生任何 tooltip quad 或字形实例

#### Scenario: 名称来源与弹条同源

- **GIVEN** 同一物品分别在悬停 tooltip 与物品名弹条中呈现
- **WHEN** 比较两者文本
- **THEN** 两者 MUST 使用同一显示名来源且文本一致
