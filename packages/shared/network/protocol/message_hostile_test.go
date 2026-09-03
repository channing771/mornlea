package protocol

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// hostileSpawnFixture 返回 3 条字段各异、ID 严格升序的合法 spawn 记录：
// 生命取非零非满的中间值，保证「字段根本没搬运」与默认值不可分辨。
func hostileSpawnFixture() []HostileSpawnRecord {
	return []HostileSpawnRecord{
		{ID: 7, Dimension: core.Overworld, Position: mgl32.Vec3{2.5, 1, -3.25}, Yaw: 1.25, Health: 14},
		{ID: 9, Dimension: core.Overworld, Position: mgl32.Vec3{-8.5, 65.5, 12.75}, Yaw: -2.5, Health: core.MaxHealth},
		{ID: 12, Dimension: core.Overworld, Position: mgl32.Vec3{30.5, 70, -3.25}, Yaw: 3, Health: 1},
	}
}

// hostileStateFixture 返回 2 条 ID 严格升序的合法 state 记录：速度取非零值，
// 保证速度分量的搬运与丢弃可分辨。
func hostileStateFixture() []HostileStateRecord {
	return []HostileStateRecord{
		{ID: 7, Position: mgl32.Vec3{2.5, 1, -3.25}, Velocity: mgl32.Vec3{0.5, -1.25, 0}, Yaw: 1.25, Health: 13},
		{ID: 9, Position: mgl32.Vec3{-8.5, 65.5, 12.75}, Velocity: mgl32.Vec3{0, 0.25, 3}, Yaw: -2.5, Health: 7},
	}
}

func hostileSpawnMessage() HostileSpawn {
	return HostileSpawn{ServerTick: 0x0102030405060708, Spawns: hostileSpawnFixture()}
}

func hostileStateMessage() HostileState {
	return HostileState{ServerTick: 0x0102030405060708, States: hostileStateFixture()}
}

func hostileDespawnMessage() HostileDespawn {
	return HostileDespawn{ServerTick: 0x0102030405060708, IDs: []uint64{7, 9, 12}}
}

// TestHostileMessageIDsAreFrozen 钉死夜行者三类消息的最终编号：S→C 22/23/24
// （21 已被 `CraftingState` 实占；`ServerPacketID` 与 `ServerPacketForID` 两处
// 对称）。上界断言写成「末项 +1」，下次追加 packet 时它跟着末项走。
func TestHostileMessageIDsAreFrozen(t *testing.T) {
	assertServerRegistry(t, []struct {
		state  State
		packet ServerPacket
		id     uint32
	}{
		{StatePlay, HostileSpawn{}, 22},
		{StatePlay, HostileState{}, 23},
		{StatePlay, HostileDespawn{}, 24},
	})
	for _, id := range []uint32{22, 23, 24, 25} {
		if _, ok := ServerPacketForID(StatePlay, id); !ok {
			t.Fatalf("Play server packet ID %d 未注册", id)
		}
	}
	if _, ok := ServerPacketForID(StatePlay, 25+1); ok {
		t.Fatal("Play server packet ID 26 必须保持未分配")
	}
}

