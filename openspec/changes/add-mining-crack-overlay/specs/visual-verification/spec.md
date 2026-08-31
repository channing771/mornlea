# visual-verification Delta

## Purpose

把两个采掘裂纹固定场景（`mining-crack-early`、`mining-crack-heavy`）纳入无窗口
视觉基线网：正式场景清单与 golden 张数随裂纹场景扩容，插入位置以顺序 MUST 条款
钉死；裂纹像素的可判读性由场景内像素断言与既有双阈值共同兜底，不放宽任何阈值。

## MODIFIED Requirements

### Requirement: 视觉基线覆盖统一方块与 HUD 风格

系统 SHALL 通过既有无窗口固定场景记录并比对当前产品默认方块材质与 HUD 呈现。地形场景 MUST 覆盖内嵌默认 layer 与没有内嵌映射时的程序化回退，HUD 场景 MUST 覆盖居中九格快捷栏、从同一生效 registry 采样的真实方块缩略图、数量阴影、工具耐久、双层选中边框、无背景的十段生命行、耗损氧气、常驻饥饿、具有颜色及形状差异的采掘进度与权威 combat hit marker，并 MUST 以独立场景覆盖打开的背包、工作台、箱子和熔炉；更新基线时 MUST 继续执行既有显式更新与双阈值规则。状态构图 MUST 与 Minecraft 官方生存 HUD 参考中“生命/饥饿分居快捷栏两侧、气泡堆叠在饥饿外侧”的可观察关系比较，官方参考 `https://www.minecraft.net/en-us/article/health-minecraft` 只可作为构图证据。心形、气泡、鸡腿与其他 HUD 像素 MUST 继续由本项目原创程序化绘制或既有授权来源生成，MUST NOT 从 Mojang 或官方参考导入、临摹或复制像素资产。

`materials-showcase` MUST 保持既有固定正午、固定相机和确定性夹具，并经与交互客户端相同的完整呈现链路收敛后无窗口抓取，不得创建或聚焦前台游戏窗口。夹具 MUST 同时覆盖 14 种新材料、八格连续草地、相邻玻璃、相邻树叶、原木顶面年轮与侧面树皮，以及干耕地与湿耕地各至少一个可见列（含下沉顶面的完整几何）。既有双阈值 MUST 保持不变。

抓帧场景清单 MUST 按以下完整顺序运行（27 景）：`terrain-noon`、`hud-hotbar-health`、`hud-survival-feedback`、`hud-item-name-popup`、`avatar-nametag`、`inventory-crafting`、`workbench-crafting`、`chest-container`、`furnace-container`、`debug-panel`、`skylight-tunnel`、`block-light-room`、`torch-night`、`bed-night`、`materials-showcase`、`target-block-feedback`、`oak-grove`、`ai-companion`、`sword-combat`、`hostile-mob`、`water-surface-slope`、`mining-crack-early`、`mining-crack-heavy`、`main-menu`、`settings-menu`、`far-horizon`、`water-underwater`。清单 MUST 保留 `target-block-feedback`、`oak-grove` 与 `ai-companion` 的既有名称及相对顺序，`ai-companion` MUST 继续紧随 `oak-grove`，并 MUST 保持 `sword-combat`、`hostile-mob`、`water-surface-slope` 的相邻顺序，`mining-crack-early` 与 `mining-crack-heavy` MUST 依次紧随 `water-surface-slope` 且先于 `main-menu`，`settings-menu` MUST 紧随 `main-menu`，`far-horizon` MUST 为倒数第二，`water-underwater` MUST 为唯一末场景。所有场景 MUST 使用与交互客户端相同的完整呈现链路收敛后无窗口抓取，且不得创建或聚焦前台游戏窗口。

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
- **THEN** 清单 MUST 依次为 `terrain-noon`、`hud-hotbar-health`、`hud-survival-feedback`、`hud-item-name-popup`、`avatar-nametag`、`inventory-crafting`、`workbench-crafting`、`chest-container`、`furnace-container`、`debug-panel`、`skylight-tunnel`、`block-light-room`、`torch-night`、`bed-night`、`materials-showcase`、`target-block-feedback`、`oak-grove`、`ai-companion`、`sword-combat`、`hostile-mob`、`water-surface-slope`、`mining-crack-early`、`mining-crack-heavy`、`main-menu`、`settings-menu`、`far-horizon`、`water-underwater`
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
- **THEN** 图像 MUST 同时显示 14 种新材料各一个近景样本及多方块表面、跨至少一个 AO 或天空光拆分边界的八格连续草地、两个相邻玻璃方块、两个相邻树叶方块，以及原木顶面年轮与侧面树皮、干耕地与湿耕地各一个可见列（两列顶面呈现在下沉高度而非整格顶面）
- **AND** 玻璃后方方块 MUST 可见，树叶孔洞和光照 MUST 可辨认，相同 cutout 方块的内部面 MUST 不可见

#### Scenario: 材料展示只走无窗口完整链路

