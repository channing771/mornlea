## Why

四季与温度体系（`seasonal-time-temperature`，协议 v37）已使冬季低地与夏季高山按局部温度呈现雪形态降水，但雪落地即消失——地面没有积雪，冬季观感单薄。用户要求下雪天优化：地面产生积雪效果，并落地四项踩雪交互（视觉脚印、踩雪音效、厚雪减速、踢雪粒子）。

## What Changes

- **雪层方块族**：新增 4 档雪层方块（`snow_layer_1..4`，厚度 2/16..5/16 格，逐档渐厚），视觉走既有短方块呈现机制（`BlockTopRaw`），无碰撞（贴地装饰层），可徒手移除、无掉落。
- **积雪与消融**：雪形态降水期间（权威天气为雨/雷暴且该处局部温度 ≤ 雪点 0℃），随机 tick 对露天可积雪地表逐档加厚至上限 4 档；局部温度回升高于融点 2℃（回差防抖）逐档消融至移除。确定性、每 tick 预算有界，经既有 mutation/chunk change/持久化管线。
- **四项踩雪交互**：
  - 视觉脚印——玩家与被动牛在雪层上的落足移动把该格雪层降一档（最薄档踩碎移除），走出脚印链，新雪可重新覆盖；
  - 厚雪减速——踩在 ≥3 档雪层上行走目标速度 ×0.7（共享物理按落足方块采样，权威与预测同表自动一致）；
  - 踩雪音效——雪上移动时程序化 crunch 提示音（客户端本地呈现、限频）；
  - 踢雪粒子——疾跑/落地时脚下踢起雪尘（复用降水粒子实例流与 256 上限预算，客户端本地派生）。
- **视觉基线**：新增 `snow-cover` capture 场景（冬季、雪形态降水、预铺满档雪层的地表），场景清单 27→28；既有 27 景零重录。
- 非目标：不做雪球/雪球物品与投掷、不做铲类工具加成、不做树冠/水面/悬崖挂雪、不做雪对生物的体温伤害、不做雪光照衰减（雪层按透明装饰处理）。

## Capabilities

### New Capabilities

- `snow-cover`: 雪层方块族、积雪/消融推进与四项踩雪交互。

### Modified Capabilities

- `short-block-presentation`: 雪层四档作为 `BlockTopRaw` 短方块新消费者（2/16..5/16）。
- `visual-verification`: capture 场景清单 27→28（新增 `snow-cover` 冬季雪景）。

## Impact

- 共享：`packages/shared/core` 雪层方块 ID 与谓词（追加在 `BlockIDMax` 前）、方块属性表；`packages/shared/physics` 落足方块采样与 ≥3 档减速（权威/预测同路径，无 ABI 变更）。
- 服务端：`packages/server/sim/realm` 随机 tick 积雪/消融（复用 `RandomTicksPerSection` 预算框架与 mutation 事务）；`sim/runtime` 把当 tick 天气与季节温度快照传入环境推进；`sim/entity` 落足脚印结算（玩家与被动牛，经既有写块路径）。
- 客户端：`packages/client/assets` 雪层材质层与 `BlockTopRaw` 映射、`mesh` 快照自动跟随；`cmd/mornlea/app` 踩雪音效与踢雪粒子本地呈现；`capture` 新场景。
- 版本：协议 v37、世界 metadata v4、engine ABI v10、client ABI v18、区块 schema v9、benchmark scenario v22 全部不变（新方块 ID 仅追加，旧存档自动兼容；踢雪粒子复用既有 precip 实例流不新增帧 TLV tag）；Rust 零改动。
- 性能：随机 tick 常数预算；脚印每实体每步至多 1 格写块；减速为落足采样常数判定；粒子共享既有 256 上限。
