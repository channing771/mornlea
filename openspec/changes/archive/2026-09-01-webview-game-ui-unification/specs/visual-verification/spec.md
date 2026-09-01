## MODIFIED Requirements

### Requirement: 视觉基线覆盖统一方块与 HUD 风格

系统 SHALL 通过既有无窗口固定场景记录并比对当前产品默认方块材质与世界呈现。地形场景 MUST 覆盖内嵌默认 layer 与没有内嵌映射时的程序化回退。常显 HUD（快捷栏贴条与选中框、状态行图标、氧气气泡、采掘/进食轨道、物品名弹条、准星、聊天呈现与权威命中 marker）的 GPU 呈现已退役，无头抓帧路径 MUST NOT 产生这部分像素；它们的呈现验收 SHALL 由 `game-overlay-webview` capability 的前端组件断言与 `frontend/visual` 部件基线承接（本机 Chrome 截图、既有双阈值），MUST NOT 再由 capture golden 承接。GPU 保留面（容器浮动面板、容器悬停 tooltip 与 HUD atlas）MUST 由 `inventory-crafting`、`workbench-crafting`、`chest-container` 与 `furnace-container` 四景继续做像素验收。世界类场景 golden 中常显 HUD 条带与准星的消失属合法波及，随本 change 经既有显式更新路径重新生成并逐图复核。更新基线时 MUST 继续执行既有显式更新、无窗口完整渲染链路和双阈值规则；不得创建或聚焦前台游戏窗口，不得导入、临摹或复制 Mojang 像素。

`materials-showcase` MUST 保持既有固定正午、固定相机和确定性夹具，并经与交互客户端相同的完整呈现链路收敛后无窗口抓取，不得创建或聚焦前台游戏窗口。夹具 MUST 同时覆盖 14 种新材料、八格连续草地、相邻玻璃、相邻树叶、原木顶面年轮与侧面树皮，以及干耕地与湿耕地各至少一个可见列（含下沉顶面的完整几何）。既有双阈值 MUST 保持不变。

抓帧场景清单 MUST 按以下完整顺序运行（22 景）：`terrain-noon`、`avatar-nametag`、`inventory-crafting`、`workbench-crafting`、`chest-container`、`furnace-container`、`debug-panel`、`skylight-tunnel`、`block-light-room`、`torch-night`、`bed-night`、`materials-showcase`、`target-block-feedback`、`oak-grove`、`ai-companion`、`sword-combat`、`hostile-mob`、`water-surface-slope`、`main-menu`、`settings-menu`、`far-horizon`、`water-underwater`。`hud-hotbar-health`、`hud-survival-feedback` 与 `hud-item-name-popup` 三景随常显层 GPU 呈现退役从清单移除，清单 MUST NOT 再包含任何只承载常显 HUD 像素的场景。清单 MUST 保留 `target-block-feedback`、`oak-grove` 与 `ai-companion` 的既有名称及相对顺序，`ai-companion` MUST 继续紧随 `oak-grove`，并 MUST 保持 `sword-combat`、`hostile-mob`、`water-surface-slope` 的相邻顺序，`settings-menu` MUST 紧随 `main-menu`，`far-horizon` MUST 为倒数第二，`water-underwater` MUST 为唯一末场景。所有场景 MUST 使用与交互客户端相同的完整呈现链路收敛后无窗口抓取，且不得创建或聚焦前台游戏窗口。

#### Scenario: 地形与 HUD 风格变化产生可审查基线

- **GIVEN** 既有固定场景与渲染链路可用
- **WHEN** 显式更新本变更影响的视觉基线
- **THEN** `terrain-noon` MUST 包含当前内嵌默认材质及没有内嵌映射 layer 的程序化回退
- **AND** `terrain-noon` 的画面 MUST NOT 出现快捷栏、状态行、氧气、采掘/进食轨道、弹条、准星、聊天或命中 marker 像素
- **AND** 该图 MUST 由无窗口完整渲染链路产出并继续使用既有双阈值

#### Scenario: 常显 HUD 像素退出无头抓帧