func TestHostileMessagesValidateRejectsInvalidRecords(t *testing.T) {
	validSpawn := hostileSpawnMessage()
	validState := hostileStateMessage()
	validDespawn := hostileDespawnMessage()

	tests := []struct {
		name   string
		packet ServerPacket
	}{
		{"spawn 重复 ID", HostileSpawn{ServerTick: 1, Spawns: []HostileSpawnRecord{
			hostileSpawnFixture()[0], hostileSpawnFixture()[0],
		}}},
		{"spawn 逆序 ID", HostileSpawn{ServerTick: 1, Spawns: []HostileSpawnRecord{
			hostileSpawnFixture()[1], hostileSpawnFixture()[0],
		}}},
		{"spawn 零 ID", HostileSpawn{ServerTick: 1, Spawns: []HostileSpawnRecord{{
			Dimension: core.Overworld, Position: mgl32.Vec3{1, 1, 1}, Yaw: 0, Health: 10,
		}}}},
		{"spawn NaN position", HostileSpawn{ServerTick: 1, Spawns: []HostileSpawnRecord{{
			ID: 1, Dimension: core.Overworld, Position: mgl32.Vec3{float32(math.NaN()), 1, 1}, Health: 10,
		}}}},
		{"spawn Inf yaw", HostileSpawn{ServerTick: 1, Spawns: []HostileSpawnRecord{{
			ID: 1, Dimension: core.Overworld, Position: mgl32.Vec3{1, 1, 1}, Yaw: float32(math.Inf(1)), Health: 10,
		}}}},
		{"spawn health 0", HostileSpawn{ServerTick: 1, Spawns: []HostileSpawnRecord{{
			ID: 1, Dimension: core.Overworld, Position: mgl32.Vec3{1, 1, 1}, Health: 0,
		}}}},
		{"spawn health 超上限", HostileSpawn{ServerTick: 1, Spawns: []HostileSpawnRecord{{
			ID: 1, Dimension: core.Overworld, Position: mgl32.Vec3{1, 1, 1}, Health: core.MaxHealth + 1,
		}}}},
		{"spawn 非法维度", HostileSpawn{ServerTick: 1, Spawns: []HostileSpawnRecord{{
			ID: 1, Dimension: core.DimensionID(5), Position: mgl32.Vec3{1, 1, 1}, Health: 10,
		}}}},
		{"spawn count 0", HostileSpawn{ServerTick: 1}},
		{"state 重复 ID", HostileState{ServerTick: 1, States: []HostileStateRecord{
			hostileStateFixture()[0], hostileStateFixture()[0],
		}}},
		{"state 逆序 ID", HostileState{ServerTick: 1, States: []HostileStateRecord{
			hostileStateFixture()[1], hostileStateFixture()[0],
		}}},
		{"state 零 ID", HostileState{ServerTick: 1, States: []HostileStateRecord{{
			Position: mgl32.Vec3{1, 1, 1}, Velocity: mgl32.Vec3{0, 0, 0}, Health: 10,
		}}}},
		{"state Inf velocity", HostileState{ServerTick: 1, States: []HostileStateRecord{{
			ID: 1, Position: mgl32.Vec3{1, 1, 1}, Velocity: mgl32.Vec3{0, float32(math.Inf(-1)), 0}, Health: 10,
		}}}},
		{"state NaN yaw", HostileState{ServerTick: 1, States: []HostileStateRecord{{
			ID: 1, Position: mgl32.Vec3{1, 1, 1}, Velocity: mgl32.Vec3{0, 0, 0}, Yaw: float32(math.NaN()), Health: 10,
		}}}},
		{"state health 0", HostileState{ServerTick: 1, States: []HostileStateRecord{{
			ID: 1, Position: mgl32.Vec3{1, 1, 1}, Velocity: mgl32.Vec3{0, 0, 0}, Health: 0,
		}}}},
		{"state health 超上限", HostileState{ServerTick: 1, States: []HostileStateRecord{{
			ID: 1, Position: mgl32.Vec3{1, 1, 1}, Velocity: mgl32.Vec3{0, 0, 0}, Health: core.MaxHealth + 1,
		}}}},
		{"state count 0", HostileState{ServerTick: 1}},
		{"despawn 重复 ID", HostileDespawn{ServerTick: 1, IDs: []uint64{7, 7}}},
		{"despawn 逆序 ID", HostileDespawn{ServerTick: 1, IDs: []uint64{9, 7}}},
		{"despawn 零 ID", HostileDespawn{ServerTick: 1, IDs: []uint64{0}}},
		{"despawn count 0", HostileDespawn{ServerTick: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateServerPacket(StatePlay, test.packet); err == nil {
				t.Fatalf("%T=%+v 被接受，想要整体拒绝", test.packet, test.packet)
			}
		})
	}

	// 边界内的合法形态必须继续通过：count 恰好 1 与上限、生命 1 与 20。
	validSpawns := make([]HostileSpawnRecord, MaxHostileRecords)
	for index := range validSpawns {
		validSpawns[index] = HostileSpawnRecord{
			ID: uint64(index + 1), Dimension: core.Overworld,
			Position: mgl32.Vec3{float32(index), 1, 0}, Yaw: 0, Health: core.MaxHealth,
		}
	}
	if err := validSpawn.Validate(); err != nil {
		t.Fatalf("合法 spawn 被拒绝: %v", err)
	}
	if err := (HostileSpawn{ServerTick: 1, Spawns: validSpawns}).Validate(); err != nil {
		t.Fatalf("count=%d 的合法 spawn 被拒绝: %v", MaxHostileRecords, err)
	}
	if err := validState.Validate(); err != nil {
		t.Fatalf("合法 state 被拒绝: %v", err)
	}
	if err := validDespawn.Validate(); err != nil {
		t.Fatalf("合法 despawn 被拒绝: %v", err)
	}
}
