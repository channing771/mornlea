## MODIFIED Requirements

### Requirement: 视觉基线覆盖统一方块与 HUD 风格

系统 SHALL 通过既有无窗口固定场景记录并比对当前产品默认方块材质与 HUD 呈现。地形场景 MUST 覆盖内嵌默认 layer 与没有内嵌映射时的程序化回退，HUD 场景 MUST 覆盖居中九格快捷栏、真实方块缩略图、数量阴影、工具耐久、双层选中边框、无背景生命行、耗损氧气、常驻饥饿、采掘反馈和权威 combat hit marker，并 MUST 以独立场景覆盖打开的背包、工作台、箱子和熔炉。更新基线时 MUST 继续执行既有显式更新、无窗口完整渲染链路和双阈值规则；不得创建或聚焦前台游戏窗口，不得导入、临摹或复制 Mojang 像素。

`materials-showcase` MUST 保持既有固定正午、固定相机和确定性夹具，并覆盖 14 种新材料、八格连续草地、相邻玻璃、相邻树叶、原木顶面年轮与侧面树皮，以及干湿耕地。正式 capture 清单 MUST 按以下完整顺序运行且恰好包含 24 项：`terrain-noon`、`hud-hotbar-health`、`hud-survival-feedback`、`avatar-nametag`、`inventory-crafting`、`workbench-crafting`、`chest-container`、`furnace-container`、`debug-panel`、`skylight-tunnel`、`block-light-room`、`torch-night`、`bed-night`、`materials-showcase`、`target-block-feedback`、`oak-grove`、`ai-companion`、`sword-combat`、`hostile-mob`、`water-surface-slope`、`main-menu`、`settings-menu`、`far-horizon`、`water-underwater`。`far-horizon` MUST 保持倒数第二，`water-underwater` MUST 保持唯一末项；两张 far-horizon diagnostic controls MUST 不计入正式场景或 golden。

#### Scenario: 地形与 HUD 风格变化产生可审查基线
- **GIVEN** 既有固定场景与渲染链路可用
- **WHEN** 显式更新本变更影响的视觉基线
- **THEN** `terrain-noon` MUST 包含当前内嵌默认材质及程序化回退
- **AND** `hud-hotbar-health` MUST 包含九格快捷栏、真实缩略图、数量、工具耐久、双层选中边框、十个满心与满饥饿
- **AND** `hud-survival-feedback` MUST 同时包含生命 `5`、氧气 `core.MaxOxygenTicks / 3`、饥饿 `9`、磨损工具和不可采目标 `4/9` 的采掘反馈
- **AND** HUD 图标 MUST 继续使用本项目原创程序化像素

#### Scenario: 完整场景顺序固定为 24 项
- **GIVEN** 完整正式 capture 场景清单
- **WHEN** 检查全部场景名称与顺序
- **THEN** 清单 MUST 逐项等于上述 24 项
- **AND** 相邻段 MUST 为 `ai-companion`、`sword-combat`、`hostile-mob`、`water-surface-slope`
- **AND** `far-horizon` MUST 是倒数第二，`water-underwater` MUST 是唯一末场景

#### Scenario: 生存反馈场景固定且不污染后续场景
- **GIVEN** `hud-survival-feedback` 已装入固定正午、相机、生命 `5`、氧气 `core.MaxOxygenTicks / 3`、饥饿 `9`、磨损工具和不可采 `4/9` 采掘夹具
- **WHEN** 场景经完整呈现链路收敛并无窗口抓取
- **THEN** 输出 MUST 同时显示全部指定生存反馈
- **AND** 场景结束后临时 predictor、生命、氧气、饥饿和采掘状态 MUST 一并恢复

#### Scenario: 打开背包场景复用同一向外状态栈
- **GIVEN** `inventory-crafting` 已打开背包并装入生命 `5`、氧气 `core.MaxOxygenTicks / 3` 与饥饿 `9`
- **WHEN** 场景经完整无窗口渲染链路抓取
- **THEN** health/hunger 主状态行和 depleted oxygen MUST 完全可见且不与 36 个可交互格和合成区域相交

#### Scenario: 远端玩家场景只继承地形背景变化
- **GIVEN** 远端玩家与名牌逻辑没有变化而默认地形背景变化
- **WHEN** 更新受影响视觉基线
- **THEN** `avatar-nametag` MUST 继承当前地形背景，远端玩家轮廓、颜色与名牌文字 MUST 保持既有语义

#### Scenario: 材料展示保持既有验收夹具
- **GIVEN** `materials-showcase` 固定夹具已装入客户端镜像
- **WHEN** 场景完成网格和上传收敛并抓帧
- **THEN** 图像 MUST 覆盖 14 种新材料、八格连续草地、相邻玻璃和树叶、原木顶/侧面及干湿耕地
- **AND** 玻璃后方方块、树叶孔洞与光照 MUST 可辨认，同类 cutout 方块内部面 MUST 不可见