- **GIVEN** 常显 HUD 的 GPU 呈现已退役且容器界面关闭
- **WHEN** 抓取任一非菜单相位的固定场景
- **THEN** 画面 MUST NOT 出现任何常显 HUD 像素，与 `survival-hud-presentation`「容器保留面 GPU 资源契约重钉」的关闭态 0 quad/0 glyph 一致
- **AND** 快捷栏、状态行、氧气、采掘/进食轨道、弹条、准星、聊天与 marker 的呈现验收 MUST 由 `game-overlay-webview` 的前端组件断言与 `frontend/visual` 部件基线承接

#### Scenario: 完整场景顺序收缩为 22 项

- **GIVEN** 完整无窗口 capture 场景清单
- **WHEN** 检查全部场景名称与顺序
- **THEN** 清单 MUST 恰好包含本 requirement 列出的 22 项，且顺序与之逐项一致
- **AND** 清单 MUST NOT 包含 `hud-hotbar-health`、`hud-survival-feedback` 或 `hud-item-name-popup`
- **AND** `far-horizon` MUST 是倒数第二个场景，`water-underwater` MUST 是唯一末场景

#### Scenario: 打开背包场景验证容器 GPU 保留面

- **GIVEN** `inventory-crafting` 装入固定背包、个人 2×2 网格中一条已匹配的真实原料形状、非空产物格和一个已选来源格
- **WHEN** 场景经完整链路收敛并无窗口抓取
- **THEN** 画面 MUST 同时呈现原创像素框、36 个凹槽、2×2 网格、产物格与背包/合成标题，全部属于容器面板保留面
- **AND** 画面 MUST NOT 出现生命/饥饿状态行、氧气气泡或快捷栏贴条，容器面板与 tooltip 保留面 MUST NOT 因常显层退役而缺失
- **AND** 打开态保留面最坏组合 MUST 继续由 `survival-hud-presentation` 的 218 quad/268 glyph 契约钉住

#### Scenario: 远端玩家场景只继承地形背景变化

- **GIVEN** 远端玩家与名牌的渲染逻辑没有变化，但场景共享的当前产品默认地形背景发生变化
- **WHEN** 更新本变更影响的视觉基线
- **THEN** `avatar-nametag` MUST 继承当前地形背景
- **AND** 远端玩家轮廓、颜色与名牌文字 MUST 保持既有可观察语义（名牌属世界呈现，不随常显层退役消失）

#### Scenario: 材料展示保持既有验收夹具

- **GIVEN** `materials-showcase` 的固定夹具已装入客户端镜像
- **WHEN** `materials-showcase` 完成网格和上传收敛并抓帧
- **THEN** 图像 MUST 同时显示 14 种新材料各一个近景样本及多方块表面、跨至少一个 AO 或天空光拆分边界的八格连续草地、两个相邻玻璃方块、两个相邻树叶方块，以及原木顶面年轮与侧面树皮、干耕地与湿耕地各一个可见列（两列顶面呈现在下沉高度而非整格顶面）
- **AND** 玻璃后方方块 MUST 可见，树叶孔洞和光照 MUST 可辨认，相同 cutout 方块的内部面 MUST 不可见

#### Scenario: 材料展示只走无窗口完整链路

- **GIVEN** `materials-showcase` 使用固定正午与固定相机
- **WHEN** 生成或比对 `materials-showcase`
- **THEN** 抓帧 MUST 使用与交互客户端相同的完整呈现链路
- **AND** MUST NOT 创建或聚焦前台游戏窗口，且 MUST 继续使用现有双阈值

#### Scenario: 全部正式基线需重新生成并完整复核

- **GIVEN** 常显 HUD 的 GPU 呈现退役与 WebView HUD 组件承接已经落地
- **WHEN** 显式更新视觉基线
- **THEN** 系统 MUST 按既有完整顺序重新生成全部 22 张正式 golden
- **AND** 调用方 MUST 逐张人工复核全部 22 张图像后才能接受更新，且既有双阈值 MUST 保持不变

#### Scenario: 伙伴场景与当前末尾顺序并存

- **GIVEN** 完整无窗口场景清单
- **WHEN** 检查 `target-block-feedback` 之后的场景名称与顺序
- **THEN** `oak-grove` 与 `ai-companion` MUST 保持既有名称，且 `ai-companion` MUST 紧随 `oak-grove`
- **AND** `water-surface-slope` MUST 位于 `ai-companion` 之后，`main-menu` 与 `settings-menu` MUST 依次相邻并位于 `far-horizon` 之前，`far-horizon` MUST 是倒数第二个场景，`water-underwater` MUST 是唯一末场景

