package entity

import (
	"testing"

	"github.com/channing771/mornlea/packages/server/sim/realm"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/tuning"
)

// TestSessionSubscriptionRadiusFromDeclaredViewDistance 钉住
// `SessionSubscription.Radius` 的派生口径：登录声明的期望视距换算为
// 未钳制半径（声明 +1）；未声明（0，未走登录协商的注册路径）派生 0，
// 由 runtime 侧按引擎缺省视界补齐，钳制也在引擎上界处单点完成。
func TestSessionSubscriptionRadiusFromDeclaredViewDistance(t *testing.T) {
	state := NewState(1)
	realmState := realm.NewState(core.Overworld)
	tunables := tuning.Tunables{}
	state.RegisterPlayer(1, PlayerRestore{
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{},
		ViewDistance:   4,
	}, realmState, tunables)
	state.RegisterPlayer(2, PlayerRestore{
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{X: 3, Z: -2},
	}, realmState, tunables)

	declared, ok := state.SessionSubscription(1)
	if !ok {
		t.Fatal("声明视距会话的订阅派生缺失")
	}
	if declared.Radius != 5 {
		t.Fatalf("声明视距 4 的派生半径 = %d，想要 5", declared.Radius)
	}
	undeclared, ok := state.SessionSubscription(2)
	if !ok {
		t.Fatal("未声明会话的订阅派生缺失")
	}
	if undeclared.Radius != 0 {
		t.Fatalf("未声明会话的派生半径 = %d，想要 0", undeclared.Radius)
	}
}
