package client

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/network"
)

// passiveDespawnMessageOf 构造携带原因位的 despawn：原因位只取 0/1，与线上
// 值域一致，非法值由镜像的 `Validate` 入口拒绝而非本夹具覆盖。
func passiveDespawnMessageOf(tick uint64, id uint64, reason uint8) network.PassiveDespawn {
	return network.PassiveDespawn{ServerTick: tick, Despawns: []network.PassiveDespawnRecord{{ID: id, Reason: reason}}}
}

// TestPassivesDeathKeepsRenderingForTwentyTicks 锁定死亡保留语义：死亡原因的
// despawn 把身体转入保留（冻结位姿、T+19 仍在、T+20 移除），保留期 state 丢
// 弃，同 ID 新 spawn 按稳定规则不污染保留态；消失原因立即移除。
func TestPassivesDeathKeepsRenderingForTwentyTicks(t *testing.T) {
	passives := &Passives{}
	position := mgl32.Vec3{1, 2, 3}
	if err := passives.ApplySpawn(passiveSpawnMessageOf(100, 7, position, 9)); err != nil {
		t.Fatalf("ApplySpawn: %v", err)
	}
	// 另一头活牛充当时间推进器：它的 state 批次携带新 tick，不碰 7 号。
	if err := passives.ApplySpawn(passiveSpawnMessageOf(100, 8, mgl32.Vec3{9, 1, 9}, 20)); err != nil {
		t.Fatalf("ApplySpawn 活牛: %v", err)
	}
	if err := passives.ApplyDespawn(passiveDespawnMessageOf(110, 7, network.PassiveDespawnDied)); err != nil {
		t.Fatalf("死亡 ApplyDespawn: %v", err)
	}
	presentations := passives.AppendPresentations(nil)
	if len(presentations) != 2 {
		t.Fatalf("死亡后呈现=%d，想要 2（含保留体）", len(presentations))
	}
	var dying *PassivePresentation
	for index := range presentations {
		if presentations[index].ID == 7 {
			dying = &presentations[index]
		}
	}
	if dying == nil || !dying.Dying || dying.DeathTick != 110 || dying.Position != position {
		t.Fatalf("保留体=%+v，想要死亡标记、死亡 tick 110 与冻结位姿", dying)
	}

	// 保留期 state 丢弃：位姿保持冻结。
	if err := passives.ApplyStates(passiveStateMessageOf(115, 7, mgl32.Vec3{6, 6, 6}, 5)); err != nil {
		t.Fatalf("保留期 ApplyStates: %v", err)
	}
	// 保留期同 ID 新 spawn 按稳定规则忽略：不污染保留态。
	if err := passives.ApplySpawn(passiveSpawnMessageOf(116, 7, mgl32.Vec3{9, 9, 9}, 20)); err != nil {
		t.Fatalf("保留期 ApplySpawn: %v", err)
	}
	for tick := uint64(111); tick <= 129; tick++ {
		if err := passives.ApplyStates(passiveStateMessageOf(tick, 8, mgl32.Vec3{9, 1, 9}, 20)); err != nil {
			t.Fatalf("推进 tick=%d: %v", tick, err)
		}
		found := false
		for _, presentation := range passives.AppendPresentations(nil) {
			if presentation.ID == 7 {
				found = true
				if !presentation.Dying || presentation.Position != position {
					t.Fatalf("tick=%d 保留体=%+v，想要冻结呈现", tick, presentation)
				}
			}
		}
		if !found {
			t.Fatalf("tick=%d 保留体提前消失，想要 T+19 仍在", tick)
		}
	}
	if err := passives.ApplyStates(passiveStateMessageOf(130, 8, mgl32.Vec3{9, 1, 9}, 20)); err != nil {
		t.Fatalf("推进 tick=130: %v", err)
	}
	for _, presentation := range passives.AppendPresentations(nil) {
		if presentation.ID == 7 {
			t.Fatalf("T+20 保留体仍在=%+v，想要已移除", presentation)
		}
	}

	// 消失原因立即移除，不进保留。
	if err := passives.ApplyDespawn(passiveDespawnMessageOf(131, 8, network.PassiveDespawnVanished)); err != nil {
		t.Fatalf("消失 ApplyDespawn: %v", err)
	}
	if got := len(passives.AppendPresentations(nil)); got != 0 {
		t.Fatalf("消失后呈现=%d，想要空", got)
	}
}