#### Scenario: 材料展示只走无窗口完整链路
- **GIVEN** `materials-showcase` 使用固定正午与固定相机
- **WHEN** 生成或比对该场景
- **THEN** MUST 使用与交互客户端相同的完整呈现链路且不得创建或聚焦前台窗口

#### Scenario: 24 张正式 golden 需逐图复核
- **GIVEN** 本变更的视觉实现与 `sword-combat` 候选已经完成
- **WHEN** 显式更新视觉基线
- **THEN** 系统 MUST 按固定顺序生成 24 张正式 golden
- **AND** 调用方 MUST 逐张复核，既有场景原则上 MUST 保持在原双阈值内，任何差异不得通过放宽阈值接受

#### Scenario: 伙伴与统一战斗场景顺序并存
- **GIVEN** 完整无窗口场景清单
- **WHEN** 检查 `oak-grove` 之后的场景
- **THEN** `ai-companion` MUST 紧随 `oak-grove`，随后 MUST 依次为 `sword-combat`、`hostile-mob`、`water-surface-slope`

#### Scenario: 橡树林通过正常渲染链路抓取
- **GIVEN** `oak-grove` 固定世界种子、区块、正午与相机已装入
- **WHEN** 场景完成预热、网格收敛和上传并抓帧
- **THEN** 图像 MUST 显示固定橡树地貌且不得创建或聚焦前台窗口

#### Scenario: AI 伙伴通过统一呈现链路抓取
- **GIVEN** `ai-companion` 已清空前一场景状态并装入固定伙伴与聊天夹具
- **WHEN** 场景完成预热和上传并抓帧
- **THEN** MUST 同时显示伙伴人形、中文名牌“阿木”、accepted 事件与打开的 `@阿木 挖石头` 输入

#### Scenario: 目标反馈通过正常渲染链路验证遮挡
- **GIVEN** `target-block-feedback` 固定夹具命中一个已注册材料方块
- **WHEN** 场景完成收敛并抓帧
- **THEN** MUST 同时显示细轮廓、中文名称和被地形正确遮挡的边

#### Scenario: 打开背包的基线不受目标提示影响
- **GIVEN** `inventory-crafting` 场景打开背包
- **WHEN** 显式更新视觉基线
- **THEN** 场景 MUST 不显示目标轮廓或名称，背包与合成语义 MUST 保持不变

#### Scenario: 合并基线更新不改变阈值或尾序
- **GIVEN** 调用方在本变更最终基线上更新正式 golden
- **WHEN** 检查生成结果和比较配置
- **THEN** `far-horizon` 与 `water-underwater` 的尾序及既有双阈值 MUST 保持不变

### Requirement: 视觉基线覆盖三类容器像素界面

系统 SHALL 在上述 24 项正式无窗口清单中保留 `inventory-crafting`、`workbench-crafting`、`chest-container` 与 `furnace-container` 的相邻顺序，并保留 `torch-night` 紧随 `block-light-room`、`bed-night` 位于 `torch-night` 与 `materials-showcase` 之间。四类容器场景 MUST 继续覆盖原创像素框、凹槽、标题、来源轮廓、个人/工作台网格、箱子 63 格与熔炉 39 格；既有显式更新、完整渲染链路和双阈值 MUST 保持不变。golden 基线 SHALL 恰好为 24 张，其中本变更只新增 `sword-combat.png`；其他 PNG 只有在共享代码产生已逐图归因并明确批准的差异时才可更新。

#### Scenario: 容器场景顺序保持相邻
- **GIVEN** 完整正式 capture 场景清单
- **WHEN** 检查容器场景
- **THEN** `workbench-crafting` MUST 紧随 `inventory-crafting`，`chest-container` 与 `furnace-container` MUST 依次紧随其后

#### Scenario: 背包与合成场景覆盖普通容器皮肤
- **GIVEN** `inventory-crafting` 装入固定背包、个人 2×2 网格、合法产物与已选来源格
- **WHEN** 场景经完整链路收敛并抓取
- **THEN** MUST 呈现原创像素框、36 个凹槽、2×2 网格、产物格、标题与来源轮廓

#### Scenario: 工作台场景覆盖 3×3 网格与不对称配方
- **GIVEN** `workbench-crafting` 装入固定背包、3×3 网格和水平镜像不对称合法配方
- **WHEN** 场景经完整链路收敛并抓取
- **THEN** MUST 呈现 3×3 网格、非空产物格与统一凹槽，且不得依赖前一场景状态

#### Scenario: 箱子场景覆盖 63 格
- **GIVEN** `chest-container` 装入固定玩家背包、27 格箱子内容和已选来源栏位
- **WHEN** 场景经完整链路收敛并抓取
- **THEN** MUST 显示箱子标题、统一像素框、63 个栏位凹槽与来源轮廓

#### Scenario: 熔炉场景覆盖 39 格和流程图示
- **GIVEN** `furnace-container` 装入固定玩家背包、熔炉三格、部分燃烧/熔炼进度和已选来源栏位
- **WHEN** 场景经完整链路收敛并抓取
- **THEN** MUST 显示熔炉标题、统一凹槽、来源轮廓、39 格与火焰/箭头图示

