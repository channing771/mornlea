# debug-panel Specification

## Purpose

## ADDED Requirements

### Requirement: 调试面板由 F3 切换显示

交互式图形客户端 SHALL 在 `-dev` 选项启用时以 F3 边沿切换调试面板的显示与隐藏；面板由隐藏变可见时 MUST 显示当前帧的顶部只读读数与参数行，由可见变隐藏时 MUST 清空累积的编辑状态。`-dev` 未启用时 SHALL 不显示任何调试面板内容。

#### Scenario: F3 边沿切换显示隐藏

- **GIVEN** `-dev` 启用的交互客户端，面板初始隐藏
- **WHEN** 用户按下并释放 F3
- **THEN** 面板变为可见
- **AND** 面板返回顶部读数与参数行状态
- **AND** 再次按下并释放 F3
- **THEN** 面板变为隐藏

#### Scenario: `-dev` 关闭时无面板

- **GIVEN** `-dev` 未启用的交互客户端
- **WHEN** 用户按下 F3
- **THEN** 调试面板 MUST NOT 出现，游戏输入按未按下面板键处理

### Requirement: 面板顶部显示只读读数

面板展开时 SHALL 在顶部显示固定读数区：帧耗时、玩家位置、朝向、权威 tick、世界时刻、已加载区块数与模式名。读数区 MUST 为只读，不可选择、不可编辑。

#### Scenario: 读数区内容可判定

- **GIVEN** 面板可见且玩家位于已知世界位置
- **WHEN** 查看面板顶部
- **THEN** 读数区 MUST 显示帧耗时、位置、朝向、tick、时刻、区块数与模式名
- **AND** 读数区 MUST 无任何编辑控件

### Requirement: 参数行按可编辑分类呈现

参数行 SHALL 按 `config.Fields()` 的只读/可编辑分类呈现：只读行 MUST 禁止选中与编辑；可编辑行 MUST 可被选中并显示当前值。一次最多一个处于编辑状态的行。行数上限 `64`、每行标签/值上限 `24` 字节的既有约束 MUST 保留。

#### Scenario: 只读行不可编辑

- **GIVEN** 面板可见且存在只读行
- **WHEN** 用户尝试选中或编辑该行
- **THEN** 选中与编辑 MUST 不发生

#### Scenario: 行数截断

- **GIVEN** `config.Fields()` 返回多于 `64` 行
- **WHEN** 面板呈现
- **THEN** 仅前 `64` 行可见

### Requirement: 行选中与值编辑

可编辑行 SHALL 支持方向键移动选中；按 Enter 进入编辑后 MUST 用文本编辑输入新值，Enter 确认写回，Esc 取消编辑恢复原值；编辑期间 MUST 阻止游戏输入与其它行的选中切换。非法新值 SHALL 被拒绝并保持原值。

#### Scenario: 方向键移动选中行

- **GIVEN** 面板可见且存在可编辑行
- **WHEN** 用户按方向键上/下
- **THEN** 选中行 SHALL 移动到相邻可编辑行

#### Scenario: Enter 编辑并确认写回

- **GIVEN** 面板可见且当前选中可编辑行
- **WHEN** 用户按 Enter、输入新值、再按 Enter
- **THEN** 目标行值 SHALL 更新
- **AND** 面板返回选中态（非编辑态）

#### Scenario: Esc 取消编辑

- **GIVEN** 面板可见且当前行处于编辑态
- **WHEN** 用户按 Esc
- **THEN** 目标行 SHALL 显示编辑前的原值
- **AND** 面板返回选中态

#### Scenario: 非法值拒绝

- **GIVEN** 面板可见且当前行处于编辑态
- **WHEN** 用户输入非法值（如对布尔字段输入非布尔文本）并确认
- **THEN** 目标行值 MUST NOT 改变
- **AND** 面板返回选中态且编辑不得留下半写状态

### Requirement: 面板可见时捕获游戏输入

面板可见时 SHALL 捕获全部键盘输入：游戏移动、挖掘、掉落等游戏键 MUST 不产生上行；仅面板键（F3、方向键、Enter、Esc、数字与编辑文本）有效。面板隐藏时 MUST 恢复游戏键盘捕获。

#### Scenario: 面板可见时游戏键无上行

- **GIVEN** 面板可见
- **WHEN** 用户按下 WASD 或其它游戏键
- **THEN** 游戏键 MUST NOT 产生任何游戏输入上行
- **AND** 玩家位置与朝向 MUST 不变

#### Scenario: 面板隐藏恢复游戏输入

- **GIVEN** 面板刚隐藏
- **WHEN** 用户按下游戏键
- **THEN** 游戏键 MUST 恢复产生游戏输入上行

### Requirement: 联机远端只读

`remote()` 为真的联机客户端 SHALL 只读面板：可以显示读数与参数行，但行选中与值编辑 MUST 被禁止，写回 MUST 不发生。

#### Scenario: 联机面板只读

- **GIVEN** `remote()` 为真且面板可见
- **WHEN** 用户尝试编辑任意参数行
- **THEN** 编辑 MUST 不发生，面板显示值 MUST 与原值一致

### Requirement: 面板值写回仅本地进程

面板确认的值写回 SHALL 只更新当前进程内的配置与 tunables（如 physics/sim/render 值），不会生成任何网络消息，也不会持久化到世界/玩家/伙伴存档。

#### Scenario: 编辑值后无网络消息

- **GIVEN** `remote()` 为假的单机或 benchmark 客户端且面板可见
- **WHEN** 用户编辑并确认一行
- **THEN** 该值 SHALL 在当前进程生效
- **AND** 任何网络退发缓冲 MUST 保持为空
