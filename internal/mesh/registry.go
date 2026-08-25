package mesh

import (
	"fmt"
	"slices"
	"sort"

	"github.com/channing771/mornlea/internal/world"
)

// RegistryReader 提供冻结网格 registry 所需的方块属性。
type RegistryReader interface {
	Opaque(world.BlockID) bool
	FaceVisible(id world.BlockID, adjacent world.BlockID) bool
	Material(id world.BlockID, face Face) uint16
	Emission(world.BlockID) uint8
	FluidHeight(world.BlockID) uint8
	LightAttenuation(world.BlockID) uint8
	BlockTopRaw(world.BlockID) uint8
}

// Registry 提供网格化需要的方块属性及其不可变快照。
type Registry interface {
	RegistryReader
	MeshSnapshot() RegistrySnapshot
}

// BlockProperties 是单个方块在网格化期间使用的冻结属性。
//
// FluidHeight、LightAttenuation 与 BlockTopRaw 跟 Emission 同形状：每方块一个
// 字节，随快照一起编码进 native 输入（见 encodeNativeInput 与 Rust 的
// REGISTRY_ENTRY_BYTES）。
type BlockProperties struct {
	ID       world.BlockID
	Opaque   bool
	Emission uint8
	// FluidHeight 是该方块**孤立时**的 4-bit 流体高度原值 h_raw，实际高度为
	// (h_raw+1)/16。0 是「非流体」哨兵：h_raw = 14 - level 且 level <= 7，真流体
	// 的 h_raw 恒在 7..=14，0 永远不是合法流体高度，因此不必额外花一个标志位，
	// 零值 BlockProperties 也天然表示非流体。等级→高度的映射只存在于 Go 一处
	// （internal/assets.Registry.FluidHeight），Rust 侧只消费这个数、不知道等级。
	FluidHeight uint8
	// LightAttenuation 是天空光穿过该方块时的额外衰减，供 Rust 光照 BFS 使用。
	// 本字段只负责把值送过 ABI 边界，衰减行为由后续变更实现。
	LightAttenuation uint8
	// BlockTopRaw 是非满格方块的 4-bit 顶面高度原值，实际呈现高度为
	// (h_raw+1)/16，由 mesher 的常量角高度路径消费（复用水面 quad 的角高度位）。
	//
	// 0 是「满格方块」哨兵：绝大多数方块是整格立方体，取 0 让既有条目零改动，
	// 与 FluidHeight 的「0=非流体」同构；1..=14 表示全部可见面的上缘按该高度
	// 下沉（首个消费者是干/湿耕地的 14，即 15/16，恰等于物理碰撞高度）；15
	// 必须被拒绝——满格只能用哨兵 0 表达，「非零即短方块」才能保持单一判定，
	// Rust 侧 RegistryView::validate 同口径拒绝。
	//
	// 与 FluidHeight 互斥：流体的角高度由 mesher 邻域平均现算（含「上方也是
	// 流体则取满格」规则）、短方块由本字段常量驱动，两条几何路径不得叠加在
	// 同一条目上——编码两侧都按「二者不同时非零」拒绝。
	BlockTopRaw uint8
	Materials   [6]uint16
}

// RegistrySnapshot 是按方块 ID 排序的网格 registry 快照。
type RegistrySnapshot struct {
	Blocks     []BlockProperties
	Visibility []uint64
}

