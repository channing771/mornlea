## Why

单维度世界已触及玩法天花板：矿洞探险、异维度挑战等后续玩法（见 `docs/feature-backlog.md` B-20）都依赖多维度地基。而地基已埋一半（`ChunkKey` 含维度、磁盘 `dimensions/{dim}/regions/`、镜像多维 map、`realm.State` 多维记账），现在是落成第二维度的最便宜时机。

## What Changes

- 启动第二个权威维度 `Depths`（`DimensionID=1`），与主世界同 tick 串行隔离结算。
- 玩家经聊天命令 `/warp depths|overworld` 在双维间传送：同一 tick 原子搬运，旧维存档、新维出生扫描，下发 `PlayerState{Dimension,Reset:true}`。
- `Depths` 地形由 `baseSeed ^ salt` 派生，`mornlea_engine` 零改动；同 `(x,z)` 双维内容不同。
- 协议 v38→v39：玩家/区块类消息放行维度 `0/1`，伙伴/敌怪/被动生物类继续只允许主世界；不新增 packet ID。
- 存档 metadata v4→v5（尾部追加维度表，旧档默认迁移）；chunk schema 保持 v9，玩家 schema 保持 v8。
- 非目标：维度选择 UI、新 HUD、新生物群系美术、第三维度、跨维度伙伴/敌怪跟随、跨维度红石。

## Capabilities

### New Capabilities

- `multidimension-world`: 第二权威维度的启动、tick 隔离、传送事务与维度隔离语义。

### Modified Capabilities

- `chunk-persistence`: `Depths` 区块纳入持久化（`dimensions/1/regions/`），维度值域收紧为 `0/1`。
- `local-data-migration`: 世界 metadata v4→v5 只读迁移与只写 v5。
- `authoritative-spawn-support`: 按维独立出生扫描；床重生要求同维床，否则回落该维出生锚点。
- `player-persistence`: 玩家位置/重生点维度在传送与重启后保持连续（`Dimension`/`RespawnDimension` 已有字段首次取非零值）。

## Impact

- 受影响包：`packages/shared/core`、`packages/shared/worldgen`、`packages/shared/network/protocol` + `codec`、`packages/server/sim/realm`、`packages/server/sim/runtime`、`packages/server/sim/entity`、`packages/server/storage`（根 + `chunk`）、`packages/server/server`（`generator.go`、`warp.go` 新文件）。
- 协议升版 v38→v39，旧客户端握手拒绝；metadata 升版 v4→v5，旧档只读迁移；chunk v9、玩家 v8、engine ABI v10、client ABI v18 不变。
- 并发：tick 仍串行（双维顺序结算），持久化 worker 拓扑不变；热路径仍有界、无阻塞 I/O。
- 性能：每 tick 多一维空转开销（无玩家时仅记账），`perfcheck` 只记录不设门禁。
