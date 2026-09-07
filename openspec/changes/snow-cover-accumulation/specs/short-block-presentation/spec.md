# short-block-presentation Specification

## MODIFIED Requirements

### Requirement: 非满格方块按 registry 高度呈现几何

系统 SHALL 让方块 registry 携带每方块的 4-bit 顶面高度原值 `block_top_raw`：`0` 表示满格方块（哨兵），`1..=14` 表示该方块所有可见面的上缘按 `(block_top_raw+1)/16` 下沉，`15` MUST 被输入校验拒绝。携带非零高度的方块的顶面与四个侧面的上缘 MUST 呈现在该高度处；其下缘与未下沉方向 MUST 保持整格边界。既有消费者：干耕地与湿耕地填 `14`（呈现高度 15/16，与权威碰撞体一致）；雪层四档填 `1..4`（呈现高度 2/16..5/16，装饰层、无碰撞，视觉高度即档位语义）。此类方块 MUST NOT 参与贪心合并，quad 实例 MUST 保持 `u64` / 8 字节。满格方块、流体与植物的既有呈现 MUST 逐位不变。

#### Scenario: 耕地几何与碰撞体一致

- **GIVEN** 一格干耕地或湿耕地、上方为空气
- **WHEN** 该区段被网格化并渲染
- **THEN** 顶面 quad 与四个侧面 quad 的上缘 MUST 呈现在 y = 15/16 处
- **AND** 玩家站立其上的脚部位置 MUST 与可见顶面齐平（碰撞体本就是 15/16）

#### Scenario: 雪层档位几何递增

- **GIVEN** 并排的 1..4 档雪层、上方为空气
- **WHEN** 该区段被网格化并渲染
- **THEN** 四格顶面 MUST 分别呈现在 y = 2/16、3/16、4/16、5/16 处，逐档可辨

#### Scenario: 下沉方块不贪心合并

- **GIVEN** 相邻两格同材质耕地
- **WHEN** 网格化完成
- **THEN** 每格 MUST 独立出 quad（1×1），MUST NOT 合并为跨格 quad

#### Scenario: 非法高度原值被原子拒绝

- **WHEN** registry 条目的 `block_top_raw` 字节为 `15`
- **WHEN** 输入解析执行
- **THEN** native 调用 MUST 返回可判定的校验失败且不产出部分网格

#### Scenario: 满格与流体呈现逐位不变

- **GIVEN** 不含任何非零 `block_top_raw` 方块的 neighborhood
- **WHEN** 以 v7 输入网格化
- **THEN** 产出的 packed quads MUST 与 v6 在同一输入下逐位一致