#### Scenario: 正式 golden 数量为 24
- **GIVEN** 最终场景实现已通过聚焦测试
- **WHEN** 显式更新视觉基线
- **THEN** golden 目录 MUST 恰好包含 24 张正式 PNG，且新增文件 MUST 只有 `sword-combat.png`
- **AND** 任何既有 PNG 变化 MUST 逐图归因并明确批准，双阈值 MUST 不放宽

#### Scenario: torch-night 继续参与 golden 比对
- **GIVEN** 非更新模式运行 capture
- **WHEN** 执行到 `torch-night`
- **THEN** 该场景 MUST 与对应 golden 按既有双阈值比对，差异图规则 MUST 与其它场景一致

### Requirement: 夜行者无窗口场景

无窗口 capture 场景表 SHALL 保留 `hostile-mob`，其前一项 MUST 为新增 `sword-combat`，后一项 MUST 为 `water-surface-slope`；完整相邻顺序 MUST 为 `ai-companion`、`sword-combat`、`hostile-mob`、`water-surface-slope`。`water-underwater` MUST 仍为唯一末场景、`far-horizon` MUST 仍为倒数第二。场景 MUST 装入固定夜间确定性夹具：火把边缘固定位置呈现 8 只夜行者（其中一只处于受击状态、一只处于追逐中），并 MUST 经完整呈现链路无窗口抓取、使用既有双阈值且不产生相关名称标签。正式场景与 golden 总数 MUST 为 24。

#### Scenario: 场景表顺序与导出
- **GIVEN** 完整 capture 场景表
- **WHEN** 检查 `ai-companion` 之后的场景
- **THEN** MUST 依次为 `sword-combat`、`hostile-mob`、`water-surface-slope`
- **AND** 抓帧运行 MUST 产出 `hostile-mob.png`，正式场景总数 MUST 为 24

#### Scenario: 夹具确定性且无名标
- **GIVEN** `hostile-mob` 装入 8 只夜行者、1 只受击和 1 只追逐的固定夹具
- **WHEN** 场景完成预热、网格收敛与上传并抓帧
- **THEN** 图像 MUST 显示 8 只夜行者且可辨认受击与追逐呈现，MUST 不出现相关名称标签
- **AND** 场景结束后临时夜行者状态 MUST 一并恢复

#### Scenario: 无窗口完整链路
- **GIVEN** `hostile-mob` 使用固定夜间世界时间与固定相机
- **WHEN** 生成或比对该场景
- **THEN** MUST 使用与交互客户端相同的完整呈现链路且不得创建或聚焦前台窗口

## ADDED Requirements

### Requirement: sword-combat 无窗口场景固定呈现权威命中反馈

无窗口 capture SHALL 新增 `sword-combat` 场景，位于 `ai-companion` 之后、`hostile-mob` 之前。场景 MUST 使用固定相机与世界时间，选中 `Durability=125` 的铁剑，通过合法 UUIDv4 远端玩家 spawn/state 镜像呈现一次权威确认后的受击者，并显示 0.35 水平击退后的姿态或位置关系及处于 6 帧窗口内的 4-quad hit marker。场景收敛后、最终帧前，`PinVolatile` MUST 重新武装 marker；场景切换的共享 reset MUST 清除 combat feedback，避免污染后续 `hostile-mob`。场景 MUST 生成并比对 `sword-combat.png`，使用既有双阈值且不得创建或聚焦前台窗口。

#### Scenario: 场景状态包含非满耐久铁剑、目标和 marker
- **GIVEN** `sword-combat` 固定夹具已装入
- **WHEN** 场景完成预热与上传并准备最终帧
- **THEN** MUST 显示选中的 `Durability=125` 铁剑、合法远端玩家、权威 hit marker 和可观察的 0.35 水平击退关系

#### Scenario: PinVolatile 在最终帧前重新武装 marker
- **GIVEN** 场景收敛帧可能已经消耗初次 marker 窗口
- **WHEN** capture 准备最终抓帧
- **THEN** `PinVolatile` MUST 把 marker 重置为 6 个成功呈现帧，最终 PNG MUST 包含 marker

#### Scenario: 场景切换清除 combat feedback
- **GIVEN** `sword-combat` 已留下 combat tick 与 marker 帧数
- **WHEN** capture 切换到 `hostile-mob`
- **THEN** shared presentation reset MUST 清除 combat feedback，后续场景 MUST 不继承 marker 或去重状态

#### Scenario: golden 只新增 sword-combat
- **GIVEN** 23 张既有正式 golden 已作为当前基线
- **WHEN** 显式生成并逐图审核本变更的 24 项清单
- **THEN** tracked golden MUST 只新增 `sword-combat.png`；任何既有 PNG 变化 MUST 逐图归因并明确批准，否则不得接受
