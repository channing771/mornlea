package protocol

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// projectileSpawnFixture 返回 3 条字段各异、ID 严格升序的合法 spawn 记录：
// 弹种 1/0/1 混排（首条取非零值，golden 的 kind 字节与零填充可分辨）、维度
// 覆盖两个合法维度，保证 kind 与 dimension 的搬运与丢弃可分辨。
func projectileSpawnFixture() []ProjectileSpawnRecord {
	return []ProjectileSpawnRecord{
		{ID: 7, Kind: ProjectileKindArrow, Dimension: core.Overworld,
			Position: mgl32.Vec3{2.5, 1, -3.25}, Velocity: mgl32.Vec3{0.5, -1.25, 0}},
		{ID: 9, Kind: ProjectileKindShard, Dimension: core.Overworld,
			Position: mgl32.Vec3{-8.5, 65.5, 12.75}, Velocity: mgl32.Vec3{0, 0.25, 3}},
		{ID: 12, Kind: ProjectileKindArrow, Dimension: core.Depths,
			Position: mgl32.Vec3{30.5, 70, -3.25}, Velocity: mgl32.Vec3{3, 0, -0.5}},
	}
}

// projectileStateFixture 返回 2 条 ID 严格升序的合法 state 记录。
func projectileStateFixture() []ProjectileStateRecord {
	return []ProjectileStateRecord{
		{ID: 7, Position: mgl32.Vec3{2.5, 1, -3.25}},
		{ID: 9, Position: mgl32.Vec3{-8.5, 65.5, 12.75}},
	}
}

func projectileSpawnMessage() ProjectileSpawn {
	return ProjectileSpawn{ServerTick: 0x0102030405060708, Spawns: projectileSpawnFixture()}
}

func projectileStateMessage() ProjectileState {
	return ProjectileState{ServerTick: 0x0102030405060708, States: projectileStateFixture()}
}

func projectileDespawnMessage() ProjectileDespawn {
	return ProjectileDespawn{ServerTick: 0x0102030405060708, IDs: []uint64{7, 9, 12}}
}

// TestProjectileMessageIDsAreFrozen 钉死投射物三类消息的最终编号：S→C
// 29/30/31（28 已被 `PassiveDespawn` 实占；`ServerPacketID` 与
// `ServerPacketForID` 两处对称）。上界断言写成「末项 +1」，下次追加 packet
// 时它跟着末项走。
func TestProjectileMessageIDsAreFrozen(t *testing.T) {
	assertServerRegistry(t, []struct {
		state  State
		packet ServerPacket
		id     uint32
	}{
		{StatePlay, ProjectileSpawn{}, 29},
		{StatePlay, ProjectileState{}, 30},
		{StatePlay, ProjectileDespawn{}, 31},
	})
	for _, id := range []uint32{22, 23, 24, 25, 26, 27, 28, 29, 30, 31} {
		if _, ok := ServerPacketForID(StatePlay, id); !ok {
			t.Fatalf("Play server packet ID %d 未注册", id)
		}
	}
	if _, ok := ServerPacketForID(StatePlay, 31+1); ok {
		t.Fatal("Play server packet ID 32 必须保持未分配")
	}
}

