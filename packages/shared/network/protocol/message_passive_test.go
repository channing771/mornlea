package protocol

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// passiveSpawnFixture 返回 3 条字段各异、ID 严格升序的合法 spawn 记录：
// 生命取非零非满的中间值，保证「字段根本没搬运」与默认值不可分辨。
func passiveSpawnFixture() []PassiveSpawnRecord {
	return []PassiveSpawnRecord{
		{ID: 5, Dimension: core.Overworld, Position: mgl32.Vec3{1.5, 2, -1.25}, Yaw: 0.75, Health: 10},
		{ID: 8, Dimension: core.Overworld, Position: mgl32.Vec3{-4.5, 63.5, 6.25}, Yaw: -1.5, Health: core.MaxHealth},
		{ID: 11, Dimension: core.Overworld, Position: mgl32.Vec3{12.5, 68, -6.5}, Yaw: 2, Health: 1},
	}
}

// passiveStateFixture 返回 2 条 ID 严格升序的合法 state 记录：速度取非零值，
// 保证速度分量的搬运与丢弃可分辨；放牧标志取 0/1 各一，保证该字节的搬运与
// 丢弃可分辨。
func passiveStateFixture() []PassiveStateRecord {
	return []PassiveStateRecord{
		{ID: 5, Position: mgl32.Vec3{1.5, 2, -1.25}, Velocity: mgl32.Vec3{0.25, -0.5, 0}, Yaw: 0.75, Health: 9, Grazing: 1},
		{ID: 8, Position: mgl32.Vec3{-4.5, 63.5, 6.25}, Velocity: mgl32.Vec3{0, 0.5, 1.5}, Yaw: -1.5, Health: 6},
	}
}

func passiveSpawnMessage() PassiveSpawn {
	return PassiveSpawn{ServerTick: 0x0102030405060708, Spawns: passiveSpawnFixture()}
}

func passiveStateMessage() PassiveState {
	return PassiveState{ServerTick: 0x0102030405060708, States: passiveStateFixture()}
}

func passiveDespawnMessage() PassiveDespawn {
	return PassiveDespawn{ServerTick: 0x0102030405060708, IDs: []uint64{5, 8, 11}}
}

// TestPassiveMessageIDsAreFrozen 钉死被动牛三类消息的最终编号：S→C 26/27/28
// （25 已被 `CombatHit` 实占；`ServerPacketID` 与 `ServerPacketForID` 两处
// 对称）。上界断言写成「末项 +1」，下次追加 packet 时它跟着末项走。
func TestPassiveMessageIDsAreFrozen(t *testing.T) {
	assertServerRegistry(t, []struct {
		state  State
		packet ServerPacket
		id     uint32
	}{
		{StatePlay, PassiveSpawn{}, 26},
		{StatePlay, PassiveState{}, 27},
		{StatePlay, PassiveDespawn{}, 28},
	})
	for _, id := range []uint32{26, 27, 28} {
		if _, ok := ServerPacketForID(StatePlay, id); !ok {
			t.Fatalf("Play server packet ID %d 未注册", id)
		}
	}
	if _, ok := ServerPacketForID(StatePlay, 28+1); ok {
		t.Fatal("Play server packet ID 29 必须保持未分配")
	}
}

