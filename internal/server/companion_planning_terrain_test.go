package server

import (
	"testing"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
)

type countingPlanningTerrainSource struct {
	ready        map[[2]int32]bool
	heights      map[[2]int32]int32
	blocks       map[core.BlockPos]core.BlockID
	defaultBlock core.BlockID
	blockCalls   int
}

func (s *countingPlanningTerrainSource) heightAt(x, z int32) (int32, bool) {
	key := [2]int32{x, z}
	if !s.ready[key] {
		return 0, false
	}
	return s.heights[key], true
}

func (s *countingPlanningTerrainSource) blockAt(x, y, z int32) (core.BlockID, bool) {
	s.blockCalls++
	if !s.ready[[2]int32{x, z}] || y < core.MinY || y >= core.MaxY {
		return core.AirID, false
	}
	if block, ok := s.blocks[core.BlockPos{X: x, Y: y, Z: z}]; ok {
		return block, true
	}
	return s.defaultBlock, true
}

func TestPlanningTerrainPrimaryReadsExactlyOnceThenExposedUsesCache(t *testing.T) {
	source := newCountingPlanningTerrainSource(core.BlockPos{X: 0, Y: 64, Z: 0}, true)
	source.defaultBlock = core.StoneID
	air := core.BlockPos{X: 0, Y: 64, Z: 0}
	source.blocks[air] = core.AirID

	projection, exposed, heights, err := buildPlanningObservation(source, [3]float32{0.75, 64.9, 0.25})
	if err != nil {
		t.Fatalf("buildPlanningObservation: %v", err)
	}
	if source.blockCalls != companion.TerrainBlockCount {
		t.Fatalf("primary blockAt=%d，want %d", source.blockCalls, companion.TerrainBlockCount)
	}
	if len(heights) != companion.TerrainColumnCount {
		t.Fatalf("height samples=%d，want %d", len(heights), companion.TerrainColumnCount)
	}
	if got := projection.Origin(); got != (core.BlockPos{X: -16, Y: 56, Z: -16}) {
		t.Fatalf("projection origin=%+v", got)
	}
	if block, height, ok := projection.Lookup(air); !ok || block != core.AirID || height != 63 {
		t.Fatalf("center lookup=(%d,%d,%v)", block, height, ok)
	}

	visible := make(map[core.BlockPos]struct{}, len(exposed))
	for _, block := range exposed {
		visible[block.Pos] = struct{}{}
	}
	for _, pos := range []core.BlockPos{
		{X: -1, Y: 64, Z: 0}, {X: 1, Y: 64, Z: 0},
		{X: 0, Y: 63, Z: 0}, {X: 0, Y: 65, Z: 0},
		{X: 0, Y: 64, Z: -1}, {X: 0, Y: 64, Z: 1},
	} {
		if _, ok := visible[pos]; !ok {
			t.Fatalf("空气洞邻居 %+v 未从缓存派生为 exposed", pos)
		}
	}
	if _, ok := visible[core.BlockPos{X: -16, Y: 64, Z: -16}]; ok {
		t.Fatal("world-valid 投影外邻居被当作空气，错误暴露边缘方块")
	}
	if source.blockCalls != companion.TerrainBlockCount {
		t.Fatalf("派生 exposed 追加了 world read: %d", source.blockCalls)
	}
}

func TestPlanningTerrainUnreadyColumnAndWorldVerticalBoundary(t *testing.T) {
	center := core.BlockPos{X: 0, Y: core.MinY + 8, Z: 0}
	source := newCountingPlanningTerrainSource(center, true)
	source.defaultBlock = core.AirID
	missing := [2]int32{1, 0}
	source.ready[missing] = false
	for _, pos := range []core.BlockPos{
		{X: 0, Y: core.MinY, Z: 0},
		{X: 0, Y: core.MinY + 16, Z: 0},
		// 目标只把 +X 未 ready 列作为第六邻居；其余五邻居均为非空气。
		{X: 0, Y: core.MinY + 5, Z: 0},
		{X: -1, Y: core.MinY + 5, Z: 0},
		{X: 0, Y: core.MinY + 4, Z: 0},
		{X: 0, Y: core.MinY + 6, Z: 0},
		{X: 0, Y: core.MinY + 5, Z: -1},
		{X: 0, Y: core.MinY + 5, Z: 1},
	} {
		source.blocks[pos] = core.StoneID
	}

	projection, exposed, _, err := buildPlanningObservation(source, [3]float32{0, float32(core.MinY + 8), 0})
	if err != nil {
		t.Fatalf("buildPlanningObservation: %v", err)
	}
	wantCalls := (companion.TerrainColumnCount - 1) * companion.TerrainHeight
	if source.blockCalls != wantCalls {
		t.Fatalf("unready primary blockAt=%d，want %d", source.blockCalls, wantCalls)
	}
	for _, y := range []int32{core.MinY, core.MinY + 16} {
		if block, _, ok := projection.Lookup(core.BlockPos{X: 0, Y: y, Z: 0}); !ok || block != core.StoneID {
			t.Fatalf("±8 endpoint y=%d lookup=(%d,%v)", y, block, ok)
		}
	}
	if _, _, ok := projection.Lookup(core.BlockPos{X: missing[0], Y: core.MinY + 1, Z: missing[1]}); ok {
		t.Fatal("未 ready 列被解释为可观察空气")
	}

	visible := make(map[core.BlockPos]struct{}, len(exposed))
	for _, block := range exposed {
		visible[block.Pos] = struct{}{}
	}
	if _, ok := visible[core.BlockPos{X: 0, Y: core.MinY, Z: 0}]; !ok {
		t.Fatal("世界垂直边界外邻居未按空气形成 exposed")
	}
	if _, ok := visible[core.BlockPos{X: 0, Y: core.MinY + 5, Z: 0}]; ok {
		t.Fatal("未 ready 邻列被按空气形成 exposed")
	}
}

func newCountingPlanningTerrainSource(center core.BlockPos, ready bool) *countingPlanningTerrainSource {
	source := &countingPlanningTerrainSource{
		ready:   make(map[[2]int32]bool, companion.TerrainColumnCount),
		heights: make(map[[2]int32]int32, companion.TerrainColumnCount),
		blocks:  make(map[core.BlockPos]core.BlockID),
	}
	for x := center.X - 16; x <= center.X+16; x++ {
		for z := center.Z - 16; z <= center.Z+16; z++ {
			source.ready[[2]int32{x, z}] = ready
			source.heights[[2]int32{x, z}] = 63
		}
	}
	return source
}
