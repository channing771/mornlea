## MODIFIED Requirements

### Requirement: 固定配方具有稳定语义

系统 SHALL 定义十九条稳定固定形状配方，recipe ID、形状、原料、数量和产物 MUST 由服务端定义，客户端不得声明或覆盖这些值。形状以裁边后的宽高与非空格序列表达，匹配遵守 `authoritative-grid-crafting` 的裁边与水平镜像规则：

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
- recipe ID `14`：2×3 两列满橡木木板，产出 3 个木门。
- recipe ID `15`：纵向 2 格、煤炭位于木棍正上方，产出 4 个火把。
- recipe ID `16`：3×3 顶排 3 个小麦、中排为空、下排 3 个橡木木板，产出 1 个床。
- recipe ID `17`：纵向 2 个橡木木板，下接 1 根木棍，产出 1 把满耐久木剑。
- recipe ID `18`：纵向 2 个圆石，下接 1 根木棍，产出 1 把满耐久石剑。
- recipe ID `19`：纵向 2 个铁锭，下接 1 根木棍，产出 1 把满耐久铁剑。

单格配方的形状可出现在归一化后网格的任一位置。三条剑配方的 1×3 归一化形状 MUST 可在 3×3 网格任一列整体横向平移；横放、倒放、材料错误或带任意多余材料 MUST 不匹配。recipe ID 只用于注册表与 UI 身份，新路径的线上消息 MUST NOT 携带 recipe ID，recipe ID MUST NOT 落盘。相同初始状态与命令序列经 Memory 和 TCP MUST 得到相同结果。

#### Scenario: 石砖配方可查询
- **GIVEN** 固定配方表已经注册
- **WHEN** 系统读取 recipe ID `1`
- **THEN** 该配方 MUST 稳定表示 2×2 石头转换为 4 个石砖

#### Scenario: 熔炉配方可查询
- **GIVEN** 固定配方表已经注册
- **WHEN** 系统读取 recipe ID `2`
- **THEN** 该配方 MUST 稳定表示 3×3 圆石圆环转换为 1 个熔炉

#### Scenario: 铁块配方可查询
- **GIVEN** 固定配方表已经注册
- **WHEN** 系统读取 recipe ID `3`
- **THEN** 该配方 MUST 稳定表示 9 个铁锭转换为 1 个铁块

#### Scenario: 石镐配方可查询
- **GIVEN** 固定配方表已经注册
- **WHEN** 系统读取 recipe ID `4`
- **THEN** 该配方 MUST 稳定表示顶排 3 个石头加中列 2 根木棍转换为 1 把石镐

#### Scenario: 铁镐配方可查询
- **GIVEN** 固定配方表已经注册
- **WHEN** 系统读取 recipe ID `5`
- **THEN** 该配方 MUST 稳定表示顶排 3 个铁锭加中列 2 根木棍转换为 1 把铁镐

#### Scenario: 箱子配方可查询
- **GIVEN** 固定配方表已经注册
- **WHEN** 系统读取 recipe ID `6`
- **THEN** 该配方 MUST 稳定表示 3×3 橡木木板圆环转换为 1 个箱子

#### Scenario: 橡木木板配方可查询
- **GIVEN** 固定配方表已经注册
- **WHEN** 系统读取 recipe ID `7`
- **THEN** 该配方 MUST 稳定表示 1 个橡木原木转换为 4 个橡木木板

#### Scenario: 发光方块配方可查询
- **GIVEN** 固定配方表已经注册
- **WHEN** 系统读取 recipe ID `8`
- **THEN** 该配方 MUST 稳定表示 4 个玻璃转换为 4 个发光方块

#### Scenario: 石锄和铁锄配方可查询
- **GIVEN** 固定配方表已经注册
- **WHEN** 系统读取 recipe ID `9` 或 `10`
- **THEN** 两条配方 MUST 分别稳定表示材料纵列旁接木棍纵列并产出满耐久石锄或铁锄

#### Scenario: 面包配方可查询并原子扣除
- **GIVEN** 玩家持有 3 个小麦
- **WHEN** 玩家合成 recipe ID `11`
- **THEN** 小麦 MUST 变为 0，玩家 MUST 获得 1 个面包；小麦不足时 MUST 原子失败且状态不变

