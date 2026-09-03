package client

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/network"
)

// hostileSpawnMessageOf 构造单条记录的合法 spawn 消息。
func hostileSpawnMessageOf(tick uint64, id uint64, position mgl32.Vec3, health uint8) network.HostileSpawn {
	return network.HostileSpawn{ServerTick: tick, Spawns: []network.HostileSpawnRecord{{
		ID: id, Position: position, Yaw: 0.25, Health: health,
	}}}
}

func hostileStateMessageOf(tick uint64, id uint64, position mgl32.Vec3, health uint8) network.HostileState {
	return network.HostileState{ServerTick: tick, States: []network.HostileStateRecord{{
		ID: id, Position: position, Velocity: mgl32.Vec3{0, 0, 0}, Yaw: 0.25, Health: health,
	}}}
}

func TestHostileMirrorLifecycle(t *testing.T) {
	hostiles := &Hostiles{}

	// spawn 建立身体：呈现携带权威位置、朝向与生命。
	if err := hostiles.ApplySpawn(hostileSpawnMessageOf(100, 7, mgl32.Vec3{1, 2, 3}, 13)); err != nil {
		t.Fatalf("ApplySpawn: %v", err)
	}
	presentations := hostiles.AppendPresentations(nil)
	if len(presentations) != 1 || presentations[0].ID != 7 ||
		presentations[0].Position != (mgl32.Vec3{1, 2, 3}) ||
		presentations[0].Health != 13 {
		t.Fatalf("spawn 后呈现=%#v，想要 ID 7 在 (1,2,3) 生命 13", presentations)
	}

	// 重复 spawn 按稳定规则忽略：既有镜像保持不变。
	if err := hostiles.ApplySpawn(hostileSpawnMessageOf(999, 7, mgl32.Vec3{9, 9, 9}, 20)); err != nil {
		t.Fatalf("重复 spawn 返回错误: %v", err)
	}
	if presentations := hostiles.AppendPresentations(nil); presentations[0].Position != (mgl32.Vec3{1, 2, 3}) {
		t.Fatalf("重复 spawn 改写了镜像：%#v", presentations[0])
	}

	// 从未 spawn 的 ID：state 丢弃且不隐式造实体。
	if err := hostiles.ApplyStates(hostileStateMessageOf(101, 8, mgl32.Vec3{5, 5, 5}, 10)); err != nil {
		t.Fatalf("未知 ID state 返回错误: %v", err)
	}
	if got := len(hostiles.AppendPresentations(nil)); got != 1 {
		t.Fatalf("未知 ID state 造出了实体，呈现数=%d", got)
	}

	// 过期 state（tick 100 不比镜像新）：丢弃并保持镜像不变。
	if err := hostiles.ApplyStates(hostileStateMessageOf(100, 7, mgl32.Vec3{6, 6, 6}, 1)); err != nil {
		t.Fatalf("过期 state 返回错误: %v", err)
	}
	if presentations := hostiles.AppendPresentations(nil); presentations[0].Position != (mgl32.Vec3{1, 2, 3}) {
		t.Fatalf("过期 state 改写了镜像：%#v", presentations[0])
	}

	// 更新 tick 的 state：位置与生命一起前进。
	if err := hostiles.ApplyStates(hostileStateMessageOf(101, 7, mgl32.Vec3{2, 2, 3}, 11)); err != nil {
		t.Fatalf("ApplyStates: %v", err)
	}
	if presentations := hostiles.AppendPresentations(nil); presentations[0].Health != 11 {
		t.Fatalf("state 后生命=%d，想要 11", presentations[0].Health)
	}

	// 未知 ID 的 despawn 丢弃；已知 ID 的 despawn 移除身体。
	if err := hostiles.ApplyDespawn(network.HostileDespawn{ServerTick: 102, IDs: []uint64{8}}); err != nil {
		t.Fatalf("未知 ID despawn 返回错误: %v", err)
	}
	if err := hostiles.ApplyDespawn(network.HostileDespawn{ServerTick: 102, IDs: []uint64{7}}); err != nil {
		t.Fatalf("ApplyDespawn: %v", err)
	}
	if got := len(hostiles.AppendPresentations(nil)); got != 0 {
		t.Fatalf("despawn 后仍有 %d 具身体", got)
	}
}

