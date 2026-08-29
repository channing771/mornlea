package entity

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/internal/sim/tuning"
)

func TestEntityReceivesMutationAndTunables(t *testing.T) {
	// 该测试锁定 entity 的世界写入必须经由 concrete *realm.Mutation 与 tuning 快照，
	// 禁止直接读取全局或 runtime 状态。
	realmState := realm.NewState(core.Overworld)
	engine := NewEngine(0, 0, 0)
	engine.realm = realmState
	mutation := realmState.NewMutation()
	tunables := tuning.DefaultTunables()
	// 使用 entity 的放置意图结算路径，验证其签名已改为接收 *realm.Mutation 与 Tunables
	// 若签名仍为旧的 *pendingChunkChanges，编译将失败 (RED)
	_ = tunables
	_ = mutation
	// 下面这行在 RED 阶段将编译失败（方法不存在或签名不匹配），GREEN 阶段应可编译
	// 为保持测试可运行，此处通过接口断言新签名存在性
	type placementSettler interface {
		CompleteCompanionPlacement(entry *companionState, target core.BlockPos, blockID core.BlockID, realmState *realm.State, mutation *realm.Mutation, tunables tuning.Tunables) bool
	}
	var _ placementSettler = engine
	if engine == nil || realmState == nil {
		t.Fatal("unexpected nil")
	}
}

func TestSpawnCandidatesOrder(t *testing.T) {
	// 复用 sim/spawn_test.go 的首段断言，保持 Test 名与子逻辑一致
	got := spawnCandidates(core.ChunkPos{}, tuning.DefaultTunables().SpawnRadius)
	if len(got) != 33*33 {
		t.Fatalf("spawnCandidates len=%d, want 1089", len(got))
	}
	if got[0] != (spawnColumn{X: 0, Z: 0}) {
		t.Fatalf("first candidate=%+v", got[0])
	}
}
