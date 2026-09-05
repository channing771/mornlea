## Why

`cream-game-experience` 落地的同格掉落散列按密度缩小（1 堆全尺寸、4 堆约 0.75、16 堆约 0.38、32 堆约 0.25），堆越多单个物品越小，用户实际游玩中难以辨认掉的是什么。用户明确要求：不缩小，允许小小的重叠，能看清是什么就可以。

## What Changes

- 同格散落缩放恒为 1：1/4/16/32 堆全部全尺寸呈现。
- 保留既有布局骨架：`(Dimension, Block)` 分组、完整 `DropID` 排序、每堆唯一 XZ cell、非零 ID 哈希抖动、800 固定 scratch、死亡隐藏堆占位、支撑锚定与 0.02 离地。
- 放宽两条旧约束：高密度档允许保守包围体轻微交叠；XZ 允许因全尺寸在密集档边缘探出（4 堆及以下仍完全不出格，16/32 堆上界 0.14 格，见 delta 规格推导）。
- 受影响的散落运动基线（`drop-scatter.gif`、`drop-density.gif`）按既有机制重出并目检；`avatar-walk.gif`、`break-burst.gif` 与人物 world 基线不动。

## Non-Goals

- 不改变权威拾取、合并、数量、网络、存档与固定容量；不新增依赖与素材；不碰前端、HUD、人物与步态。

## Impact

仅 `packages/client/render` 的散列几何及其测试、对应 delta spec/design 与运动基线。`cream-game-experience` 的缩放钉住测试与两条 MUST 需要同步放宽（见本 change delta）。
