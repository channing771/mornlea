package client

import (
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

func passiveSpawnMessageOf(tick uint64, id uint64, position mgl32.Vec3, health uint8) network.PassiveSpawn {
	return network.PassiveSpawn{ServerTick: tick, Spawns: []network.PassiveSpawnRecord{{
		ID: id, Dimension: core.Overworld, Position: position, Yaw: 0.5, Health: health,
	}}}
}

func passiveStateMessageOf(tick uint64, id uint64, position mgl32.Vec3, health uint8) network.PassiveState {
	return network.PassiveState{ServerTick: tick, States: []network.PassiveStateRecord{{
		ID: id, Position: position, Velocity: mgl32.Vec3{}, Yaw: 0.5, Health: health,
	}}}
}

// passiveGrazingStateMessageOf 构造携带放牧位的权威 state：放牧字节只取 0/1，
// 与线上值域一致，非法值由镜像的 `Validate` 入口拒绝而非本夹具覆盖。
func passiveGrazingStateMessageOf(tick uint64, id uint64, position mgl32.Vec3, health uint8, grazing uint8) network.PassiveState {
	return network.PassiveState{ServerTick: tick, States: []network.PassiveStateRecord{{
		ID: id, Position: position, Velocity: mgl32.Vec3{}, Yaw: 0.5, Health: health, Grazing: grazing,
	}}}
}

func TestPassivesLatestWinsMirror(t *testing.T) {
	passives := &Passives{}

	if err := passives.ApplySpawn(passiveSpawnMessageOf(100, 7, mgl32.Vec3{1, 2, 3}, 9)); err != nil {
		t.Fatalf("ApplySpawn: %v", err)
	}
	presentations := passives.AppendPresentations(nil)
	if len(presentations) != 1 || presentations[0].ID != 7 || presentations[0].Health != 9 {
		t.Fatalf("spawn 镜像=%+v，想要 1 条 ID 7 生命 9", presentations)
	}

	// 重复 spawn 按稳定规则忽略：既有镜像保持不变。
	if err := passives.ApplySpawn(passiveSpawnMessageOf(999, 7, mgl32.Vec3{9, 9, 9}, 20)); err != nil {
		t.Fatalf("重复 ApplySpawn: %v", err)
	}
	if presentations := passives.AppendPresentations(nil); presentations[0].Position != (mgl32.Vec3{1, 2, 3}) {
		t.Fatalf("重复 spawn 后位置=%v，想要镜像保持不变", presentations[0].Position)
	}

	// 未 spawn 的 state 丢弃且不隐式造实体。
	if err := passives.ApplyStates(passiveStateMessageOf(101, 8, mgl32.Vec3{5, 5, 5}, 10)); err != nil {
		t.Fatalf("未知 ID ApplyStates: %v", err)
	}
	if got := len(passives.AppendPresentations(nil)); got != 1 {
		t.Fatalf("未知 ID state 后镜像=%d，想要仍为 1", got)
	}

	// 过期 tick 的 state 丢弃：镜像保持 tick 100 的值。
	if err := passives.ApplyStates(passiveStateMessageOf(100, 7, mgl32.Vec3{6, 6, 6}, 1)); err != nil {
		t.Fatalf("过期 ApplyStates: %v", err)
	}
	if presentations := passives.AppendPresentations(nil); presentations[0].Position != (mgl32.Vec3{1, 2, 3}) {
		t.Fatalf("过期 state 后位置=%v，想要 tick 100 的值", presentations[0].Position)
	}

	// 更新 tick 的 state 生效：位置经插值起点推进，生命直读最新值。
	if err := passives.ApplyStates(passiveStateMessageOf(101, 7, mgl32.Vec3{2, 2, 3}, 7)); err != nil {
		t.Fatalf("ApplyStates: %v", err)
	}
	if presentations := passives.AppendPresentations(nil); presentations[0].Health != 7 {
		t.Fatalf("state 后生命=%d，想要 7", presentations[0].Health)
	}

	// 未知 ID 的 despawn 丢弃；已知 ID 的 despawn 移除镜像。
	if err := passives.ApplyDespawn(network.PassiveDespawn{ServerTick: 102, IDs: []uint64{8}}); err != nil {
		t.Fatalf("未知 ID ApplyDespawn: %v", err)
	}
	if err := passives.ApplyDespawn(network.PassiveDespawn{ServerTick: 102, IDs: []uint64{7}}); err != nil {
		t.Fatalf("ApplyDespawn: %v", err)
	}
	if got := len(passives.AppendPresentations(nil)); got != 0 {
		t.Fatalf("despawn 后镜像=%d，想要空", got)
	}
}

func TestPassivesMirrorCapacityMatchesAuthority(t *testing.T) {
	passives := &Passives{}
	for id := uint64(1); id <= MaxPassives; id++ {
		if err := passives.ApplySpawn(passiveSpawnMessageOf(1, id, mgl32.Vec3{float32(id), 0, 0}, 20)); err != nil {
			t.Fatalf("ApplySpawn id=%d: %v", id, err)
		}
	}
	if got := len(passives.AppendPresentations(nil)); got != MaxPassives {
		t.Fatalf("镜像容量=%d，想要 %d", got, MaxPassives)
	}
	// 满容量后的新个体按稳定规则忽略，不驱逐既有身体。
	if err := passives.ApplySpawn(passiveSpawnMessageOf(2, MaxPassives+1, mgl32.Vec3{99, 0, 0}, 20)); err != nil {
		t.Fatalf("超容量 ApplySpawn: %v", err)
	}
	presentations := passives.AppendPresentations(nil)
	if len(presentations) != MaxPassives {
		t.Fatalf("超容量后镜像=%d，想要仍为 %d", len(presentations), MaxPassives)
	}
	for _, presentation := range presentations {
		if presentation.ID == MaxPassives+1 {
			t.Fatalf("超容量个体进入镜像: %+v", presentation)
		}
	}
}

func TestPassivesMirrorAdvancesInterpolation(t *testing.T) {
	passives := &Passives{}
	positions := []mgl32.Vec3{{0, 1, 0}, {2, 1, 0}, {4, 1, 0}}
	if err := passives.ApplySpawn(passiveSpawnMessageOf(100, 7, positions[0], 20)); err != nil {
		t.Fatalf("ApplySpawn: %v", err)
	}
	for tick := uint64(101); tick <= 102; tick++ {
		if err := passives.ApplyStates(passiveStateMessageOf(tick, 7, positions[tick-100], 20)); err != nil {
			t.Fatalf("ApplyStates tick=%d: %v", tick, err)
		}
	}

	// 零推进：呈现位于插值滞后窗内（恰好是 tick 100 的已确认位置），
	// 绝不显示未确认的最新位置。
	passives.Advance(0)
	presentations := passives.AppendPresentations(nil)
	if presentations[0].Position != positions[0] {
		t.Fatalf("零推进呈现=%v，想要滞后窗内的 %v", presentations[0].Position, positions[0])
	}

	// 半个 tick（25ms）：呈现落在 tick 100 与 101 的权威位置之间。
	passives.Advance(time.Second / 40)
	presentations = passives.AppendPresentations(nil)
	mid := positions[0].Add(positions[1].Sub(positions[0]).Mul(0.5))
	if presentations[0].Position != mid {
		t.Fatalf("半 tick 呈现=%v，想要插值区间内的 %v", presentations[0].Position, mid)
	}

	// 长时间推进：推进量被钳制在单个 tick，滞后窗因此最多回退到「最新确认
	// 位置的前一格」（tick 101，同样是已确认位置），绝不越过最新确认位置
	// （tick 102）。
	passives.Advance(time.Second)
	presentations = passives.AppendPresentations(nil)
	if presentations[0].Position != positions[1] {
		t.Fatalf("长时间推进呈现=%v，想要钳制在已确认位置 %v", presentations[0].Position, positions[1])
	}
}

func TestPassivesMirrorRejectsInvalidMessages(t *testing.T) {
	passives := &Passives{}
	for name, apply := range map[string]func() error{
		"spawn": func() error {
			return passives.ApplySpawn(passiveSpawnMessageOf(1, 7, mgl32.Vec3{1, 1, 1}, 0))
		},
		"state": func() error {
			return passives.ApplyStates(network.PassiveState{ServerTick: 1, States: []network.PassiveStateRecord{
				{ID: 7, Position: mgl32.Vec3{1, 1, 1}, Health: core.MaxHealth + 1},
			}})
		},
		"despawn": func() error {
			return passives.ApplyDespawn(network.PassiveDespawn{ServerTick: 1})
		},
	} {
		if err := apply(); err == nil {
			t.Fatalf("%s 非法消息被接受", name)
		}
	}
}

// TestPassivesMirrorCarriesGrazingFlag 锁定放牧位的 latest-wins 镜像：权威
// state 的放牧字节经镜像直达呈现（置位即低头呈现的唯一事实来源），与生命值
// 共用同一套新旧语义——过期 tick 与未知 ID 不改既有值，新 tick 清位即恢复；
// 出生批次不带瞬态，新身体默认非放牧。
func TestPassivesMirrorCarriesGrazingFlag(t *testing.T) {
	passives := &Passives{}
	position := mgl32.Vec3{1, 2, 3}
	if err := passives.ApplySpawn(passiveSpawnMessageOf(100, 7, position, 9)); err != nil {
		t.Fatalf("ApplySpawn: %v", err)
	}
	if presentations := passives.AppendPresentations(nil); presentations[0].Grazing {
		t.Fatalf("出生后放牧位=%v，想要默认非放牧", presentations[0].Grazing)
	}

	if err := passives.ApplyStates(passiveGrazingStateMessageOf(101, 7, position, 9, 1)); err != nil {
		t.Fatalf("置位 ApplyStates: %v", err)
	}
	if presentations := passives.AppendPresentations(nil); !presentations[0].Grazing {
		t.Fatalf("置位后放牧位=%v，想要 true", presentations[0].Grazing)
	}

	// 过期 tick 的清位丢弃：镜像保持置位。
	if err := passives.ApplyStates(passiveGrazingStateMessageOf(100, 7, position, 9, 0)); err != nil {
		t.Fatalf("过期 ApplyStates: %v", err)
	}
	if presentations := passives.AppendPresentations(nil); !presentations[0].Grazing {
		t.Fatalf("过期清位后放牧位=%v，想要仍为 true", presentations[0].Grazing)
	}

	// 新 tick 清位恢复常态。
	if err := passives.ApplyStates(passiveGrazingStateMessageOf(102, 7, position, 9, 0)); err != nil {
		t.Fatalf("清位 ApplyStates: %v", err)
	}
	if presentations := passives.AppendPresentations(nil); presentations[0].Grazing {
		t.Fatalf("清位后放牧位=%v，想要 false", presentations[0].Grazing)
	}

	// 未知 ID 的置位丢弃且不隐式造实体。
	if err := passives.ApplyStates(passiveGrazingStateMessageOf(103, 8, position, 9, 1)); err != nil {
		t.Fatalf("未知 ID ApplyStates: %v", err)
	}
	if got := len(passives.AppendPresentations(nil)); got != 1 {
		t.Fatalf("未知 ID 置位后镜像=%d，想要仍为 1", got)
	}
}

func TestPassivesMirrorReset(t *testing.T) {
	passives := &Passives{}
	if err := passives.ApplySpawn(passiveSpawnMessageOf(1, 7, mgl32.Vec3{1, 1, 1}, 20)); err != nil {
		t.Fatalf("ApplySpawn: %v", err)
	}
	passives.Reset()
	if got := len(passives.AppendPresentations(nil)); got != 0 {
		t.Fatalf("Reset 后镜像=%d，想要空", got)
	}
	if err := passives.ApplySpawn(passiveSpawnMessageOf(2, 8, mgl32.Vec3{2, 1, 1}, 18)); err != nil {
		t.Fatalf("Reset 后 ApplySpawn: %v", err)
	}
	if got := len(passives.AppendPresentations(nil)); got != 1 {
		t.Fatalf("Reset 后重建镜像=%d，想要 1", got)
	}
}
