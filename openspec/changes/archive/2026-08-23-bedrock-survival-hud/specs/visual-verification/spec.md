## MODIFIED Requirements

### Requirement: 视觉基线覆盖统一方块与 HUD 风格

系统 SHALL 通过既有无窗口固定场景记录并比对当前产品默认方块材质与 HUD 呈现。地形场景 MUST 覆盖内嵌默认 layer 与没有内嵌映射时的程序化回退，HUD 场景 MUST 覆盖居中九格快捷栏、从同一生效 registry 采样的真实方块缩略图、数量阴影、工具耐久、双层选中边框、无背景的十段生命行、耗损氧气、常驻饥饿与具有颜色及形状差异的采掘进度，并 MUST 以独立场景覆盖打开的背包与合成区域；更新基线时 MUST 继续执行既有显式更新与双阈值规则。状态构图 MUST 与 Minecraft 官方生存 HUD 参考中“生命/饥饿分居快捷栏两侧、气泡堆叠在饥饿外侧”的可观察关系比较，官方参考 `https://www.minecraft.net/en-us/article/health-minecraft` 只可作为构图证据。心形、气泡、鸡腿与其他 HUD 像素 MUST 继续由本项目原创程序化绘制或既有授权来源生成，MUST NOT 从 Mojang 或官方参考导入、临摹或复制像素资产。

`materials-showcase` MUST 保持既有固定正午、固定相机和确定性夹具，并经与交互客户端相同的完整呈现链路收敛后无窗口抓取，不得创建或聚焦前台游戏窗口。夹具 MUST 同时覆盖 14 种新材料、八格连续草地、相邻玻璃、相邻树叶，以及原木顶面年轮与侧面树皮。既有双阈值 MUST 保持不变。

抓帧场景清单 MUST 按以下完整顺序运行：`terrain-noon`、`hud-hotbar-health`、`hud-survival-feedback`、`avatar-nametag`、`inventory-crafting`、`debug-panel`、`skylight-tunnel`、`block-light-room`、`materials-showcase`、`target-block-feedback`、`oak-grove`、`ai-companion`、`water-surface-slope`、`far-horizon`、`water-underwater`。清单 MUST 保留 `target-block-feedback`、`oak-grove` 与 `ai-companion` 的既有名称及相对顺序，`ai-companion` MUST 继续紧随 `oak-grove`，并 MUST 保留当前末尾的 `water-surface-slope`、倒数第二的 `far-horizon` 与唯一末场景 `water-underwater`。`target-block-feedback` MUST 使用固定正午、固定相机和确定性夹具，且经与交互客户端相同的完整呈现链路收敛后无窗口抓取。它 MUST 命中一个已注册材料方块，并同时可审查细轮廓、中文名称和正确的遮挡关系；抓帧或比对 MUST NOT 使用隐藏目标提示的专用开关。`inventory-crafting` 因打开背包而隐藏目标提示，其目标提示隐藏状态、背包与合成区域语义 MUST 保持不变；若内嵌默认材质、程序化回退或共享地形背景改变可观察像素，其 golden MAY 在逐图复核后更新。`oak-grove` MUST 使用固定世界种子、固定生成区块、固定正午和固定相机，并经与交互客户端相同的完整呈现链路收敛后无窗口抓取，不得创建或聚焦前台游戏窗口。`ai-companion` MUST 使用固定世界时间、相机、伙伴身份、维度、位置与朝向，显示中文名牌“阿木”、一条 accepted ChatEvent 和打开的 `@阿木 挖石头` 输入，并经统一的人形、名牌和聊天 HUD 呈现链路无窗口抓取。

#### Scenario: 地形与 HUD 风格变化产生可审查基线

