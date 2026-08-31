package companion

import (
	"testing"
	"unsafe"

	"github.com/channing771/mornlea/internal/core"
)

func TestTerrainProjectionReadyEmptyAndVerticalEndpoints(t *testing.T) {
	origin := core.BlockPos{X: -16, Y: 56, Z: -16}
	projection := NewTerrainProjection(origin)

	if !projection.SetReadyColumn(-16, -16, core.MinY-1) {
		t.Fatal("投影拒绝 ready 空列")
	}
	if !projection.SetReadyColumn(0, 0, 63) {
		t.Fatal("投影拒绝中心 ready 列")
	}
	for _, sample := range []struct {
		pos   core.BlockPos
		block core.BlockID
	}{
		{pos: core.BlockPos{X: 0, Y: 56, Z: 0}, block: core.ChestID},
		{pos: core.BlockPos{X: 0, Y: 72, Z: 0}, block: core.FurnaceID},
	} {
		if !projection.SetBlock(sample.pos, sample.block) {
			t.Fatalf("SetBlock(%+v) 被拒绝", sample.pos)
		}
		block, height, ok := projection.Lookup(sample.pos)
		if !ok || block != sample.block || height != 63 {
			t.Fatalf("Lookup(%+v)=(%d,%d,%v)，want (%d,63,true)", sample.pos, block, height, ok, sample.block)
		}
	}

	for _, pos := range []core.BlockPos{
		{X: 0, Y: 55, Z: 0},
		{X: 0, Y: 73, Z: 0},
		{X: 17, Y: 64, Z: 0},
		{X: -15, Y: 64, Z: -16}, // 投影内但列未 ready。
	} {
		if block, height, ok := projection.Lookup(pos); ok || block != core.AirID || height != 0 {
			t.Fatalf("Lookup(%+v)=(%d,%d,%v)，want fail closed", pos, block, height, ok)
		}
	}
	if block, height, ok := projection.Lookup(core.BlockPos{X: -16, Y: 64, Z: -16}); !ok || block != core.AirID || height != core.MinY-1 {
		t.Fatalf("ready 空列 lookup=(%d,%d,%v)，want (air,%d,true)", block, height, ok, core.MinY-1)
	}
	if !projection.ColumnReady(-16, -16) || projection.ColumnReady(-15, -16) {
		t.Fatal("ready bitmap 未区分空列与未加载列")
	}
	if err := projection.Validate(); err != nil {
		t.Fatalf("合法投影 Validate: %v", err)
	}
}

func TestTerrainProjectionDataPlaneBoundsAndFailClosed(t *testing.T) {
	if TerrainReadyBitmapBytes != 137 || TerrainColumnCount != 1089 || TerrainBlockCount != 18513 {
		t.Fatalf("投影常量漂移: bitmap=%d columns=%d blocks=%d", TerrainReadyBitmapBytes, TerrainColumnCount, TerrainBlockCount)
	}
	if TerrainDataPlaneBytes != 137+1089*2+18513*2 {
		t.Fatalf("data plane formula=%d", TerrainDataPlaneBytes)
	}
	if TerrainDataPlaneBytes != 39341 {
		t.Fatalf("data plane=%d，want 39341", TerrainDataPlaneBytes)
	}
	if TerrainDataPlaneBytes > 40<<10 || 4*TerrainDataPlaneBytes > 160<<10 {
		t.Fatalf("投影容量超界: one=%d four=%d", TerrainDataPlaneBytes, 4*TerrainDataPlaneBytes)
	}
	projection := NewTerrainProjection(core.BlockPos{X: 0, Y: core.MinY - 8, Z: 0})
	if size := unsafe.Sizeof(projection); size > 40<<10 {
		t.Fatalf("TerrainProjection struct=%d bytes，超过 40 KiB", size)
	}
	if projection.SetReadyColumn(-1, 0, 0) || projection.SetReadyColumn(0, -1, 0) {
		t.Fatal("投影接受范围外列")
	}
	if projection.SetReadyColumn(0, 0, core.MaxY) {
		t.Fatal("投影接受越界 height")
	}
	if projection.SetBlock(core.BlockPos{X: 0, Y: core.MinY, Z: 0}, core.BlockIDMax) {
		t.Fatal("投影接受未注册 BlockID")
	}
	projection.readyColumns[TerrainReadyBitmapBytes-1] = 0xff
	if err := projection.Validate(); err == nil {
		t.Fatal("ready bitmap 末 7 个 unused bits 非零被接受")
	}
}
