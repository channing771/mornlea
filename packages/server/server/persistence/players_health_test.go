package persistence

import (
	"testing"

	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/shared/core"
)

// TestPlayerPersistenceDirtyDetectionIncludesHealth 覆盖"只有生命值变化也必须被
// 持久化"：玩家原地站着回血时位置与物品都不变，快照比较与存档比较都必须把生命值
// 算进去，否则这次变化会被当成无变化而丢失。
func TestPlayerPersistenceDirtyDetectionIncludesHealth(t *testing.T) {
	full := contract.PlayerSnapshot{Health: core.MaxHealth}
	hurt := full
	hurt.Health = 7
	if playerSnapshotsEqual(full, hurt) {
		t.Fatal("快照比较忽略了生命值")
	}

	player := &cachedPlayer{hasSnapshot: true, snapshot: full}
	save := player.save(1)
	if save.Health != core.MaxHealth {
		t.Fatalf("存档生命值 = %d，想要 %d", save.Health, core.MaxHealth)
	}
	if !player.matchesSave(save) {
		t.Fatal("同值存档必须匹配缓存快照")
	}
	save.Health = 7
	if player.matchesSave(save) {
		t.Fatal("存档比较忽略了生命值")
	}
}