- **GIVEN** 既有固定场景与渲染链路可用
- **WHEN** 显式更新本变更影响的视觉基线
- **THEN** `terrain-noon` MUST 包含当前内嵌默认材质及没有内嵌映射 layer 的程序化回退
- **AND** `hud-hotbar-health` MUST 包含正常九格快捷栏、从同一生效 registry 采样的真实方块缩略图、紧凑两位间距的数量数字、工具耐久、双层选中边框、无背景的十个满心与满饥饿；生命起点 MUST 精确对齐快捷栏左边缘，饥饿终点 MUST 精确对齐快捷栏右边缘，满氧 MUST 完全隐藏且不得留下空的水平中栏
- **AND** `hud-survival-feedback` MUST 在同一固定帧同时包含生命 `5`、氧气 `core.MaxOxygenTicks / 3`、饥饿 `9`、磨损工具和不可采目标 `4/9` 的中段采掘进度；氧气 MUST 沿饥饿右边缘堆叠在饥饿上方，采掘反馈 MUST 位于完整两行状态栈上方，并显示不可采状态的颜色与形状标记
- **AND** `inventory-crafting` MUST 包含打开的 3×9 背包区、1×9 快捷栏区和十行固定合成区域；生命/饥饿主状态行 MUST 位于快捷栏下方并对齐快捷栏左右边缘，氧气 MUST 沿饥饿右边缘继续向下堆叠，且两行不得覆盖或相交 36 个可交互格与十行配方
- **AND** 四张图 MUST 由无窗口完整渲染链路产出并继续使用既有双阈值
- **AND** 三个 HUD 场景 MUST 与官方生存 HUD 参考只比较上述构图关系，并继续使用本项目原创图标，MUST NOT 导入 Mojang 像素

#### Scenario: 完整场景顺序加入生存反馈

- **GIVEN** 完整无窗口 capture 场景清单
- **WHEN** 检查全部场景名称与顺序
- **THEN** 清单 MUST 依次为 `terrain-noon`、`hud-hotbar-health`、`hud-survival-feedback`、`avatar-nametag`、`inventory-crafting`、`debug-panel`、`skylight-tunnel`、`block-light-room`、`materials-showcase`、`target-block-feedback`、`oak-grove`、`ai-companion`、`water-surface-slope`、`far-horizon`、`water-underwater`
- **AND** `far-horizon` MUST 是倒数第二个场景，`water-underwater` MUST 是唯一末场景

#### Scenario: 生存反馈场景固定且不污染后续场景

- **GIVEN** `hud-survival-feedback` 已装入固定正午、固定相机、生命 `5`、氧气 `core.MaxOxygenTicks / 3`、饥饿 `9`、磨损工具和不可采 `4/9` 采掘夹具
- **WHEN** 场景经与交互客户端相同的完整呈现链路收敛并无窗口抓取
- **THEN** 输出 MUST 在同一帧显示全部指定生存反馈并继续使用既有双阈值
- **AND** 场景结束后临时 predictor、生命、氧气、饥饿和采掘状态 MUST 一并恢复，使后续场景不继承任何夹具值

#### Scenario: 打开背包场景复用同一向外状态栈

- **GIVEN** `inventory-crafting` 已打开背包并装入生命 `5`、氧气 `core.MaxOxygenTicks / 3` 与饥饿 `9`
- **WHEN** 场景经完整无窗口渲染链路抓取
- **THEN** health / hunger 主状态行 MUST 位于快捷栏下方并分别对齐其左右边缘，depleted oxygen MUST 沿 hunger 右边缘堆叠在其下方，且两行完全可见
- **AND** 两行状态 MUST 与全部 36 个可交互格和十行配方保持清晰分离

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

#### Scenario: 合并后的全部正式基线需重新生成并完整复核

- **GIVEN** 当前分支的 Pixel Perfection HUD 与 main 的 authoritative hunger 已语义合并
- **WHEN** 显式更新视觉基线
- **THEN** 系统 MUST 按既有完整顺序重新生成全部 15 张正式 golden
- **AND** 调用方 MUST 逐张人工复核全部 15 张图像后才能接受更新，且既有双阈值 MUST 保持不变

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

#### Scenario: 合并基线更新不改变阈值或场景尾序

- **GIVEN** 调用方在合并后的 Pixel Perfection + hunger 基线上更新全部正式 golden
- **WHEN** 检查生成结果和比较配置
- **THEN** `water-surface-slope`、倒数第二的 `far-horizon` 与唯一末场景 `water-underwater` 的尾序 MUST 保持不变
- **AND** 既有双阈值 MUST 保持不变，任何差异 MUST 经逐图人工复核而不得通过放宽阈值接受
