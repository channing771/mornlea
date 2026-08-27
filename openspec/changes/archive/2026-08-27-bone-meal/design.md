# Design: bone-meal

## Context

耕地与作物已由 `authoritative-farming` 交付：翻地走 `CommandTillSoil`（仅带朝向，目标由射线决定），作物由随机 tick 推进。B-03 要求同形追加“立即推进 N 阶段”动作，保持确定性、原子性与拒绝零消耗契约。

## Goals / Non-Goals

- 目标：`ItemBoneMeal` + `BoneMeal` 命令对 `WheatStage0..6` 原子推进 1 阶，成熟不变，消耗 1，拒绝零消耗，协议只追加 ID 14。
- 非目标：随机多阶段、配方、成熟特效、光照/流体联动。

## Decisions

- **命令形态复用 `TillSoil`**：`BoneMeal{Sequence,Yaw,Pitch}`，`Translate` 仅搬运序号与朝向，目标与栏位由 `sim` 从权威状态取得（`LookDirection` + 同一份 `blockRaycastSampler` + `InteractionReach`），与放置/翻地/开容器共享同一射线语义，避免第二套距离与“区块未就绪”实现。
- **校验顺序**（结构上保证拒绝零消耗）：
  1. `PlayerActive`
  2. 射线有效且命中
  3. 命中格所属区块就绪
  4. 命中块为 `WheatStage0..6`（`Stage7` 单独拒绝）
  5. 权威选中格为 `ItemBoneMeal` 且 `Count>=1`
  全部通过后才执行：`SetBlock(下一阶段)` → `recordChange` → `Consume(1)` + `inventoryDirty`。失败路径在任一 `return` 前不触背包与方块。
- **推进量固定 1**：确定性单步，零随机源，测试可在单 tick 内精确断言阶段+消耗；后续若要 `2..5` 随机，可在同函数内引入 `hash(seed,tick,pos)` 而不改命令形状。
- **协议升版**：`ProtocolVersion 26→27`，`clientPacketID(StatePlay,BoneMeal)=14` 置于 `TillSoil(13)` 之后，不重排。`ValidateClientPacket`/`codec` 追加分支，`Packet` ID 冻结由 `registry_test.go` 钉死。
- **客户端分流**：`app_input.go:placeBlock` 在 `TillSoil` 分支前插入骨粉分支：`hotbar.Selected==BoneMeal && InteractionTarget(作物)` 时发 `BoneMeal`，否则走既有翻地/放置。本地不预测方块，等待权威广播。

## Alternatives Considered

- **复用 `PlaceBlock` 携带 Slot**：`PlaceBlock` 带 `Slot` 且走放置表，骨粉不可放置，强行复用会让放置表的拒绝语义与催熟语义混淆，已否决。
- **服务端额外带坐标的 `UseItemOnBlock`**：客户端声明坐标可减少射线，但与翻地/开容器“目标由权威射线决定”的统一边界分歧，且增加 wire 字段，已否决。
- **随机多阶段**：增加可玩性但引入确定性来源与成熟边界测试复杂度，留待后续可配化。

## Risks / Trade-offs

- 新增协议 ID 需同步 `archcheck` 基线版本与 `openspec/config.yaml` 的版本矩阵，属机械同步。
- `ItemIDMax` +1 会让 `hotbarTextureUV` 图集宽度变化 0.115% 级别亚像素漂移，但 `hud-atlas-texel-stable-uv` 已用 1/256 对称收进消除该漂移，golden 无回归。
- 与在途 `A-01` 的 `core/item.go` 与 `E-12` 的 `codec_client.go` 为 append-only 受控重叠，已在 backlog 备注裁决。

## Migration Plan

无存档迁移：`ItemBoneMeal` 为新物品，旧存档无该物品即零；`WheatStage` 编号不变，旧作物按新规则自然可被催熟。协议升版为不兼容变更，旧客户端需更新。

## Open Questions

无。
