# 权威近战夜行者（第一夜批次四号功能）

## Why

当前 `main` 只有玩家与伙伴两类主体，没有任何服务端权威敌怪框架；第一夜生存批次需要一条可验证的威胁闭环：夜间会确定性出现、追击、近战攻击、白昼灼烧、死亡掉落、远离消失且跨重启恢复的「夜行者」。本 change 交付该闭环作为批次四号功能，为后续敌怪能力（B 组候选）提供实体与持久化先例。

## What Changes

- **服务端权威夜行者身体**：每只有独立稳定非零 ID（确定性 hash 派生）、20 生命、位置/速度（复用既有 `physics.State`）、攻击/受击/灼烧冷却与目标状态；`sim` 持有按 ID 严格排序、容量 64 的切片（无 map/无 ECS）；移动与玩家/伙伴相同走 per-actor `physics.Step`（Rust 积分），tick 内顺序为玩家 → 伙伴 → 夜行者（ID 升序）。
- **确定性夜间生成**：仅显示相位 `13000..23000`（overflow-safe `core.DisplayDayPhase`，本分支 offset 恒为 0）；从已排序 active sessions 以 `WorldTimeTicks % n` 选锚点玩家，`splitmix64` 派生候选坐标与 hash；候选需水平距锚点 24..48、局部 block light ≤7（预分配 `29³` 16-bucket BFS，半径 14，规则来自新的单一发光表）、双格空气、下方 solid、非流体、完整 loaded；每 tick 至多验证 1 个候选；全服 ≤64、每玩家 48 格内 ≤8。
- **有界追逐**：`server` 每 tick 至多为到期夜行者构造 2 份不可变 A* 快照（33×9×33、4096 nodes，复用 `internal/companion` 的 `NewPathGrid`/`FindPath`），两槽 worker（channel cap 2，generation/latest-wins 回收），每个 waypoint 前重验 chunk revisions（既有 `PathRevision` 语义）；到 1.8 格内停止移动并冻结一次攻击意图；过期/失败结果丢弃并在下一 tick 重规划。
- **近战攻击**：3 点伤害、20 tick 冷却、水平距离 ≤1.8 且同维；同 tick 先冻结全部意图再统一结算，命中经既有 `applyDamage` 入口；呼吸不扣任何玩家物品。本分支以专属 damage test seam 验证伤害与冷却；剑/夜行者统一 combat settlement 由 A-06 接通并删除 seam（批次设计任务三、任务四）。
- **白昼灼烧与消失**：显示相位为白昼且露天（上方依次无实体遮挡方块）时每 20 tick 扣 1 生命，遮顶或夜间重置计时；距全部 active 玩家 >64 格累计 600 active tick 后 despawn（回到范围内清零）。
- **死亡掉落**：健康归零同 tick 移除，经既有 `PrepareDropBatch` 在死亡 chunk 环形尝试（按已排序 Ready chunk）放 1 个腐肉；全满时确定性省略掉落但仍完成死亡。
- **腐肉食物**：新增 `ItemRottenFlesh`（stack 64）与食物表条目（饥饿 4、饱和 0），复用既有 `advanceEating` 状态机；本批不引入中毒或任何状态效果系统。
- **持久化**：新增独立 `hostile_mobs.bin`（32-byte 头：magic `MHST`、envelope v1、schema v1、revision u64、count u32、payload u32、CRC-32C，最多 64 条 72-byte 记录）；记录按 ID 严格升序；未来版本/损坏/非法状态/越界在起点稳定拒绝且不得覆盖旧文件；Memory 与 Disk 同构实现；异步保存 worker 复用 `companion` 持久化状态机形状（jobs/completions 容量 1、revision、dirty/inFlight/retry、autosave tick、`Flush`/`Close`，关服屏障失败返回错误）；路径与 worker generation 不落盘。
- **协议（追加消息，版本号不变）**：S→C 新增 `HostileSpawn`/`HostileState`/`HostileDespawn` 三类消息（`ServerTick` u64 + count u8 + ≤64 条 record、record 按 ID 严格升序；spawn 携带 ID/dimension/position/yaw/health，state 携带 ID/position/velocity/yaw/health，despawn 携带 ID）；按会话已订阅 chunk 订阅发布（进入视野发 spawn、持续发 state、离开/死亡发 despawn）；协议版本号本分支保持 v26，v27 由 A-07 集成任务统一升；消息编号终值由 A-06 按固定合流顺序锁定。
- **客户端呈现**：latest-wins 镜像（spawn 建立、state 只接受更新 tick、despawn 删除；未知 state 请求下一 spawn 不隐式造实体）；移动插值复用远端玩家/伙伴的既有时间边界；avatar pass 容量 11→75 bodies（66→450 个 80-byte instance），新增 `EntityHostile` kind（原创暗青/灰紫调色、不同头身比例、仍恰好 6 cuboids）；**永远不**为夜行者生成 nametag。
- **client ABI v8→v9**：本分支在 `engine/crates/mornlea_client` 与客户端边界同步升版（ABI 常量、容量与尺寸对齐、早期拒绝旧动态库），并按 A-02-q2 裁决先例对 `AGENTS.md`/`CLAUDE.md` 做**最小同步**：只改 client ABI 版本那一处、两份逐字节相同；其余基线内容、协议 v27、`hostile_mobs` v1 版本基线、golden 与 benchmark scenario 仍归 A-07。
- **单一发光/透明度表**：`internal/core` 新增 `BlockEmission`/`BlockLightAttenuation`/`BlockOpaque` 单一表（把现有 `internal/assets`/`internal/mesh` 的发光、衰减与不透明谓词逻辑改为委托 core；若 A-02 契约（`core.BlockEmission`）先行落地，则直接消费其表、本分支不重复创建，`BlockOpaque` 同判据）。
- **视觉场景构造**：`cmd/mornlea` 追加 `hostile-mob` capture 场景构造（固定夜间火把边缘 8 只夜行者，其中一只受击、一只追逐；无 nametag 断言），插入 `ai-companion` 与 `water-surface-slope` 之间；**不写 golden PNG**（golden 归一 A-07）。

