package hostile

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/channing771/mornlea/internal/storage/storagedef"
	"github.com/channing771/mornlea/packages/shared/core"
)

// hostileWireOffsets 是单条 72-byte 记录内各字段的字节偏移，与
// appendHostileMob/decodeHostileMob 的固定布局一一对应。布局变化时本表
// 必须同步更新（与 companion codec 的 offset 测试同一纪律）。
const (
	hostileWireID         = 0
	hostileWireDimension  = 8
	hostileWirePosition   = 12
	hostileWireVelocity   = 24
	hostileWireOnGround   = 36
	hostileWireYaw        = 37
	hostileWireHealth     = 41
	hostileWireAttack     = 42
	hostileWireHurt       = 43
	hostileWireBurn       = 44
	hostileWireHasTarget  = 45
	hostileWirePlayerID   = 46
	hostileWireNextRepath = 62
	hostileWireDistant    = 70
)

// hostileRecordOffset 返回第 index 条记录在文件中的起始偏移。
func hostileRecordOffset(index int) int {
	return hostileHeaderLength + index*hostileRecordLength
}

// fixtureHostileTargetPlayerID 返回一个合法 UUIDv4 目标玩家 ID：byte[6]
// 高半字节为版本 4，byte[8] 高两位为变体 10。
func fixtureHostileTargetPlayerID() core.PlayerID {
	return core.PlayerID{
		0x6f, 0xce, 0x82, 0x77, 0xa9, 0x33, 0x46, 0xcb,
		0x9a, 0x1f, 0xda, 0x13, 0xb7, 0xee, 0x56, 0x44,
	}
}

// fixtureHostileRecords 返回三条字段各异的合法记录。顺序刻意逆序：编码端
// 必须按 ID 升序写出，磁盘形态与调用方传入顺序解耦（companion 先例）。
func fixtureHostileRecords() []StoredHostileMob {
	tracking := StoredHostileMob{
		ID: 0x8000000000000002, Dimension: core.Overworld,
		Position: [3]float32{-12.5, 70.25, 3.5}, Velocity: [3]float32{-1.25, 0, 0.5},
		OnGround: true, Yaw: 1.25,
		Health: 17, AttackCooldown: 3, HurtCooldown: 1, BurnCooldown: 5,
		HasTarget: true, PlayerID: fixtureHostileTargetPlayerID(),
		NextRepathTicks: 905, DistantTicks: 120,
	}
	idle := StoredHostileMob{
		ID: 0x4000000000000001, Dimension: core.Overworld,
		Position: [3]float32{0.5, 64, -9.75}, Velocity: [3]float32{0, -3.25, 0},
		OnGround: false, Yaw: -2.5,
		Health: core.MaxHealth,
	}
	far := StoredHostileMob{
		ID: 1, Dimension: core.Overworld,
		Position: [3]float32{8.5, 65.5, 9.75}, Velocity: [3]float32{2, 0, -2},
		OnGround: true, Yaw: 3,
		Health: 1, BurnCooldown: 19, DistantTicks: maxHostileDistantTicks,
	}
	return []StoredHostileMob{tracking, idle, far}
}

