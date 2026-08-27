## MODIFIED Requirements

### Requirement: 固定配方具有稳定语义

系统 SHALL 定义十五条稳定固定形状配方，recipe ID、形状、原料、数量和产物 MUST 由服务端定义，客户端不得声明或覆盖这些值。形状以裁边后的宽高与非空格序列表达，匹配遵守 `authoritative-grid-crafting` 的裁边与水平镜像规则：

- recipe ID `1`：2×2 石头，产出 4 个石砖。
- recipe ID `2`：3×3 圆石圆环（中格为空），产出 1 个熔炉。
- recipe ID `3`：3×3 铁锭，产出 1 个铁块。
- recipe ID `4`：顶排 3 个石头、中列 2 根木棍，产出 1 把石镐。
- recipe ID `5`：顶排 3 个铁锭、中列 2 根木棍，产出 1 把铁镐。
- recipe ID `6`：3×3 橡木木板圆环（中格为空），产出 1 个箱子。
- recipe ID `7`：1 个橡木原木，产出 4 个橡木木板。
- recipe ID `8`：2×2 玻璃，产出 4 个发光方块。
- recipe ID `9`：2 个石头纵列旁接 2 根木棍纵列，产出 1 把石锄。
- recipe ID `10`：2 个铁锭纵列旁接 2 根木棍纵列，产出 1 把铁锄。
- recipe ID `11`：横排 3 个小麦，产出 1 个面包。
- recipe ID `12`：纵向 2 个橡木木板，产出 4 根木棍。
- recipe ID `13`：2×2 橡木木板，产出 1 个工作台。
- recipe ID `14`：2×3 两列满橡木木板，产出 3 个木门（门批次合入后的既有语义）。
- recipe ID `15`：纵向 2 格、煤炭位于木棍正上方，产出 4 个火把。

单格配方的形状可出现在归一化后网格的任一位置。recipe ID 只用于注册表与 UI 身份，新路径的线上消息 MUST NOT 携带 recipe ID，recipe ID MUST NOT 落盘。recipe `16..18`（木剑、石剑、铁剑、白床）由批次其余功能行在各自变更中追加，追加前查询这些 ID MUST 稳定拒绝。相同初始状态与命令序列经 Memory 和 TCP MUST 得到相同结果。

#### Scenario: 石砖配方可查询

- **WHEN** 系统读取 recipe ID `1`
- **THEN** 该配方稳定表示 2×2 石头转换为 4 个石砖

#### Scenario: 熔炉配方可查询

- **WHEN** 系统读取 recipe ID `2`
- **THEN** 该配方稳定表示 3×3 圆石圆环（中格为空）转换为 1 个熔炉

#### Scenario: 镐配方使用木棍

- **WHEN** 系统读取 recipe ID `4` 或 `5`
- **THEN** 该配方的形状为顶排三原料加中列两根木棍

#### Scenario: 木棍与工作台配方可查询

- **WHEN** 系统读取 recipe ID `12` 或 `13`
- **THEN** recipe `12` 稳定表示纵向两木板转换为 4 根木棍，recipe `13` 稳定表示 2×2 木板转换为 1 个工作台

#### Scenario: 火把配方可查询且可在个人网格摆放

- **WHEN** 系统读取 recipe ID `15`
- **THEN** 该配方稳定表示「煤炭位于木棍正上方」的纵向两格形状转换为 4 个火把
- **AND** 该形状宽 1 高 2，MUST 能在 2×2 个人网格内摆放并匹配；倒置（木棍在煤炭上方）MUST NOT 匹配

#### Scenario: 单格配方的位置无关

- **GIVEN** recipe ID `7` 的单个橡木原木
- **WHEN** 原木分别放在 2×2 网格的四个格位
- **THEN** 四种摆放都匹配 recipe `7`

#### Scenario: 未登记配方被拒绝

- **GIVEN** 玩家具有任意有效完整物品状态
- **WHEN** 系统查询 recipe ID `0`、大于 `15` 的值或尚未由批次其余功能行追加的 `16..18`
- **THEN** 查询 MUST 稳定拒绝且不产生任何产物

#### Scenario: 既有配方编号不因新增而位移

- **GIVEN** 追加 recipe `15` 后的配方表
- **WHEN** 系统查询 recipe ID `1`..`14`
- **THEN** 每条的编号 MUST 保持不变，语义与追加前完全一致

#### Scenario: 锄头配方产出满耐久工具

- **GIVEN** 玩家网格摆放出 recipe ID `9` 或 `10` 的形状
- **WHEN** 玩家执行一次产物取出
- **THEN** 产出的锄头 MUST 具有该工具类型的满耐久，且匹配形状的每个非空格恰减 1

#### Scenario: Memory 与 TCP 的形状合成一致

- **GIVEN** 相同初始完整物品状态与相同的网格命令序列
- **WHEN** 场景分别通过 Memory 与 TCP 执行
- **THEN** 最终完整物品状态与拒绝结果 MUST 一致
