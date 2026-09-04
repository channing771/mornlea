# Design: passive-graze-lure

## Context

见 `proposal.md` Why。当前真相：牛权威集为 `passiveSet`（定容升序、ID 哈希派生），推进阶段为生成→移动→死亡结算（`advancePassives`）；手持判定有 `farming.go` 查 `Hotbar.Slots[Selected]` 先例；世界单格写入有死亡掉落 `touchChunk` + mutation 路径；`PassiveState` record 为固定字节（`PassiveStateWireBytes` 字节推导测试锁死）；客户端牛头经 `Avatar.Pitch` 通道旋转。详见 specs 两能力。

## Goals / Non-Goals

- Goals：吃草事件（低头 20 tick + 草→泥土 + 中断语义）与小麦引诱（8 格跟随/2.5 格止步/优先级）可用；放牧位 state 追加 1 字节（v34）；capture 加 `passive-graze` 一景。
- Non-Goals：不改存档 schema（瞬态不落盘）；不动夜行者/伙伴；不做繁殖/消耗；死亡动画与 GIF 基线归后续 change。

## Decisions

### D1：吃草时序用 splitmix64 无状态抽选，不加持久字段

- 选择：每 tick `splitmix64(worldSeed, tick, id)` 命中（如 1/600）才开事件；事件剩余 tick 同样可由（起始 tick + 20）派生，但中断条件（受击/移动/换块）需记“事件中”位——该位放运行时 `passiveState` 内存字段，不进 `StoredPassiveMob`（存储校验矩阵不动）。
- 理由：零存档变更；重启后事件自然消失符合“瞬态”spec。
- 否决：存档加冷却字段——为装饰性行为升 `passive_mobs` schema v2 不值。

### D2：放牧位只进 State，不进 Spawn/Despawn

- 选择：`PassiveStateRecord` 尾部 +1 字节 `grazing` u8；spawn/despawn 不动。
- 理由：出生即低头无语义；despawn 原因位是后续 change 的 v35，两 change 各 +1 版、串行无冲突。
- 否决：复用 yaw 符号位等位偷渡——破坏固定字节推导纪律。

### D3：吃草写块走死亡掉落同形的 mutation 路径

- 选择：事件结算时经当前 tick mutation 写触发格（以事件触发时记录的 `BlockPos` 为准，结算时重验仍为草且 chunk Ready，否则丢弃），`touchChunk` 记 revision。
- 理由：与掉落写入同一提交点，无第二写入通道；失败即丢弃不重试（漏一次只是晚长一次，不是不动点问题）。
- 否决：复用耕地湿度队列——那是为扇出有界造的 тяжелая 机制，单格写入不需要。

### D4：引诱复用最近玩家扫描，不新增订阅或命令

- 选择：在被动移动输入阶段内联判定（最近同维 active 玩家 ≤8 格 + 选中格 `== ItemWheat`），转向覆盖 wander；逃跑分支先行 return。
- 理由：hostile 追逐目标选择同形；手持事实服务端已有，零协议变更。
- 否决：新增 `TemptUpdate` 发布——客户端不需要知道“为什么走”，位置 state 已表达一切。

### D5：低头映射到既有 Pitch 头部通道

- 选择：镜像 `grazing==1` →牛头 Pitch 下压固定角；`0` →恢复。角度常量由呈现测试锁定。
- 理由：Avatar 96B 布局、ABI、容量全不动；夜行者头部俯仰同通道。
- 否决：新增实例字段——为装饰位姿改 ABI 不值。

## Risks / Trade-offs

- [抽选率体感] → 1/600 是初值（约 30s/牛期望），capture 场景用固定种子夹具钉死可复现；体感 tuning 在任务评审凭试玩裁决，只改常量不改结构。
- [吃草与寻路冲突] → 事件期间牛静止（MC 同形），移动输入即中断，无需与移动仲裁。
- [v34/v35 串行] → 本 change 合入前 B change 不得 rebase 写 wire；ledger 记录版本归属。
- [golden 膨胀] → 只加 `passive-graze` 一景；旧景逐字节门禁先行。

## Migration Plan

- 部署：协议 v33→v34 只追加；旧客户端握手拒绝（既有语义）；存档不动，老世界直接兼容。
- 回滚：整分支 revert；残留放牧位 state 被旧版按版本拒绝，无脏数据。

## Open Questions

- 无。抽选分母与引诱半径在任务内按常量锁定，评审可调。
