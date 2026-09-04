# survival-hud-presentation Specification

## Purpose

为玩家提供紧凑、颜色无关且在窄窗口与容器界面中仍可读的生存 HUD，同时保持所有状态的既有权威来源和固定渲染资源契约。
## Requirements
### Requirement: 快捷栏固定为居中九格并明确标识选中格

系统 SHALL 在关闭容器界面时经 WebView HUD 组件显示固定九格、水平居中的快捷栏（呈现职责自 GPU HUD 迁移，语义逐项平移，见 `game-overlay-webview` capability）。快捷栏贴条 MUST 为透明悬浮排布（无深色长条底带）；每格 MUST 是独立粉彩方块——逐格不同的低饱和底色、深可可描边、大圆角、顶部浅高光加底部沉边加柔投影。选中格 MUST 具有暖橙赭石外扩外框加方块抬起阴影，使其在忽略颜色后仍可由外扩几何与阴影抬起与未选中格区分；呈现数据 MUST 仅来自已确认权威镜像。

#### Scenario: 九格快捷栏和选中格可由几何判定

- **GIVEN** 已确认的有效快捷栏镜像和任一合法选中下标
- **WHEN** HUD 组件在关闭容器界面时呈现快捷栏
- **THEN** 快捷栏 MUST 恰好包含九个等尺寸格并整体水平居中
- **AND** 只有选中格 MUST 具有暖橙外扩外框加抬起阴影，且该标识 MUST NOT 仅靠颜色区别于其他格

#### Scenario: 粉彩悬浮外观

- **GIVEN** 已确认的有效快捷栏镜像
- **WHEN** HUD 组件呈现快捷栏
- **THEN** 贴条 MUST 无深色底带背景与内缩缘阴影
- **AND** 每格 MUST 有圆角、深色描边、高光与沉边，且九格底色 MUST 逐格不同

### Requirement: 数量与耐久保持权威镜像语义

快捷栏物品数量和工具耐久 SHALL 继续只由已确认的权威物品镜像驱动（经 WebView 组件呈现）。数量 MUST 继续只对多于一件的堆叠显示最多两位的数字；耐久 MUST 继续只对耐久上限存在且 `0 < Durability < MaxDurability` 的工具显示背景与剩余比例填充。数量数字与耐久条 MUST 在亮底粉彩格上以深棕前景呈现并保持可读对比。

#### Scenario: 单件与满耐久不产生附加标记

- **GIVEN** 一个数量为一的物品堆叠和一个满耐久工具
- **WHEN** HUD 组件呈现快捷栏
- **THEN** 单件堆叠 MUST NOT 显示数量数字
- **AND** 满耐久工具 MUST NOT 显示耐久条

#### Scenario: 多件与磨损工具沿用确认值

- **GIVEN** 已确认镜像包含数量大于一的堆叠和一个部分磨损工具
- **WHEN** HUD 组件呈现快捷栏
- **THEN** 数量 MUST 以深棕前景显示，且最多显示两位
- **AND** 耐久填充比例 MUST 由该镜像中的当前耐久和耐久上限决定

### Requirement: 生命以十个空半满心呈现

系统 SHALL 只消费已确认的权威生命值（经 WebView 组件呈现），并 MUST 先把该值钳制到 `0..core.MaxHealth`，再让固定十个槽位各自直接解析为空心、半心或满心。每个槽位 MUST 恰好产生一个完整 cell，生命行 MUST NOT 绘制背景面板；未确认生命值 MUST 完全隐藏。

#### Scenario: 奇数生命显示半心

- **GIVEN** 已确认生命值为一个落在 `0..core.MaxHealth` 内的奇数
- **WHEN** HUD 组件呈现生命状态行
- **THEN** MUST 显示十个 resolved 心形槽位，其中对应数量的槽位使用满心 cell，并有恰好一个槽位使用半心 cell
- **AND** 未填充的其余槽位 MUST 使用空心 cell，每个槽位 MUST 只有一个实例，且 MUST NOT 出现任何背景面板