#### Scenario: 橡树林通过正常渲染链路抓取

- **GIVEN** `oak-grove` 的固定世界种子、生成区块、正午时间与相机已经装入客户端镜像
- **WHEN** 场景完成预热、网格收敛和上传并抓帧
- **THEN** 图像 MUST 由与交互客户端相同的完整呈现链路产出，且 MUST 显示固定橡树地貌
- **AND** 抓帧 MUST NOT 创建或聚焦前台游戏窗口，且 MUST 继续使用既有双阈值

#### Scenario: AI 伙伴通过统一呈现链路抓取

- **GIVEN** `ai-companion` 已重置前一场景的 remote、companion、chat、inventory、panel、container、mining、damage 和 item-drop 状态，并装入固定伙伴和聊天夹具
- **WHEN** 场景完成预热和上传并抓帧
- **THEN** 图像 MUST 由统一的人形与名牌呈现链路产出，且 MUST 同时显示伙伴人形与中文名牌“阿木”
- **AND** accepted 事件与 `@阿木 挖石头` 输入属聊天呈现，已迁 WebView HUD 组件，画面 MUST NOT 出现聊天行或聊天输入框像素；其验收由前端组件断言承接
- **AND** 抓帧 MUST NOT 创建或聚焦前台游戏窗口，且 MUST 继续使用既有双阈值

#### Scenario: 目标反馈通过正常渲染链路验证遮挡

- **GIVEN** `target-block-feedback` 的固定夹具命中一个已注册材料方块
- **WHEN** 场景完成预热、网格收敛和上传并抓帧
- **THEN** 图像 MUST 同时显示该方块的细轮廓、中文名称和被地形正确遮挡的边
- **AND** 场景 MUST 使用与交互客户端相同的完整呈现链路并保持正确遮挡，不得创建或聚焦前台游戏窗口

#### Scenario: 打开背包的基线不受目标提示影响

- **GIVEN** `inventory-crafting` 场景打开背包
- **WHEN** 显式更新所有视觉基线
- **THEN** `inventory-crafting` MUST 不显示目标轮廓或名称
- **AND** 背包与合成区域的容器保留面语义 MUST 保持不变
- **AND** 只有经逐图复核确认由常显层退役、当前产品默认材质或共享地形背景变化引起时，它的 golden MAY 更新

#### Scenario: 基线更新不改变阈值或场景尾序

- **GIVEN** 调用方在常显层退役后的基线上更新全部正式 golden
- **WHEN** 检查生成结果和比较配置
- **THEN** `water-surface-slope`、`main-menu`、`settings-menu`、倒数第二的 `far-horizon` 与唯一末场景 `water-underwater` 的尾序 MUST 保持不变
- **AND** 既有双阈值 MUST 保持不变，任何差异 MUST 经逐图人工复核而不得通过放宽阈值接受

#### Scenario: 完整场景顺序加入生存反馈

- **GIVEN** 常显 HUD 像素已迁 WebView 组件（本 change 退役 `hud-survival-feedback` 景）
- **WHEN** 检查 capture 场景清单
- **THEN** 生存反馈的呈现验收 MUST 由 `game-overlay-webview` 的前端组件断言与 `frontend/visual` 部件基线承接
- **AND** 场景顺序约束由「完整场景顺序收缩为 22 项」承载

#### Scenario: 生存反馈场景固定且不污染后续场景

- **GIVEN** 退役场景的状态恢复纪律（临时 predictor、生命、氧气、饥饿和采掘状态在场景结束一并恢复）
- **WHEN** 保留场景依次运行
- **THEN** 该纪律 MUST 由保留场景与呈现状态机继续遵守，后续场景 MUST NOT 继承任何夹具值

#### Scenario: 打开背包场景复用同一向外状态栈

- **GIVEN** `inventory-crafting` 保留景验证容器 GPU 保留面
- **WHEN** 场景呈现打开的背包与状态栈构图
- **THEN** 状态栈（生命/饥饿/氧气）呈现已迁 WebView，GPU 画面 MUST 只包含容器保留面
- **AND** 保留面与 WebView 状态栈互不相交的构图由前端组件断言承接

#### Scenario: 合并后的全部正式基线需重新生成并完整复核

