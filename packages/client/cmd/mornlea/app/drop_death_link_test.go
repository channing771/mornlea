//go:build darwin

package app

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// TestAppendItemDropInstancesLinksDeathWindowDrops 锁定死亡掉落关联：死亡牛
// 身旁（2 格邻域）tick 窗内新出现的掉落带上关联死亡 tick，呈现侧据此滞后；
// 远处或窗外掉落不关联；密集击杀下任取其一但必须确定。
func TestAppendItemDropInstancesLinksDeathWindowDrops(t *testing.T) {
	deaths := []client.PassivePresentation{{
		ID: 7, Dimension: core.Overworld, Position: mgl32.Vec3{0.8, 1, -0.8},
		Dying: true, DeathTick: 100,
	}}
	nearBlock := core.BlockPos{X: 0, Y: 1, Z: -1}
	nearIndex, ok := world.ChunkBlockIndex(nearBlock)
	if !ok {
		t.Fatalf("关联锚点 %+v 不在区块索引内", nearBlock)
	}
	nearID := core.DropID{Dimension: core.Overworld, Chunk: nearBlock.Chunk(), Slot: 0, Generation: 1}
	farBlock := core.BlockPos{X: 10, Y: 1, Z: 10}
	farIndex, ok := world.ChunkBlockIndex(farBlock)
	if !ok {
		t.Fatalf("远处锚点 %+v 不在区块索引内", farBlock)
	}
	farID := core.DropID{Dimension: core.Overworld, Chunk: farBlock.Chunk(), Slot: 1, Generation: 1}
	drops := []client.ItemDropPresentation{
		{ID: nearID, BlockIndex: nearIndex, Item: core.ItemRawBeef, Count: 1, UpsertTick: 100},
		{ID: farID, BlockIndex: farIndex, Item: core.ItemRawBeef, Count: 1, UpsertTick: 100},
	}
	instances := appendItemDropInstances(nil, drops, client.NewMirror(), deaths)
	if len(instances) != 2 {
		t.Fatalf("实例=%d，想要 2", len(instances))
	}
	byID := make(map[core.DropID]uint64)
	for _, instance := range instances {
		byID[instance.ID] = instance.DeathTick
	}
	if byID[nearID] != 100 {
		t.Fatalf("身旁掉落关联=%d，想要死亡 tick 100", byID[nearID])
	}
	if byID[farID] != 0 {
		t.Fatalf("远处掉落关联=%d，想要不关联", byID[farID])
	}
	// 窗外 upsert 不关联：同位置但 tick 早于死亡。
	stale := []client.ItemDropPresentation{
		{ID: nearID, BlockIndex: nearIndex, Item: core.ItemRawBeef, Count: 1, UpsertTick: 50},
	}
	if got := appendItemDropInstances(nil, stale, client.NewMirror(), deaths); len(got) != 1 || got[0].DeathTick != 0 {
		t.Fatalf("窗外掉落=%+v，想要不关联", got)
	}
}