func fixtureHostileRecordsSorted() []StoredHostileMob {
	sorted := slices.Clone(fixtureHostileRecords())
	slices.SortFunc(sorted, func(a, b StoredHostileMob) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return sorted
}

// repairHostileCRC 按既有 companion 存档惯例重算 CRC-32C（覆盖 [8:28] 头段
// 与 [32:] payload 段），供补丁用例把非 CRC 字段改掉后重新变成「校验和合法
// 但内容非法」的输入，确保拒绝来自字段校验而不是校验和。
func repairHostileCRC(payload []byte) {
	hasher := crc32.New(crc32.MakeTable(crc32.Castagnoli))
	_, _ = hasher.Write(payload[8:28])
	_, _ = hasher.Write(payload[32:])
	binary.LittleEndian.PutUint32(payload[28:], hasher.Sum32())
}

// patchAndRepair 返回把 offset 处的 u32 改为 value 并修复 CRC 的补丁。
func patchAndRepairU32(offset int, value uint32) func([]byte) {
	return func(payload []byte) {
		binary.LittleEndian.PutUint32(payload[offset:], value)
		repairHostileCRC(payload)
	}
}

// putF32NoRepair 修改 offset 处的 f32；头段用例（CRC 检查之前就拒绝）使用
// 不修复的变体，记录字段用例必须再接 repairHostileCRC。
func putF32(offset int, value float32) func([]byte) {
	return func(payload []byte) {
		binary.LittleEndian.PutUint32(payload[offset:], math.Float32bits(value))
	}
}

func TestHostileCodecHeaderAndRecordLayout(t *testing.T) {
	record := fixtureHostileRecords()[0]
	encoded, err := Encode(HostileMobsSave{
		Revision: 7, Records: []StoredHostileMob{record},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != hostileHeaderLength+hostileRecordLength {
		t.Fatalf("单记录文件长度=%d，想要 %d", len(encoded), hostileHeaderLength+hostileRecordLength)
	}
	if string(encoded[0:4]) != "MHST" {
		t.Fatalf("magic=%q，想要 MHST", encoded[0:4])
	}
	if got := binary.LittleEndian.Uint32(encoded[4:8]); got != 1 {
		t.Fatalf("envelope=%d，想要 1", got)
	}
	if got := binary.LittleEndian.Uint32(encoded[8:12]); got != 1 {
		t.Fatalf("schema=%d，想要 1", got)
	}
	if got := binary.LittleEndian.Uint64(encoded[12:20]); got != 7 {
		t.Fatalf("revision=%d，想要 7", got)
	}
	if got := binary.LittleEndian.Uint32(encoded[20:24]); got != 1 {
		t.Fatalf("count=%d，想要 1", got)
	}
	if got := binary.LittleEndian.Uint32(encoded[24:28]); got != hostileRecordLength {
		t.Fatalf("payloadLen=%d，想要 %d", got, hostileRecordLength)
	}
	hasher := crc32.New(crc32.MakeTable(crc32.Castagnoli))
	_, _ = hasher.Write(encoded[8:28])
	_, _ = hasher.Write(encoded[32:])
	if got := binary.LittleEndian.Uint32(encoded[28:32]); got != hasher.Sum32() {
		t.Fatalf("CRC=%d，想要 %d（覆盖 [8:28]+payload 段）", got, hasher.Sum32())
	}

	base := hostileRecordOffset(0)
	if got := binary.LittleEndian.Uint64(encoded[base+hostileWireID:]); got != record.ID {
		t.Fatalf("ID=%d，想要 %d", got, record.ID)
	}
	if got := binary.LittleEndian.Uint32(encoded[base+hostileWireDimension:]); got != 0 {
		t.Fatalf("dimension=%d，想要 Overworld=0", got)
	}
	for index := range record.Position {
		got := math.Float32frombits(binary.LittleEndian.Uint32(encoded[base+hostileWirePosition+4*index:]))
		if got != record.Position[index] {
			t.Fatalf("position[%d]=%v，想要 %v", index, got, record.Position[index])
		}
	}
	for index := range record.Velocity {
		got := math.Float32frombits(binary.LittleEndian.Uint32(encoded[base+hostileWireVelocity+4*index:]))
		if got != record.Velocity[index] {
			t.Fatalf("velocity[%d]=%v，想要 %v", index, got, record.Velocity[index])
		}
	}
	if got := encoded[base+hostileWireOnGround]; got != 1 {
		t.Fatalf("onGround=%d，想要 1", got)
	}
	if got := math.Float32frombits(binary.LittleEndian.Uint32(encoded[base+hostileWireYaw:])); got != record.Yaw {
		t.Fatalf("yaw=%v，想要 %v", got, record.Yaw)
	}
	if got := encoded[base+hostileWireHealth]; got != record.Health {
		t.Fatalf("health=%d，想要 %d", got, record.Health)
	}
	if got := encoded[base+hostileWireAttack]; got != record.AttackCooldown {
		t.Fatalf("attackCooldown=%d，想要 %d", got, record.AttackCooldown)
	}
	if got := encoded[base+hostileWireHurt]; got != record.HurtCooldown {
		t.Fatalf("hurtCooldown=%d，想要 %d", got, record.HurtCooldown)
	}
	if got := encoded[base+hostileWireBurn]; got != record.BurnCooldown {
		t.Fatalf("burnCooldown=%d，想要 %d", got, record.BurnCooldown)
	}
	if got := encoded[base+hostileWireHasTarget]; got != 1 {
		t.Fatalf("hasTarget=%d，想要 1", got)
	}
	if !bytes.Equal(encoded[base+hostileWirePlayerID:base+hostileWirePlayerID+16], record.PlayerID[:]) {
		t.Fatal("playerID 字节与目标玩家 ID 不一致")
	}
	if got := binary.LittleEndian.Uint64(encoded[base+hostileWireNextRepath:]); got != record.NextRepathTicks {
		t.Fatalf("nextRepath=%d，想要 %d", got, record.NextRepathTicks)
	}
	if got := binary.LittleEndian.Uint16(encoded[base+hostileWireDistant:]); got != record.DistantTicks {
		t.Fatalf("distant=%d，想要 %d", got, record.DistantTicks)
	}
}

func TestHostileCodecRoundTripIsExactAndSorted(t *testing.T) {
	input := fixtureHostileRecords()
	before := slices.Clone(input)
	encoded, err := Encode(HostileMobsSave{Revision: 9, Records: input})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(input, before) {
		t.Fatalf("编码修改了调用方 records：got=%+v want=%+v", input, before)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	want := fixtureHostileRecordsSorted()
	if decoded.Revision != 9 || !reflect.DeepEqual(decoded.Records, want) {
		t.Fatalf("decode=%+v，想要 revision=9 records=%+v", decoded, want)
	}
}

func TestHostileCodecAcceptsMaximumRecordsAndEnforcesFileLimit(t *testing.T) {
	records := make([]StoredHostileMob, MaxHostileMobs)
	for index := range records {
		records[index] = StoredHostileMob{ID: uint64(index) + 1, Dimension: core.Overworld, Health: 1}
	}
	encoded, err := Encode(HostileMobsSave{Revision: 23, Records: records})
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != hostileHeaderLength+MaxHostileMobs*hostileRecordLength {
		t.Fatalf(
			"64 条记录长度=%d，想要 %d",
			len(encoded), hostileHeaderLength+MaxHostileMobs*hostileRecordLength,
		)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Revision != 23 || len(decoded.Records) != MaxHostileMobs ||
		!reflect.DeepEqual(decoded.Records, records) {
		t.Fatalf("64 条记录 decode=%+v", decoded)
	}
	if _, err := Decode(append(encoded, 0)); !errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("尾随字节 decode error=%v，想要 storagedef.ErrCorrupt", err)
	}

	tooMany := make([]StoredHostileMob, MaxHostileMobs+1)
	for index := range tooMany {
		tooMany[index] = StoredHostileMob{ID: uint64(index) + 1, Dimension: core.Overworld, Health: 1}
	}
	if _, err := Encode(HostileMobsSave{Revision: 1, Records: tooMany}); !errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("encode 65 条 error=%v，想要 storagedef.ErrCorrupt", err)
	}

	oversized := bytes.Repeat([]byte{0x5a}, MaxFileLength+1)
	_, err = Decode(oversized)
	if !errors.Is(err, storagedef.ErrCorrupt) || !strings.Contains(err.Error(), "length") {
		t.Fatalf("超长输入 error=%v，想要分配前长度门禁", err)
	}
}

func TestHostileCodecEmptyCollectionRoundTrip(t *testing.T) {
	encoded, err := Encode(HostileMobsSave{Revision: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != hostileHeaderLength {
		t.Fatalf("空集合文件长度=%d，想要 %d", len(encoded), hostileHeaderLength)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Revision != 4 || len(decoded.Records) != 0 {
		t.Fatalf("空集合 decode=%+v，想要 revision=4 无记录", decoded)
	}
}

func TestHostileCodecRejectsInvalidSaves(t *testing.T) {
	validTarget := fixtureHostileTargetPlayerID()
	badVersionTarget := validTarget
	badVersionTarget[6] = 0x36 // 版本半字节为 3，不再是 UUIDv4
	badVariantTarget := validTarget
	badVariantTarget[8] = 0x1f // 变体高两位为 00，不再是 UUIDv4
	worldBottom := float32(core.MinY)
	worldTop := float32(core.MaxY)

	tests := []struct {
		name   string
		mutate func(*StoredHostileMob)
	}{
		{"zero ID", func(r *StoredHostileMob) { r.ID = 0 }},
		{"unknown dimension", func(r *StoredHostileMob) { r.Dimension = core.DimensionID(1) }},
		{"NaN position", func(r *StoredHostileMob) { r.Position[1] = float32(math.NaN()) }},
		{"Inf velocity", func(r *StoredHostileMob) { r.Velocity[0] = float32(math.Inf(-1)) }},
		{"NaN yaw", func(r *StoredHostileMob) { r.Yaw = float32(math.NaN()) }},
		{"health zero", func(r *StoredHostileMob) { r.Health = 0 }},
		{"health above max", func(r *StoredHostileMob) { r.Health = core.MaxHealth + 1 }},
		{"attack cooldown above period", func(r *StoredHostileMob) { r.AttackCooldown = 21 }},
		{"hurt cooldown above period", func(r *StoredHostileMob) { r.HurtCooldown = 21 }},
		{"burn cooldown above period", func(r *StoredHostileMob) { r.BurnCooldown = 21 }},
		{"distant above despawn threshold", func(r *StoredHostileMob) { r.DistantTicks = 601 }},
		{"position below world", func(r *StoredHostileMob) { r.Position[1] = worldBottom - 0.5 }},
		{"position at world top", func(r *StoredHostileMob) { r.Position[1] = worldTop }},
		{"no target keeps player ID", func(r *StoredHostileMob) {
			r.HasTarget = false
			r.PlayerID = validTarget
		}},
		{"target with zero player ID", func(r *StoredHostileMob) {
			r.HasTarget = true
			r.PlayerID = core.PlayerID{}
		}},
		{"target with non-v4 player ID", func(r *StoredHostileMob) {
			r.HasTarget = true
			r.PlayerID = badVersionTarget
		}},
		{"target with non-variant player ID", func(r *StoredHostileMob) {
			r.HasTarget = true
			r.PlayerID = badVariantTarget
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			record := StoredHostileMob{
				ID: 42, Dimension: core.Overworld, Position: [3]float32{0, 64, 0},
				Health: 20, HasTarget: true, PlayerID: validTarget,
			}
			tc.mutate(&record)
			_, err := Encode(HostileMobsSave{Revision: 1, Records: []StoredHostileMob{record}})
			if !errors.Is(err, storagedef.ErrCorrupt) {
				t.Fatalf("encode error=%v，想要 storagedef.ErrCorrupt", err)
			}
		})
	}

	if _, err := Encode(HostileMobsSave{Records: fixtureHostileRecords()}); !errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("encode 零 revision error=%v，想要 storagedef.ErrCorrupt", err)
	}
	duplicate := fixtureHostileRecordsSorted()
	duplicate[1].ID = duplicate[0].ID
	if _, err := Encode(HostileMobsSave{Revision: 1, Records: duplicate}); !errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("encode 重复 ID error=%v，想要 storagedef.ErrCorrupt", err)
	}
}

func TestHostileCodecRejectsCorruptFiles(t *testing.T) {
	encoded, err := Encode(HostileMobsSave{Revision: 9, Records: fixtureHostileRecords()})
	if err != nil {
		t.Fatal(err)
	}
	valid := bytes.Clone(encoded)
	base0 := hostileRecordOffset(0)
	base1 := hostileRecordOffset(1)
	base2 := hostileRecordOffset(2)

	// 头段补丁（magic/envelope/schema/revision/count/payloadLen）在 CRC 检查
	// 之前就会被对应门禁拒绝，无需修复校验和（companion 同款用例先例）；
	// 记录字段补丁则必须修复 CRC，让拒绝确实来自字段校验。
	tests := []struct {
		name   string
		want   error
		mutate func([]byte)
	}{
		{"magic", storagedef.ErrCorrupt, func(payload []byte) { payload[0] ^= 0x01 }},
		{"old envelope", storagedef.ErrCorrupt, func(payload []byte) {
			binary.LittleEndian.PutUint32(payload[4:], 0)
		}},
		{"future envelope", storagedef.ErrFutureVersion, func(payload []byte) {
			binary.LittleEndian.PutUint32(payload[4:], 2)
		}},
		{"old schema", storagedef.ErrCorrupt, func(payload []byte) {
			binary.LittleEndian.PutUint32(payload[8:], 0)
		}},
		{"future schema", storagedef.ErrFutureVersion, func(payload []byte) {
			binary.LittleEndian.PutUint32(payload[8:], CurrentSchema+1)
		}},
		{"zero revision", storagedef.ErrCorrupt, func(payload []byte) {
			binary.LittleEndian.PutUint64(payload[12:], 0)
		}},
		{"payload length mismatch", storagedef.ErrCorrupt, func(payload []byte) {
			binary.LittleEndian.PutUint32(payload[24:], 1)
		}},
		{"count beyond payload", storagedef.ErrCorrupt, func(payload []byte) {
			binary.LittleEndian.PutUint32(payload[20:], 4)
		}},
		{"CRC", storagedef.ErrCorrupt, func(payload []byte) { payload[28] ^= 0x01 }},
		{"duplicate ID", storagedef.ErrCorrupt, func(payload []byte) {
			copy(payload[base1+hostileWireID:], payload[base0+hostileWireID:base0+hostileWireID+8])
			repairHostileCRC(payload)
		}},
		{"descending IDs", storagedef.ErrCorrupt, func(payload []byte) {
			first := bytes.Clone(payload[base0+hostileWireID : base0+hostileWireID+8])
			copy(payload[base0+hostileWireID:], payload[base1+hostileWireID:base1+hostileWireID+8])
			copy(payload[base1+hostileWireID:], first)
			repairHostileCRC(payload)
		}},
		{"zero ID", storagedef.ErrCorrupt, func(payload []byte) {
			clear(payload[base0+hostileWireID : base0+hostileWireID+8])
			repairHostileCRC(payload)
		}},
		{"unknown dimension", storagedef.ErrCorrupt, patchAndRepairU32(base1+hostileWireDimension, 7)},
		{"NaN position", storagedef.ErrCorrupt, func(payload []byte) {
			putF32(base0+hostileWirePosition+4, float32(math.NaN()))(payload)
			repairHostileCRC(payload)
		}},
		{"Inf velocity", storagedef.ErrCorrupt, func(payload []byte) {
			putF32(base2+hostileWireVelocity+8, float32(math.Inf(1)))(payload)
			repairHostileCRC(payload)
		}},
		{"NaN yaw", storagedef.ErrCorrupt, func(payload []byte) {
			putF32(base1+hostileWireYaw, float32(math.NaN()))(payload)
			repairHostileCRC(payload)
		}},
		{"health zero", storagedef.ErrCorrupt, func(payload []byte) {
			payload[base0+hostileWireHealth] = 0
			repairHostileCRC(payload)
		}},
		{"health above max", storagedef.ErrCorrupt, func(payload []byte) {
			payload[base0+hostileWireHealth] = core.MaxHealth + 1
			repairHostileCRC(payload)
		}},
		{"attack cooldown above period", storagedef.ErrCorrupt, func(payload []byte) {
			payload[base0+hostileWireAttack] = 21
			repairHostileCRC(payload)
		}},
		{"hurt cooldown above period", storagedef.ErrCorrupt, func(payload []byte) {
			payload[base0+hostileWireHurt] = 21
			repairHostileCRC(payload)
		}},
		{"burn cooldown above period", storagedef.ErrCorrupt, func(payload []byte) {
			payload[base2+hostileWireBurn] = 21
			repairHostileCRC(payload)
		}},
		{"distant above threshold", storagedef.ErrCorrupt, func(payload []byte) {
			binary.LittleEndian.PutUint16(payload[base2+hostileWireDistant:], maxHostileDistantTicks+1)
			repairHostileCRC(payload)
		}},
		{"position below world", storagedef.ErrCorrupt, func(payload []byte) {
			putF32(base0+hostileWirePosition+4, -64.5)(payload)
			repairHostileCRC(payload)
		}},
		{"position at world top", storagedef.ErrCorrupt, func(payload []byte) {
			putF32(base0+hostileWirePosition+4, float32(core.MaxY))(payload)
			repairHostileCRC(payload)
		}},
		{"invalid bool", storagedef.ErrCorrupt, func(payload []byte) {
			payload[base0+hostileWireHasTarget] = 2
			repairHostileCRC(payload)
		}},
		{"no target keeps player ID", storagedef.ErrCorrupt, func(payload []byte) {
			// base2 是携带目标的 tracking 记录：只清 hasTarget、保留其合法
			// PlayerID，恰好构成「无目标却携带 PlayerID」的位形。
			payload[base2+hostileWireHasTarget] = 0
			repairHostileCRC(payload)
		}},
		{"target with zero player ID", storagedef.ErrCorrupt, func(payload []byte) {
			clear(payload[base2+hostileWirePlayerID : base2+hostileWirePlayerID+16])
			repairHostileCRC(payload)
		}},
		{"target with non-v4 player ID", storagedef.ErrCorrupt, func(payload []byte) {
			payload[base2+hostileWirePlayerID+6] = 0x36
			repairHostileCRC(payload)
		}},
		{"target with non-variant player ID", storagedef.ErrCorrupt, func(payload []byte) {
			payload[base2+hostileWirePlayerID+8] = 0x1f
			repairHostileCRC(payload)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := bytes.Clone(valid)
			tc.mutate(payload)
			if _, err := Decode(payload); !errors.Is(err, tc.want) {
				t.Fatalf("decode error=%v，想要 %v", err, tc.want)
			}
		})
	}

	// 截断与尾随：长度形态逐一拒绝，且不得部分接受前缀记录。
	for name, payload := range map[string][]byte{
		"truncation":    valid[:len(valid)-1],
		"header only":   valid[:hostileHeaderLength],
		"short record":  valid[:hostileHeaderLength+hostileRecordLength-1],
		"trailing byte": append(bytes.Clone(valid), 0),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(payload); !errors.Is(err, storagedef.ErrCorrupt) {
				t.Fatalf("decode error=%v，想要 storagedef.ErrCorrupt", err)
			}
		})
	}

	// 分配前 count 门禁：count=65 的头声明 65 条 payload，即使输入本身不超
	// 文件上界也必须在解析与分配前拒绝。
	oversizedCount := make([]byte, 32)
	copy(oversizedCount, "MHST")
	binary.LittleEndian.PutUint32(oversizedCount[4:], 1)
	binary.LittleEndian.PutUint32(oversizedCount[8:], 1)
	binary.LittleEndian.PutUint64(oversizedCount[12:], 1)
	binary.LittleEndian.PutUint32(oversizedCount[20:], MaxHostileMobs+1)
	binary.LittleEndian.PutUint32(oversizedCount[24:], (MaxHostileMobs+1)*hostileRecordLength)
	_, err = Decode(oversizedCount)
	if !errors.Is(err, storagedef.ErrCorrupt) || !strings.Contains(err.Error(), "count") {
		t.Fatalf("count 超限 error=%v，想要分配前 count 门禁", err)
	}

	// 世界 Y 边界的合法端点必须被接受：position.Y 恰为 MinY 与低于 MaxY 的
	// 值都是合法持久状态（判据是左闭右开区间）。
	for _, y := range []float32{float32(core.MinY), 319.5} {
		record := StoredHostileMob{ID: 8, Dimension: core.Overworld, Health: 20}
		record.Position[1] = y
		bounded, err := Encode(HostileMobsSave{Revision: 2, Records: []StoredHostileMob{record}})
		if err != nil {
			t.Fatalf("Y=%v 应当合法：encode error=%v", y, err)
		}
		decoded, err := Decode(bounded)
		if err != nil {
			t.Fatalf("Y=%v 应当合法：decode error=%v", y, err)
		}
		if decoded.Records[0].Position[1] != y {
			t.Fatalf("Y 边界往返=%v，想要 %v", decoded.Records[0].Position[1], y)
		}
	}
}

func TestHostileCodecCanonicalFormIndependentOfInputOrder(t *testing.T) {
	sorted, err := Encode(HostileMobsSave{
		Revision: 3, Records: fixtureHostileRecordsSorted(),
	})
	if err != nil {
		t.Fatal(err)
	}
	shuffled, err := Encode(HostileMobsSave{
		Revision: 3, Records: fixtureHostileRecords(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sorted, shuffled) {
		t.Fatal("升序与乱序输入的编码不一致：磁盘形态必须唯一")
	}
	decoded, err := Decode(sorted)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.Records, fixtureHostileRecordsSorted()) {
		t.Fatalf("升序输入往返=%+v，想要 %+v", decoded.Records, fixtureHostileRecordsSorted())
	}
}