func TestProjectileMessagesValidateRejectsInvalidRecords(t *testing.T) {
	validSpawn := projectileSpawnMessage()
	validState := projectileStateMessage()
	validDespawn := projectileDespawnMessage()

	tests := []struct {
		name   string
		packet ServerPacket
	}{
		{"spawn 重复 ID", ProjectileSpawn{ServerTick: 1, Spawns: []ProjectileSpawnRecord{
			projectileSpawnFixture()[0], projectileSpawnFixture()[0],
		}}},
		{"spawn 逆序 ID", ProjectileSpawn{ServerTick: 1, Spawns: []ProjectileSpawnRecord{
			projectileSpawnFixture()[1], projectileSpawnFixture()[0],
		}}},
		{"spawn 零 ID", ProjectileSpawn{ServerTick: 1, Spawns: []ProjectileSpawnRecord{{
			Kind: ProjectileKindShard, Dimension: core.Overworld,
			Position: mgl32.Vec3{1, 1, 1}, Velocity: mgl32.Vec3{0, 0, 0},
		}}}},
		{"spawn NaN position", ProjectileSpawn{ServerTick: 1, Spawns: []ProjectileSpawnRecord{{
			ID: 1, Kind: ProjectileKindShard, Dimension: core.Overworld,
			Position: mgl32.Vec3{float32(math.NaN()), 1, 1}, Velocity: mgl32.Vec3{0, 0, 0},
		}}}},
		{"spawn Inf velocity", ProjectileSpawn{ServerTick: 1, Spawns: []ProjectileSpawnRecord{{
			ID: 1, Kind: ProjectileKindShard, Dimension: core.Overworld,
			Position: mgl32.Vec3{1, 1, 1}, Velocity: mgl32.Vec3{0, float32(math.Inf(1)), 0},
		}}}},
		{"spawn kind 2", ProjectileSpawn{ServerTick: 1, Spawns: []ProjectileSpawnRecord{{
			ID: 1, Kind: 2, Dimension: core.Overworld,
			Position: mgl32.Vec3{1, 1, 1}, Velocity: mgl32.Vec3{0, 0, 0},
		}}}},
		{"spawn 非法维度", ProjectileSpawn{ServerTick: 1, Spawns: []ProjectileSpawnRecord{{
			ID: 1, Kind: ProjectileKindShard, Dimension: core.DimensionID(2),
			Position: mgl32.Vec3{1, 1, 1}, Velocity: mgl32.Vec3{0, 0, 0},
		}}}},
		{"spawn count 0", ProjectileSpawn{ServerTick: 1}},
		{"state 重复 ID", ProjectileState{ServerTick: 1, States: []ProjectileStateRecord{
			projectileStateFixture()[0], projectileStateFixture()[0],
		}}},
		{"state 逆序 ID", ProjectileState{ServerTick: 1, States: []ProjectileStateRecord{
			projectileStateFixture()[1], projectileStateFixture()[0],
		}}},
		{"state 零 ID", ProjectileState{ServerTick: 1, States: []ProjectileStateRecord{{
			Position: mgl32.Vec3{1, 1, 1},
		}}}},
		{"state NaN position", ProjectileState{ServerTick: 1, States: []ProjectileStateRecord{{
			ID: 1, Position: mgl32.Vec3{1, float32(math.NaN()), 1},
		}}}},
		{"state Inf position", ProjectileState{ServerTick: 1, States: []ProjectileStateRecord{{
			ID: 1, Position: mgl32.Vec3{1, 1, float32(math.Inf(-1))},
		}}}},
		{"state count 0", ProjectileState{ServerTick: 1}},
		{"despawn 重复 ID", ProjectileDespawn{ServerTick: 1, IDs: []uint64{7, 7}}},
		{"despawn 逆序 ID", ProjectileDespawn{ServerTick: 1, IDs: []uint64{9, 7}}},
		{"despawn 零 ID", ProjectileDespawn{ServerTick: 1, IDs: []uint64{0}}},
		{"despawn count 0", ProjectileDespawn{ServerTick: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateServerPacket(StatePlay, test.packet); err == nil {
				t.Fatalf("%T=%+v 被接受，想要整体拒绝", test.packet, test.packet)
			}
		})
	}

	// 上界拒绝：count 恰好超出上限一条必须被拒（spawn/state/despawn 同规）。
	oversizedIDs := make([]uint64, MaxProjectileRecords+1)
	for index := range oversizedIDs {
		oversizedIDs[index] = uint64(index + 1)
	}
	if err := (ProjectileDespawn{ServerTick: 1, IDs: oversizedIDs}).Validate(); err == nil {
		t.Fatal("despawn count 超上限被接受，想要整体拒绝")
	}

	// 边界内的合法形态必须继续通过：count 恰好 1 与上限 128、kind 两值、
	// 维度两值都在域内。
	if err := validSpawn.Validate(); err != nil {
		t.Fatalf("合法 spawn 被拒绝: %v", err)
	}
	fullSpawns := make([]ProjectileSpawnRecord, MaxProjectileRecords)
	for index := range fullSpawns {
		fullSpawns[index] = ProjectileSpawnRecord{
			ID: uint64(index + 1), Kind: ProjectileKindShard, Dimension: core.Overworld,
			Position: mgl32.Vec3{float32(index), 64, 0}, Velocity: mgl32.Vec3{0, -1, 0},
		}
	}
	if err := (ProjectileSpawn{ServerTick: 1, Spawns: fullSpawns}).Validate(); err != nil {
		t.Fatalf("count=%d 的合法 spawn 被拒绝: %v", MaxProjectileRecords, err)
	}
	if err := validState.Validate(); err != nil {
		t.Fatalf("合法 state 被拒绝: %v", err)
	}
	fullStates := make([]ProjectileStateRecord, MaxProjectileRecords)
	for index := range fullStates {
		fullStates[index] = ProjectileStateRecord{
			ID: uint64(index + 1), Position: mgl32.Vec3{float32(index), 64, 0},
		}
	}
	if err := (ProjectileState{ServerTick: 1, States: fullStates}).Validate(); err != nil {
		t.Fatalf("count=%d 的合法 state 被拒绝: %v", MaxProjectileRecords, err)
	}
	if err := validDespawn.Validate(); err != nil {
		t.Fatalf("合法 despawn 被拒绝: %v", err)
	}
}
