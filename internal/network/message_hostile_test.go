package network

import (
	"encoding/binary"
	"encoding/hex"
	"math"
	"reflect"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
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

// TestHostileMessagesWireLayoutIsFrozen 用 golden hex 钉死三类消息的 wire
// 布局：record 字段次序、维度 i32、生命 u8 与 count u8 的位置一变即红。
func TestHostileMessagesWireLayoutIsFrozen(t *testing.T) {
	spawn := hostileSpawnMessage()
	spawn.Spawns = spawn.Spawns[:1]
	state := hostileStateMessage()
	state.States = state.States[:1]
	despawn := hostileDespawnMessage()
	despawn.IDs = despawn.IDs[:1]
	tests := []struct {
		name    string
		packet  ServerPacket
		wantID  uint32
		wantHex string
	}{
		// u64 tick + count 1 + [u64 ID + i32 dimension + 3×f32 position +
		// f32 yaw + u8 health]，全部 little-endian。
		{"spawn", spawn, 22, "0807060504030201" + "01" +
			"0700000000000000" + "00000000" +
			"000020400000803f000050c0" + "0000a03f" + "0e"},
		// u64 tick + count 1 + [u64 ID + 3×f32 position + 3×f32 velocity +
		// f32 yaw + u8 health]。
		{"state", state, 23, "0807060504030201" + "01" +
			"0700000000000000" +
			"000020400000803f000050c0" +
			"0000003f0000a0bf00000000" + "0000a03f" + "0d"},
		// u64 tick + count 1 + u64 ID。
		{"despawn", despawn, 24, "0807060504030201" + "01" + "0700000000000000"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotID, got, err := encodeServerControlPayload(StatePlay, tc.packet)
			if err != nil || gotID != tc.wantID || hex.EncodeToString(got) != tc.wantHex {
				t.Fatalf("%T id=%d payload=%s err=%v，想要 id=%d payload=%s", tc.packet, gotID, hex.EncodeToString(got), err, tc.wantID, tc.wantHex)
			}
			round, err := decodeServerControlPayload(StatePlay, gotID, got)
			if err != nil || !reflect.DeepEqual(round, tc.packet) {
				t.Fatalf("round=(%#v, %v)，想要 %#v", round, err, tc.packet)
			}
		})
	}
}

// TestHostileMessageIDsAreFrozen 钉死夜行者三类消息的最终编号：S→C 22/23/24
// （21 已被 `CraftingState` 实占；25 已被 `CombatHit` 实占；`serverPacketID` 与 `serverPacketForID` 两处
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
	for _, id := range []uint32{22, 23, 24} {
		if _, ok := serverPacketForID(StatePlay, id); !ok {
			t.Fatalf("Play server packet ID %d 未注册", id)
		}
	}
	if _, ok := serverPacketForID(StatePlay, 25); !ok {
		t.Fatal("Play server packet ID 25 必须已分配给 CombatHit")
	}
	if _, ok := serverPacketForID(StatePlay, 26); ok {
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
	validSpawns := make([]HostileSpawnRecord, maxHostileRecords)
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
		t.Fatalf("count=%d 的合法 spawn 被拒绝: %v", maxHostileRecords, err)
	}
	if err := validState.Validate(); err != nil {
		t.Fatalf("合法 state 被拒绝: %v", err)
	}
	if err := validDespawn.Validate(); err != nil {
		t.Fatalf("合法 despawn 被拒绝: %v", err)
	}
}

