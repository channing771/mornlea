# 兼容性与升级

本文承接 README「兼容性与升级」一节，给出线上协议、世界/玩家/伙伴存档的版本契约、升级前后的备份与回退纪律，以及 benchmark 报告兼容规则；全部条目以当前代码与 `openspec/specs/` 为准。

## 线上协议

- 线上协议为 v32（定义在 `internal/network/protocol` 的 `ProtocolVersion`，根包 `internal/network` 别名再导出）；所有不匹配版本都会在握手阶段、进入 Play 前被稳定拒绝，不提供版本协商或降级解码；更早版本的逐版语义见 `internal/network/protocol/packet.go` 顶部注释；
- 近几版协议全部是既有 packet 尾部追加或新增消息，不改变任何既有长度上限，也不新增 `RejectReason`：
  - v26 新增 Play S→C ID 20 `PlaceBlockSucceeded(sequence)`，只回发给放置发起会话作为成功放置确认；
  - v27 新增 Play C→S ID 14 `BoneMeal`，与 `TillSoil` 同形（序号 + 朝向），目标格由权威射线决定；
  - v28 在 `PlayerInput` 尾部 `Eating` 之后追加 1 字节 `Sprinting` 疾跑意图位；
  - v29 在 `PlayerState` 尾部 `Hunger` 之后、`WorldTimeTicks` 之前追加 1 字节 `SaturationZero` 饱和度归零提示位；
  - v30 新增 Play S→C ID 22/23/24 三类夜行者消息 `HostileSpawn`/`HostileState`/`HostileDespawn`：每类为 `ServerTick` u64 加 count u8 加至多 64 条按 ID 严格升序的记录（spawn 携带 ID/维度/位置/朝向/生命，state 携带 ID/位置/速度/朝向/生命，despawn 只携带 ID），按会话视野订阅发布；
  - v31 在 `PlayerState` 尾部 `SaturationZero` 之后、`WorldTimeTicks` 之前追加 2 字节 `DayPhaseOffset` 显示相位偏移（u16，值域 `0..23999`，越界在校验与编解码处稳定拒绝）；显示相位按 `(WorldTimeTicks + DayPhaseOffset) % 24000` 计算，偏移只平移呈现相位、不回写绝对时间；
- v32 在 Play S→C registry 尾部追加 ID 25 `CombatHit(ServerTick, Damage, TargetKind)`；固定 10-byte 载荷只私发给成功攻击的玩家会话，受击者、旁观者、trusted observer 与 hostile 攻击目标均不接收；
- 客户端只声明按键意图，权威结算全在服务端；`SaturationZero` 是瞬态提示位、不入任何存档，饱和度与疲劳数值是纯服务端量、不占 wire 字段；`DayPhaseOffset` 只经世界 metadata v3 持久化，不进玩家存档。

## 存档版本

- 世界 metadata 保持 v3，在 v2 载荷末尾追加 8 字节 `DayPhaseOffset`（u64，值域 `0..23999`，越界旧值读入时归一），既有段布局一字不动；v1/v2 世界读入即迁移：世界时间保持原值、偏移取 `0`（显示相位行为不变），并在下一次正常自动保存或关服时写为 v3；只认识旧版本的程序遇到未来 metadata 必须稳定拒绝且不得覆盖原文件；
- 玩家存档保持 schema v8：在饥饿状态之后追加定长 17 字节重生点尾段（present 标志 1 字节 + 床尾格坐标 3×f32 12 字节 + 维度 u32 4 字节，无重生点时以 present=0 占满 17 字节）；受支持的 v1..v7 沿既有迁移链读取（例如 v4 一律补为满血、v6 按新玩家初值补齐三层饥饿状态、v7 一律迁移为「无重生点」，死亡重生沿用世界出生锚点语义），迁移结果在下一次正常保存时改写为当前版本；未来版本必须稳定拒绝且不得覆盖；
- 区块存档保持 schema v9：v1..v8 沿既有迁移链读取，其中 v8→v9 是恒等迁移，不为旧区块追注水，已接受的代价是新旧区块之间出现干湿边界；未来版本必须稳定拒绝且不得覆盖；
- 夜行者独立写入世界根目录的 `hostile_mobs.bin` schema v1：固定头加至多 64 条定长记录，CRC 与逐项合法性校验覆盖全文件，任何损坏、越界或非法记录都整份拒绝且视为无有效存档（缺失文件视为空集合，恢复失败不会以空集合覆盖旧文件）；该文件没有历史版本，未来版本必须稳定拒绝；
- 伙伴状态独立写入世界根目录的 `companions.ai` schema v4：v1..v3 只读迁移；active 与 inactive 身体记录合计最多 64 条；active 记录可持久化当前任务、FIFO 与近期对话摘要，名称和生效 persona 始终来自当前配置；
- 列顶高度表、天空光和静态方块光只从权威方块镜像派生，不写入区块、玩家或伙伴存档，也不进入网络 payload；程序化天空只消费权威世界时间。

## 备份与回退

- 升级前必须正常关服，等待玩家、伙伴与世界存储刷写完成并备份完整世界目录，再启动新版本程序；
- 回退时必须先停服，再恢复升级前的完整备份；不承诺把 metadata v3、schema v9 区块、schema v8 玩家档、`hostile_mobs.bin` v1、`companions.ai` v4 或新物品降级写回，不能让旧程序直接打开已升级目录后继续写入；
- 异常退出时玩家、伙伴与区块文件各自原子，但它们之间没有跨文件事务。

## benchmark 报告兼容

- benchmark producer 为 scenario v20，固定输入仍是七名远端玩家、零伙伴、不注入聊天，被测世界不注水且不含农业方块；scenario 版本变化记录的是被测进程本身的改变（HUD 固定上传布局、HUD 图集列数、权威 tick 工作量等），性能数值只在同 scenario 版本内可比；
- 旧 scenario 版本的报告仍可读取并做同版本比较；跨 scenario 比较只接受显式 `--allow-scenario-upgrade 19:20`，这是当前唯一显式迁移授权（历史的 `18:19` 随 producer 升到 scenario v20 退役，只作归档证据，不再是当前可授权迁移）；
- 跨 transport 比较要求两侧 scenario 版本与 `git_commit` 都一致，否则拒绝；
- 性能数值只记录，不改变退出状态；报告结构、来源身份、真实 overflow、数据丢失和 I/O 错误仍然硬失败。