#### Scenario: 木棍与工作台配方可查询
- **GIVEN** 固定配方表已经注册
- **WHEN** 系统读取 recipe ID `12` 或 `13`
- **THEN** recipe `12` MUST 表示纵向两木板转换为 4 根木棍，recipe `13` MUST 表示 2×2 木板转换为 1 个工作台

#### Scenario: 门与火把配方可查询
- **GIVEN** 固定配方表已经注册
- **WHEN** 系统读取 recipe ID `14` 或 `15`
- **THEN** recipe `14` MUST 表示 2×3 两列木板产出 3 个木门，recipe `15` MUST 表示煤炭在木棍上方产出 4 个火把

#### Scenario: 床配方保持 ID 16
- **GIVEN** 当前配方尾部为 `RecipeBed=16`
- **WHEN** 系统读取 recipe ID `16`
- **THEN** 它 MUST 保持 3×3 顶排小麦、中排空、下排橡木木板并产出 1 个床，新增剑配方 MUST 不改写其编号或形状

#### Scenario: 三把剑在任一列横向平移均匹配
- **GIVEN** 3×3 网格中按 `材料,材料,木棍` 纵向摆放 recipe `17`、`18` 或 `19`
- **WHEN** 该 1×3 形状分别位于左、中、右列并执行匹配
- **THEN** 三列摆放 MUST 命中同一 recipe，产物 MUST 为对应数量 1 的满耐久剑

#### Scenario: 横放倒放错料和多料拒绝
- **GIVEN** 剑材料被横放、木棍置顶、任一材料类型错误或网格存在额外物品
- **WHEN** 系统匹配 recipe `17..19`
- **THEN** MUST 不命中任何剑配方，完整合成状态 MUST 保持不变

#### Scenario: 未知配方被拒绝
- **GIVEN** 玩家具有任意有效完整物品状态
- **WHEN** 玩家请求 recipe ID `0` 或大于 `19` 的值
- **THEN** 系统 MUST 稳定拒绝且完整物品状态保持不变

#### Scenario: 发光方块配方失败保持原子
- **GIVEN** 玩家玻璃不足 4 个，或扣料后仍没有可接收全部 4 个发光方块的容量
- **WHEN** 玩家请求 recipe ID `8`
- **THEN** 服务端 MUST 拒绝请求，完整物品状态 MUST 保持不变且不得产生部分发光方块

#### Scenario: 锄头配方产出满耐久工具
- **GIVEN** 玩家网格摆放出 recipe ID `9` 或 `10` 的形状
- **WHEN** 玩家执行一次产物取出
- **THEN** 产出的锄头 MUST 具有该工具类型的满耐久，匹配形状的每个非空格 MUST 恰减 1

#### Scenario: 锄头配方原料不足时整体失败
- **GIVEN** 玩家原料少于 recipe `9` 或 `10` 所需数量
- **WHEN** 玩家请求该配方
- **THEN** 服务端 MUST 拒绝请求，完整物品状态 MUST 保持一字不变且不得产生锄头

#### Scenario: 既有配方编号不因新增而位移
- **GIVEN** 已追加 recipe `17..19` 的配方表
- **WHEN** 系统查询 recipe ID `1..16`
- **THEN** 每条编号、形状、原料、数量和产物 MUST 与追加前完全一致

#### Scenario: 镐配方使用木棍
- **GIVEN** 固定配方表已经注册
- **WHEN** 系统读取 recipe ID `4` 或 `5`
- **THEN** 配方形状 MUST 为顶排三原料加中列两根木棍

#### Scenario: 火把配方可在个人网格摆放
- **GIVEN** 玩家在 2×2 个人网格纵向摆放煤炭和木棍
- **WHEN** 系统执行形状匹配
- **THEN** MUST 命中 recipe `15` 并产出 4 个火把

#### Scenario: 单格配方的位置无关
- **GIVEN** recipe ID `7` 的单个橡木原木
- **WHEN** 原木分别放在 2×2 网格的四个格位
- **THEN** 四种摆放 MUST 都匹配 recipe `7`

#### Scenario: Memory 与 TCP 的形状合成一致
- **GIVEN** 相同初始完整物品状态与包含既有配方和剑配方的相同网格命令序列
- **WHEN** 场景分别通过 Memory 与 TCP 执行
- **THEN** 最终完整物品状态、产物耐久与拒绝结果 MUST 一致