// TestHostileMessagesDecodeRejectsInvalidWire 在 wire 层（解码端）复验同一
// 组拒绝：校验必须落在解码路径上，而不是只守卫内存中的构造入口。
func TestHostileMessagesDecodeRejectsInvalidWire(t *testing.T) {
	encode := func(packet ServerPacket) []byte {
		_, payload, err := encodeServerControlPayload(StatePlay, packet)
		if err != nil {
			t.Fatalf("编码合法夹具: %v", err)
		}
		return payload
	}
	spawnBase := encode(hostileSpawnMessage())
	stateBase := encode(hostileStateMessage())
	despawnBase := encode(hostileDespawnMessage())

	// recordSize 分别为 spawn 29（8+4+12+4+1）、state 37（8+12+12+4+1）、
	// despawn 8；头部固定 9 字节（u64 tick + u8 count）。
	spawnRecord, stateRecord, despawnRecord := 29, 37, 8
	mutateID := func(payload []byte, recordSize, index int, id uint64) {
		offset := 9 + index*recordSize
		for byteIndex := 0; byteIndex < 8; byteIndex++ {
			payload[offset+byteIndex] = byte(id >> (8 * byteIndex))
		}
	}
	mutateCount := func(payload []byte, count uint8) { payload[8] = count }

	duplicate := append([]byte(nil), spawnBase...)
	mutateID(duplicate, spawnRecord, 1, 7)
	descending := append([]byte(nil), spawnBase...)
	mutateID(descending, spawnRecord, 0, 20)
	despawnDescending := append([]byte(nil), despawnBase...)
	mutateID(despawnDescending, despawnRecord, 0, 12)
	mutateID(despawnDescending, despawnRecord, 2, 7)
	zeroID := append([]byte(nil), spawnBase...)
	mutateID(zeroID, spawnRecord, 2, 0)
	spawnNaN := append([]byte(nil), spawnBase...)
	// 第一条记录 position.X 的偏移：9 + 8(ID) + 4(dimension) = 21。
	binary.LittleEndian.PutUint32(spawnNaN[21:], math.Float32bits(float32(math.NaN())))
	spawnHealth := append([]byte(nil), spawnBase...)
	// 第一条记录 health 的偏移：9 + 29 - 1 = 37。
	spawnHealth[9+spawnRecord-1] = 0
	spawnDimension := append([]byte(nil), spawnBase...)
	// 第一条记录 dimension 的偏移：9 + 8 = 17。
	binary.LittleEndian.PutUint32(spawnDimension[17:], 5)
	spawnCount65 := append([]byte(nil), spawnBase...)
	mutateCount(spawnCount65, 65)
	spawnCount0 := append([]byte(nil), spawnBase...)
	mutateCount(spawnCount0, 0)
	stateInfVelocity := append([]byte(nil), stateBase...)
	// 第一条记录 velocity.Y 的偏移：9 + 8(ID) + 12(position) + 4 = 33。
	binary.LittleEndian.PutUint32(stateInfVelocity[33:], math.Float32bits(float32(math.Inf(-1))))
	stateNaNYaw := append([]byte(nil), stateBase...)
	// 第一条记录 yaw 的偏移：9 + 8 + 12 + 12 = 41。
	binary.LittleEndian.PutUint32(stateNaNYaw[41:], math.Float32bits(float32(math.NaN())))
	stateHealth21 := append([]byte(nil), stateBase...)
	stateHealth21[9+stateRecord-1] = core.MaxHealth + 1
	despawnCount65 := append([]byte(nil), despawnBase...)
	mutateCount(despawnCount65, 65)
	// 尾随字节：在合法 despawn 载荷后多补 1 字节。
	trailing := append(append([]byte(nil), despawnBase...), 0)

	tests := []struct {
		name    string
		id      uint32
		payload []byte
	}{
		{"spawn 重复 ID", 22, duplicate},
		{"spawn 逆序 ID", 22, descending},
		{"spawn 零 ID", 22, zeroID},
		{"spawn NaN position", 22, spawnNaN},
		{"spawn health 0", 22, spawnHealth},
		{"spawn 非法维度", 22, spawnDimension},
		{"spawn count 65", 22, spawnCount65},
		{"spawn count 0", 22, spawnCount0},
		{"spawn 截断", 22, spawnBase[:len(spawnBase)-1]},
		{"state Inf velocity", 23, stateInfVelocity},
		{"state NaN yaw", 23, stateNaNYaw},
		{"state health 超上限", 23, stateHealth21},
		{"state 截断", 23, stateBase[:len(stateBase)-1]},
		{"despawn 逆序 ID", 24, despawnDescending},
		{"despawn count 65", 24, despawnCount65},
		{"despawn 尾随", 24, trailing},
		{"despawn 截断", 24, despawnBase[:len(despawnBase)-1]},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if packet, err := decodeServerControlPayload(StatePlay, test.id, test.payload); err == nil {
				t.Fatalf("payload %x 被解码为 %#v，想要整体拒绝", test.payload, packet)
			}
		})
	}
}

