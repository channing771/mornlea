# Design: multidimension-world

## Context

动机见 `proposal.md`。现状：`DimensionID`、`ChunkKey{Dimension,Pos}`、磁盘 `dimensions/{dim}/regions/`、客户端镜像多维 map、`realm.State.dimensions` 多维记账均已存在；生产侧写死单维——全网 `Validate` 拒非零维度（`message_command/player/container/companion/hostile/passive` 与 `codec_server validateServerWirePacket`）、`runtime.NewEngine` 只 `NewState(Overworld)`、`worldgen.GenerateChunk` 与 `server.Generator` 接口无维度参数、metadata v4 只有单出生锚点。约束：tick 串行、`mornlea_engine` 零改动、不新增 packet ID、不碰 UI/渲染。

## Goals / Non-Goals

- 达成：双维启动与串行隔离结算、原子传送、按维存档与迁移、v39 协议放宽。
- 设计层排除：并行 tick（determinism 风险）、专用传送 packet（ID 面膨胀）、第三维（测试量翻倍）、`HeightAt` 保持单维（会导致新维出生读错高度图，故必须一并维度化，见 Decision 3）。

## Decisions

### 1. 传送触发复用 `ChatCommand`，不新增 packet

- Why：新增 C→S 需要 registry ID、双向 codec、Validate 矩阵与 golden 全链改动；聊天命令解析已在服务端 tick 内，结果同样经权威校验后结算，可观察行为一致。
- Alternative：专用 `TeleportRequest` packet——留待传送需要带坐标参数时再做。

### 2. 世界生成按维派生种子，Rust 零改动

- `dimSeed = baseSeed`（主世界）/ `baseSeed ^ 0x9E3779B97F4A7C15`（Depths）；Go 侧持有两个预编码 `MGW1` header 实例，复用既有 `nativeabi` 调用。主世界输出逐字节不变，既有世界不受影响。
- Alternative：Rust 侧加维度分支——破坏“Go 侧门控材料表、engine 无分支”的既有纪律，否决。

### 3. `HeightAt`/`TerrainProbe` 一并维度化

- 出生扫描依赖高度图；只改 `GenerateChunk` 会让新维出生读主世界高度。`TerrainProbe` 携带维度并透传，旧调用方默认主世界。
- Alternative：传送后掉落重定位——引入额外失败态，否决。

### 4. metadata v5 尾部追加，读 v1..v5 写只写 v5

- 沿用仓库“旧版只读迁移、写出只用当前格式”纪律；v4 缺尾部时 `Depths` 锚点默认主世界锚点。chunk 信封已有维度字段，只收紧值域为 0/1。
- Alternative：独立维度配置文件——多一份原子替换与锁协议，否决。

### 5. 协议只放宽、不加字段

- 玩家/区块类消息 wire 上已有 `Dimension` 字段，v39 仅放宽值域；伙伴系保持拒非零（本期不跨维）。v38 握手拒绝且不触碰任何状态。

## Risks / Trade-offs

- [Risk] 双维 tick 使单 tick CPU 上限翻倍 → Mitigation：串行有界工作不变，空维仅记账开销；`perfcheck` 记录数值。
- [Risk] `Generator` 接口改签名波及约 20 个测试 helper → Mitigation：实现任务以编译器 sweeping 为准，全量构建验证。
- [Risk] 版本号（协议 v39、metadata v5）与其他在途行冲突 → Mitigation：按“并行与冲突规则”，版本号基于实现时 `main` 重排；本 change 在归档前不预占绝对号之外的任何资源。
- [Risk] 玩家在新维下线后旧客户端读取存档 → Mitigation：维度为已有存档字段，旧程序按既有未知维度路径拒绝，不静默迁移。

## Migration Plan

1. 部署：随版本发布，旧档首次启动只读迁移，首次保存写出 v5。
2. 回滚：v5 写出后旧版本拒绝加载（未来版本路径）；回滚需从升级前备份恢复——发布说明必须提示备份世界目录。

## Open Questions

无。传送坐骑/船只等实体均不在本期范围内，无需决策。
