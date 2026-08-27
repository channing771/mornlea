# Proposal: bone-meal

## Why

`authoritative-farming` 首版提供随机 tick 自然生长（`RandomTicksPerSection=3`、`CropGrowthChancePercent=50`，成熟期望约 16 分钟），但没有任何“立刻催熟”手段，耕后等待过长。`docs/feature-backlog.md` B-03（farming 遗留 3）要求补上骨粉最小闭环，走翻地同形的“仅带朝向、目标由射线决定、栏位取权威选中格”命令路径，零新增状态字节。

## What Changes

- 新增 **1 个物品** `ItemBoneMeal`（`MaxStack 64`，`ItemIDMax` 前 append-only，`ItemStackLimit` 64）。
- 新增 **1 条客户端命令** `BoneMeal`（`Play/C→S` 下一 ID `14`，载荷 `u64 Sequence + f32 Yaw + f32 Pitch`，与 `TillSoil` 同形），`ProtocolVersion 26 → 27`。
- 新增 **1 条服务端命令种** `CommandBoneMeal`，校验序与 `executeTillSoil` 同源，成功时把命中的 `WheatStage0..6` 原子推进到下一阶段（`Stage7` 不变），并从权威选中栏位消耗恰好 1 个骨粉；拒绝路径零消耗零写入。
- 客户端“使用”键在手持骨粉且本地镜像命中作物时优先发 `BoneMeal`，否则保持既有 `TillSoil`/`PlaceBlock` 分流。

### 用户可观察结果

- 手持骨粉对未成熟小麦使用，作物立刻长高一阶段并扣 1 骨粉；多次使用可催熟至成熟。
- 对成熟小麦、非作物、空气、超出触及距离或手持非骨粉时拒绝且不扣料；成熟作物保持原阶段。

## Capabilities

### New Capabilities

- `authoritative-farming` 新增骨粉催熟行为（本 change 的 delta）。

### Modified Capabilities

- `authoritative-farming`: 追加骨粉催熟契约。
- `common-block-materials`: `ItemIDMax` +1 由骨粉追加驱动（度量同既有农业物品）。
- `network-protocol`: `ProtocolVersion 26 → 27`（新增 `BoneMeal` packet ID 14，不重排既有 ID）。

## Impact

- **代码**：`internal/core/item.go`（`ItemBoneMeal`）、`internal/network`（`message_command.go`/`packet.go`/`registry.go`/`codec_client.go`/`codec_server.go` 验证与编解码）、`internal/sim/command.go`+新建 `internal/sim/bone_meal.go` 与测试、`internal/server/session_ingress.go`、`cmd/mornlea/app_input.go`（使用键分流）。
- **兼容性**：协议 `26 → 27`（仅追加 packet，不改存档 `玩家/区块/世界/metadata`、`companions.ai`、`engine/client ABI`、`benchmark scenario`、`visual golden`）。
- **性能与并发**：单格射线 + 单格读写 + 单次背包扣除，`recordChange` 汇入本 tick `pending`，无新 goroutine 或队列。

## Non-Goals

- 不做 `1→N` 随机多阶段推进（本批固定 1 阶，后续可配化）。
- 不做骨头/骨粉来源与合成配方（获取路径由 `B-27/B-33` 等交付前以测试直给为准）。
- 不做成熟后再施肥特效、粒子或音效（呈现层不变）。
- 不做光照/生物/流体联动。
