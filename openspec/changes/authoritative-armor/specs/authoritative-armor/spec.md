# Spec: authoritative-armor

## ADDED Requirements

### Requirement: 护甲域是 core 单一真源

系统 SHALL 在 `packages/shared/core` 新建护甲域，单一真源地定义：四个槽位（头/胸/腿/脚）与每件护甲物品到槽位的映射；每件铁质护甲的护甲点数（头盔 2、胸甲 6、护腿 5、靴子 2）；`MaxArmorPoints = 20` 的总点数上限；护甲耐久上限（头盔 165、胸甲 240、护腿 225、靴子 195）；以及整数确定性减免函数。点数与减免只允许从该域读取，其他包 MUST NOT 复制表值或内联公式。

减免函数 SHALL 为 `ReducedDamage(damage, points) = max(1, damage × (100 − 4 × points) / 100)`，其中乘除为 int32 整数运算、除法向零取整，且 `damage` MUST 为正。穿戴总点数 SHALL 为四槽中完好护甲件点数之和并截断到 `MaxArmorPoints`；损坏形态的护甲件贡献 0 点。

#### Scenario: 减免公式向量

- **GIVEN** 点数 0、15、20 与伤害 3
- **WHEN** 调用减免函数
- **THEN** 结果依次为 3（无减免）、1（`3×40/100 = 1` 取整后 max 1）、1

#### Scenario: 最低 1 点伤害

- **GIVEN** 穿戴满套护甲（15 点）且受到 1 点伤害
- **WHEN** 调用减免函数
- **THEN** 结果为 1，任何点数组合 MUST NOT 把有效伤害减到 0

#### Scenario: 损坏件不贡献点数

- **GIVEN** 头盔与胸甲为完好件（2+6 点）、护腿与靴子为损坏形态
- **WHEN** 计算穿戴总点数
- **THEN** 总点数为 8

### Requirement: 铁质护甲物品编号与耐久

系统 SHALL 在 `ItemSapling` 之后、`ItemIDMax` 哨兵之前只追加 `ItemIronHelmet = 58`、`ItemIronChestplate = 59`、`ItemIronLeggings = 60`、`ItemIronBoots = 61`，并把哨兵更新为 `ItemIDMax = 62`。四件物品 MUST 堆叠上限为 1、MUST NOT 出现在任何 `BlockDrop` 表、MUST NOT 经 `ItemPlacement` 放置、MUST 各自登记耐久上限并沿用既有耐久形态机制（完好/损坏两态）。新增编号 MUST NOT 重排或复用任何既有物品编号。

#### Scenario: 编号追加与哨兵推进

- **GIVEN** 既有物品注册表（`ItemSapling = 57`、`ItemIDMax = 58`）
- **WHEN** 护甲四件加入注册表
- **THEN** 四件编号为 58..61 且 `ItemIDMax = 62`，穷举测试按哨兵覆盖全部物品

#### Scenario: 耐久跨存档与协议无损传递

- **GIVEN** 一件耐久非满的已装备护甲
- **WHEN** 经存档保存/加载与线上协议同步
- **THEN** 耐久值与损坏形态逐位保持（沿用 `tool-durability` 主规格的传递门禁语义）

### Requirement: 装备动作是权威原子互换

系统 SHALL 提供新 C→S 命令 `EquipArmor`（Play C→S ID 18，载荷仅 `Sequence`）：服务端 MUST 校验该会话玩家当前所选快捷栏格持有护甲物品，目标槽位由护甲件类映射唯一确定；验证成功后在同一权威 tick 内原子互换所选快捷栏格与该护甲槽位的内容（手中旧件换回手、原装备件上台），并按既有背包同步纪律标记 `inventoryDirty`。所选格未持有护甲时服务端 MUST 以 `CommandRejected{Reason: not_armor}` 拒绝且不产生任何状态变更。本命令 MUST NOT 新增私有成功确认消息（快捷栏变更经既有 `InventoryUpdate` 广播覆盖）。

#### Scenario: 空槽穿戴

- **GIVEN** 玩家所选快捷栏格持有完好铁头盔且对应槽位为空
- **WHEN** 发送 `EquipArmor`
- **THEN** 头盔进入头部槽位、快捷栏该格变空，`ArmorPoints` 在后续玩家状态中反映 2 点

#### Scenario: 占用槽位互换

- **GIVEN** 头部槽位已有护甲且所选快捷栏格持有铁胸甲
- **WHEN** 发送 `EquipArmor`
- **THEN** 胸甲上台、原头部护甲件回到所选快捷栏格，交换在同 tick 原子完成

#### Scenario: 非护甲拒绝

- **GIVEN** 玩家所选快捷栏格持有石剑
- **WHEN** 发送 `EquipArmor`
- **THEN** 收到 `CommandRejected{Reason: not_armor}`，快捷栏与护甲槽位逐位不变

### Requirement: 近战伤害减免只在近战结算点生效

系统 SHALL 在近战结算点（`settleCombatIntent` 提交前）对目标为玩家的 intent 按其冻结护甲点数计算有效伤害后再进入 `applyDamage`：覆盖敌怪→玩家与玩家→玩家两类 intent，快照 MUST 冻结结算时刻的点数。摔落、溺水与饥饿伤害 MUST NOT 减免（它们不经近战结算点，直接调用 `applyDamage`）。护甲减免 MUST NOT 改变击退、仇恨、疲劳与战斗确认的既有语义。