#### Scenario: 越界和未确认生命安全处理

- **GIVEN** 生命值未确认或高于 `core.MaxHealth`
- **WHEN** HUD 组件呈现生命状态行
- **THEN** 未确认值 MUST 不产生任何生命呈现
- **AND** 高于上限的已确认值 MUST 按 `core.MaxHealth` 呈现且不得超过十个满心

### Requirement: 氧气以耗损时可见的十段气泡呈现

系统 SHALL 只消费已确认的权威氧气值并将其钳制到 `0..core.MaxOxygenTicks`（经 WebView 组件呈现）。满氧与未确认氧气 MUST 完全隐藏；氧气耗损时 MUST 把十个槽位中的 `ceil(value * 10 / core.MaxOxygenTicks)` 个解析为满气泡，其余解析为空气泡，零值 MUST 产生十个空气泡。

#### Scenario: 满氧完全不占用呈现实例

- **GIVEN** 氧气未确认或已确认值等于 `core.MaxOxygenTicks`
- **WHEN** HUD 组件呈现氧气状态行
- **THEN** 氧气 MUST 完全不呈现（不留空位或占位框）

#### Scenario: 耗损氧气按向上取整分段

- **GIVEN** 已确认氧气值小于 `core.MaxOxygenTicks`
- **WHEN** HUD 组件呈现氧气状态行
- **THEN** 满气泡槽位数 MUST 等于 `ceil(value * 10 / core.MaxOxygenTicks)`，其余 MUST 为空气泡，零值 MUST 呈现十个空气泡

### Requirement: 饥饿以十个常驻空槽和权威填充呈现

系统 SHALL 只消费已确认的权威饥饿值（经 WebView 组件呈现），并 MUST 先把该值钳制到 `0..core.MaxHunger`。未确认饥饿 MUST 完全隐藏；已确认饥饿无论是否满值都 MUST 显示十个空鸡腿槽，并在其上追加 `ceil(Hunger / 2)` 个填充鸡腿；奇数值的最后一个填充 MUST 只覆盖对应鸡腿的右半边；饥饿行 MUST NOT 绘制背景面板。

#### Scenario: 已确认饥饿始终显示十格刻度

- **GIVEN** 已确认饥饿值为 `0`、任一中间值、`core.MaxHunger` 或高于 `core.MaxHunger`
- **WHEN** HUD 组件呈现饥饿状态行
- **THEN** MUST 恰好显示十个空鸡腿槽，填充数等于 `ceil(Hunger / 2)`，且无背景面板

#### Scenario: 奇数饥饿使用右半格且未确认值隐藏

- **GIVEN** 一份奇数的已确认饥饿值和一份未确认饥饿值
- **WHEN** HUD 组件分别呈现两份状态
- **THEN** 已确认值的最后一个填充 MUST 只显示对应鸡腿的右半边
- **AND** 未确认值 MUST 不产生任何鸡腿呈现

### Requirement: 主状态行锚定快捷栏边缘且氧气向外堆叠

生命与饥饿 SHALL 组成稳定的主状态行（经 WebView 组件呈现）：生命起点 MUST 对齐快捷栏左边缘，饥饿终点 MUST 对齐快捷栏右边缘，两者之间的布局空间 MUST 由快捷栏实际宽度自然得出。耗损氧气 SHALL 作为沿饥饿右边缘对齐的次状态行：关闭容器时 MUST 位于饥饿上方，打开任一容器时 MUST 位于饥饿下方，行距 MUST 等效保持一个状态行高；满氧或未确认氧气不呈现，但次状态行的构图空间 MUST 保留，周边元素 MUST NOT 随氧气显隐跳动。

#### Scenario: 生命和饥饿精确对齐快捷栏两端

- **GIVEN** 已确认生命与饥饿状态
- **WHEN** HUD 组件呈现关闭或打开容器的主状态行
- **THEN** 生命起点 MUST 与快捷栏左边缘对齐，饥饿终点 MUST 与快捷栏右边缘对齐且从右向左排列

