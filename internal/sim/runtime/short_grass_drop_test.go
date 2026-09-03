package runtime_test

import (
	"math"
	"testing"

	"github.com/channing771/mornlea/internal/sim/runtime"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/tuning"
	"github.com/channing771/mornlea/packages/shared/world"
)

// short_grass_drop_test.go：短草采掘的 runtime 对等证据（change
// natural-grass-seeds）。entity 白盒测试已锁定规则、判定与原子性；这里用生产
// runtime 引擎（固定 world seed=0、Overworld、flat 世界）走完整输入→FinishWorld→
// mutation commit 管线，证明种子掉落经由既有权威世界掉落物系统落地、revision
// 与区块变更同 tick 发布。
//
// 两个固定坐标是按规格冻结的 salt 折叠链（worldSeed ^ salt → 维度 → x → y → z，
// 全部经 uint32 位模式）事先算出的判定常量：(0,1,1) 命中 1/8，(0,1,4) 未命中。
// 玩家出生在 (0.5,1,0.5)，yaw=PI 朝 +Z；命中格就在脚前 1 格（pitch 加陡穿过
// 脚前空气直插目标），未命中格在 4 格外（与 entity 夹具同斜率的浅俯角）。

// TestMiningShortGrassHitDropsSeedsToWorld 锁定命中路径的 runtime 视角：1 tick
// 采除、恰好 1 颗小麦种子进入世界掉落物（锚定目标格、带既有 mining pickup
// delay）、背包不被直接写入。
func TestMiningShortGrassHitDropsSeedsToWorld(t *testing.T) {
	target := core.BlockPos{X: 0, Y: 1, Z: 1}
	engine, session := readyFlatPlayerWithTarget(t, map[core.BlockPos]core.BlockID{
		target: core.ShortGrassID,
	})
	_, beforeRevision, _ := engine.CloneReadyChunk(core.ChunkKey{Dimension: core.Overworld})

	sequence := uint64(1)
	result := mineUntilComplete(t, engine, session, &sequence, float32(math.Pi), -1.0, 1)

	if len(result.Rejected) != 0 {
		t.Fatalf("采除短草被拒绝: %+v", result.Rejected)
	}
	if len(result.Changes) != 1 || len(result.Changes[0].Changes) != 1 ||
		result.Changes[0].Changes[0] != (runtime.BlockChange{Position: target, Block: core.AirID}) {
		t.Fatalf("采除短草区块变更=%+v", result.Changes)
	}
	chunk, revision, _ := engine.CloneReadyChunk(core.ChunkKey{Dimension: core.Overworld})
	if chunk.BlockAt(0, 1, 1) != core.AirID {
		t.Fatalf("采除后方块=%d，想要空气", chunk.BlockAt(0, 1, 1))
	}
	if revision != beforeRevision+1 {
		t.Fatalf("revision=%d，想要恰好推进一次 %d", revision, beforeRevision+1)
	}
	index, ok := world.ChunkBlockIndex(target)
	if !ok {
		t.Fatal("短草目标没有区块索引")
	}
	found := 0
	for slot := range core.DropsPerChunk {
		drop := chunk.Drop(slot)
		if !drop.Active {
			continue
		}
		found++
		if drop.Stack != (core.ItemStack{Item: core.ItemWheatSeeds, Count: 1}) {
			t.Fatalf("掉落槽 %d = %+v，想要恰好 1 颗小麦种子", slot, drop.Stack)
		}
		if drop.BlockIndex != index {
			t.Fatalf("掉落槽 %d 锚定 %d，想要目标格 %d", slot, drop.BlockIndex, index)
		}
		if drop.PickupDelayTicks != tuning.DefaultTunables().DropPickupDelayTicks {
			t.Fatalf("掉落槽 %d pickup delay=%d，想要既有 mining 延迟 %d",
				slot, drop.PickupDelayTicks, tuning.DefaultTunables().DropPickupDelayTicks)
		}
	}
	if found != 1 {
		t.Fatalf("活动掉落槽数=%d，想要恰好 1", found)
	}
	if len(result.Inventories) != 0 {
		t.Fatalf("采除结算直接写入了背包: %+v", result.Inventories)
	}
	if got := currentInventory(t, engine, session); got != (core.Inventory{}) {
		t.Fatalf("空手采除短草改动了背包: %+v", got)
	}
}

// TestMiningShortGrassMissClearsBlockWithoutDrop 锁定未命中路径的 runtime 视角：
// 采除成功、区块按一次普通方块修改推进，世界不产生任何掉落物。
func TestMiningShortGrassMissClearsBlockWithoutDrop(t *testing.T) {
	target := core.BlockPos{X: 0, Y: 1, Z: 4}
	engine, session := readyFlatPlayerWithTarget(t, map[core.BlockPos]core.BlockID{
		target: core.ShortGrassID,
	})
	_, beforeRevision, _ := engine.CloneReadyChunk(core.ChunkKey{Dimension: core.Overworld})

	sequence := uint64(1)
	result := mineUntilComplete(t, engine, session, &sequence, float32(math.Pi), -0.4, 1)

	if len(result.Rejected) != 0 {
		t.Fatalf("未命中采除被拒绝: %+v", result.Rejected)
	}
	if len(result.Changes) != 1 || len(result.Changes[0].Changes) != 1 ||
		result.Changes[0].Changes[0] != (runtime.BlockChange{Position: target, Block: core.AirID}) {
		t.Fatalf("未命中采除区块变更=%+v", result.Changes)
	}
	chunk, revision, _ := engine.CloneReadyChunk(core.ChunkKey{Dimension: core.Overworld})
	if chunk.BlockAt(0, 1, 4) != core.AirID {
		t.Fatalf("未命中采除后方块=%d，想要空气", chunk.BlockAt(0, 1, 4))
	}
	if revision != beforeRevision+1 {
		t.Fatalf("revision=%d，想要推进一次 %d", revision, beforeRevision+1)
	}
	for slot := range core.DropsPerChunk {
		if drop := chunk.Drop(slot); drop.Active {
			t.Fatalf("未命中采除产生了掉落物: 槽 %d = %+v", slot, drop)
		}
	}
}
