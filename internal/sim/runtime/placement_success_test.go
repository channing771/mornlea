package runtime_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/channing771/mornlea/internal/sim/runtime"
	"github.com/channing771/mornlea/packages/shared/core"
)

func TestPlayerPlacementSuccessesPreserveEverySequence(t *testing.T) {
	engine, session, _ := readyFlatEngineStocked(t, stockedHotbar(core.ItemDirt))
	for _, sequence := range []uint64{2, 3} {
		engine.Enqueue(runtime.Command{
			Session: session, Sequence: sequence, Kind: runtime.CommandPlaceBlock,
			Yaw: float32(math.Pi), Slot: 0,
		})
	}
	sameTick := engine.Step()
	wantSameTick := []runtime.PlacementSuccess{
		{Session: session, Sequence: 2},
		{Session: session, Sequence: 3},
	}
	if len(sameTick.Rejected) != 0 || !reflect.DeepEqual(sameTick.PlacementSuccesses, wantSameTick) {
		t.Fatalf("同 tick 放置结果=%+v，想要 successes=%+v", sameTick, wantSameTick)
	}

	engine.Enqueue(runtime.Command{
		Session: session, Sequence: 4, Kind: runtime.CommandPlaceBlock,
		Yaw: float32(math.Pi), Slot: 0,
	})
	nextTick := engine.Step()
	wantNextTick := []runtime.PlacementSuccess{{Session: session, Sequence: 4}}
	if len(nextTick.Rejected) != 0 || !reflect.DeepEqual(nextTick.PlacementSuccesses, wantNextTick) {
		t.Fatalf("跨 tick 放置结果=%+v，想要 successes=%+v", nextTick, wantNextTick)
	}
}

func TestRejectedPlayerPlacementHasNoSuccess(t *testing.T) {
	engine, session, _ := readyFlatEngineStocked(t, stockedHotbar(core.ItemDirt))
	engine.Enqueue(runtime.Command{
		Session: session, Sequence: 2, Kind: runtime.CommandPlaceBlock,
		Yaw: float32(math.Pi), Slot: core.HotbarSlots,
	})
	result := engine.Step()
	if len(result.Rejected) != 1 || result.Rejected[0].Sequence != 2 {
		t.Fatalf("拒绝结果=%+v", result.Rejected)
	}
	if len(result.PlacementSuccesses) != 0 {
		t.Fatalf("拒绝放置产生成功结果=%+v", result.PlacementSuccesses)
	}
}