- **GIVEN** 常显层退役与容器保留面钉值已落地
- **WHEN** 显式更新视觉基线
- **THEN** 系统 MUST 按既有完整顺序重新生成全部 22 张正式 golden
- **AND** 调用方 MUST 逐张人工复核全部 22 张图像后才能接受更新，且既有双阈值 MUST 保持不变

#### Scenario: 合并基线更新不改变阈值或场景尾序

- **GIVEN** 常显层退役后的基线重生成
- **WHEN** 检查比较配置与场景尾序
- **THEN** 既有双阈值 MUST 保持不变，任何差异 MUST 经逐图人工复核而不得通过放宽阈值接受
- **AND** `far-horizon` MUST 保持倒数第二，`water-underwater` MUST 保持唯一末场景

### Requirement: 视觉基线覆盖三类容器像素界面

系统 SHALL 具有恰好 22 个正式无窗口场景，`workbench-crafting` MUST 紧随 `inventory-crafting`，`chest-container` 与 `furnace-container` MUST 依次紧随 `workbench-crafting`，`torch-night` MUST 紧随 `block-light-room` 且先于 `bed-night`，`sword-combat` MUST 紧随 `ai-companion` 且先于 `hostile-mob`。完整顺序 MUST 与当前 `captureScenes` 表一致，`far-horizon` MUST 为倒数第二且 `water-underwater` MUST 为唯一末场景。既有显式更新、无窗口完整渲染链路和双阈值 MUST 保持不变；两张 far-horizon diagnostic controls MUST 继续不计入正式场景或 golden。golden 基线 SHALL 恰好为 22 张；四类容器场景验证的是容器面板与 tooltip 的 GPU 保留面，打开态保留面最坏组合 MUST 继续满足 `survival-hud-presentation` 的 218 quad/268 glyph 契约，关闭态 MUST 保持 0 quad/0 glyph。本变更 MUST NOT 借机放宽任何阈值。

#### Scenario: 完整场景顺序固定为 19 项

> 标题沿用历史名（openspec 1.7 的 MODIFIED 漂移守卫不支持 Scenario 改名）；当前正式清单为 22 项，语义以下述断言为准。

- **GIVEN** 完整正式 capture 场景清单
- **WHEN** 检查场景数量、名称与顺序
- **THEN** 清单 MUST 恰好包含上述 22 项
- **AND** `workbench-crafting` MUST 紧随 `inventory-crafting` 且在 `chest-container` 之前
- **AND** `torch-night` MUST 紧随 `block-light-room` 且在 `bed-night` 之前
- **AND** `sword-combat` MUST 紧随 `ai-companion` 且在 `hostile-mob` 之前
- **AND** `far-horizon` MUST 保持倒数第二，`water-underwater` MUST 保持唯一末项

#### Scenario: 背包与合成场景覆盖普通容器皮肤

- **GIVEN** `inventory-crafting` 装入固定背包、个人 2×2 网格中一条已匹配的真实原料形状、非空产物格和一个已选来源格
- **WHEN** 场景经完整链路收敛并无窗口抓取
- **THEN** 场景构造 MUST 同时呈现原创像素框、36 个凹槽、2×2 网格、产物格、背包/合成标题与来源轮廓
- **AND** 画面 MUST NOT 出现常显 HUD 的状态行或快捷栏贴条，命中区域与目标提示隐藏语义 MUST 保持不变

#### Scenario: 工作台场景覆盖 3×3 网格与镜像不对称配方

- **GIVEN** `workbench-crafting` 装入固定背包、已打开的工作台 3×3 网格、至少一条水平镜像不对称配方的合法摆放与合法产物
- **WHEN** 场景经完整链路收敛并无窗口抓取
- **THEN** 场景构造 MUST 同时呈现 3×3 网格、非空产物格与统一凹槽风格
- **AND** 场景 MUST NOT 依赖前一场景留下的容器或网格状态

#### Scenario: 箱子场景覆盖 63 格

- **GIVEN** `chest-container` 装入固定玩家背包、27 格箱子内容和一个已选来源栏位
- **WHEN** 场景经完整链路收敛并无窗口抓取
- **THEN** 图像 MUST 同时显示箱子标题、统一像素框、63 个栏位凹槽与来源轮廓
- **AND** 场景不得依赖前一场景留下的熔炉或箱子状态

#### Scenario: 熔炉场景覆盖 39 格和流程图示

