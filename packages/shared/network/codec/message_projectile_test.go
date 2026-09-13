package codec

import (
	"encoding/binary"
	"encoding/hex"
	"math"
	"reflect"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// projectileSpawnFixture 返回 3 条字段各异、ID 严格升序的合法 spawn 记录：
// 弹种 1/0/1 混排（首条取非零值，golden 的 kind 字节与零填充可分辨）、维度
// 覆盖两个合法维度，保证 kind 与 dimension 的搬运与丢弃可分辨。与 protocol
// 侧同值夹具是按被测主体拆分的机械结果，两侧各自钉死取值。
func projectileSpawnFixture() []protocol.ProjectileSpawnRecord {
	return []protocol.ProjectileSpawnRecord{
		{ID: 7, Kind: protocol.ProjectileKindArrow, Dimension: core.Overworld,
			Position: mgl32.Vec3{2.5, 1, -3.25}, Velocity: mgl32.Vec3{0.5, -1.25, 0}},
		{ID: 9, Kind: protocol.ProjectileKindShard, Dimension: core.Overworld,
			Position: mgl32.Vec3{-8.5, 65.5, 12.75}, Velocity: mgl32.Vec3{0, 0.25, 3}},
		{ID: 12, Kind: protocol.ProjectileKindArrow, Dimension: core.Depths,
			Position: mgl32.Vec3{30.5, 70, -3.25}, Velocity: mgl32.Vec3{3, 0, -0.5}},
	}
}

// projectileStateFixture 返回 2 条 ID 严格升序的合法 state 记录。
func projectileStateFixture() []protocol.ProjectileStateRecord {
	return []protocol.ProjectileStateRecord{
		{ID: 7, Position: mgl32.Vec3{2.5, 1, -3.25}},
		{ID: 9, Position: mgl32.Vec3{-8.5, 65.5, 12.75}},
	}
}

func projectileSpawnMessage() protocol.ProjectileSpawn {
	return protocol.ProjectileSpawn{ServerTick: 0x0102030405060708, Spawns: projectileSpawnFixture()}
}

func projectileStateMessage() protocol.ProjectileState {
	return protocol.ProjectileState{ServerTick: 0x0102030405060708, States: projectileStateFixture()}
}

func projectileDespawnMessage() protocol.ProjectileDespawn {
	return protocol.ProjectileDespawn{ServerTick: 0x0102030405060708, IDs: []uint64{7, 9, 12}}
}

// TestProjectileMessagesWireLayoutIsFrozen 用 golden hex 钉死三类消息的 wire
// 布局：record 字段次序（ID、kind、dimension、position、velocity）、维度 i32、
// 弹种 u8 与 count u8 的位置一变即红。
func TestProjectileMessagesWireLayoutIsFrozen(t *testing.T) {
	spawn := projectileSpawnMessage()
	spawn.Spawns = spawn.Spawns[:1]
	state := projectileStateMessage()
	state.States = state.States[:1]
	despawn := projectileDespawnMessage()
	despawn.IDs = despawn.IDs[:1]
	tests := []struct {
		name    string
		packet  protocol.ServerPacket
		wantID  uint32
		wantHex string
	}{
		// u64 tick + count 1 + [u64 ID + u8 kind + i32 dimension + 3×f32
		// position + 3×f32 velocity]，全部 little-endian。
		{"spawn", spawn, 29, "0807060504030201" + "01" +
			"0700000000000000" + "01" + "00000000" +
			"000020400000803f000050c0" + "0000003f0000a0bf00000000"},
		// u64 tick + count 1 + [u64 ID + 3×f32 position]。
		{"state", state, 30, "0807060504030201" + "01" +
			"0700000000000000" +
			"000020400000803f000050c0"},
		// u64 tick + count 1 + u64 ID。
		{"despawn", despawn, 31, "0807060504030201" + "01" + "0700000000000000"},
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

// TestProjectileMessagesDecodeRejectsInvalidWire 在 wire 层（解码端）复验同一
// 组拒绝：校验必须落在解码路径上，而不是只守卫内存中的构造入口。
func TestProjectileMessagesDecodeRejectsInvalidWire(t *testing.T) {
	encode := func(packet protocol.ServerPacket) []byte {
		_, payload, err := encodeServerControlPayload(protocol.StatePlay, packet)
		if err != nil {
			t.Fatalf("编码合法夹具: %v", err)
		}
		return payload
	}
	spawnBase := encode(projectileSpawnMessage())
	stateBase := encode(projectileStateMessage())
	despawnBase := encode(projectileDespawnMessage())

	// recordSize 分别为 spawn 37（8+1+4+12+12）、despawn 8；state record 为
	// 20（8+12），其浮点偏移在用例注释内逐字推导。头部固定 9 字节
	// （u64 tick + u8 count）。
	spawnRecord, despawnRecord := 37, 8
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
	// 第一条记录 position.X 的偏移：9 + 8(ID) + 1(kind) + 4(dimension) = 22。
	binary.LittleEndian.PutUint32(spawnNaN[22:], math.Float32bits(float32(math.NaN())))
	spawnInf := append([]byte(nil), spawnBase...)
	// 第一条记录 velocity.Y 的偏移：9 + 8 + 1 + 4 + 12 + 4 = 38。
	binary.LittleEndian.PutUint32(spawnInf[38:], math.Float32bits(float32(math.Inf(1))))
	spawnKind := append([]byte(nil), spawnBase...)
	// 第一条记录 kind 的偏移：9 + 8 = 17。
	spawnKind[17] = 2
	spawnDimension := append([]byte(nil), spawnBase...)
	// 第一条记录 dimension 的偏移：9 + 8 + 1 = 18。
	binary.LittleEndian.PutUint32(spawnDimension[18:], 5)
	spawnCount129 := append([]byte(nil), spawnBase...)
	mutateCount(spawnCount129, 129)
	spawnCount0 := append([]byte(nil), spawnBase...)
	mutateCount(spawnCount0, 0)
	stateNaN := append([]byte(nil), stateBase...)
	// 第一条记录 position.X 的偏移：9 + 8(ID) = 17。
	binary.LittleEndian.PutUint32(stateNaN[17:], math.Float32bits(float32(math.NaN())))
	stateInf := append([]byte(nil), stateBase...)
	// 第一条记录 position.Z 的偏移：9 + 8 + 8 = 25。
	binary.LittleEndian.PutUint32(stateInf[25:], math.Float32bits(float32(math.Inf(-1))))
	stateCount129 := append([]byte(nil), stateBase...)
	mutateCount(stateCount129, 129)
	despawnCount129 := append([]byte(nil), despawnBase...)
	mutateCount(despawnCount129, 129)
	// 尾随字节：在合法 despawn 载荷后多补 1 字节。
	trailing := append(append([]byte(nil), despawnBase...), 0)

	tests := []struct {
		name    string
		id      uint32
		payload []byte
	}{
		{"spawn 重复 ID", 29, duplicate},
		{"spawn 逆序 ID", 29, descending},
		{"spawn 零 ID", 29, zeroID},
		{"spawn NaN position", 29, spawnNaN},
		{"spawn Inf velocity", 29, spawnInf},
		{"spawn kind 2", 29, spawnKind},
		{"spawn 非法维度", 29, spawnDimension},
		{"spawn count 129", 29, spawnCount129},
		{"spawn count 0", 29, spawnCount0},
		{"spawn 截断", 29, spawnBase[:len(spawnBase)-1]},
		{"state NaN position", 30, stateNaN},
		{"state Inf position", 30, stateInf},
		{"state count 129", 30, stateCount129},
		{"state 截断", 30, stateBase[:len(stateBase)-1]},
		{"despawn 逆序 ID", 31, despawnDescending},
		{"despawn count 129", 31, despawnCount129},
		{"despawn 尾随", 31, trailing},
		{"despawn 截断", 31, despawnBase[:len(despawnBase)-1]},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if packet, err := decodeServerControlPayload(protocol.StatePlay, test.id, test.payload); err == nil {
				t.Fatalf("payload %x 被解码为 %#v，想要整体拒绝", test.payload, packet)
			}
		})
	}
}

// TestProjectileMessagesWireLimitsAreFrozen 钉死三类消息的固定 wire 上限与
// record 步长推导：字段宽度一变这里先红，防止上限与布局静默漂移。
func TestProjectileMessagesWireLimitsAreFrozen(t *testing.T) {
	if protocol.MaxProjectileRecords != 128 {
		t.Fatalf("protocol.MaxProjectileRecords=%d，想要 128", protocol.MaxProjectileRecords)
	}
	tests := []struct {
		name        string
		recordBytes int
		wantMax     int
	}{
		// 8 tick + 1 count + 128×[8 ID + 1 kind + 4 dimension + 12 position + 12 velocity] = 4745。
		{"spawn", 37, 4745},
		// 8 tick + 1 count + 128×[8 ID + 12 position] = 2569。
		{"state", 20, 2569},
		// 8 tick + 1 count + 128×8 ID = 1033。
		{"despawn", 8, 1033},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := 9 + protocol.MaxProjectileRecords*test.recordBytes; got != test.wantMax {
				t.Fatalf("%s 载荷上限=%d，想要 %d", test.name, got, test.wantMax)
			}
		})
	}
	_, spawnPayload, err := encodeServerControlPayload(protocol.StatePlay, projectileSpawnMessage())
	if err != nil {
		t.Fatal(err)
	}
	if len(spawnPayload) != 9+3*37 {
		t.Fatalf("spawn 载荷=%d 字节，想要 %d", len(spawnPayload), 9+3*37)
	}
}