#### Scenario: 耗损氧气沿饥饿右边缘向外堆叠

- **GIVEN** 已确认的耗损氧气、生命与饥饿状态
- **WHEN** 分别呈现关闭态与打开态 HUD
- **THEN** 氧气终点 MUST 与饥饿终点对齐，关闭态位于饥饿上方、打开态位于饥饿下方各一个状态行

#### Scenario: 满氧隐藏不改变主行或周边锚点

- **GIVEN** 两帧具有相同的生命与饥饿值，其中一帧氧气耗损、另一帧氧气为满值
- **WHEN** HUD 组件呈现两帧状态栈与聊天
- **THEN** 满氧帧 MUST 不呈现气泡
- **AND** 两帧的主状态行与快捷栏位置 MUST 逐项一致，构图空间保留

### Requirement: 生存 HUD 在窄 framebuffer 内整体等比缩小

系统 SHALL 把快捷栏、状态行、氧气、进食反馈、准星与物品名弹条作为同一布局整体响应窗口尺寸（经 WebView 组件布局）：空间不足时所有元素 MUST 以单一比例协调缩小，构图关系 MUST 在缩放中保持；常显 HUD MUST 完全位于视口内且 MUST NOT 遮挡容器界面内容；零尺寸或非法视口 MUST 安全降级为不呈现。状态行的净空间距（状态行底与快捷栏贴条外沿之间）MUST 经令牌供给并在缩放中保持比例。

#### Scenario: 窄窗口保持同一比例并不越界

- **GIVEN** 一个不足以按基准尺寸容纳完整常显 HUD 的窗口
- **WHEN** HUD 组件布局全部常显元素
- **THEN** 全部元素 MUST 以同一比例缩小，构图关系保持，每个元素 MUST 完全位于视口内

#### Scenario: 零尺寸不生成实例

- **GIVEN** 视口宽或高为零
- **WHEN** HUD 组件布局
- **THEN** MUST 安全降级为不呈现且不产生布局异常

### Requirement: 两行状态栈按容器状态向快捷栏外侧避让

关闭背包、合成、箱子或熔炉界面时，生命/饥饿主状态行 SHALL 位于快捷栏上方（保留状态行-快捷栏净空间距），耗损氧气沿饥饿右缘堆叠在主状态行上方，进食反馈位于完整状态栈上方。任一容器界面打开时，主状态行 MUST 位于快捷栏下方并保持可见，氧气沿饥饿右缘继续向下堆叠；两条状态行 MUST 完全位于视口内，MUST NOT 遮挡或改变任何既有可交互物品格或配方区域的呈现与交互。

#### Scenario: 关闭容器时状态与采掘按行堆叠

> 场景名沿用历史名（openspec 1.7 的 MODIFIED 漂移守卫不支持 Scenario 改名）；
> 采掘的屏幕进度条已移除，本场景断言的堆叠行现在只承载进食反馈。

- **GIVEN** 容器界面关闭且状态栈与进食反馈均可见
- **WHEN** HUD 组件布局
- **THEN** 主状态行位于快捷栏上方，氧气沿饥饿右缘在其上方堆叠，进食反馈位于状态栈上方

#### Scenario: 打开容器后两行状态栈保持可见且不拦截格子

- **GIVEN** 已打开任一容器界面，并已确认低生命、耗损氧气与饥饿
- **WHEN** HUD 组件布局
- **THEN** 主状态行位于快捷栏下方、氧气继续向下堆叠，且全部位于视口内
- **AND** 状态行 MUST NOT 遮挡任何可交互物品格或配方区域

### Requirement: 生存 HUD 保持固定资源和实例兼容性

