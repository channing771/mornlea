package runtime

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// sleep_persistence_test.go：重生点与显示相位偏移在 sim 与持久化层之间的接线。
// 权威内存态经 `PlayerSnapshot` 流向存档批次，登录恢复经 `PlayerRestore` 流回；
// 任何一侧漏接线都会让「睡一觉设置的重生点」在重启后静默丢失。

// TestRespawnPointFlowsThroughSnapshotAndRestore 覆盖重启语义的 sim 一半：
// 入睡记录的重生点必须出现在权威快照里；把快照按服务端装配的同一转换喂给
// `PlayerRestore` 重新注册后，死亡重生仍能经 `bedRespawnCandidate` 回到床尾格。
func TestRespawnPointFlowsThroughSnapshotAndRestore(t *testing.T) {
	engine := twoPlayerWorld(t)
	session, yaw, pitch := placeSleepBed(t, engine, sleepBedFoot, 3.5)
	if result := interactBed(engine, session, 10, yaw, pitch); len(result.Rejected) != 0 {
		t.Fatalf("入睡被拒绝: %+v", result.Rejected)
	}

	snapshot, ok := engine.PlayerSnapshot(session)
	if !ok {
		t.Fatal("活跃玩家应能取得权威快照")
	}
	if !snapshot.RespawnPresent {
		t.Fatal("入睡后的权威快照缺少重生点")
	}
	if snapshot.RespawnPosition != [3]float32{
		float32(sleepBedFoot.X), float32(sleepBedFoot.Y), float32(sleepBedFoot.Z),
	} {
		t.Fatalf("快照重生点坐标 = %+v，想要床尾格 %+v", snapshot.RespawnPosition, sleepBedFoot)
	}
	if snapshot.RespawnDimension != core.Overworld {
		t.Fatalf("快照重生点维度 = %d，想要 %d", snapshot.RespawnDimension, core.Overworld)
	}

	// 重启：按服务端 restore() 的同一形态重建 PlayerRestore，注册进一个放了
	// 同一张床的全新引擎。
	restarted, _, _ := doorTestReadyEngine(t, core.Hotbar{})
	restarted.SetBlockForTest(sleepBedFoot, core.BedFootSouthID)
	restarted.SetBlockForTest(sleepBedHead, core.BedHeadSouthID)
	restoredSession := SessionID(2)
	restarted.RegisterPlayer(restoredSession, PlayerRestore{
		SpawnDimension:   core.Overworld,
		SpawnAnchor:      core.ChunkPos{},
		RespawnPresent:   snapshot.RespawnPresent,
		RespawnPosition:  snapshot.RespawnPosition,
		RespawnDimension: snapshot.RespawnDimension,
	})
	for range 8 {
		restarted.Step()
	}
	if player, ok := restarted.Player(restoredSession); !ok || !player.Ready {
		t.Fatalf("重启后玩家未激活: %+v", player)
	}

	player := respawnWhenDead(t, restarted, restoredSession)
	pos := player.State.Position
	if float32(sleepBedFoot.X)+0.5 != pos.X() || float32(sleepBedFoot.Z)+0.5 != pos.Z() {
		t.Fatalf("重启后重生位置 = %+v，想要床尾格中心", pos)
	}
}

// TestRestoreDayPhaseOffsetFeedsDisplayPhase 锁定宿主装配恢复路径：从 metadata
// 恢复的偏移必须直接参与显示相位计算，首个 tick 之前即可观察。
func TestRestoreDayPhaseOffsetFeedsDisplayPhase(t *testing.T) {
	engine := NewEngine(0, 18000, 0)
	if got := engine.DayPhaseOffset(); got != 0 {
		t.Fatalf("未恢复前偏移 = %d，想要 0", got)
	}
	engine.RestoreDayPhaseOffset(6000)
	if got := engine.DayPhaseOffset(); got != 6000 {
		t.Fatalf("恢复后偏移 = %d，想要 6000", got)
	}
	// (18000 + 6000) % 24000 = 0：恢复的偏移立刻进入相位，而不是等首个 tick。
	if phase := engine.displayDayPhase(); phase != 0 {
		t.Fatalf("恢复后的显示相位 = %d，想要 0", phase)
	}
	// 绝对时间不受恢复的偏移影响：从恢复值继续每 tick 恰好 +1。
	if result := engine.Step(); result.WorldTimeTicks != 18001 {
		t.Fatalf("恢复偏移后首个 tick 世界时间 = %d，想要 18001", result.WorldTimeTicks)
	}
}
