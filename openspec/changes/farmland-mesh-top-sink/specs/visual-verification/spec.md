# visual-verification 规格增量

## MODIFIED Requirements

### Requirement: 视觉基线覆盖统一方块与 HUD 风格

`materials-showcase` MUST 保持既有固定正午、固定相机和确定性夹具，并经与交互客户端相同的完整呈现链路收敛后无窗口抓取，不得创建或聚焦前台游戏窗口。夹具 MUST 同时覆盖 14 种新材料、八格连续草地、相邻玻璃、相邻树叶、原木顶面年轮与侧面树皮，以及干耕地与湿耕地各至少一个可见列（含下沉顶面的完整几何）。既有双阈值 MUST 保持不变。

抓帧场景清单 MUST 按以下完整顺序运行：`terrain-noon`、`hud-hotbar-health`、`hud-survival-feedback`、`avatar-nametag`、`inventory-crafting`、`chest-container`、`furnace-container`、`debug-panel`、`skylight-tunnel`、`block-light-room`、`materials-showcase`、`target-block-feedback`、`oak-grove`、`ai-companion`、`water-surface-slope`、`far-horizon`、`water-underwater`。清单 MUST 保留 `target-block-feedback`、`oak-grove` 与 `ai-companion` 的既有名称及相对顺序，`ai-companion` MUST 继续紧随 `oak-grove`，并 MUST 保留当前末尾的 `water-surface-slope`、倒数第二的 `far-horizon` 与唯一末场景 `water-underwater`。`target-block-feedback` MUST 使用固定正午、固定相机和确定性夹具，且经与交互客户端相同的完整呈现链路收敛后无窗口抓取。它 MUST 命中一个已注册材料方块，并同时可审查细轮廓、中文名称和正确的遮挡关系；抓帧或比对 MUST NOT 使用隐藏目标提示的专用开关。`inventory-crafting` 因打开背包而隐藏目标提示，其目标提示隐藏状态、背包与合成区域语义 MUST 保持不变；若内嵌默认材质、程序化回退或共享地形背景改变可观察像素，其 golden MAY 在逐图复核后更新。`oak-grove` MUST 使用固定世界种子、固定生成区块、固定正午和固定相机，并经与交互客户端相同的完整呈现链路收敛后无窗口抓取，不得创建或聚焦前台游戏窗口。`ai-companion` MUST 使用固定世界时间、相机、伙伴身份、维度、位置与朝向，显示中文名牌“阿木”、一条 accepted ChatEvent 和打开的 `@阿木 挖石头` 输入，并经统一的人形、名牌和聊天 HUD 呈现链路无窗口抓取。

#### Scenario: 场景清单与既有顺序一致

- **GIVEN** 抓帧模式被调用且未做任何场景筛选
- **WHEN** 全部场景依次运行
- **THEN** 清单 MUST 依次为 `terrain-noon`、`hud-hotbar-health`、`hud-survival-feedback`、`avatar-nametag`、`inventory-crafting`、`chest-container`、`furnace-container`、`debug-panel`、`skylight-tunnel`、`block-light-room`、`materials-showcase`、`target-block-feedback`、`oak-grove`、`ai-companion`、`water-surface-slope`、`far-horizon`、`water-underwater`

#### Scenario: materials-showcase 夹具覆盖耕地两态

- **GIVEN** `materials-showcase` 的固定夹具已装入客户端镜像
- **WHEN** 场景完成网格与上传收敛并抓帧
- **THEN** 画面中 MUST 同时存在干耕地与湿耕地两个可见列
- **AND** 两列的顶面 MUST 呈现在下沉高度而非整格顶面
