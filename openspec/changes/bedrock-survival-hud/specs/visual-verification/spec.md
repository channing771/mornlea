## MODIFIED Requirements

### Requirement: 视觉基线覆盖统一方块与 HUD 风格

系统 SHALL 通过既有无窗口固定场景记录并比对当前产品默认方块材质与 HUD 呈现。地形场景 MUST 覆盖内嵌默认 layer 与没有内嵌映射时的程序化回退，HUD 场景 MUST 覆盖居中九格快捷栏、从同一生效 registry 采样的真实方块缩略图、数量阴影、工具耐久、双层选中边框、十段生命、耗损氧气和具有颜色及形状差异的采掘进度，并 MUST 以独立场景覆盖打开的背包与合成区域；更新基线时 MUST 继续执行既有显式更新与双阈值规则。

`materials-showcase` MUST 保持既有固定正午、固定相机和确定性夹具，并经与交互客户端相同的完整呈现链路收敛后无窗口抓取，不得创建或聚焦前台游戏窗口。夹具 MUST 同时覆盖 14 种新材料、八格连续草地、相邻玻璃、相邻树叶，以及原木顶面年轮与侧面树皮。既有双阈值 MUST 保持不变。

抓帧场景清单 MUST 按以下完整顺序运行：`terrain-noon`、`hud-hotbar-health`、`hud-survival-feedback`、`avatar-nametag`、`inventory-crafting`、`debug-panel`、`skylight-tunnel`、`block-light-room`、`materials-showcase`、`target-block-feedback`、`oak-grove`、`ai-companion`、`water-surface-slope`、`far-horizon`、`water-underwater`。清单 MUST 保留 `target-block-feedback`、`oak-grove` 与 `ai-companion` 的既有名称及相对顺序，`ai-companion` MUST 继续紧随 `oak-grove`，并 MUST 保留当前末尾的 `water-surface-slope`、倒数第二的 `far-horizon` 与唯一末场景 `water-underwater`。`target-block-feedback` MUST 使用固定正午、固定相机和确定性夹具，且经与交互客户端相同的完整呈现链路收敛后无窗口抓取。它 MUST 命中一个已注册材料方块，并同时可审查细轮廓、中文名称和正确的遮挡关系；抓帧或比对 MUST NOT 使用隐藏目标提示的专用开关。`inventory-crafting` 因打开背包而隐藏目标提示，其目标提示隐藏状态、背包与合成区域语义 MUST 保持不变；若内嵌默认材质、程序化回退或共享地形背景改变可观察像素，其 golden MAY 在逐图复核后更新。`oak-grove` MUST 使用固定世界种子、固定生成区块、固定正午和固定相机，并经与交互客户端相同的完整呈现链路收敛后无窗口抓取，不得创建或聚焦前台游戏窗口。`ai-companion` MUST 使用固定世界时间、相机、伙伴身份、维度、位置与朝向，显示中文名牌“阿木”、一条 accepted ChatEvent 和打开的 `@阿木 挖石头` 输入，并经统一的人形、名牌和聊天 HUD 呈现链路无窗口抓取。

#### Scenario: 地形与 HUD 风格变化产生可审查基线

- **GIVEN** 既有固定场景与渲染链路可用
- **WHEN** 显式更新本变更影响的视觉基线
- **THEN** `terrain-noon` MUST 包含当前内嵌默认材质及没有内嵌映射 layer 的程序化回退
- **AND** `hud-hotbar-health` MUST 包含正常九格快捷栏、从同一生效 registry 采样的真实方块缩略图、紧凑两位间距的数量数字、工具耐久、双层选中边框与十个满心，且满氧 MUST 完全隐藏
- **AND** `hud-survival-feedback` MUST 在同一固定帧同时包含低生命、耗损氧气、磨损工具和不可采目标的中段采掘进度，并显示不可采状态的颜色与形状标记
- **AND** `inventory-crafting` MUST 包含打开的 3×9 背包区、1×9 快捷栏区和固定合成区域，生命与耗损氧气 MUST 位于快捷栏下方且不覆盖可交互格子
- **AND** 四张图 MUST 由无窗口完整渲染链路产出并继续使用既有双阈值

#### Scenario: 完整场景顺序加入生存反馈

- **GIVEN** 完整无窗口 capture 场景清单
- **WHEN** 检查全部场景名称与顺序
- **THEN** 清单 MUST 依次为 `terrain-noon`、`hud-hotbar-health`、`hud-survival-feedback`、`avatar-nametag`、`inventory-crafting`、`debug-panel`、`skylight-tunnel`、`block-light-room`、`materials-showcase`、`target-block-feedback`、`oak-grove`、`ai-companion`、`water-surface-slope`、`far-horizon`、`water-underwater`
- **AND** `far-horizon` MUST 是倒数第二个场景，`water-underwater` MUST 是唯一末场景

