// Package region 承载 region 文件的格式原语：superblock/bank 编解码、扇区
// 空间分配与坐标换算，以及跨根包与 chunk 包共享的 `RegionKey`。
//
// 依赖方向上它是 storagedef 之上的格式叶子：只依赖 core 与 storagedef，
// 不感知 chunk 记录层容器（现由 internal/storage/chunk 承载），也不感知
// 根包编排。导出面只收 chunk 包与根包实际调用的符号。
package region

import "github.com/channing771/mornlea/internal/core"

// RegionKey 标识一个 region 文件：维度加 region 坐标（一 region 为 32×32
// 区块）。它同时被根包 DiskStore 的容器缓存编排与 chunk 记录层容器持有，
// 因此定义在格式原语包而非任一使用方。
type RegionKey struct {
	Dimension core.DimensionID
	X, Z      int32
}