// TestProjectileMessageCodecRoundTripsProperty 对三类消息做编码-解码性质测试：
// 全字段往返一致，类型不匹配立即失败。
func TestProjectileMessageCodecRoundTripsProperty(t *testing.T) {
	packets := []protocol.ServerPacket{projectileSpawnMessage(), projectileStateMessage(), projectileDespawnMessage()}
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

func FuzzProjectileMessageCodec(f *testing.F) {
	for _, packet := range []protocol.ServerPacket{
		projectileSpawnMessage(),
		projectileStateMessage(),
		projectileDespawnMessage(),
	} {
		id, payload, err := encodeServerControlPayload(protocol.StatePlay, packet)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(id, payload)
	}
	// 边界种子：count 128 的满载 spawn；再补两个已知非法形态（count 0、
	// count 129 的最小 despawn 载荷），帮助 fuzzer 直接落到拒绝路径。
	full := projectileSpawnMessage()
	full.Spawns = make([]protocol.ProjectileSpawnRecord, protocol.MaxProjectileRecords)
	for index := range full.Spawns {
		full.Spawns[index] = protocol.ProjectileSpawnRecord{
			ID: uint64(index + 1), Kind: protocol.ProjectileKindShard, Dimension: core.Overworld,
			Position: mgl32.Vec3{float32(index), 64, 0}, Velocity: mgl32.Vec3{0, -1, 0},
		}
	}
	id, payload, err := encodeServerControlPayload(protocol.StatePlay, full)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(id, payload)
	_, minimal, err := encodeServerControlPayload(protocol.StatePlay, protocol.ProjectileDespawn{ServerTick: 9, IDs: []uint64{1}})
	if err != nil {
		f.Fatal(err)
	}
	// count 0：合法载荷把 count 字节改写为 0。
	f.Add(uint32(31), append(append([]byte(nil), minimal[:8]...), 0x00))
	// count 129：count 字节直接写 0x81。
	f.Add(uint32(31), append(append([]byte(nil), minimal[:8]...), 0x81))
	f.Fuzz(func(t *testing.T, packetID uint32, payload []byte) {
		packet, err := decodeServerControlPayload(protocol.StatePlay, packetID, payload)
		if err != nil {
			return
		}
		switch packet.(type) {
		case protocol.ProjectileSpawn, protocol.ProjectileState, protocol.ProjectileDespawn:
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
