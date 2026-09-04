package passive

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

	"github.com/channing771/mornlea/packages/server/storage/storagedef"
	"github.com/channing771/mornlea/packages/shared/core"
)

// passiveWireOffsets 是单条 72-byte 记录内各字段的字节偏移，与
// appendPassiveMob/decodePassiveMob 的固定布局一一对应。布局变化时本表
// 必须同步更新（与 hostile codec 的 offset 测试同一纪律）。
const (
	passiveWireID        = 0
	passiveWireDimension = 8
	passiveWirePosition  = 12
	passiveWireVelocity  = 24
	passiveWireOnGround  = 36
	passiveWireYaw       = 37
	passiveWireHealth    = 41
	passiveWireReserved  = 42
)

// passiveRecordOffset 返回第 index 条记录在文件中的起始偏移。
func passiveRecordOffset(index int) int {
	return passiveHeaderLength + index*passiveRecordLength
}

// fixturePassiveRecords 返回三条字段各异的合法记录。顺序刻意逆序：编码端
// 必须按 ID 升序写出，磁盘形态与调用方传入顺序解耦（hostile 先例）。
func fixturePassiveRecords() []StoredPassiveMob {
	grazer := StoredPassiveMob{
		ID: 0x8000000000000002, Dimension: core.Overworld,
		Position: [3]float32{-12.5, 70.25, 3.5}, Velocity: [3]float32{-1.25, 0, 0.5},
		OnGround: true, Yaw: 1.25, Health: 17,
	}
	idle := StoredPassiveMob{
		ID: 0x4000000000000001, Dimension: core.Overworld,
		Position: [3]float32{0.5, 64, -9.75}, Velocity: [3]float32{0, -3.25, 0},
		OnGround: false, Yaw: -2.5, Health: core.MaxHealth,
	}
	calf := StoredPassiveMob{
		ID: 1, Dimension: core.Overworld,
		Position: [3]float32{8.5, 65.5, 9.75}, Velocity: [3]float32{2, 0, -2},
		OnGround: true, Yaw: 3, Health: 1,
	}
	return []StoredPassiveMob{grazer, idle, calf}
}

