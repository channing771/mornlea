# Spec: authoritative-projectiles

## ADDED Requirements

### Requirement: 投射物是服务端权威的瞬态实体

系统 SHALL 维护一个服务端唯一权威的投射物集合：全服上限 128 条、按 ID 严格升序维护、ID 由世界种子与权威 tick 经确定性哈希派生（非零、无进程级随机源）；集合满时新的投射物 MUST 以「最旧 ID 先清」腾位，且腾位 MUST 产生 despawn 发布。投射物 MUST 分为两类弹种：骨刺（远程敌怪发射）与箭（玩家弓发射）。在飞投射物 MUST NOT 持久化：存档与重启 MUST 不写也不读投射物状态，重启后在飞投射物消失。投射物 MUST NOT 由客户端预测；客户端只持有按服务器消息演进的镜像。

#### Scenario: 集合上限与最旧先清

- **GIVEN** 投射物集合已含 128 条在飞投射物
- **WHEN** 生成第 129 条投射物
- **THEN** 系统 MUST 移除当前集合中 ID 最旧的一条并发布其 despawn，新投射物 MUST 正常存在

#### Scenario: 重启不保留在飞投射物

- **GIVEN** 世界中存在 3 条在飞投射物
- **WHEN** 服务端停机并重启恢复
- **THEN** 存档文件 MUST 不含投射物数据，重启后 MUST 不存在任何在飞投射物，且敌怪与玩家状态恢复不受影响

#### Scenario: 同输入重放逐位一致

- **GIVEN** 相同世界种子、相同输入序列（含射击时机与方向）
- **WHEN** 两次独立运行完整推进相同权威 tick 区间
- **THEN** 投射物的生成、轨迹、命中与消失事件序列 MUST 逐位一致

### Requirement: 弹道推进与方块命中确定性

系统 SHALL 以固定步长推进每条投射物：每权威 tick 恰好一步，重力与初速为固定数值契约（重力 18 格/秒²；箭短档初速 16、满档 30、骨刺 22 格/秒）。方块命中 SHALL 以「上一位置到新位置」的线段经既有生产射线出口按 `core.InteractionTarget` 谓词求首个命中；命中方块的投射物 MUST 在同 tick 消失且不掉落。投射物寿命 SHALL 为 100 tick，寿命耗尽、离开全部会话订阅区或越出世界边界时 MUST 消失。推进与命中检测 MUST 全部有界：每 tick 至多 128 条投射物 × 常数级候选扫描，MUST NOT 执行无界工作、map 遍历或阻塞 I/O。

#### Scenario: 命中方块即消失

- **GIVEN** 一条箭的下一位置穿过一面石墙
- **WHEN** 该权威 tick 完成推进
- **THEN** 该箭 MUST 停在命中点且消失，MUST NOT 掉落箭物品，订阅会话 MUST 收到 despawn

#### Scenario: 寿命耗尽消失

- **GIVEN** 一条未命中任何东西的骨刺
- **WHEN** 第 101 个存活 tick 完成
- **THEN** 该骨刺 MUST 已消失并发布 despawn

#### Scenario: 掠过实体未命中则继续飞行

- **GIVEN** 一条箭的飞行线段与某敌怪 AABB 最近距离小于半个格但不相交
- **WHEN** 该权威 tick 完成推进
- **THEN** 该箭 MUST 继续按弹道飞行，MUST NOT 结算命中

### Requirement: 命中结算复用统一伤害面

投射物命中实体 SHALL 在命中 tick 按弹种规则结算：箭命中其他玩家、敌怪与被动牛；骨刺只命中玩家；两类弹种 MUST NOT 命中其发射者本人。对玩家目标 SHALL 按命中时点冻结的护甲点数经既有减免函数折算有效伤害，产生减免时消耗全部参与件各 1 点耐久，并沿弹速水平分量施加击退，最终 MUST 经既有伤害唯一入口结算；玩家所有的箭命中实体时 MUST 向其持有者会话追加既有近战命中私有确认。对敌怪与被动目标 SHALL 分别经敌怪受伤入口与被动受伤入口结算并施加击退。弹击致死 MUST 与近战致死同 tick 完成掉落与重生结算，任何 0 生命实体 MUST NOT 存活到下一权威 tick。

#### Scenario: 护甲减免骨刺伤害

- **GIVEN** 玩家穿戴 15 点护甲且被 3 点伤害的骨刺命中
- **WHEN** 命中结算
- **THEN** 有效伤害 MUST 为既有减免公式的结果，参与护甲件 MUST 各消耗 1 点耐久，玩家 MUST 被沿弹速水平方向击退

#### Scenario: 箭命中敌怪并同 tick 死亡

- **GIVEN** 满档箭命中一只只剩 1 点生命的夜行者
- **WHEN** 该权威 tick 完成
- **THEN** 该夜行者 MUST 同 tick 完成死亡掉落与移除，玩家持有者 MUST 收到命中确认

#### Scenario: 发射者不被自己的弹种命中

- **GIVEN** 掷骨者刚发射一条骨刺且骨刺线段与其自身 AABB 相交
- **WHEN** 该权威 tick 完成推进
- **THEN** 该掷骨者 MUST NOT 受到该骨刺伤害，骨刺 MUST 继续飞行

#### Scenario: 掷骨者不攻击被动牛

- **GIVEN** 骨刺线段同时穿过一头牛与一名玩家的 AABB
- **WHEN** 命中结算
- **THEN** 只有玩家 MUST 受到伤害（牛 MUST NOT 被骨刺命中）

### Requirement: 投射物按会话订阅发布且客户端镜像不预测

系统 SHALL 提供三类 S→C 消息：`ProjectileSpawn`（ID、弹种、dimension、位置、速度）、`ProjectileState`（ID、位置）与 `ProjectileDespawn`（ID 列表），每类每会话每 tick 至多一包、记录按 ID 严格升序、数量有固定上界，发布顺序每 tick 为 despawn → spawn → state。可见性 SHALL 与敌怪同谓词：仅向订阅了投射物所在 chunk 的会话发布；Memory 与 TCP 对同一世界序列 MUST 给出逐字段相同的发布序列。客户端 SHALL 以 latest-wins 镜像演进（未知 ID 的 state 丢弃、重复 spawn 忽略、 despawn 移除）并做插值呈现，MUST NOT 预测弹道。

#### Scenario: 订阅会话收到完整生命周期

- **GIVEN** 某会话订阅了骨刺飞行路径上的 chunk
- **WHEN** 骨刺生成、飞行至命中
- **THEN** 该会话 MUST 依次收到该 ID 的 spawn、逐 tick state 与 despawn，且从未订阅该路径的会话 MUST 收不到

#### Scenario: 双传输发布序列一致

- **GIVEN** 相同世界状态与投掷序列
- **WHEN** Memory 与 TCP 各运行一次并记录全部会话发布
- **THEN** 两边的投射物消息序列 MUST 逐字段相同

#### Scenario: 客户端忽略未知 ID 的 state

- **GIVEN** 客户端镜像中不存在某投射物 ID
- **WHEN** 收到该 ID 的 state 消息
- **THEN** 镜像 MUST 静默丢弃该条且 MUST NOT 隐式生成实体
