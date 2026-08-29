# tiered-swords-combat Specification

## Purpose

为可制作、可磨损的三级剑和玩家/夜行者统一近战提供服务端权威、固定容量、确定仲裁、私有命中确认与明确兼容边界，使 Memory 与 TCP 对同一输入产生相同结果。

## Requirements

### Requirement: 三级剑及损坏形态使用稳定追加编号和固定数值

系统 SHALL 在当前 `ItemBed=46` 后按顺序注册 `ItemWoodenSword=47`、`ItemStoneSword=48`、`ItemIronSword=49`、`ItemBrokenWoodenSword=50`、`ItemBrokenStoneSword=51`、`ItemBrokenIronSword=52`，并令 `ItemIDMax=53`。六种物品的 stack limit MUST 均为 1；木剑、石剑、铁剑的最大耐久 MUST 分别为 59、131、250，权威实体命中伤害 MUST 分别为 4、5、6；空手、普通物品和三个损坏剑的伤害 MUST 为 2。完好剑 MUST 分别映射到同材质损坏形态，损坏形态 MUST 不再具有耐久上限或分级武器伤害。

#### Scenario: 编号只在现有尾部追加

- **GIVEN** 当前物品尾部是 `ItemBed=46`
- **WHEN** 系统枚举新增剑物品和哨兵
- **THEN** 六个新增 ID MUST 依次为 `47..52`，`ItemIDMax` MUST 为 53，全部既有物品 ID MUST 保持不变

#### Scenario: 三档完好剑使用固定伤害和耐久

- **GIVEN** 玩家分别选中满耐久木剑、石剑和铁剑
- **WHEN** 三次攻击各自成功结算一次实体命中
- **THEN** 三次冻结伤害 MUST 分别为 4、5、6，三把剑的初始耐久 MUST 分别为 59、131、250

#### Scenario: 普通物品和损坏剑按基础伤害结算

- **GIVEN** 玩家分别空手、选中普通物品或任一损坏剑
- **WHEN** 攻击各自成功结算一次实体命中
- **THEN** 每次伤害 MUST 恰好为 2，损坏剑 MUST 不产生耐久损耗

#### Scenario: 当前存档路径无损往返六种剑

- **GIVEN** 玩家背包、区块容器、区块掉落物或伙伴背包含六种新增物品以及一把部分磨损的完好剑
- **WHEN** 当前程序按现有 schema 编码后重新解码
- **THEN** item ID、数量和耐久 MUST 逐字段保持不变，玩家 schema v8、区块 schema v9、`companions.ai` schema v4 MUST 不升版

### Requirement: 战斗身份和热路径容量固定且溢出整阶段失败

系统 SHALL 以 `(TargetKind, stable ID)` 标识战斗 actor，其中 player kind MUST 为 1、hostile kind MUST 为 2，其他 kind MUST 被拒绝；player stable ID MUST 无损承载 `SessionID`，hostile stable ID MUST 承载夜行者稳定 ID。每个 tick 的战斗阶段 MUST 最多构造 72 个 actor snapshot 和 72 条 raw intent，对应生产上限 8 名玩家与 64 只夜行者。任一追加超过传入边界或固定生产容量时，整个 tick 的战斗阶段 MUST fail closed：除进入阶段时清零所有 active player 的 tick-local 采掘抑制标志外，MUST NOT 递减或设置 cooldown，也 MUST NOT 改变 health、velocity、inventory、durability、fatigue 或 `CombatHit` 事实。

#### Scenario: 72 个 actor 和 72 条 intent 可完整处理

- **GIVEN** 测试 seam 的 actor 与 intent 上限均为 72，且输入恰好达到两个上限
- **WHEN** 权威战斗阶段推进一个 tick
- **THEN** 阶段 MUST 完成而不得因容量边界截断或产生部分结果

#### Scenario: actor 的下一次追加溢出整阶段失败

- **GIVEN** 测试 seam 的 actor 上限为 1，构造第二个 actor 将超过该上限
- **WHEN** 权威战斗阶段尝试构造第二个 snapshot
- **THEN** 阶段 MUST 返回失败，四类 cooldown、health、velocity、inventory、durability、fatigue 与 hit 事实 MUST 全部保持进入阶段前的值
- **AND** active player 的 tick-local 采掘抑制标志 MUST 已清零

#### Scenario: intent 的下一次追加溢出整阶段失败

- **GIVEN** 测试 seam 的 intent 上限为 1，两个合法攻击者各自可冻结一条 raw intent
- **WHEN** 权威战斗阶段尝试追加第二条 intent
- **THEN** 阶段 MUST 不结算第一条 intent，也 MUST 不提交 cooldown 递减或任何部分副作用

### Requirement: 全局 victim reservation 和结算顺序确定

系统 SHALL 在任何 live 状态写入前冻结全部 raw intent，再按攻击者全序执行全局 victim reservation：hostile MUST 排在 player 前，同 kind 内按无符号 stable ID 升序。每个 victim 每 tick MUST 最多接受一条 intent，竞争失败者 MUST 无任何副作用。指向不同 victim 的 A→B 与 B→A intent MUST 均保留，即使任一 actor 在本 tick 随后降至零血；死亡处理 MUST 位于全部 accepted intent 结算之后。每条 accepted intent MUST 按伤害、水平击退、攻击/受击 cooldown、player side effects、剑耐久、私有 hit 事实的顺序提交；任一身份、维度、目标或冻结栏位不变量在提交前无法解析时，该 intent MUST fail closed，其他独立 intent MUST 继续按稳定顺序处理。

#### Scenario: hostile 与 player 竞争同一 victim 时 hostile 胜出

