## Why

被动牛已交付但行为单薄：不会吃草、不会被小麦引诱，与 MC 对照缺少可验证的生命感。本 change 补齐吃草（含低头动作）与小麦引诱跟随，形成第二个可玩的牛行为闭环。

## What Changes

- 牛周期性吃草：命中脚下草方块时低头 20 tick，随后该草方块变为泥土；受击/移动打断则不写块。
- 手持小麦的玩家可引诱牛跟随（8 格内接近、2.5 格止步），逃跑优先于引诱；不消耗小麦、不繁殖。
- `PassiveState` record 追加 1 字节放牧标志，客户端据此下压牛头（复用既有头部俯仰通道）。
- 协议 v33→v34（只追加 state 放牧位）；存档、ABI、benchmark scenario 均不变；放牧/引诱态为瞬态，不落盘。
- capture 新增 `passive-graze` 场景一景。

非目标：繁殖/喂食消耗、夜行者行为变更、死亡动画/纹理掉落/GIF 基线（另起 change 承接）。

## Capabilities

### New Capabilities

- `passive-graze-lure`: 牛吃草（低头事件、草→泥土单格写入、中断语义）与小麦引诱跟随（范围、止步、优先级）。

### Modified Capabilities

- `passive-mob-protocol`: `PassiveState` record 追加放牧标志位，wire 上限与拒绝矩阵同步更新。

## Impact

- 影响包：`packages/server/sim/entity`（放牧/引诱推进）、`packages/shared/network`（state 编解码 + 协议 v34）、`packages/client/client`（放牧位镜像）、`packages/client/render`（牛头俯仰映射）、`packages/client/cmd/mornlea/capture`（新场景）、`packages/audit`（基线矩阵同步）。
- 兼容性：协议只追加；旧客户端按版本拒绝混装（既有握手语义）；存档格式不变，老世界缺失即无放牧态。
- 并发/性能：每牛每 tick 有界判定（哈希抽选 + 最近玩家扫描复用既有模式）；世界写入每事件 ≤1 格且经既有 mutation 路径。