func fixturePassiveRecordsSorted() []StoredPassiveMob {
	sorted := slices.Clone(fixturePassiveRecords())
	slices.SortFunc(sorted, func(a, b StoredPassiveMob) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return sorted
}

// repairPassiveCRC 按既有 hostile 存档惯例重算 CRC-32C（覆盖 [8:28] 头段
// 与 [32:] payload 段），供补丁用例把非 CRC 字段改掉后重新变成「校验和合法
// 但内容非法」的输入，确保拒绝来自字段校验而不是校验和。
func repairPassiveCRC(payload []byte) {
	hasher := crc32.New(crc32.MakeTable(crc32.Castagnoli))
	_, _ = hasher.Write(payload[8:28])
	_, _ = hasher.Write(payload[32:])
	binary.LittleEndian.PutUint32(payload[28:], hasher.Sum32())
}

// patchAndRepairU32 返回把 offset 处的 u32 改为 value 并修复 CRC 的补丁。
func patchAndRepairU32(offset int, value uint32) func([]byte) {
	return func(payload []byte) {
		binary.LittleEndian.PutUint32(payload[offset:], value)
		repairPassiveCRC(payload)
	}
}

// putF32 修改 offset 处的 f32；头段用例（CRC 检查之前就拒绝）使用
// 不修复的变体，记录字段用例必须再接 repairPassiveCRC。
func putF32(offset int, value float32) func([]byte) {
	return func(payload []byte) {
		binary.LittleEndian.PutUint32(payload[offset:], math.Float32bits(value))
	}
}

func TestPassiveCodecHeaderAndRecordLayout(t *testing.T) {
	record := fixturePassiveRecords()[0]
	encoded, err := Encode(PassiveMobsSave{
		Revision: 7, Records: []StoredPassiveMob{record},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != passiveHeaderLength+passiveRecordLength {
		t.Fatalf("单记录文件长度=%d，想要 %d", len(encoded), passiveHeaderLength+passiveRecordLength)
	}
	if string(encoded[0:4]) != "PMST" {
		t.Fatalf("magic=%q，想要 PMST", encoded[0:4])
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
	if got := binary.LittleEndian.Uint32(encoded[24:28]); got != passiveRecordLength {
		t.Fatalf("payloadLen=%d，想要 %d", got, passiveRecordLength)
	}
	hasher := crc32.New(crc32.MakeTable(crc32.Castagnoli))
	_, _ = hasher.Write(encoded[8:28])
	_, _ = hasher.Write(encoded[32:])
	if got := binary.LittleEndian.Uint32(encoded[28:32]); got != hasher.Sum32() {
		t.Fatalf("CRC=%d，想要 %d（覆盖 [8:28]+payload 段）", got, hasher.Sum32())
	}

	base := passiveRecordOffset(0)
	if got := binary.LittleEndian.Uint64(encoded[base+passiveWireID:]); got != record.ID {
		t.Fatalf("ID=%d，想要 %d", got, record.ID)
	}
	if got := binary.LittleEndian.Uint32(encoded[base+passiveWireDimension:]); got != 0 {
		t.Fatalf("dimension=%d，想要 Overworld=0", got)
	}
	for index := range record.Position {
		got := math.Float32frombits(binary.LittleEndian.Uint32(encoded[base+passiveWirePosition+4*index:]))
		if got != record.Position[index] {
			t.Fatalf("position[%d]=%v，想要 %v", index, got, record.Position[index])
		}
	}
	for index := range record.Velocity {
		got := math.Float32frombits(binary.LittleEndian.Uint32(encoded[base+passiveWireVelocity+4*index:]))
		if got != record.Velocity[index] {
			t.Fatalf("velocity[%d]=%v，想要 %v", index, got, record.Velocity[index])
		}
	}
	if got := encoded[base+passiveWireOnGround]; got != 1 {
		t.Fatalf("onGround=%d，想要 1", got)
	}
	if got := math.Float32frombits(binary.LittleEndian.Uint32(encoded[base+passiveWireYaw:])); got != record.Yaw {
		t.Fatalf("yaw=%v，想要 %v", got, record.Yaw)
	}
	if got := encoded[base+passiveWireHealth]; got != record.Health {
		t.Fatalf("health=%d，想要 %d", got, record.Health)
	}
	if !bytes.Equal(encoded[base+passiveWireReserved:base+passiveRecordLength], make([]byte, passiveReservedLength)) {
		t.Fatal("保留尾段必须全零")
	}
}

func TestPassiveCodecRoundTripIsExactAndSorted(t *testing.T) {
	input := fixturePassiveRecords()
	before := slices.Clone(input)
	encoded, err := Encode(PassiveMobsSave{Revision: 9, Records: input})
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
	want := fixturePassiveRecordsSorted()
	if decoded.Revision != 9 || !reflect.DeepEqual(decoded.Records, want) {
		t.Fatalf("decode=%+v，想要 revision=9 records=%+v", decoded, want)
	}
}

func TestPassiveCodecAcceptsMaximumRecordsAndEnforcesFileLimit(t *testing.T) {
	if MaxFileLength != passiveHeaderLength+MaxPassiveMobs*passiveRecordLength {
		t.Fatalf("MaxFileLength=%d，想要 %d", MaxFileLength, passiveHeaderLength+MaxPassiveMobs*passiveRecordLength)
	}
	records := make([]StoredPassiveMob, MaxPassiveMobs)
	for index := range records {
		records[index] = StoredPassiveMob{ID: uint64(index) + 1, Dimension: core.Overworld, Health: 1}
	}
	encoded, err := Encode(PassiveMobsSave{Revision: 23, Records: records})
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != passiveHeaderLength+MaxPassiveMobs*passiveRecordLength {
		t.Fatalf(
			"32 条记录长度=%d，想要 %d",
			len(encoded), passiveHeaderLength+MaxPassiveMobs*passiveRecordLength,
		)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Revision != 23 || len(decoded.Records) != MaxPassiveMobs ||
		!reflect.DeepEqual(decoded.Records, records) {
		t.Fatalf("32 条记录 decode=%+v", decoded)
	}
	if _, err := Decode(append(encoded, 0)); !errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("尾随字节 decode error=%v，想要 storagedef.ErrCorrupt", err)
	}

	tooMany := make([]StoredPassiveMob, MaxPassiveMobs+1)
	for index := range tooMany {
		tooMany[index] = StoredPassiveMob{ID: uint64(index) + 1, Dimension: core.Overworld, Health: 1}
	}
	if _, err := Encode(PassiveMobsSave{Revision: 1, Records: tooMany}); !errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("encode 33 条 error=%v，想要 storagedef.ErrCorrupt", err)
	}

	oversized := bytes.Repeat([]byte{0x5a}, MaxFileLength+1)
	_, err = Decode(oversized)
	if !errors.Is(err, storagedef.ErrCorrupt) || !strings.Contains(err.Error(), "length") {
		t.Fatalf("超长输入 error=%v，想要分配前长度门禁", err)
	}
}

func TestPassiveCodecEmptyCollectionRoundTrip(t *testing.T) {
	encoded, err := Encode(PassiveMobsSave{Revision: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != passiveHeaderLength {
		t.Fatalf("空集合文件长度=%d，想要 %d", len(encoded), passiveHeaderLength)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Revision != 4 || len(decoded.Records) != 0 {
		t.Fatalf("空集合 decode=%+v，想要 revision=4 无记录", decoded)
	}
}

func TestPassiveCodecRejectsInvalidSaves(t *testing.T) {
	worldBottom := float32(core.MinY)
	worldTop := float32(core.MaxY)

	tests := []struct {
		name   string
		mutate func(*StoredPassiveMob)
	}{
		{"zero ID", func(r *StoredPassiveMob) { r.ID = 0 }},
		{"unknown dimension", func(r *StoredPassiveMob) { r.Dimension = core.DimensionID(1) }},
		{"NaN position", func(r *StoredPassiveMob) { r.Position[1] = float32(math.NaN()) }},
		{"Inf velocity", func(r *StoredPassiveMob) { r.Velocity[0] = float32(math.Inf(-1)) }},
		{"NaN yaw", func(r *StoredPassiveMob) { r.Yaw = float32(math.NaN()) }},
		{"health zero", func(r *StoredPassiveMob) { r.Health = 0 }},
		{"health above max", func(r *StoredPassiveMob) { r.Health = core.MaxHealth + 1 }},
		{"position below world", func(r *StoredPassiveMob) { r.Position[1] = worldBottom - 0.5 }},
		{"position at world top", func(r *StoredPassiveMob) { r.Position[1] = worldTop }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			record := StoredPassiveMob{
				ID: 42, Dimension: core.Overworld, Position: [3]float32{0, 64, 0},
				Health: 20,
			}
			tc.mutate(&record)
			_, err := Encode(PassiveMobsSave{Revision: 1, Records: []StoredPassiveMob{record}})
			if !errors.Is(err, storagedef.ErrCorrupt) {
				t.Fatalf("encode error=%v，想要 storagedef.ErrCorrupt", err)
			}
		})
	}

	if _, err := Encode(PassiveMobsSave{Records: fixturePassiveRecords()}); !errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("encode 零 revision error=%v，想要 storagedef.ErrCorrupt", err)
	}
	duplicate := fixturePassiveRecordsSorted()
	duplicate[1].ID = duplicate[0].ID
	if _, err := Encode(PassiveMobsSave{Revision: 1, Records: duplicate}); !errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("encode 重复 ID error=%v，想要 storagedef.ErrCorrupt", err)
	}
}

func TestPassiveCodecRejectsCorruptFiles(t *testing.T) {
	encoded, err := Encode(PassiveMobsSave{Revision: 9, Records: fixturePassiveRecords()})
	if err != nil {
		t.Fatal(err)
	}
	valid := bytes.Clone(encoded)
	base0 := passiveRecordOffset(0)
	base1 := passiveRecordOffset(1)
	base2 := passiveRecordOffset(2)

	// 头段补丁（magic/envelope/schema/revision/count/payloadLen）在 CRC 检查
	// 之前就会被对应门禁拒绝，无需修复校验和（hostile 同款用例先例）；
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
			copy(payload[base1+passiveWireID:], payload[base0+passiveWireID:base0+passiveWireID+8])
			repairPassiveCRC(payload)
		}},
		{"descending IDs", storagedef.ErrCorrupt, func(payload []byte) {
			first := bytes.Clone(payload[base0+passiveWireID : base0+passiveWireID+8])
			copy(payload[base0+passiveWireID:], payload[base1+passiveWireID:base1+passiveWireID+8])
			copy(payload[base1+passiveWireID:], first)
			repairPassiveCRC(payload)
		}},
		{"zero ID", storagedef.ErrCorrupt, func(payload []byte) {
			clear(payload[base0+passiveWireID : base0+passiveWireID+8])
			repairPassiveCRC(payload)
		}},
		{"unknown dimension", storagedef.ErrCorrupt, patchAndRepairU32(base1+passiveWireDimension, 7)},
		{"NaN position", storagedef.ErrCorrupt, func(payload []byte) {
			putF32(base0+passiveWirePosition+4, float32(math.NaN()))(payload)
			repairPassiveCRC(payload)
		}},
		{"Inf velocity", storagedef.ErrCorrupt, func(payload []byte) {
			putF32(base2+passiveWireVelocity+8, float32(math.Inf(1)))(payload)
			repairPassiveCRC(payload)
		}},
		{"NaN yaw", storagedef.ErrCorrupt, func(payload []byte) {
			putF32(base1+passiveWireYaw, float32(math.NaN()))(payload)
			repairPassiveCRC(payload)
		}},
		{"health zero", storagedef.ErrCorrupt, func(payload []byte) {
			payload[base0+passiveWireHealth] = 0
			repairPassiveCRC(payload)
		}},
		{"health above max", storagedef.ErrCorrupt, func(payload []byte) {
			payload[base0+passiveWireHealth] = core.MaxHealth + 1
			repairPassiveCRC(payload)
		}},
		{"position below world", storagedef.ErrCorrupt, func(payload []byte) {
			putF32(base0+passiveWirePosition+4, -64.5)(payload)
			repairPassiveCRC(payload)
		}},
		{"position at world top", storagedef.ErrCorrupt, func(payload []byte) {
			putF32(base0+passiveWirePosition+4, float32(core.MaxY))(payload)
			repairPassiveCRC(payload)
		}},
		{"invalid bool", storagedef.ErrCorrupt, func(payload []byte) {
			payload[base0+passiveWireOnGround] = 2
			repairPassiveCRC(payload)
		}},
		{"nonzero reserved tail", storagedef.ErrCorrupt, func(payload []byte) {
			payload[base0+passiveWireReserved+7] = 1
			repairPassiveCRC(payload)
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
		"header only":   valid[:passiveHeaderLength],
		"short record":  valid[:passiveHeaderLength+passiveRecordLength-1],
		"trailing byte": append(bytes.Clone(valid), 0),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(payload); !errors.Is(err, storagedef.ErrCorrupt) {
				t.Fatalf("decode error=%v，想要 storagedef.ErrCorrupt", err)
			}
		})
	}

	// 分配前 count 门禁：count=33 的头声明 33 条 payload，即使输入本身不超
	// 文件上界也必须在解析与分配前拒绝。
	oversizedCount := make([]byte, 32)
	copy(oversizedCount, "PMST")
	binary.LittleEndian.PutUint32(oversizedCount[4:], 1)
	binary.LittleEndian.PutUint32(oversizedCount[8:], 1)
	binary.LittleEndian.PutUint64(oversizedCount[12:], 1)
	binary.LittleEndian.PutUint32(oversizedCount[20:], MaxPassiveMobs+1)
	binary.LittleEndian.PutUint32(oversizedCount[24:], (MaxPassiveMobs+1)*passiveRecordLength)
	_, err = Decode(oversizedCount)
	if !errors.Is(err, storagedef.ErrCorrupt) || !strings.Contains(err.Error(), "count") {
		t.Fatalf("count 超限 error=%v，想要分配前 count 门禁", err)
	}

	// 世界 Y 边界的合法端点必须被接受：position.Y 恰为 MinY 与低于 MaxY 的
	// 值都是合法持久状态（判据是左闭右开区间）。
	for _, y := range []float32{float32(core.MinY), 319.5} {
		record := StoredPassiveMob{ID: 8, Dimension: core.Overworld, Health: 20}
		record.Position[1] = y
		bounded, err := Encode(PassiveMobsSave{Revision: 2, Records: []StoredPassiveMob{record}})
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

func TestPassiveCodecCanonicalFormIndependentOfInputOrder(t *testing.T) {
	sorted, err := Encode(PassiveMobsSave{
		Revision: 3, Records: fixturePassiveRecordsSorted(),
	})
	if err != nil {
		t.Fatal(err)
	}
	shuffled, err := Encode(PassiveMobsSave{
		Revision: 3, Records: fixturePassiveRecords(),
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
	if !reflect.DeepEqual(decoded.Records, fixturePassiveRecordsSorted()) {
		t.Fatalf("升序输入往返=%+v，想要 %+v", decoded.Records, fixturePassiveRecordsSorted())
	}
}
