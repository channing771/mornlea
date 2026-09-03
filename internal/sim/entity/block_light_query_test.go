package entity

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// 本文件锁定夜行者的局部区块光判定：以候选为中心的 29³（半径 14）窗口、
// 初始值等于 `core.BlockEmission` 发射值、每步衰减 1 + `core.BlockLightAttenuation`、
// `core.BlockOpaque` 阻挡、unknown/unloaded 视同阻挡，以及重复调用零分配。
// 另含与既有客户端/Rust 方块光 oracle 的固定小夹具逐位对照。

func TestLocalBlockLightSeedsEmissionAndSpreadsByOne(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	dimension := engine.dimension(core.Overworld)
	// 落地火把：所在格发光 14，向六邻域逐步 −1。
	engine.SetBlockForTest(core.BlockPos{X: 8, Y: 1, Z: 8}, core.TorchStandingID)
	scratch := newBlockLightScratch()
	cases := []struct {
		center core.BlockPos
		want   uint8
	}{
		{core.BlockPos{X: 8, Y: 1, Z: 8}, 14},  // 光源格本身
		{core.BlockPos{X: 8, Y: 1, Z: 7}, 13},  // 曼哈顿距离 1
		{core.BlockPos{X: 8, Y: 8, Z: 8}, 7},   // 距离 7：暗度判定的临界值
		{core.BlockPos{X: 8, Y: 9, Z: 8}, 6},   // 距离 8
		{core.BlockPos{X: 15, Y: 1, Z: 15}, 0}, // 距离 14：衰减到 0
	}
	for _, tc := range cases {
		if got := localBlockLight(dimension, scratch, tc.center); got != tc.want {
			t.Fatalf("center=%v 局部区块光=%d，想要 %d", tc.center, got, tc.want)
		}
	}
	// 发光方块（不透明）同样以发射值作种子并向外传播。
	engine.SetBlockForTest(core.BlockPos{X: 3, Y: 1, Z: 3}, core.LightBlockID)
	if got := localBlockLight(dimension, scratch, core.BlockPos{X: 3, Y: 9, Z: 3}); got != 7 {
		t.Fatalf("发光方块距离 8 的局部区块光=%d，想要 7", got)
	}
	if got := localBlockLight(dimension, scratch, core.BlockPos{X: 3, Y: 8, Z: 3}); got != 8 {
		t.Fatalf("发光方块距离 7 的局部区块光=%d，想要 8", got)
	}
}

func TestLocalBlockLightOpaqueBlocksAndFluidAttenuates(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	dimension := engine.dimension(core.Overworld)
	scratch := newBlockLightScratch()
	engine.SetBlockForTest(core.BlockPos{X: 8, Y: 1, Z: 8}, core.TorchStandingID)
	// 完整不透明隔层：整个区块的 y=3 平面填实，光无法越过（区块边界外为
	// 未加载，视同阻挡，不能绕行）。
	for x := int32(0); x < core.SectionSize; x++ {
		for z := int32(0); z < core.SectionSize; z++ {
			engine.SetBlockForTest(core.BlockPos{X: x, Y: 3, Z: z}, core.StoneID)
		}
	}
	if got := localBlockLight(dimension, scratch, core.BlockPos{X: 8, Y: 8, Z: 8}); got != 0 {
		t.Fatalf("不透明隔层上方的局部区块光=%d，想要 0", got)
	}
	// 流体额外衰减：光源 → 水格步长 2 → 空气恢复步长 1。
	engine.SetBlockForTest(core.BlockPos{X: 9, Y: 1, Z: 8}, core.WaterSourceID)
	if got := localBlockLight(dimension, scratch, core.BlockPos{X: 9, Y: 1, Z: 8}); got != 12 {
		t.Fatalf("水格局部区块光=%d，想要 12（14−(1+1)）", got)
	}
	if got := localBlockLight(dimension, scratch, core.BlockPos{X: 10, Y: 1, Z: 8}); got != 11 {
		t.Fatalf("水后空气局部区块光=%d，想要 11", got)
	}
	// 玻璃非不透明且零衰减：透过玻璃每格 −1。
	engine.SetBlockForTest(core.BlockPos{X: 9, Y: 1, Z: 8}, core.GlassID)
	if got := localBlockLight(dimension, scratch, core.BlockPos{X: 10, Y: 1, Z: 8}); got != 12 {
		t.Fatalf("玻璃后空气局部区块光=%d，想要 12", got)
	}
}