常显 HUD 层的 GPU 呈现（快捷栏贴条与选中框、状态行图标、氧气气泡、采掘/进食轨道、物品名弹条、准星、聊天呈现与权威命中 marker 绘制）SHALL 自本 change 起退役，容器浮动面板（背包/合成、箱子、熔炉的 GPU 呈现）与容器悬停 tooltip SHALL 保留既有 HUD pass。退役后的保留面最坏组合 SHALL 钉为：关闭容器界面 0 quad/0 glyph（容器面板、tooltip 与容器内容只在打开态布局）；打开任一容器界面 218 quad/268 glyph（悬停 tooltip 背景、来源轮廓等保留项全数计入，由箱子视图见证）。权威命中 marker 的呈现职责已迁 WebView 组件，GPU 保留面 MUST NOT 再产生 marker quad；若过渡实现仍保留 marker 绘制路径，两分支最坏 MUST 只分别增加 4 个 quad。保留面 MUST 继续使用 48-byte instance 编码、256-byte 区间对齐与稳定态零每帧动态资源；固定 quad 上限 320、glyph 上限 768、glyph offset 15616 bytes 与固定上传总容量 52480 bytes MUST 保持不变，各最坏组合 MUST NOT 超过固定上限。禁止为退役路径保留死资源或放宽任何真实 overflow 门禁。

#### Scenario: 关闭和打开界面的合法最坏组合均有界

- **GIVEN** 打开箱子界面的合法最坏组合（统一栏位与箱子 27 格全满两位数量、面板快捷栏行九格磨损耐久、选中格与来源格高亮、悬停 tooltip 背景）且常显层已退役
- **WHEN** HUD 准备 quad 与 glyph 实例
- **THEN** 打开态 quad 数 MUST 恰好为面板族 7 + 选中格与来源格高亮 2 + 统一栏位凹槽 36 + 双层物品 tile 72 + 耐久条 18 + 箱子内容 81 + tooltip 背景 2 = 218，且 MUST NOT 超过固定上限 320
- **AND** 打开态 glyph 预算 MUST 恰好为统一栏位两位数量 144 + 箱子两位数量 108 + tooltip 双层显示名 16 = 268（tooltip 按 8 rune 截断上限封顶；注册表实测最长名 5 rune 双层 10，对应实测见证 262），且 MUST NOT 超过固定上限 768
- **AND** 48-byte 编码、256-byte 对齐、glyph offset 15616 bytes 与固定上传总容量 52480 bytes MUST 保持不变

#### Scenario: marker 只增加四个 quad 且不扩容

- **GIVEN** 上述合法最坏组合收到新鲜 combat 确认
- **WHEN** HUD 同时准备呈现（marker 绘制已迁 WebView 组件）
- **THEN** GPU 保留面 quad 数 MUST NOT 因 marker 增加（迁移后组件呈现不经 GPU 保留面）；若过渡实现仍保留 marker 绘制路径，两分支最坏 MUST 只分别增加 4 个 quad 至 222
- **AND** 所有 buffer offset 与总容量 MUST 不变化

#### Scenario: 关闭态保留面不产生实例

- **GIVEN** 容器界面关闭且常显层已退役
- **WHEN** HUD 准备 quad 与 glyph 实例
- **THEN** 关闭态 MUST 恰好产生 0 quad 与 0 glyph，快捷栏贴条、选中框、状态行图标、氧气、采掘/进食轨道、物品名弹条、准星与聊天呈现 MUST NOT 在 GPU 保留面出现
- **AND** 固定上传区间 MUST 保持既有容量、offset 与对齐，MUST NOT 因关闭态零实例而收缩或重排

#### Scenario: 稳定态不创建每帧动态 GPU 资源

- **GIVEN** HUD atlas 与固定上传资源已预热完成
- **WHEN** 系统连续准备和呈现多帧容器 HUD
- **THEN** 每帧 MUST 只更新固定资源中的实际实例前缀
- **AND** MUST NOT 创建新的每帧动态 GPU 资源、HUD pass 或 shader

### Requirement: 权威命中 marker 使用四个 quad 并按成功呈现帧计时