- **GIVEN** `furnace-container` 装入固定玩家背包、已确认熔炉三格、部分燃烧/熔炼进度和一个已选来源栏位
- **WHEN** 场景经完整链路收敛并无窗口抓取
- **THEN** 图像 MUST 同时显示熔炉标题、统一凹槽、来源轮廓、输入/燃料/输出与可辨认的火焰/箭头图示
- **AND** 39 个统一栏位的布局 MUST 完整可审查且场景不得依赖前一场景留下的容器状态

#### Scenario: 全部正式 golden 重新生成并逐图复核

- **GIVEN** 容器保留面、火把纹理层与全部 overlay 的最终实现已经通过聚焦测试
- **WHEN** 显式更新视觉基线
- **THEN** 系统 MUST 重新生成全部 22 张正式 golden，并只提交实际场景文件
- **AND** 调用方 MUST 逐张人工复核 22 张图像后才能接受，且 MUST NOT 通过放宽双阈值接受差异
- **AND** 抓帧 MUST NOT 创建或聚焦前台游戏窗口，MUST NOT 导入、临摹或复制 Mojang 像素

#### Scenario: torch-night 纳入 golden 比对

- **GIVEN** 非更新模式运行 capture
- **WHEN** 执行到 `torch-night`
- **THEN** 该场景 MUST 与对应 golden 按既有双阈值比对，差异图规则与其它场景一致
- **AND** golden 目录 MUST 存在 `torch-night.png`，正式 golden 总数 MUST 恰好为 22 张

#### Scenario: 未受影响场景 golden 逐字节不变

- **GIVEN** 常显层退役的显式基线更新只波及携带常显 HUD 像素或共享世界背景的场景
- **WHEN** 运行 capture 并与本变更合入前的 golden 比对
- **THEN** `main-menu.png` 与 `settings-menu.png` 的 PNG 字节 MUST 逐字节不变
- **AND** 退役的 `hud-hotbar-health.png`、`hud-survival-feedback.png` 与 `hud-item-name-popup.png` MUST 从 golden 目录移除，golden 目录 MUST 恰好有 22 张 PNG
- **AND** 集成后全部 22 张 golden 在 compare 模式下 MUST 全部通过既有双阈值

### Requirement: 视觉基线覆盖调试面板

调试面板的呈现（读数区、参数分组段头、可编辑行与只读行对比、选中行高亮）SHALL 由 WebView 组件承担，其结构、可编辑语义与像素验收由 `game-overlay-webview` 的前端组件断言与 `frontend/visual` 部件基线承接。无头抓帧路径的程序化面板渲染路径 MUST NOT 保留：`debug-panel` 场景 MUST 继续存在并装入面板可见态，用于钉住「面板可见不产生任何无头面板像素」这一边界，其 golden SHALL 为同一相位与相机下的纯世界底图。既有双阈值 MUST 保持不变。

#### Scenario: 面板场景产出可审查基线

- **WHEN** 显式更新视觉基线
- **THEN** `debug-panel` 的 golden MUST 为固定正午、固定相机的纯世界底图
- **AND** 画面 MUST NOT 出现读数区、参数分组、可编辑行高亮或任何面板 chrome 像素
- **AND** 该图 MUST 由无窗口完整渲染链路产出

#### Scenario: 面板默认隐藏不影响其余场景

- **GIVEN** 抓帧路径为该场景装入面板可见态
- **WHEN** 抓取其余不涉及面板的场景
- **THEN** 那些场景的画面 MUST NOT 出现面板
- **AND** 它们的基线 MUST NOT 因面板状态机的存在而改变

#### Scenario: 面板可见不产生无头像素

- **GIVEN** `debug-panel` 场景已装入面板可见态
- **WHEN** 该场景与同一相机、同一世界时间的非面板世界帧比较
- **THEN** 两者 MUST 不存在面板像素差异
- **AND** 面板读数与参数行的呈现验收 MUST 由前端组件断言承接

### Requirement: sword-combat 无窗口场景固定呈现权威命中反馈