#### Scenario: 生存反馈场景固定且不污染后续场景

- **GIVEN** `hud-survival-feedback` 已装入固定正午、固定相机、低生命、耗损氧气、磨损工具和不可采中段采掘夹具
- **WHEN** 场景经与交互客户端相同的完整呈现链路收敛并无窗口抓取
- **THEN** 输出 MUST 在同一帧显示全部指定生存反馈并继续使用既有双阈值
- **AND** 场景结束后临时 HUD 状态 MUST 恢复，使后续场景不继承其生命、氧气或采掘夹具

#### Scenario: 远端玩家场景只继承地形背景变化

- **GIVEN** 远端玩家与名牌的渲染逻辑没有变化，但场景共享的当前产品默认地形背景发生变化
- **WHEN** 更新本变更影响的视觉基线
- **THEN** `avatar-nametag` MUST 继承当前地形背景
- **AND** 远端玩家轮廓、颜色与名牌文字 MUST 保持既有可观察语义

#### Scenario: 材料展示保持既有验收夹具

- **GIVEN** `materials-showcase` 的固定夹具已装入客户端镜像
- **WHEN** `materials-showcase` 完成网格和上传收敛并抓帧
- **THEN** 图像 MUST 同时显示 14 种新材料各一个近景样本及多方块表面、跨至少一个 AO 或天空光拆分边界的八格连续草地、两个相邻玻璃方块、两个相邻树叶方块，以及原木顶面年轮与侧面树皮
- **AND** 玻璃后方方块 MUST 可见，树叶孔洞和光照 MUST 可辨认，相同 cutout 方块的内部面 MUST 不可见

#### Scenario: 材料展示只走无窗口完整链路

- **GIVEN** `materials-showcase` 使用固定正午与固定相机
- **WHEN** 生成或比对 `materials-showcase`
- **THEN** 抓帧 MUST 使用与交互客户端相同的完整呈现链路
- **AND** MUST NOT 创建或聚焦前台游戏窗口，且 MUST 继续使用现有双阈值

#### Scenario: 共享视觉变化需完整复核

- **GIVEN** 内嵌默认材质、程序化回退、世界生成或世界坐标 UV 改变了多个既有场景的共享可观察像素
- **WHEN** 显式更新视觉基线
- **THEN** 系统 MUST 只写入实际变化的 golden，调用方 MUST 逐张复核全部场景图像后才能接受更新

#### Scenario: 伙伴场景与当前末尾顺序并存

- **GIVEN** 完整无窗口场景清单
- **WHEN** 检查 `target-block-feedback` 之后的场景名称与顺序
- **THEN** `oak-grove` 与 `ai-companion` MUST 保持既有名称，且 `ai-companion` MUST 紧随 `oak-grove`
- **AND** `water-surface-slope` MUST 位于 `ai-companion` 之后，`far-horizon` MUST 是倒数第二个场景，`water-underwater` MUST 是唯一末场景

#### Scenario: 橡树林通过正常渲染链路抓取

- **GIVEN** `oak-grove` 的固定世界种子、生成区块、正午时间与相机已经装入客户端镜像
- **WHEN** 场景完成预热、网格收敛和上传并抓帧
- **THEN** 图像 MUST 由与交互客户端相同的完整呈现链路产出，且 MUST 显示固定橡树地貌
- **AND** 抓帧 MUST NOT 创建或聚焦前台游戏窗口，且 MUST 继续使用既有双阈值

#### Scenario: AI 伙伴通过统一呈现链路抓取

- **GIVEN** `ai-companion` 已重置前一场景的 remote、companion、chat、inventory、panel、container、mining、damage 和 item-drop 状态，并装入固定伙伴和聊天夹具
- **WHEN** 场景完成预热和上传并抓帧
- **THEN** 图像 MUST 由统一的人形、名牌与聊天 HUD 呈现链路产出，且 MUST 同时显示伙伴人形、中文名牌“阿木”、accepted 事件与打开的 `@阿木 挖石头` 输入
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
- **AND** 背包与合成区域的可观察语义 MUST 保持不变
- **AND** 只有经逐图复核确认由当前产品默认材质或共享地形背景变化引起时，它的 golden MAY 更新

#### Scenario: 默认材质变化允许复核后更新既有 golden

- **GIVEN** 内嵌默认材质改变了一个或多个既有场景的可观察像素
- **WHEN** 调用方显式更新视觉基线
- **THEN** 版本控制中的 golden 变化 MUST 只包含实际变化的场景，且调用方 MUST 逐张复核全部候选图后才能接受
- **AND** 系统 MUST NOT 仅因既有 golden 被当前产品默认材质改变而拒绝整次更新