func TestHostileMirrorCapacityIsStableAtSixtyFour(t *testing.T) {
	hostiles := &Hostiles{}
	const overflowID = 65
	for id := uint64(1); id <= overflowID; id++ {
		if err := hostiles.ApplySpawn(hostileSpawnMessageOf(1, id, mgl32.Vec3{float32(id), 0, 0}, 20)); err != nil {
			t.Fatalf("spawn %d: %v", id, err)
		}
	}
	if got := len(hostiles.AppendPresentations(nil)); got != MaxHostiles {
		t.Fatalf("镜像容量=%d，想要 %d", got, MaxHostiles)
	}
	// 满镜像拒绝的是第 65 具（按稳定规则忽略），既有身体不受影响。
	presentations := hostiles.AppendPresentations(nil)
	if !slices.ContainsFunc(presentations, func(p HostilePresentation) bool { return p.ID == 1 }) {
		t.Fatal("首个 spawn 的身体被驱逐")
	}
	if slices.ContainsFunc(presentations, func(p HostilePresentation) bool { return p.ID == overflowID }) {
		t.Fatal("溢出的第 65 具身体被接纳")
	}
}

func TestHostilePresentationStaysInsideInterpolationWindow(t *testing.T) {
	hostiles := &Hostiles{}
	positions := []mgl32.Vec3{{0, 2, 0}, {2, 2, 0}, {4, 2, 0}}
	if err := hostiles.ApplySpawn(hostileSpawnMessageOf(100, 7, positions[0], 20)); err != nil {
		t.Fatalf("ApplySpawn: %v", err)
	}
	for tick := uint64(101); tick <= 102; tick++ {
		if err := hostiles.ApplyStates(hostileStateMessageOf(tick, 7, positions[tick-100], 20)); err != nil {
			t.Fatalf("ApplyStates: %v", err)
		}
	}

	// 零推进：呈现位于插值滞后窗内（恰好是 tick 100 的已确认位置），
	// 绝不显示未确认的最新位置。
	hostiles.Advance(0)
	presentations := hostiles.AppendPresentations(nil)
	if presentations[0].Position != positions[0] {
		t.Fatalf("零推进呈现=%v，想要滞后窗内的 %v", presentations[0].Position, positions[0])
	}

	// 半个 tick（25ms）：呈现落在 tick 100 与 101 的权威位置之间。
	hostiles.Advance(time.Second / 40)
	presentations = hostiles.AppendPresentations(nil)
	mid := positions[0].Add(positions[1].Sub(positions[0]).Mul(0.5))
	if presentations[0].Position != mid {
		t.Fatalf("半 tick 呈现=%v，想要插值区间内的 %v", presentations[0].Position, mid)
	}

	// 长时间推进：推进量被钳制在单个 tick（remoteMaxElapsed），滞后窗因此
	// 最多回退到「最新确认位置的前一格」（tick 101，同样是已确认位置），
	// 绝不越过最新确认位置（tick 102）。
	hostiles.Advance(time.Second)
	presentations = hostiles.AppendPresentations(nil)
	if presentations[0].Position != positions[1] {
		t.Fatalf("长时间推进呈现=%v，想要钳制在已确认位置 %v", presentations[0].Position, positions[1])
	}
}

func TestHostileMirrorRejectsInvalidMessages(t *testing.T) {
	hostiles := &Hostiles{}
	tests := []struct {
		name  string
		apply func() error
	}{
		{"spawn 非法生命", func() error {
			return hostiles.ApplySpawn(hostileSpawnMessageOf(1, 7, mgl32.Vec3{1, 1, 1}, 0))
		}},
		{"state 非法排序", func() error {
			return hostiles.ApplyStates(network.HostileState{ServerTick: 1, States: []network.HostileStateRecord{
				{ID: 9, Position: mgl32.Vec3{1, 1, 1}, Health: 20},
				{ID: 7, Position: mgl32.Vec3{1, 1, 1}, Health: 20},
			}})
		}},
		{"despawn 空批次", func() error {
			return hostiles.ApplyDespawn(network.HostileDespawn{ServerTick: 1})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.apply(); !errors.Is(err, ErrHostileProtocol) {
				t.Fatalf("错误=%v，想要 ErrHostileProtocol", err)
			}
		})
	}
}

func TestHostileResetClearsMirror(t *testing.T) {
	hostiles := &Hostiles{}
	if err := hostiles.ApplySpawn(hostileSpawnMessageOf(1, 7, mgl32.Vec3{1, 1, 1}, 20)); err != nil {
		t.Fatalf("ApplySpawn: %v", err)
	}
	hostiles.Reset()
	if got := len(hostiles.AppendPresentations(nil)); got != 0 {
		t.Fatalf("Reset 后仍有 %d 具身体", got)
	}
	// Reset 后镜像可继续工作。
	if err := hostiles.ApplySpawn(hostileSpawnMessageOf(2, 8, mgl32.Vec3{2, 1, 1}, 18)); err != nil {
		t.Fatalf("Reset 后 ApplySpawn: %v", err)
	}
	if got := len(hostiles.AppendPresentations(nil)); got != 1 {
		t.Fatalf("Reset 后 spawn 数=%d，想要 1", got)
	}
}