func TestLocalBlockLightTreatsUnknownAndUnloadedAsBlocking(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	dimension := engine.dimension(core.Overworld)
	scratch := newBlockLightScratch()
	// 只有原点区块已加载：光源贴近区块边界，跨边界即进入未加载区块。
	engine.SetBlockForTest(core.BlockPos{X: 0, Y: 1, Z: 0}, core.TorchStandingID)
	if got := localBlockLight(dimension, scratch, core.BlockPos{X: 1, Y: 1, Z: 0}); got != 13 {
		t.Fatalf("已加载一侧的局部区块光=%d，想要 13", got)
	}
	if got := localBlockLight(dimension, scratch, core.BlockPos{X: -1, Y: 1, Z: 0}); got != 0 {
		t.Fatalf("未加载区块内的局部区块光=%d，想要 0（视同阻挡）", got)
	}
}

// airOnlyBlockLightOracle 是既有客户端/Rust 方块光 pass（Rust light.rs 的
// build_block）在同一 29³ 窗口上的测试内复刻：种子 = 发射值，传播每步固定
// −1、只进入纯空气格。它就是 mesh 包 light_oracle_test.go 里那份 Go oracle
// 的同款语义。
func airOnlyBlockLightOracle(dimension *Dimension, center core.BlockPos) []uint8 {
	levels := make([]uint8, hostileLightVolume)
	type cell struct{ rx, ry, rz int }
	queue := make([]cell, 0, hostileLightVolume)
	baseX, baseY, baseZ := int(center.X)-hostileLightRadius, int(center.Y)-hostileLightRadius, int(center.Z)-hostileLightRadius
	blockAt := func(rx, ry, rz int) (core.BlockID, bool) {
		return dimension.BlockAt(core.BlockPos{
			X: int32(baseX + rx), Y: int32(baseY + ry), Z: int32(baseZ + rz),
		})
	}
	for rx := range hostileLightSide {
		for ry := range hostileLightSide {
			for rz := range hostileLightSide {
				block, ready := blockAt(rx, ry, rz)
				if !ready {
					continue
				}
				if level := core.BlockEmission(block); level > 0 {
					levels[hostileLightIndex(rx, ry, rz)] = level
					queue = append(queue, cell{rx, ry, rz})
				}
			}
		}
	}
	for head := 0; head < len(queue); head++ {
		current := levels[hostileLightIndex(queue[head].rx, queue[head].ry, queue[head].rz)]
		if current <= 1 {
			continue
		}
		for _, dir := range [6]cell{{-1, 0, 0}, {1, 0, 0}, {0, -1, 0}, {0, 1, 0}, {0, 0, -1}, {0, 0, 1}} {
			nx, ny, nz := queue[head].rx+dir.rx, queue[head].ry+dir.ry, queue[head].rz+dir.rz
			if nx < 0 || nx >= hostileLightSide || ny < 0 || ny >= hostileLightSide ||
				nz < 0 || nz >= hostileLightSide {
				continue
			}
			index := hostileLightIndex(nx, ny, nz)
			if levels[index] >= current-1 {
				continue
			}
			block, ready := blockAt(nx, ny, nz)
			if !ready || block != core.AirID {
				continue
			}
			levels[index] = current - 1
			queue = append(queue, cell{nx, ny, nz})
		}
	}
	return levels
}

func TestLocalBlockLightMatchesClientOracleOnSharedRuleFixtures(t *testing.T) {
	// 两个规则集的公共定义域：只含空气、不透明方块与发射方块（含火把）。
	// 在该定义域内「不透明阻挡 + 1+衰减(=1) 步长」与客户端的「只进空气、
	// 固定 −1」逐位等价，因此整窗逐格对照必须全等。
	engine, _ := readyMovementPlayer(t)
	dimension := engine.dimension(core.Overworld)
	loadFlatChunks(t, dimension, -1, 1, -1, 1)
	engine.SetBlockForTest(core.BlockPos{X: 8, Y: 5, Z: 8}, core.TorchStandingID)
	engine.SetBlockForTest(core.BlockPos{X: 8, Y: 4, Z: 8}, core.StoneID)
	engine.SetBlockForTest(core.BlockPos{X: 12, Y: 3, Z: 10}, core.LightBlockID)
	engine.SetBlockForTest(core.BlockPos{X: 2, Y: 2, Z: 14}, core.TorchWallPosXID)
	for x := int32(5); x < 9; x++ {
		for z := int32(5); z < 9; z++ {
			engine.SetBlockForTest(core.BlockPos{X: x, Y: 2, Z: z}, core.StoneID)
		}
	}
	center := core.BlockPos{X: 8, Y: 5, Z: 8}
	scratch := newBlockLightScratch()
	localBlockLight(dimension, scratch, center)
	want := airOnlyBlockLightOracle(dimension, center)
	for index := range want {
		if scratch.levels[index] != want[index] {
			t.Fatalf("局部区块光与客户端 oracle 在窗口格 %d 不一致：sim=%d oracle=%d",
				index, scratch.levels[index], want[index])
		}
	}
}

