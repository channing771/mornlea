package worldgen

// 本文件是运行时树形几何的 ABI 桥:把「世界种子 + 树苗根坐标」编码成
// `MTB1` 请求交给 engine,再把返回记录解码为 `TreeBlock`。树形几何本身
// 全部在 Rust `mornlea_engine` 内计算(engine ABI `mornlea_tree_blocks`),
// 本包不保留任何 Go 侧几何实现或 fallback——与世界生成一样,生产数值只有
// 一个真源。

import (
	"encoding/binary"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/nativeabi"
)

// `MTB1` ABI 编码常量,必须与 engine `worldgen.rs` 的布局逐字一致:
// magic(4) + layout u32(4,必须 1) + 世界种子 i64(8) + 根坐标 x/y/z i32(12)。
const (
	treeBlocksMagic  = "MTB1"
	treeBlocksLayout = 1
)

// TreeBlock 是运行时树形几何的一条记录:相对树苗根坐标的方块偏移与方块编号。
//
// 偏移全零的记录是树干底(根格自身),恒为第一条;`DY` 恒不小于 0——普通
// 橡树只向根以上生长。
type TreeBlock struct {
	DX, DY, DZ int8
	Block      core.BlockID
}

// TreeBlocks 返回由世界种子与树苗根坐标确定性派生的普通橡树几何。
//
// 几何在世界生成的 8×8 候选格网格之外由独立冻结 salt 派生,因此同一输入
// 恒返回逐条一致的记录,且与世界生成结果无关(既有 worldgen golden 字节
// 不因本入口改变)。返回切片按 engine 的记录顺序(相对偏移的 dy→dz→dx
// 遍历序)排列,根格自身包含在内,记录数不超过 `nativeabi.TreeBlocksMaxRecords`。
//
// 任何 engine 错误状态(ABI 版本不匹配、输入违约、记录数超限)都以
// `nativeabi` 的稳定中文文案 panic,没有静默回退;调用方必须先保证根坐标
// 落在 engine 合法域内(`core.MinY` 到 `core.MaxY - 9`:最坏普通橡树高 7,
// 顶格在根上 8 格),越界根坐标会 panic 而不是返回空几何。
func TreeBlocks(seed int64, root core.BlockPos) []TreeBlock {
	input := make([]byte, 0, nativeabi.TreeBlocksInputBytes)
	input = append(input, treeBlocksMagic...)
	input = binary.LittleEndian.AppendUint32(input, treeBlocksLayout)
	input = binary.LittleEndian.AppendUint64(input, uint64(seed))
	input = binary.LittleEndian.AppendUint32(input, uint32(root.X))
	input = binary.LittleEndian.AppendUint32(input, uint32(root.Y))
	input = binary.LittleEndian.AppendUint32(input, uint32(root.Z))

	output := make([]byte, nativeabi.TreeBlocksMaxOutputBytes)
	count := nativeabi.TreeBlocks(input, output)
	records := make([]TreeBlock, 0, count)
	for index := 0; index < count; index++ {
		offset := nativeabi.TreeBlocksCountBytes + index*nativeabi.TreeBlocksRecordBytes
		records = append(records, TreeBlock{
			DX:    int8(output[offset]),
			DY:    int8(output[offset+1]),
			DZ:    int8(output[offset+2]),
			Block: core.BlockID(binary.LittleEndian.Uint16(output[offset+4 : offset+6])),
		})
	}
	return records
}