- **GIVEN** `materials-showcase` 使用固定正午与固定相机
- **WHEN** 生成或比对 `materials-showcase`
- **THEN** 抓帧 MUST 使用与交互客户端相同的完整呈现链路
- **AND** MUST NOT 创建或聚焦前台游戏窗口，且 MUST 继续使用现有双阈值

#### Scenario: 合并后的全部正式基线需重新生成并完整复核

- **GIVEN** 当前分支的 Pixel Perfection HUD 与 main 的 authoritative hunger 已语义合并
- **WHEN** 显式更新视觉基线
- **THEN** 系统 MUST 按既有完整顺序重新生成全部 27 张正式 golden
- **AND** 调用方 MUST 逐张人工复核全部 27 张图像后才能接受更新，且既有双阈值 MUST 保持不变

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
- **THEN** `water-surface-slope`、`main-menu`、`settings-menu`、倒数第二的 `far-horizon` 与唯一末场景 `water-underwater` 的尾序 MUST 保持不变
- **AND** 既有双阈值 MUST 保持不变，任何差异 MUST 经逐图人工复核而不得通过放宽阈值接受

### Requirement: 视觉基线覆盖三类容器像素界面

系统 SHALL 具有恰好 27 个正式无窗口场景，`workbench-crafting` MUST 紧随 `inventory-crafting`，`chest-container` 与 `furnace-container` MUST 依次紧随 `workbench-crafting`，`torch-night` MUST 紧随 `block-light-room` 且先于 `bed-night`，`sword-combat` MUST 紧随 `ai-companion` 且先于 `hostile-mob`。完整顺序 MUST 与当前 `captureScenes` 表一致，`far-horizon` MUST 为倒数第二且 `water-underwater` MUST 为唯一末场景。既有显式更新、无窗口完整渲染链路和双阈值 MUST 保持不变；两张 far-horizon diagnostic controls MUST 继续不计入正式场景或 golden。golden 基线 SHALL 为 27 张；本变更 MUST NOT 借机放宽任何阈值。

#### Scenario: 完整场景顺序固定为 19 项

> 标题沿用历史名（openspec 1.7 的 MODIFIED 漂移守卫不支持 Scenario 改名）；当前正式清单为 27 项，语义以下述断言为准。

- **GIVEN** 完整正式 capture 场景清单
- **WHEN** 检查场景数量、名称与顺序
- **THEN** 清单 MUST 恰好包含上述 27 项
- **AND** `workbench-crafting` MUST 紧随 `inventory-crafting` 且在 `chest-container` 之前
- **AND** `torch-night` MUST 紧随 `block-light-room` 且在 `bed-night` 之前
- **AND** `sword-combat` MUST 紧随 `ai-companion` 且在 `hostile-mob` 之前
- **AND** `mining-crack-early` 与 `mining-crack-heavy` MUST 依次紧随 `water-surface-slope` 且在 `main-menu` 之前
- **AND** `far-horizon` MUST 保持倒数第二，`water-underwater` MUST 保持唯一末项

#### Scenario: 背包与合成场景覆盖普通容器皮肤

- **GIVEN** `inventory-crafting` 装入固定背包、个人 2×2 网格中一条已匹配的真实原料形状、非空产物格和一个已选来源格
- **WHEN** 场景经完整链路收敛并无窗口抓取
- **THEN** 场景构造 MUST 同时呈现原创像素框、36 个凹槽、2×2 网格、产物格、背包/合成标题与来源轮廓
- **AND** 既有 health/hunger/oxygen 外向状态栈、命中区域与目标提示隐藏语义 MUST 保持不变

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

- **GIVEN** 容器 atlas、火把纹理层与全部 overlay 的最终实现已经通过聚焦测试
- **WHEN** 显式更新视觉基线
- **THEN** 系统 MUST 重新生成全部 27 张正式 golden，并只提交实际场景文件
- **AND** 调用方 MUST 逐张人工复核 27 张图像后才能接受，且 MUST NOT 通过放宽双阈值接受差异
- **AND** 抓帧 MUST NOT 创建或聚焦前台游戏窗口，MUST NOT 导入、临摹或复制 Mojang 像素

#### Scenario: torch-night 纳入 golden 比对

- **GIVEN** 非更新模式运行 capture
- **WHEN** 执行到 `torch-night`
- **THEN** 该场景 MUST 与对应 golden 按既有双阈值比对，差异图规则与其它场景一致
- **AND** golden 目录 MUST 存在 `torch-night.png`，正式 golden 总数 MUST 为 27 张

#### Scenario: 未受影响场景 golden 逐字节不变

- **GIVEN** 一次显式基线更新只新增当次变更引入的场景基线
- **WHEN** 运行 capture 并与本变更合入前的 golden 比对
- **THEN** 除新增的场景基线外，MUST NOT 改动任何既有 golden 的内容
- **AND** 集成后全部 27 张 golden 在 compare 模式下 MUST 全部通过既有双阈值