// TestHostileMessagesWireLimitsAreFrozen 钉死三类消息的固定 wire 上限与
// record 步长推导：字段宽度一变这里先红，防止上限与布局静默漂移。
func TestHostileMessagesWireLimitsAreFrozen(t *testing.T) {
	if maxHostileRecords != 64 {
		t.Fatalf("maxHostileRecords=%d，想要 64", maxHostileRecords)
	}
	tests := []struct {
		name        string
		recordBytes int
		wantMax     int
	}{
		// 8 tick + 1 count + 64×[8 ID + 4 dimension + 12 position + 4 yaw + 1 health] = 1865。
		{"spawn", 29, 1865},
		// 8 tick + 1 count + 64×[8 ID + 12 position + 12 velocity + 4 yaw + 1 health] = 2377。
		{"state", 37, 2377},
		// 8 tick + 1 count + 64×8 ID = 521。
		{"despawn", 8, 521},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := 9 + maxHostileRecords*test.recordBytes; got != test.wantMax {
				t.Fatalf("%s 载荷上限=%d，想要 %d", test.name, got, test.wantMax)
			}
		})
	}
	_, spawnPayload, err := encodeServerControlPayload(StatePlay, hostileSpawnMessage())
	if err != nil {
		t.Fatal(err)
	}
	if len(spawnPayload) != 9+3*29 {
		t.Fatalf("spawn 载荷=%d 字节，想要 %d", len(spawnPayload), 9+3*29)
	}
}

// TestHostileMessageCodecRoundTripsProperty 对三类消息做编码-解码性质测试：
// 全字段往返一致，类型不匹配立即失败。
func TestHostileMessageCodecRoundTripsProperty(t *testing.T) {
	packets := []ServerPacket{hostileSpawnMessage(), hostileStateMessage(), hostileDespawnMessage()}
	for _, packet := range packets {
		id, payload, err := encodeServerControlPayload(StatePlay, packet)
		if err != nil {
			t.Fatalf("%T 编码: %v", packet, err)
		}
		decoded, err := decodeServerControlPayload(StatePlay, id, payload)
		if err != nil {
			t.Fatalf("%T 解码: %v", packet, err)
		}
		if !reflect.DeepEqual(decoded, packet) {
			t.Fatalf("%T 往返不一致: %#v", packet, decoded)
		}
	}
}

func FuzzHostileMessageCodec(f *testing.F) {
	for _, packet := range []ServerPacket{
		hostileSpawnMessage(),
		hostileStateMessage(),
		hostileDespawnMessage(),
	} {
		id, payload, err := encodeServerControlPayload(StatePlay, packet)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(id, payload)
	}
	// 边界种子：count 64 的满载 spawn；再补两个已知非法形态（count 0、
	// count 65 的最小 despawn 载荷），帮助 fuzzer 直接落到拒绝路径。
	full := hostileSpawnMessage()
	full.Spawns = make([]HostileSpawnRecord, maxHostileRecords)
	for index := range full.Spawns {
		full.Spawns[index] = HostileSpawnRecord{
			ID: uint64(index + 1), Dimension: core.Overworld,
			Position: mgl32.Vec3{float32(index), 64, 0}, Yaw: 0.5, Health: core.MaxHealth,
		}
	}
	id, payload, err := encodeServerControlPayload(StatePlay, full)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(id, payload)
	_, minimal, err := encodeServerControlPayload(StatePlay, HostileDespawn{ServerTick: 9, IDs: []uint64{1}})
	if err != nil {
		f.Fatal(err)
	}
	// count 0：合法载荷把 count 字节改写为 0。
	f.Add(uint32(24), append(append([]byte(nil), minimal[:8]...), 0x00))
	// count 65：count 字节直接写 0x41。
	f.Add(uint32(24), append(append([]byte(nil), minimal[:8]...), 0x41))
	f.Fuzz(func(t *testing.T, packetID uint32, payload []byte) {
		packet, err := decodeServerControlPayload(StatePlay, packetID, payload)
		if err != nil {
			return
		}
		switch packet.(type) {
		case HostileSpawn, HostileState, HostileDespawn:
		default:
			return
		}
		reencodedID, reencoded, reencodeErr := encodeServerControlPayload(StatePlay, packet)
		if reencodeErr != nil {
			t.Fatalf("解码成功的载荷重新编码失败: %v", reencodeErr)
		}
		if reencodedID != packetID || string(reencoded) != string(payload) {
			t.Fatalf("round trip 不稳定: id %d->%d payload %x->%x",
				packetID, reencodedID, payload, reencoded)
		}
	})
}
