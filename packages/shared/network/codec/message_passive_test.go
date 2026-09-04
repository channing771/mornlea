package codec

import (
	"encoding/binary"
	"encoding/hex"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
	"math"
	"reflect"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// passiveSpawnFixture 返回 3 条字段各异、ID 严格升序的合法 spawn 记录：
// 生命取非零非满的中间值，保证「字段根本没搬运」与默认值不可分辨。
func passiveSpawnFixture() []protocol.PassiveSpawnRecord {
	return []protocol.PassiveSpawnRecord{
		{ID: 5, Dimension: core.Overworld, Position: mgl32.Vec3{1.5, 2, -1.25}, Yaw: 0.75, Health: 10},
		{ID: 8, Dimension: core.Overworld, Position: mgl32.Vec3{-4.5, 63.5, 6.25}, Yaw: -1.5, Health: core.MaxHealth},
		{ID: 11, Dimension: core.Overworld, Position: mgl32.Vec3{12.5, 68, -6.5}, Yaw: 2, Health: 1},
	}
}

// passiveStateFixture 返回 2 条 ID 严格升序的合法 state 记录：速度取非零值，
// 保证速度分量的搬运与丢弃可分辨。
func passiveStateFixture() []protocol.PassiveStateRecord {
	return []protocol.PassiveStateRecord{
		{ID: 5, Position: mgl32.Vec3{1.5, 2, -1.25}, Velocity: mgl32.Vec3{0.25, -0.5, 0}, Yaw: 0.75, Health: 9},
		{ID: 8, Position: mgl32.Vec3{-4.5, 63.5, 6.25}, Velocity: mgl32.Vec3{0, 0.5, 1.5}, Yaw: -1.5, Health: 6},
	}
}

func passiveSpawnMessage() protocol.PassiveSpawn {
	return protocol.PassiveSpawn{ServerTick: 0x0102030405060708, Spawns: passiveSpawnFixture()}
}

func passiveStateMessage() protocol.PassiveState {
	return protocol.PassiveState{ServerTick: 0x0102030405060708, States: passiveStateFixture()}
}

func passiveDespawnMessage() protocol.PassiveDespawn {
	return protocol.PassiveDespawn{ServerTick: 0x0102030405060708, IDs: []uint64{5, 8, 11}}
}

// TestPassiveMessagesWireLayoutIsFrozen 用 golden hex 钉死三类消息的 wire
// 布局：record 字段次序、维度 i32、生命 u8 与 count u8 的位置一变即红。
func TestPassiveMessagesWireLayoutIsFrozen(t *testing.T) {
	spawn := passiveSpawnMessage()
	spawn.Spawns = spawn.Spawns[:1]
	state := passiveStateMessage()
	state.States = state.States[:1]
	despawn := passiveDespawnMessage()
	despawn.IDs = despawn.IDs[:1]
	tests := []struct {
		name    string
		packet  protocol.ServerPacket
		wantID  uint32
		wantHex string
	}{
		// u64 tick + count 1 + [u64 ID + i32 dimension + 3×f32 position +
		// f32 yaw + u8 health]，全部 little-endian。
		{"spawn", spawn, 26, "0807060504030201" + "01" +
			"0500000000000000" + "00000000" +
			"0000c03f000000400000a0bf" + "0000403f" + "0a"},
		// u64 tick + count 1 + [u64 ID + 3×f32 position + 3×f32 velocity +
		// f32 yaw + u8 health]。
		{"state", state, 27, "0807060504030201" + "01" +
			"0500000000000000" +
			"0000c03f000000400000a0bf" +
			"0000803e000000bf00000000" + "0000403f" + "09"},
		// u64 tick + count 1 + u64 ID。
		{"despawn", despawn, 28, "0807060504030201" + "01" + "0500000000000000"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotID, got, err := encodeServerControlPayload(protocol.StatePlay, tc.packet)
			if err != nil || gotID != tc.wantID || hex.EncodeToString(got) != tc.wantHex {
				t.Fatalf("%T id=%d payload=%s err=%v，想要 id=%d payload=%s", tc.packet, gotID, hex.EncodeToString(got), err, tc.wantID, tc.wantHex)
			}
			round, err := decodeServerControlPayload(protocol.StatePlay, gotID, got)
			if err != nil || !reflect.DeepEqual(round, tc.packet) {
				t.Fatalf("round=(%#v, %v)，想要 %#v", round, err, tc.packet)
			}
		})
	}
}

