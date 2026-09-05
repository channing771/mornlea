## Why

被动牛已被吃草与引诱补齐生命感，但死亡仍是生硬消失：出视野与被击杀走同一条无原因移除，客户端瞬间删体；掉落小方块仍是纯色块，与材质 atlas 脱节；吃草/引诱/击杀的连续动作没有可回放的动态基线。本 change 补齐死亡过渡、纹理掉落与 GIF 动态基线，形成牛行为的第三个可验证闭环。

## What Changes

- `PassiveDespawn` record 追加 1 字节原因位（0=消失/出视野，1=死亡）；协议 v34→v35（只追加 despawn 原因位）；存档、ABI、benchmark scenario 均不变。
- 服务端发布区分死亡与消失：死亡 tick 的移除原因位置 1，其余为 0；Memory/TCP 同一契约。
- 客户端收到死亡原因后保留渲染 20 tick：红闪（颜色向红插值）+ 侧倒（roll 转 90°）后移除；相位由 despawn 的 `ServerTick` 派生，禁用墙钟；夜行者不动。
- 掉落小方块采样材质 atlas：方块类走注册表现有材质，食物走牛肉/小麦层；先审计全部可掉落物品与已有层的缺口，缺的层才在枚举末位追加（植物 31..54 不动），缺层用原创程序化像素并溯源；禁止 Mojang/官方提取物。
- 新增 GIF 动态基线：剧本场景（吃草前后/持麦靠近/击杀/牛肉掉落）按 tick 步进抓帧（禁用墙钟），标准库 `image/gif` 编码（帧预算参照录制上限纪律，建议 ≤8fps×6s=48 帧），存 `testdata/` 下 `.gif` 基线，逐帧解码沿用双阈值比对；只加新基线，旧 PNG 逐字节不动。

非目标：夜行者死亡动画、繁殖/喂食消耗、存档 schema 变更、client/engine ABI 变更、旧 PNG 基线重生成。

## Capabilities

### New Capabilities

- `passive-death-presentation`: 被动牛死亡过渡呈现（原因位消费、20 tick 红闪侧倒、确定性相位）与纹理掉落采样（atlas 层选择）的可观察行为。

### Modified Capabilities

- `passive-mob-protocol`: `PassiveDespawn` record 追加原因位，wire 上限与拒绝矩阵同步更新，协议 v34→v35。
- `persistent-item-drops`: 掉落小方块的材质采样规则（方块类/食物类的层选择与缺层追加纪律）。
- `visual-verification`: GIF 动态基线的场景、帧预算、编码与逐帧双阈值比对纪律。

## Impact

- 影响包：`packages/server/sim/entity`（死亡原因事实来源）、`packages/server/server`（发布携带原因位）、`packages/shared/network`（despawn 编解码 + 协议 v35）、`packages/client/client`（死亡保留镜像）、`packages/client/render`（红闪侧倒 + 掉落 atlas 采样）、`packages/client/assets`（缺层审计与追加）、`packages/client/cmd/mornlea/capture`（GIF 剧本与比对）、`packages/audit`（基线矩阵同步）。
- 兼容性：协议只追加；旧客户端按版本拒绝混装（既有握手语义）；存档格式不变，原因位与死亡态为瞬态，不落盘。
- 并发/性能：死亡原因为每 tick 有界集合投影；客户端死亡保留至多 32 具 × 20 tick；掉落采样为逐实例常数查表；GIF 抓帧只走离屏确定性路径，不进权威 tick 热路径。