#### Scenario: 敌怪近战被减免

- **GIVEN** 玩家穿戴满套铁甲（15 点）且被夜行者近战命中（intent 伤害 3）
- **WHEN** 结算该 intent
- **THEN** 玩家实际扣血 1，击退与受击冷却语义不变

#### Scenario: PvP 同点减免

- **GIVEN** 攻击方玩家持木剑（伤害 4）近战命中穿戴 15 点护甲的玩家
- **WHEN** 结算该 intent
- **THEN** 受击玩家实际扣血 `max(1, 4×40/100) = 1`

#### Scenario: 摔落不受减免

- **GIVEN** 穿戴满套铁甲的玩家从超过安全高度处落地
- **WHEN** 摔落伤害结算
- **THEN** 扣血量与未穿戴护甲时逐位相同

### Requirement: 护甲耐久随减免消耗

系统 SHALL 在一次近战 intent 对玩家目标产生减免（有效伤害低于原始伤害且点数大于 0）时，对目标四槽中每件**完好**护甲件消耗恰好 1 点耐久，沿既有耐久消耗机制；损坏形态件与空槽 MUST NOT 消耗。耐久归零转损坏形态的语义沿 `tool-durability` 主规格。拒绝路径（`not_armor` 等）与未产生减免的受击（点数为 0）MUST NOT 消耗耐久。

#### Scenario: 受击全件损耗

- **GIVEN** 玩家四槽全部穿戴完好铁甲且被近战命中
- **WHEN** 结算产生减免
- **THEN** 四件耐久各恰好减 1，点数计算使用消耗前的完好状态

#### Scenario: 点数为零不消耗

- **GIVEN** 玩家仅穿戴损坏形态护甲（0 点）且被近战命中
- **WHEN** 结算该 intent
- **THEN** 有效伤害等于原始伤害，耐久不发生任何变化

### Requirement: 装备随玩家 schema v9 持久化

玩家存档 schema SHALL 由 v8 升至 v9：在既有载荷尾部按槽位顺序追加四个护甲槽，每槽沿用背包格同一 5 字节栈编码（item u16 小端 + count 1 字节 + durability u16 小端，共 20 字节）。v1..v8 旧档 MUST 按只读迁移加载（装备为空），首次保存 MUST 写出 schema v9，未来版本 MUST 被拒绝。重启后装备与点数 MUST 保值；`PlayerHash` MUST 追加装备区使 Memory 与 TCP 两条传输的 parity 断言覆盖装备状态。玩家死亡时 MUST 按既有死亡掉落纪律把已装备护甲与背包一并掉落。

#### Scenario: v8 旧档迁移

- **GIVEN** 一份升级前已保存的有效玩家 schema v8 存档
- **WHEN** 新版本加载并再次保存
- **THEN** 加载成功且四槽为空，保存写出 schema v9 且原 v8 字节段逐位保留

#### Scenario: 装备跨重启保值

- **GIVEN** 玩家穿戴部分护甲后正常下线
- **WHEN** 服务端重启并加载该玩家
- **THEN** 四槽内容、耐久形态与 `ArmorPoints` 与下线时逐位一致

#### Scenario: 死亡掉落装备

- **GIVEN** 穿戴护甲的玩家死亡
- **WHEN** 死亡掉落结算
- **THEN** 四槽护甲按既有环形外扩纪律掉落，装备槽清空

### Requirement: 护甲点数同步与 HUD 护甲条

`PlayerState` SHALL 在载荷尾部（`Temperature` 之后）追加 `ArmorPoints uint8`（协议 v42），合法区间 0..`MaxArmorPoints`，越界值在 Validate、编码与解码三处被拒绝；点数为 0 时 MUST 与协议 v41 的载荷逐位可区分字段仅为追加字节。桥 `uiState` SHALL 追加 armor 分节（client ABI v18→v19，Go/Rust/TS 三端钉值），WebView 状态行组件族 SHALL 在心形行上方渲染护甲条：10 档图标、半档粒度、以 `MaxArmorPoints` 为满档基准；`ArmorPoints` 为 0 时 MUST NOT 渲染任何护甲条像素（既有场景零像素差异）。客户端 SHALL 在使用键上升沿且手持护甲时上行 `EquipArmor`（不发送 `PlaceBlock`，沿 F-03 判定先例）。

#### Scenario: 点数同步与越界拒绝

- **GIVEN** 服务端权威点数为 7
- **WHEN** 玩家状态同步到客户端
- **THEN** `ArmorPoints = 7`；构造 `ArmorPoints = 21` 的玩家状态 MUST 被三处校验一致拒绝

#### Scenario: 零点数零像素

- **GIVEN** 玩家未穿戴任何护甲
- **WHEN** 渲染任一既有 capture 场景
- **THEN** 与协议 v41 基线的 golden 逐位零差异

#### Scenario: 护甲条半档粒度

- **GIVEN** 权威点数为 7
- **WHEN** 渲染穿甲 capture 场景
- **THEN** 护甲条呈现 3 个完整图标与 1 个半图标（7/20 按 2 点每档、半档收尾）