func TestLocalBlockLightDocumentsTransparentCellDivergence(t *testing.T) {
	// 已记录并裁决的真实差异：透明非空气方块（玻璃/树叶/门/作物/流体）。
	// 服务端暗度判定按设计采用 core 单一表语义——`core.BlockOpaque` 阻挡、
	// 每步 1 + `core.BlockLightAttenuation`；客户端网格化的方块光 pass 刻意
	// 只进入纯空气格（网格呈现只需要空气面上的光值）。两侧在判定定义域内
	// （候选格必为空气）的常见夹具上逐位一致（见上一测试），差异只在光路
	// 穿过透明非空气方块时出现；本测试把差异域与服务端取值显式钉住，防止
	// 无声漂移成第三套规则。
	engine, _ := readyMovementPlayer(t)
	dimension := engine.dimension(core.Overworld)
	scratch := newBlockLightScratch()
	engine.SetBlockForTest(core.BlockPos{X: 8, Y: 1, Z: 8}, core.TorchStandingID)
	engine.SetBlockForTest(core.BlockPos{X: 8, Y: 1, Z: 7}, core.GlassID)
	if got := localBlockLight(dimension, scratch, core.BlockPos{X: 8, Y: 1, Z: 7}); got != 13 {
		t.Fatalf("玻璃格局部区块光=%d，想要 13（服务端按 core 表透射）", got)
	}
	if got := localBlockLight(dimension, scratch, core.BlockPos{X: 8, Y: 1, Z: 6}); got != 12 {
		t.Fatalf("玻璃后空气局部区块光=%d，想要 12", got)
	}
	engine.SetBlockForTest(core.BlockPos{X: 8, Y: 1, Z: 7}, core.AirID)
	engine.SetBlockForTest(core.BlockPos{X: 8, Y: 1, Z: 7}, core.WaterSourceID)
	if got := localBlockLight(dimension, scratch, core.BlockPos{X: 8, Y: 1, Z: 7}); got != 12 {
		t.Fatalf("水格局部区块光=%d，想要 12（14−(1+1)）", got)
	}
}

func TestLocalBlockLightQueryIsAllocationFree(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	dimension := engine.dimension(core.Overworld)
	engine.SetBlockForTest(core.BlockPos{X: 8, Y: 1, Z: 8}, core.TorchStandingID)
	center := core.BlockPos{X: 8, Y: 2, Z: 8}
	if got := localBlockLight(dimension, engine.hostileLight, center); got == 0 {
		t.Fatal("预热查询意外返回 0")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		localBlockLight(dimension, engine.hostileLight, center)
	}); allocations != 0 {
		t.Fatalf("重复局部区块光查询分配=%v，想要 0", allocations)
	}
}

func TestLocalBlockLightFullWindowOfEmissiveCellsStaysBounded(t *testing.T) {
	// 最坏情形：整个窗口全是发射方块（种子数 = 29³）。预分配 scratch 必须
	// 恰好有界，溢出以 panic 暴露而不是静默截断。
	engine, _ := readyMovementPlayer(t)
	dimension := engine.dimension(core.Overworld)
	loadFlatChunks(t, dimension, -1, 1, -1, 1)
	center := core.BlockPos{X: 8, Y: 40, Z: 8}
	baseX, baseY, baseZ := center.X-hostileLightRadius, center.Y-hostileLightRadius, center.Z-hostileLightRadius
	for dx := int32(0); dx < hostileLightSide; dx++ {
		for dy := int32(0); dy < hostileLightSide; dy++ {
			for dz := int32(0); dz < hostileLightSide; dz++ {
				engine.SetBlockForTest(core.BlockPos{
					X: baseX + dx, Y: baseY + dy, Z: baseZ + dz,
				}, core.LightBlockID)
			}
		}
	}
	if got := localBlockLight(dimension, engine.hostileLight, center); got != 15 {
		t.Fatalf("全光源窗口中心局部区块光=%d，想要 15", got)
	}
}