- **GIVEN** 同一 tick 一只 hostile 和一名 player 各有一条指向同一 victim 的合法 raw intent
- **WHEN** 系统执行全局 victim reservation
- **THEN** hostile intent MUST 成为唯一 accepted intent，player loser MUST 不设置 cooldown、不收疲劳、不抑制采掘、不耗耐久也不产生 hit 事实

#### Scenario: 同 kind 竞争按最小 stable ID 胜出

- **GIVEN** 两个同 kind 攻击者同 tick 指向同一 victim
- **WHEN** 系统执行 reservation
- **THEN** stable ID 较小者 MUST 成为唯一 accepted intent，结果 MUST 不依赖 map、追加顺序或 goroutine 完成顺序

#### Scenario: 双向互击在死亡前全部成立

- **GIVEN** A→B 与 B→A 指向不同 victim，且两次伤害都足以使对方生命归零
- **WHEN** 系统结算该 tick
- **THEN** 双方伤害、击退、cooldown 和 player 剑耐久 MUST 全部成立，随后才处理双方死亡

#### Scenario: 水平击退为固定速度增量

- **GIVEN** accepted intent 的攻击者与目标水平位置不同
- **WHEN** 系统提交击退
- **THEN** 目标现有 XZ velocity MUST 加上沿 `target-attacker` 水平单位方向、大小恰为 0.35 的增量，Y velocity MUST 保持不变

#### Scenario: 水平重合使用攻击者 yaw

- **GIVEN** accepted intent 的攻击者与目标水平位置完全重合
- **WHEN** 系统提交击退
- **THEN** 方向 MUST 只由攻击者 yaw 的水平朝向导出，所有 velocity 分量 MUST 有限且不得出现 NaN 或 Inf

### Requirement: CombatHit 是协议 v32 的私有固定确认

协议 SHALL 从 v31 升为 v32，并在 Play S→C registry 尾部以 ID 25 追加固定 10-byte `CombatHit`，载荷依次为 little-endian `ServerTick u64`、`Damage u8`、`TargetKind u8`。消息 MUST 要求 `ServerTick > 0`、`Damage` 位于 `1..core.MaxHealth`、`TargetKind` 仅为 player 或 hostile，并 MUST 拒绝任意截断、尾随、未知 kind、错误状态及 ID 26。每个成功玩家攻击每 session 每 tick MUST 至多产生一条领域事实；server MUST 使用最终 tick 填充 `ServerTick`，并只在 inventory/container mirror 与 `PlayerState` 成功入队后向对应攻击者私发，受击者、旁观者、trusted observer 和 hostile 攻击目标 MUST 不收到。慢客户端 MUST 沿用既有有界发送队列与断开策略，不得增加战斗专用重试或缓冲。

#### Scenario: 固定载荷按 little-endian 往返

- **GIVEN** `ServerTick=0x0102030405060708`、`Damage=6`、`TargetKind=hostile`
- **WHEN** 系统编码 `CombatHit`
- **THEN** payload MUST 恰为 `08070605040302010602`，解码后字段 MUST 逐项相同

#### Scenario: 非法确认在网络边界拒绝

- **GIVEN** tick 为 0、damage 为 0 或 21、kind 为 0 或 3、payload 截断或存在任意尾随之一
- **WHEN** network trust boundary 解码或校验该消息
- **THEN** 消息 MUST 被拒绝且不得进入客户端 combat feedback 状态

#### Scenario: 确认只发送给玩家攻击者

- **GIVEN** 一名玩家成功攻击实体，受击者、旁观者和 trusted observer 同时在线
- **WHEN** server 发布该 tick 结果
- **THEN** 只有攻击者 session MUST 在其 `PlayerState` 之后收到一条 `CombatHit`

#### Scenario: Memory 与 TCP 的战斗业务事实一致

- **GIVEN** Memory 与 TCP 具有相同玩家、hostile、剑和输入脚本
- **WHEN** 两端分别结算 player→player 与 player→hostile 战斗
- **THEN** player/hostile health、velocity、剑耐久或损坏形态和确认载荷 MUST 一致，各 transport 内 `ServerTick` MUST 严格递增

### Requirement: 版本与存档兼容矩阵明确

变更后的版本矩阵 MUST 为：协议 v32、玩家 schema v8、区块 schema v9、世界 metadata v3、`companions.ai` schema v4、`hostile_mobs` schema v1、engine ABI v8、client ABI v11、benchmark scenario v20。player attack/hurt cooldown MUST 是重连清零的运行态；hostile MUST 复用 schema v1 既有 attack/hurt cooldown 字段且上限保持 20。当前程序 MUST 在玩家、区块容器/掉落和伙伴背包路径无损保存新增 ItemID；不知道新增 ItemID 的旧程序 MUST 安全拒绝，项目 MUST NOT 提供向后降级写入。

#### Scenario: v31 对端在 Play 前被拒绝

- **GIVEN** 客户端或服务端声明协议 v31
- **WHEN** 它连接到协议 v32 对端
- **THEN** 对端 MUST 按既有版本不匹配规则在进入 Play 前拒绝连接

#### Scenario: 非协议版本保持不变

- **GIVEN** 变更已完整实现并通过验证
- **WHEN** 检查持久化 schema、ABI 与 benchmark 版本
- **THEN** player/chunk/world/companions/hostile MUST 保持 8/9/3/4/1，engine/client ABI MUST 保持 8/11，benchmark scenario MUST 保持 20

#### Scenario: 含新剑的存档不能降级解释

- **GIVEN** 当前程序已保存含任一新剑 ItemID 的玩家或世界记录
- **WHEN** 不认识该 ItemID 的旧程序尝试读取
- **THEN** 旧程序 MUST 拒绝且不得把未知物品改为空气、覆盖记录或执行降级写入