权威命中 marker 的显示 SHALL 由 WebView HUD 组件承担（中心四向短标记，几何比例与迁移前一致并随 HUD 整体缩放）；marker 的武装、去重与计时状态机 SHALL 完整保留在 Go 侧：仅严格递增的合法 `CombatHit` 确认可武装，每条新鲜确认重置为 6 个成功呈现帧，只有 renderer 实际成功返回 true 后才可消耗一帧，零 framebuffer、prepare 失败或返回 false MUST NOT 消耗；断线、退回主菜单、建立新会话、权威 reset 和 capture 场景切换 MUST 清零计时。状态机的武装/到期变化 MUST 经 HUD 状态下行驱动组件显隐。

#### Scenario: marker 几何恰好为四个 quad

- **GIVEN** marker 已武装且游戏相位 HUD 可见
- **WHEN** HUD 组件呈现 marker
- **THEN** MUST 在准星中心四向各呈现一个白色不透明短标记，上下与左右尺寸比例、内缘距中心比例 MUST 与迁移前一致并随 HUD 缩放（呈现通道迁 WebView 后该 historical 名称指四向标记本体）

#### Scenario: 六次成功呈现后消失

- **GIVEN** 一条新鲜确认把 marker 计时重置为 6 个成功呈现帧
- **WHEN** renderer 连续六次成功返回 true
- **THEN** 前六个成功帧 MUST 可见，第六次之后 MUST 不再呈现

#### Scenario: 失败呈现不消耗帧

- **GIVEN** marker 仍有 6 帧
- **WHEN** 遇到零 framebuffer、prepare 失败或 renderer 返回 false
- **THEN** 剩余帧数 MUST 保持 6，下一次成功呈现 MUST 仍显示 marker

#### Scenario: 新确认重置窗口且生命周期 reset 清空

- **GIVEN** marker 只剩 1 帧
- **WHEN** 收到更大 `ServerTick` 的合法确认
- **THEN** 剩余帧数 MUST 重置为 6
- **WHEN** 随后断线、退回主菜单、建立新 session、收到权威 reset 或切换 capture 场景
- **THEN** combat 去重状态与 marker 帧数 MUST 一并清零

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

### Requirement: 进食进度以客户端预测呈现并与采掘反馈互斥

> 标题沿用历史名（openspec 1.7 的 MODIFIED 漂移守卫不支持 Requirement 改名）；
> 本 delta 起正文语义为「不再与采掘反馈互斥」——采掘的屏幕进度条已移除，
> 进度反馈由世界空间裂纹承载，以下述断言为准。

进食输入持续满足（手持食物、保持进食输入位、未打开容器或菜单，且权威确认的饥饿值未满）时，HUD 组件 MUST 在状态栈上方呈现进食进度条。进度值 MUST 为客户端预测：按连续满足的输入时长以权威 tick 周期累积，分母为与权威 `EatingTicks` 默认值同源的呈现层常量；MUST NOT 为此新增协议字段。进食输入位归零、选中栏位变化或栏位物品变化时，预测进度 MUST 立即清零。采掘不再呈现屏幕进度条（采掘进度反馈由世界空间裂纹承载），进食条因此不再与采掘互斥。

#### Scenario: 持续进食呈现递增进度条

- **GIVEN** 玩家手持食物并保持进食输入
- **WHEN** 输入持续权威 tick 周期推进
- **THEN** 进度条填充 MUST 从零单调递增，达到分母后钳制为满

#### Scenario: 中断输入立即清零

- **GIVEN** 进度条已呈现部分填充
- **WHEN** 玩家松开使用键、打开容器或进入菜单使进食输入位归零
- **THEN** 进度条 MUST 立即消失，再次满足输入时从零重新累积

#### Scenario: 切换栏位或物品清零

- **GIVEN** 玩家手持食物进食中
- **WHEN** 选中栏位变化或该格物品变化
- **THEN** 预测进度 MUST 清零并从新选择重新累积

#### Scenario: 饥饿已满不呈现进度条

- **GIVEN** 玩家手持食物、保持进食输入，且权威确认的饥饿值已满
- **WHEN** 输入持续任意时长
- **THEN** 进食进度条 MUST NOT 出现，与权威侧「饥饿已满不推进」对齐

#### Scenario: 与采掘反馈互斥