## Capabilities

### New Capabilities

- `authoritative-hostile-nightwalker`：夜行者行动闭环（确定性生成 → 有界追逐 → 近战攻击/受击 → 灼烧/消失 → 死亡掉落与上限）。
- `hostile-mob-persistence`：`hostile_mobs.bin` schema v1 的布局、校验错误矩阵、Memory/Disk 存储契约、重启恢复与备份语义。
- `hostile-mob-protocol`：三类 S→C 消息的值域/排序/长度边界、每会话订阅发布与客户端镜像/插值语义。

### Modified Capabilities

- `authoritative-hunger`：食物表追加腐肉（饥饿 4、饱和 0、stack 64），进食状态机复用，无状态效果。
- `authoritative-health`：新增敌怪近战伤害来源（3 点、1.8 格、20 tick 冷却、同 tick 意图冻结统一结算、既有 `applyDamage` 入口）。
- `rust-client-render-entities`：avatar 容量 11→75 bodies/450 instances、`EntityHostile` kind、名称标签永不包含夜行者；client ABI v9。
- `visual-verification`：新增 `hostile-mob` 场景（位置：`ai-companion` 与 `water-surface-slope` 之间；8 只夜行者、受击/追逐呈现、无 nametag、实体数与健康断言），本分支只追加场景构造与像素不变量测试，不写 golden。

## Impact

- **受影响的包**：`internal/core`（`DisplayDayPhase`、发光/衰减单一表、`ItemRottenFlesh` 与食物表、`HostileMobID`）、`internal/sim`（hostile 身体/spawn/暗度/生命周期/掉落、`engine_step.go` 新阶段）、`internal/server`（`hostile_manager`/`hostile_snapshot`/`hostile_path_worker`/`hostile_publication`/`hostile_persistence`、`host.go`/`server.go`/`shutdown.go` 装配）、`internal/storage`（hostile codec/store、`world_files.go`/`backup.go`）、`internal/network`（三类消息与 codec/registry）、`internal/client`（镜像/插值/frame 组装）、`internal/render`（`avatar.go` 容量与 EntityKind）、`cmd/mornlea`（presentation 接线、`hostile-mob` 场景构造）、`engine/crates/mornlea_client`（`entity.rs`/`ffi.rs`/`lib.rs` 容量与 ABI v9）。
- **存档**：新增独立 `hostile_mobs.bin`（v1）；玩家 schema v7、区块 schema v9、metadata v2、`companions.ai` v4 **完全不动**；`hostile_mobs.bin` 缺失视为空集合；未来版本不兼容行拒载不覆盖。
- **协议**：追加三类 S→C 消息；协议版本号本分支不变（v26 → v27 由 A-07）。老客户端握手拒绝语义在 v27 由集成任务维护。
- **兼容性**：无旧存档迁移；重启不再是清怪手段（正常路线恢复全部夜行者）。
- **并发与性能**：权威 tick 只做快照（每 tick ≤2）、绝不等待 A* 结果；保存异步不持锁不阻 tick；每 tick ≤1 生成候选验证；局部光 BFS 复用预分配 `29³` scratch，不建世界级光照缓存；全部上限（64/8/2/1/4096/72/75/450/4640 bytes）由边界测试锁定；写入路径统一走既有原子保存纪律（临时文件 + fsync + rename + 目录 fsync、0600）。
- **依赖方向（archcheck）**：`sim` 不新增任何依赖（hostile 逻辑全部在 `sim` 内部，调用 `core` 新函数）；`server` 复用既有到 `companion`/`sim`/`storage` 边；`storage` 不新增依赖。发光表从 `assets`/`mesh` 迁入 `core` 后二者改为委托——依赖方向不变（`assets`→`mesh` 边保持）。
- **批次边界**：统一 combat settlement、剑×夜行者候选合并、配方/编号终值、协议与 ABI/scenario 版本基线、golden、`AGENTS.md`/`CLAUDE.md`/`progress.md` 全文基线由 A-06/A-07 集成任务独占；本分支攻击走既有 `applyDamage` 通道 + 专属 seam，不触碰 `internal/network/message_inventory.go`（A-01 区）与 `codec_client.go`（E-12 区）。

## 延期与放弃

- 不做刷怪笼、难度档、装备、门、群体战术、远程攻击与投射物协议、声音感知（对应 B 组后续候选行）。
- 不建设通用 ECS、AI 框架或 mob registry；出现第二类行为显著不同的 mob 后再评估共享抽象（B-26 联动）。
- 腐肉无中毒/状态效果；有第二个持续状态消费者时再立柱有界状态效果系统（B-25）。
- 夜行者不游泳、不避水寻路、不搭桥、不投掷；无限世界寻路等伙伴 C 组能力不涉及。
- 统一战斗契约（同维最近命中、等距按 ID、击退与冷却）由 A-03 交付、A-06 接通；本分支只锁夜行者自身伤害入口与冷却。
- 每 tick 1 候选、2 快照、64 上限、13/256 生成 hash 阈值等数值在集成前不视为稳定契约（分支内按本 change 固定，A-06 锁定终值）。
- 本批不做床跨区块原子事务（B-21），与夜行者无关。