// BuildRegistrySnapshot 复制并冻结指定方块 ID 的网格属性。
func BuildRegistrySnapshot(ids []world.BlockID, reader RegistryReader) (RegistrySnapshot, error) {
	sorted := slices.Clone(ids)
	slices.Sort(sorted)
	for i := 1; i < len(sorted); i++ {
		if sorted[i] == sorted[i-1] {
			return RegistrySnapshot{}, fmt.Errorf("mesh: 重复 block ID %d", sorted[i])
		}
	}

	blocks := make([]BlockProperties, len(sorted))
	for i, id := range sorted {
		block := BlockProperties{
			ID:               id,
			Opaque:           reader.Opaque(id),
			Emission:         reader.Emission(id),
			FluidHeight:      reader.FluidHeight(id),
			LightAttenuation: reader.LightAttenuation(id),
			BlockTopRaw:      reader.BlockTopRaw(id),
		}
		if block.Emission > 15 {
			return RegistrySnapshot{}, fmt.Errorf("mesh: block %d emission=%d 超过 15", id, block.Emission)
		}
		// 15 被保留给「上方也是流体」的满格情形，只能由 mesher 现算，不得作为
		// 单方块属性出现；Rust 侧的 validate 同样拒绝，这里提前给出可读错误。
		if block.FluidHeight > 14 {
			return RegistrySnapshot{}, fmt.Errorf("mesh: block %d fluidHeight=%d 超过 14", id, block.FluidHeight)
		}
		// 合法域是 0..=1。这个 1 不是天空光值域（那是 0..15，数字碰巧相近），而是
		// Rust light::build_sky 分桶推进的算法前提：证明依赖「每格扣减只可能是 1 或 2」，
		// 衰减到 2 会让桶不再单亮度、「每格至多入队一次」失效，队列溢出成渲染热路径
		// 上的 panic。Rust 侧 RegistryView::validate 同口径拒绝，这里提前给可读错误。
		if block.LightAttenuation > 1 {
			return RegistrySnapshot{}, fmt.Errorf("mesh: block %d lightAttenuation=%d 超过 1", id, block.LightAttenuation)
		}
		// 合法域是哨兵 0（满格）加 1..=14（呈现高度 (h+1)/16）。15 无从表达任何
		// 合法几何——满格必须写哨兵 0，「非零即短方块」是 mesher 的单一判定前提；
		// Rust 侧 validate 同口径拒绝，这里提前给出可读错误。
		if block.BlockTopRaw > 14 {
			return RegistrySnapshot{}, fmt.Errorf("mesh: block %d blockTopRaw=%d 超过 14", id, block.BlockTopRaw)
		}
		// 流体与短方块互斥：流体的角高度由 mesher 邻域平均现算、短方块由
		// BlockTopRaw 常量驱动，同一条目同时携带两套语义时行为无从定义。
		// Rust 侧 validate 同口径拒绝，这里提前给出可读错误。
		if block.FluidHeight != 0 && block.BlockTopRaw != 0 {
			return RegistrySnapshot{}, fmt.Errorf("mesh: block %d 流体条目携带非零 blockTopRaw=%d", id, block.BlockTopRaw)
		}
		for face := Face(0); face < 6; face++ {
			block.Materials[face] = reader.Material(id, face)
		}
		blocks[i] = block
	}

	wordsPerRow := (len(sorted) + 63) / 64
	visibility := make([]uint64, len(sorted)*wordsPerRow)
	for i, id := range sorted {
		for j, adjacent := range sorted {
			if reader.FaceVisible(id, adjacent) {
				visibility[i*wordsPerRow+j/64] |= uint64(1) << (j % 64)
			}
		}
	}
	return RegistrySnapshot{Blocks: blocks, Visibility: visibility}, nil
}

// FaceVisible 返回快照中两个方块 ID 之间冻结的可见性。
func (s RegistrySnapshot) FaceVisible(id, adjacent world.BlockID) bool {
	i := sort.Search(len(s.Blocks), func(i int) bool { return s.Blocks[i].ID >= id })
	j := sort.Search(len(s.Blocks), func(i int) bool { return s.Blocks[i].ID >= adjacent })
	if i == len(s.Blocks) || s.Blocks[i].ID != id ||
		j == len(s.Blocks) || s.Blocks[j].ID != adjacent {
		return false
	}
	wordsPerRow := (len(s.Blocks) + 63) / 64
	word := i*wordsPerRow + j/64
	return word < len(s.Visibility) && s.Visibility[word]&(uint64(1)<<(j%64)) != 0
}