func TestPassiveMessagesValidateRejectsInvalidRecords(t *testing.T) {
	validSpawn := passiveSpawnMessage()
	validState := passiveStateMessage()
	validDespawn := passiveDespawnMessage()

	tests := []struct {
		name   string
		packet ServerPacket
	}{
		{"spawn 重复 ID", PassiveSpawn{ServerTick: 1, Spawns: []PassiveSpawnRecord{
			passiveSpawnFixture()[0], passiveSpawnFixture()[0],
		}}},
		{"spawn 逆序 ID", PassiveSpawn{ServerTick: 1, Spawns: []PassiveSpawnRecord{
			passiveSpawnFixture()[1], passiveSpawnFixture()[0],
		}}},
		{"spawn 零 ID", PassiveSpawn{ServerTick: 1, Spawns: []PassiveSpawnRecord{{
			Dimension: core.Overworld, Position: mgl32.Vec3{1, 1, 1}, Yaw: 0, Health: 10,
		}}}},
		{"spawn NaN position", PassiveSpawn{ServerTick: 1, Spawns: []PassiveSpawnRecord{{
			ID: 1, Dimension: core.Overworld, Position: mgl32.Vec3{float32(math.NaN()), 1, 1}, Health: 10,
		}}}},
		{"spawn Inf yaw", PassiveSpawn{ServerTick: 1, Spawns: []PassiveSpawnRecord{{
			ID: 1, Dimension: core.Overworld, Position: mgl32.Vec3{1, 1, 1}, Yaw: float32(math.Inf(1)), Health: 10,
		}}}},
		{"spawn health 0", PassiveSpawn{ServerTick: 1, Spawns: []PassiveSpawnRecord{{
			ID: 1, Dimension: core.Overworld, Position: mgl32.Vec3{1, 1, 1}, Health: 0,
		}}}},
		{"spawn health 超上限", PassiveSpawn{ServerTick: 1, Spawns: []PassiveSpawnRecord{{
			ID: 1, Dimension: core.Overworld, Position: mgl32.Vec3{1, 1, 1}, Health: core.MaxHealth + 1,
		}}}},
		{"spawn 非法维度", PassiveSpawn{ServerTick: 1, Spawns: []PassiveSpawnRecord{{
			ID: 1, Dimension: core.DimensionID(5), Position: mgl32.Vec3{1, 1, 1}, Health: 10,
		}}}},
		{"spawn count 0", PassiveSpawn{ServerTick: 1}},
		{"state 重复 ID", PassiveState{ServerTick: 1, States: []PassiveStateRecord{
			passiveStateFixture()[0], passiveStateFixture()[0],
		}}},
		{"state 逆序 ID", PassiveState{ServerTick: 1, States: []PassiveStateRecord{
			passiveStateFixture()[1], passiveStateFixture()[0],
		}}},
		{"state 零 ID", PassiveState{ServerTick: 1, States: []PassiveStateRecord{{
			Position: mgl32.Vec3{1, 1, 1}, Velocity: mgl32.Vec3{0, 0, 0}, Health: 10,
		}}}},
		{"state Inf velocity", PassiveState{ServerTick: 1, States: []PassiveStateRecord{{
			ID: 1, Position: mgl32.Vec3{1, 1, 1}, Velocity: mgl32.Vec3{0, float32(math.Inf(-1)), 0}, Health: 10,
		}}}},
		{"state NaN yaw", PassiveState{ServerTick: 1, States: []PassiveStateRecord{{
			ID: 1, Position: mgl32.Vec3{1, 1, 1}, Velocity: mgl32.Vec3{0, 0, 0}, Yaw: float32(math.NaN()), Health: 10,
		}}}},
		{"state health 0", PassiveState{ServerTick: 1, States: []PassiveStateRecord{{
			ID: 1, Position: mgl32.Vec3{1, 1, 1}, Velocity: mgl32.Vec3{0, 0, 0}, Health: 0,
		}}}},
		{"state health 超上限", PassiveState{ServerTick: 1, States: []PassiveStateRecord{{
			ID: 1, Position: mgl32.Vec3{1, 1, 1}, Velocity: mgl32.Vec3{0, 0, 0}, Health: core.MaxHealth + 1,
		}}}},
		{"state 放牧标志非 0/1", PassiveState{ServerTick: 1, States: []PassiveStateRecord{{
			ID: 1, Position: mgl32.Vec3{1, 1, 1}, Velocity: mgl32.Vec3{0, 0, 0}, Health: 10, Grazing: 2,
		}}}},
		{"state count 0", PassiveState{ServerTick: 1}},
		{"despawn 重复 ID", PassiveDespawn{ServerTick: 1, IDs: []uint64{5, 5}}},
		{"despawn 逆序 ID", PassiveDespawn{ServerTick: 1, IDs: []uint64{8, 5}}},
		{"despawn 零 ID", PassiveDespawn{ServerTick: 1, IDs: []uint64{0}}},
		{"despawn count 0", PassiveDespawn{ServerTick: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateServerPacket(StatePlay, test.packet); err == nil {
				t.Fatalf("%T=%+v 被接受，想要整体拒绝", test.packet, test.packet)
			}
		})
	}

	// 边界内的合法形态必须继续通过：count 恰好 1 与上限、生命 1 与满值。
	validSpawns := make([]PassiveSpawnRecord, MaxPassiveRecords)
	for index := range validSpawns {
		validSpawns[index] = PassiveSpawnRecord{
			ID: uint64(index + 1), Dimension: core.Overworld,
			Position: mgl32.Vec3{float32(index), 1, 0}, Yaw: 0, Health: core.MaxHealth,
		}
	}
	if err := validSpawn.Validate(); err != nil {
		t.Fatalf("合法 spawn 被拒绝: %v", err)
	}
	if err := (PassiveSpawn{ServerTick: 1, Spawns: validSpawns}).Validate(); err != nil {
		t.Fatalf("count=%d 的合法 spawn 被拒绝: %v", MaxPassiveRecords, err)
	}
	if err := validState.Validate(); err != nil {
		t.Fatalf("合法 state 被拒绝: %v", err)
	}
	if err := validDespawn.Validate(); err != nil {
		t.Fatalf("合法 despawn 被拒绝: %v", err)
	}
}