> 场景名沿用历史名（openspec 1.7 的 MODIFIED 漂移守卫不支持 Scenario 改名）；
> 语义随本 delta 反转——采掘不再有屏幕进度条，并发时屏幕上只有进食条，
> 采掘的进度反馈只出现在被采掘方块表面（世界空间裂纹）。

- **GIVEN** 采掘进度处于激活状态（方块表面呈现裂纹）
- **WHEN** 进食输入同时保持
- **THEN** 屏幕上 MUST 只呈现进食进度条，采掘的进度反馈 MUST 只出现在被采掘方块的表面

#### Scenario: 最坏容量不增加

- **GIVEN** 进食进度条激活
- **WHEN** 计算本帧 HUD 布局 quad 数
- **THEN** 其呈现迁 WebView 后 MUST NOT 占用 GPU 固定预算

### Requirement: 准星以屏幕中心十字呈现

系统 SHALL 在游戏相位常显阶段于 viewport 中心经 WebView HUD 组件呈现原创十字准星：横向与纵向两条等宽臂组成十字，MUST 以深色投影与亮色前景双层矩形呈现，使其在任意世界背景上可辨认；准星 MUST 随 HUD 整体缩放且中心 MUST 与 viewport 几何中心重合；主菜单、设置页与暂停覆盖层等菜单相位 MUST 不呈现准星。准星呈现 MUST NOT 进入 glyph 类呈现通道，MUST NOT 使用新 shader、GPU pass 或固定容量之外的资源。

#### Scenario: 游戏相位常显且居中

- **GIVEN** 已登录且 HUD 可见的游戏相位
- **WHEN** HUD 组件布局
- **THEN** viewport 中心 MUST 出现由横竖两臂组成的十字准星，其中心 MUST 与 viewport 几何中心重合，且由深色投影与亮色前景两层构成

#### Scenario: 菜单相位不绘制

- **GIVEN** 主菜单、设置页或暂停覆盖层可见
- **WHEN** 系统布局该帧
- **THEN** MUST NOT 呈现准星

#### Scenario: 容器面板覆盖准星

- **GIVEN** 任一容器浮动面板打开且准星位于面板矩形内
- **WHEN** 系统呈现该帧
- **THEN** 面板 MUST 遮挡二者重叠区域（WebView 层以抑制呈现实现等效遮挡），准星呈现与面板呈现及命中互不干扰

### Requirement: 选中栏位变化以物品名弹条呈现

系统 SHALL 在已确认权威镜像的选中栏位下标发生变化时，经 WebView HUD 组件于状态栈上方呈现所选物品的中文名弹条：文本 MUST 来自既有注册表显示名（物品级缺名时回退其方块显示名，均缺省则不显示），以阴影加前景双层样式居中于快捷栏宽度；弹条自确认变化所属权威 tick 起持续恰好 40 tick 后消失；容器界面打开或菜单相位 MUST 抑制弹条。弹条 MUST NOT 使用任何新协议字段，MUST 与进食进度条同帧共存时互不遮挡。

#### Scenario: 确认切换显示物品名

- **GIVEN** 已确认镜像中选中栏位变化为含已知中文名物品的格
- **WHEN** 系统布局弹条
- **THEN** 弹条 MUST 居中显示该物品中文名且文字带阴影层

#### Scenario: 40 tick 后消失

- **GIVEN** 弹条自确认变化所属 tick 起持续呈现
- **WHEN** 权威 tick 前进超过 40 tick
- **THEN** 弹条 MUST 不再呈现

#### Scenario: 未确认变化不触发

- **GIVEN** 客户端发出的选中请求尚未获得服务端确认
- **WHEN** 系统布局弹条
- **THEN** 弹条 MUST 保持上一确认状态，MUST NOT 因未确认变化显示或刷新

#### Scenario: 容器与菜单抑制

- **GIVEN** 任一容器浮动面板或菜单相位可见
- **WHEN** 选中栏位的确认值发生变化
- **THEN** 弹条 MUST 不产生任何呈现实例

