# 多维度世界（引擎基建）设计

日期：2026-09-09 ｜ 状态：brainstorming 设计文档（未进入 OpenSpec change，不含实现） ｜ 约束：不碰 UI/渲染

## 1. 背景与目标

Mornlea 服务端是唯一权威，当前生产只跑单个维度（`Overworld=0`）。但多维地基已埋一半：`DimensionID`、`ChunkKey{Dimension, Pos}`、磁盘 `dimensions/{dim}/regions/`、`Metadata.SpawnDimension`、客户端镜像多维 map、`sim/realm.State.dimensions` 多维记账都已存在；生产侧写死单维（全网 `Validate` 拒非 0 维、`Engine` 只 `NewState(Overworld)`、`worldgen` 无维度参数）。

本设计目标：跑起第 2 个权威维度（暂名 `Depths`，`DimensionID=1`），与主世界同 tick 隔离推进，可传送往返。不做 UI/渲染，不加新玩法美术。

非目标：维度选择界面、新 HUD、第三维度、新生物群系美术、跨维度伙伴/敌怪跟随、跨维度红石。传送触发先用聊天命令占位。伙伴/敌怪/被动生物本期不跨维度（传送时 despawn，返回时 respawn）。

## 2. 架构总览

`sim/realm.State` 已有多维 map，本期只做最小闭环：`Engine` 启动时 `EnsureDimension(1)`，tick 内 `for dim in [0,1]` 串行结算，持久化复用现有跨维候选收集（按 `chunkKeyLess` 排序），协议复用已有 `Dimension` 字段（只放宽校验 + 升版）。

受影响面：`packages/server/sim/realm`、`packages/server/sim/runtime`、`packages/server/sim/entity`（出生/重生）、`packages/shared/worldgen`、`packages/shared/network/protocol` + `codec`、`packages/server/storage`（metadata v4→v5）、`packages/server/server`（传送意图解析）。`mornlea_engine` 零改动，`packages/client` 仅复用已有镜像 map。

被否决的替代方案：

- 并行 tick 双维：引入 `State.dimensions` 竞争与 determinism 风险，收益不抵复杂度，否决。
- 新增专用传送 packet：需占新 ID 并改 registry/codec 全链，首版用 `ChatCommand` 占位即可，否决。
- 第三维一步到位：迁移与测试量翻倍，留后续 change，否决。

## 3. 数据流

传送事务（服务端权威原子事务，复用现有消息）：

1. 客户端发 `ChatCommand "/warp depths|overworld"`，服务端解析为传送意图。
2. 同一 tick 内：冻结该玩家输入 1 tick，保存当前维位置；`realm.State` 跨维搬运（旧维 `records` 删、新维插，玩家 `Dimension` 翻转）；新维出生扫描（复用 `sim/entity/spawn*`，按维独立高度图）。
3. 下发 `PlayerState{Dimension, Reset:true}` + `ForgetChunks{Dimension:旧维}` + `ChunkSnapshot{Dimension:新维}` 洪流；旧维伙伴/敌怪发 `Despawn`。
4. 失败回退：新维区块未就绪、死亡/`PendingSpawn` 中、维度值非法 → `CommandRejected`，玩家留在旧维不动。

存档与迁移：

- 磁盘路径不动（已是 `dimensions/{dim}/regions/r.x.z.region`）；chunk 信封不动（已有 `dimension` 字段，只放宽 codec 允许 `0/1`）。
- `world.meta` v5 = v4 41B 尾部追加 `dimCount + perDim{spawnX/Z, seedSalt}`；v4 读入时 `Depths` 默认主世界锚点 + 固定 salt，写出只写 v5。

世界生成（不改 Rust）：`Generator.GenerateChunk(pos)` 扩展为 `GenerateChunk(dim,pos)`，`dimSeed = baseSeed ^ salt(dim)`，Go 侧持有两个 `MGW1` header 实例并复用 `nativeabi` 调用。

协议（v38→v39，纯放宽无新 ID）：放行 `Dimension 0/1` 的消息为 `ChunkSnapshot/BlockChanges/ForgetChunks/RequestChunkResync/PlayerState/RemotePlayer*`；`Companion/Hostile/Passive` 相关继续只允许 `Overworld`。同步 `packet.go` 常量注释、`TestProtocolVersionPinned`、codec golden 与 `packages/audit` 基线。

## 4. 错误处理与并发边界

错误处理：`Dimension >1` 在 codec + `Validate` 直接拒；传送时机不对或新维未就绪则拒绝且不跳变；`world.meta` 损坏/未来版本走现有 `ErrCorrupt/ErrFutureVersion`，拒绝加载不清空；传送中途保存失败回滚旧维，不出现双维分身。既有长度/容量/overflow 上界原样保留。

并发：tick 仍串行双维结算，不引入并行；tick 内只做内存搬运与不可变快照下发；区块生成与落盘继续走有界队列与 `persistence` worker，tick 只做非阻塞 `Observe/Drain`；跨 goroutine 消息发送后视为不可变。

## 5. 测试策略

无 UI/渲染，无 capture golden。单测：双维记账隔离、同 `(x,z)` 双维内容不同、`metadata` v4→v5 round-trip、协议 `0/1` 放行/`2` 拒绝、传送原子性。集成：Memory/TCP 同路径 warp 往返（背包/血量/重生点不变），重启后双维 reload。门禁（真开工时）：`gofmt`、受影响包 `-race`、`packages/audit`、OpenSpec `validate --strict`。

## 6. 后续

本设计只是 brainstorming 产物。真开工必须按仓库流程新建 OpenSpec change（提案/规格/设计/任务），经评审后再按 `subagent-driven-development` 执行。