// TestPassiveMessagesDecodeRejectsInvalidWire 在 wire 层（解码端）复验同一
// 组拒绝：校验必须落在解码路径上，而不是只守卫内存中的构造入口。
func TestPassiveMessagesDecodeRejectsInvalidWire(t *testing.T) {
	encode := func(packet protocol.ServerPacket) []byte {
		_, payload, err := encodeServerControlPayload(protocol.StatePlay, packet)
		if err != nil {
			t.Fatalf("编码合法夹具: %v", err)
		}
		return payload
	}
	spawnBase := encode(passiveSpawnMessage())
	stateBase := encode(passiveStateMessage())
	despawnBase := encode(passiveDespawnMessage())

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
	mutateID(duplicate, spawnRecord, 1, 5)
	descending := append([]byte(nil), spawnBase...)
	mutateID(descending, spawnRecord, 0, 20)
	despawnDescending := append([]byte(nil), despawnBase...)
	mutateID(despawnDescending, despawnRecord, 0, 11)
	mutateID(despawnDescending, despawnRecord, 2, 5)
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
		{"spawn 重复 ID", 26, duplicate},
		{"spawn 逆序 ID", 26, descending},
		{"spawn 零 ID", 26, zeroID},
		{"spawn NaN position", 26, spawnNaN},
		{"spawn health 0", 26, spawnHealth},
		{"spawn 非法维度", 26, spawnDimension},
		{"spawn count 65", 26, spawnCount65},
		{"spawn count 0", 26, spawnCount0},
		{"spawn 截断", 26, spawnBase[:len(spawnBase)-1]},
		{"state Inf velocity", 27, stateInfVelocity},
		{"state NaN yaw", 27, stateNaNYaw},
		{"state health 超上限", 27, stateHealth21},
		{"state 截断", 27, stateBase[:len(stateBase)-1]},
		{"despawn 逆序 ID", 28, despawnDescending},
		{"despawn count 65", 28, despawnCount65},
		{"despawn 尾随", 28, trailing},
		{"despawn 截断", 28, despawnBase[:len(despawnBase)-1]},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if packet, err := decodeServerControlPayload(protocol.StatePlay, test.id, test.payload); err == nil {
				t.Fatalf("payload %x 被解码为 %#v，想要整体拒绝", test.payload, packet)
			}
		})
	}
}

// TestPassiveMessagesWireLimitsAreFrozen 钉死三类消息的固定 wire 上限与
// record 步长推导：字段宽度一变这里先红，防止上限与布局静默漂移。
func TestPassiveMessagesWireLimitsAreFrozen(t *testing.T) {
	if protocol.MaxPassiveRecords != 64 {
		t.Fatalf("protocol.MaxPassiveRecords=%d，想要 64", protocol.MaxPassiveRecords)
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
			if got := 9 + protocol.MaxPassiveRecords*test.recordBytes; got != test.wantMax {
				t.Fatalf("%s 载荷上限=%d，想要 %d", test.name, got, test.wantMax)
			}
		})
	}
	_, spawnPayload, err := encodeServerControlPayload(protocol.StatePlay, passiveSpawnMessage())
	if err != nil {
		t.Fatal(err)
	}
	if len(spawnPayload) != 9+3*29 {
		t.Fatalf("spawn 载荷=%d 字节，想要 %d", len(spawnPayload), 9+3*29)
	}
}

// TestPassiveMessageCodecRoundTripsProperty 对三类消息做编码-解码性质测试：
// 全字段往返一致，类型不匹配立即失败。
func TestPassiveMessageCodecRoundTripsProperty(t *testing.T) {
	packets := []protocol.ServerPacket{passiveSpawnMessage(), passiveStateMessage(), passiveDespawnMessage()}
	for _, packet := range packets {
		id, payload, err := encodeServerControlPayload(protocol.StatePlay, packet)
		if err != nil {
			t.Fatalf("%T 编码: %v", packet, err)
		}
		decoded, err := decodeServerControlPayload(protocol.StatePlay, id, payload)
		if err != nil {
			t.Fatalf("%T 解码: %v", packet, err)
		}
		if !reflect.DeepEqual(decoded, packet) {
			t.Fatalf("%T 往返不一致: %#v", packet, decoded)
		}
	}
}

func FuzzPassiveMessageCodec(f *testing.F) {
	for _, packet := range []protocol.ServerPacket{
		passiveSpawnMessage(),
		passiveStateMessage(),
		passiveDespawnMessage(),
	} {
		id, payload, err := encodeServerControlPayload(protocol.StatePlay, packet)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(id, payload)
	}
	// 边界种子：count 64 的满载 spawn；再补两个已知非法形态（count 0、
	// count 65 的最小 despawn 载荷），帮助 fuzzer 直接落到拒绝路径。
	full := passiveSpawnMessage()
	full.Spawns = make([]protocol.PassiveSpawnRecord, protocol.MaxPassiveRecords)
	for index := range full.Spawns {
		full.Spawns[index] = protocol.PassiveSpawnRecord{
			ID: uint64(index + 1), Dimension: core.Overworld,
			Position: mgl32.Vec3{float32(index), 64, 0}, Yaw: 0.5, Health: core.MaxHealth,
		}
	}
	id, payload, err := encodeServerControlPayload(protocol.StatePlay, full)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(id, payload)
	_, minimal, err := encodeServerControlPayload(protocol.StatePlay, protocol.PassiveDespawn{ServerTick: 9, IDs: []uint64{1}})
	if err != nil {
		f.Fatal(err)
	}
	// count 0：合法载荷把 count 字节改写为 0。
	f.Add(uint32(28), append(append([]byte(nil), minimal[:8]...), 0x00))
	// count 65：count 字节直接写 0x41。
	f.Add(uint32(28), append(append([]byte(nil), minimal[:8]...), 0x41))
	f.Fuzz(func(t *testing.T, packetID uint32, payload []byte) {
		packet, err := decodeServerControlPayload(protocol.StatePlay, packetID, payload)
		if err != nil {
			return
		}
		switch packet.(type) {
		case protocol.PassiveSpawn, protocol.PassiveState, protocol.PassiveDespawn:
		default:
			return
		}
		reencodedID, reencoded, reencodeErr := encodeServerControlPayload(protocol.StatePlay, packet)
		if reencodeErr != nil {
			t.Fatalf("解码成功的载荷重新编码失败: %v", reencodeErr)
		}
		if reencodedID != packetID || string(reencoded) != string(payload) {
			t.Fatalf("round trip 不稳定: id %d->%d payload %x->%x",
				packetID, reencodedID, payload, reencoded)
		}
	})
}