无窗口 capture SHALL 保留 `sword-combat` 场景，位于 `ai-companion` 之后、`hostile-mob` 之前。场景 MUST 使用固定相机与世界时间，选中 `Durability=125` 的铁剑，通过合法 UUIDv4 远端玩家 spawn/state 镜像呈现一次权威确认后的受击者，并显示 0.35 水平击退后的姿态或位置关系。权威命中 marker 的像素呈现已迁 WebView HUD 组件：画面 MUST NOT 出现 marker 像素，但场景 MUST 继续在收敛后、最终帧前经 `PinVolatile` 重新武装 marker 计时状态机，钉住「6 个成功呈现帧窗口」的权威语义；场景切换的共享 reset MUST 清除 combat feedback，避免污染后续 `hostile-mob`。场景 MUST 生成并比对 `sword-combat.png`，使用既有双阈值且不得创建或聚焦前台游戏窗口。

#### Scenario: 场景状态包含非满耐久铁剑、目标和 marker

- **GIVEN** `sword-combat` 固定夹具已装入
- **WHEN** 场景完成预热与上传并准备最终帧
- **THEN** MUST 显示选中的 `Durability=125` 铁剑、合法远端玩家与可观察的 0.35 水平击退关系
- **AND** 画面 MUST NOT 出现 marker、快捷栏或状态行像素；marker 的呈现验收由 WebView HUD 组件断言承接

#### Scenario: PinVolatile 在最终帧前重新武装 marker

- **GIVEN** 场景收敛帧可能已经消耗初次 marker 窗口
- **WHEN** capture 准备最终抓帧
- **THEN** `PinVolatile` MUST 把 marker 重置为 6 个成功呈现帧，权威侧 `CombatMarkerVisible` MUST 为真
- **AND** 该计时语义 MUST 不因 marker 像素已迁 WebView 而改变

#### Scenario: 场景切换清除 combat feedback

- **GIVEN** `sword-combat` 已留下 combat tick 与 marker 帧数
- **WHEN** capture 切换到 `hostile-mob`
- **THEN** shared presentation reset MUST 清除 combat feedback，后续场景 MUST 不继承 marker 或去重状态

#### Scenario: golden 只新增 sword-combat

> 标题沿用历史名（openspec 1.7 的 MODIFIED 漂移守卫不支持 Scenario 改名）；本变更不新增任何场景，语义以下述断言为准。

- **GIVEN** 22 张正式 golden 是当前清单的全部基线
- **WHEN** 显式生成并逐图审核本清单
- **THEN** tracked golden MUST 恰好覆盖 22 个场景名，MUST NOT 包含已退役场景的 PNG
- **AND** 任何 PNG 变化 MUST 逐图归因并明确批准，否则不得接受

### Requirement: 未受影响场景 golden 逐字节不变

菜单相位场景 `main-menu` 与紧随其后的 `settings-menu` 的 golden SHALL 为纯 wgpu 全景底图，不携带常显 HUD 像素与菜单 chrome（WebView 层由前端组件断言覆盖）。凡不影响全景渲染路径的呈现层变更（含常显 HUD 的 GPU 呈现退役）MUST NOT 改变这两张 golden 的字节；其余场景的 golden MUST 只经既有显式更新路径变化，且每一处差异 MUST 可归因到已声明的呈现层或共享世界背景变化，不得以放宽双阈值吸收。

#### Scenario: 非设置场景不受变更影响

- **GIVEN** 全部菜单相位场景（`main-menu` 与 `settings-menu`）
- **WHEN** 常显 HUD 的 GPU 呈现退役后显式更新整套 golden
- **THEN** 两张菜单 golden 的 PNG 字节 MUST 保持逐字节不变
- **AND** 其余场景的 golden 变化 MUST 归因于常显 HUD 条带消失或共享世界背景变化，MUST NOT 产生无归因的字节漂移

## REMOVED Requirements

### Requirement: 选中弹条无窗口场景

**Reason**: 物品名弹条与准星属常显 HUD，其 GPU 呈现已退役并由 WebView HUD 组件承担，无头抓帧路径不再产生这部分像素，`hud-item-name-popup` 场景没有可验收的 golden 内容；场景清单收缩为 22 项（见「视觉基线覆盖统一方块与 HUD 风格」）。
**Migration**: 弹条的确认变化触发、40 tick 可见窗口与容器/菜单相位抑制语义保留在 `survival-hud-presentation` 的「选中栏位变化以物品名弹条呈现」，准星语义保留在同 capability 的「准星以屏幕中心十字呈现」；两者与 marker 的像素验收由 `game-overlay-webview` 的前端组件断言与 `frontend/visual` 部件基线承接。
