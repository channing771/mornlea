# Proposal: farmland-mesh-top-sink

## Why

耕地的碰撞体自 authoritative-farming 起就是 15/16 高（`internal/physics/types.go` 的 `farmlandCollisionHeight`），但 mesher 仍把它当满格立方体渲染——玩家站在耕地上时脚陷进方块一格纹素，选取轮廓与可见几何也和碰撞边界错位。这是 farming 遗留 13 的呈现半边。

## What Changes

- 方块 registry 条目追加 1 字节 `block_top_raw`（0 = 满格哨兵，1..=14 为 4-bit 顶面高度原值），条目从 18 字节扩到 19 字节；engine ABI 随输入布局 **v6 → v7**
- mesher 对非零 `block_top_raw` 的方块走常量角高度路径：全部可见面的上缘按 `(h_raw+1)/16` 下沉（耕地填 14 → 15/16，恰等于碰撞高度），该类方块不参与贪心合并，quad 仍是 8 字节
- Go 侧 `internal/assets` 填充干/湿耕地的高度字节，`internal/mesh/native_input.go` 编码并做域校验；Rust 侧 `input.rs` 解析、域校验并与 fluid 互斥
- `materials-showcase` capture 夹具追加干耕地与湿耕地两列，使本行为进入视觉回归网；对应 golden 单景再生，其余 18 景必须逐字节不变

## Capabilities

### New Capabilities

- `short-block-presentation`: registry 驱动的非满格方块顶面呈现——高度数据通道、mesher 几何规则、与流体的互斥及 ABI 升版约束；耕地（干/湿）为第一个消费者

### Modified Capabilities

- `visual-verification`: `materials-showcase` 固定夹具的必覆盖集合从「14 种新材料、八格连续草地、相邻玻璃、相邻树叶、原木年轮」扩展为同时覆盖干耕地与湿耕地两列；场景清单顺序不变

## 兼容性与影响面

- 无 wire / 存档 schema / benchmark scenario / client ABI 变更；协议、区块、玩家、伙伴存档格式全部不动
- engine ABI v7：`mornlea_engine_abi_version` 返回值变化，旧 dylib 与新二进制混装被既有版本握手拒绝（二者本就是同一不可跨版本混装的 release unit）；Go 侧经 cgo 读 header 常量自动同源
- golden 影响仅 `materials-showcase.png` 一景（新增夹具所致），按仓库「基线更新必须显式」要求逐图复核后再生
- 性能：registry 每 entry 多 1 字节（48 条上限 → 至多 +48 bytes 输入），mesher 对耕地走 1×1 出面（同流体/植物的既定模式），无热路径回退
