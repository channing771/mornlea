package companion

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/storage/storagedef"
)

func fixtureCompanionID(last byte) companion.ID {
	return companion.ID{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, last}
}

func fixtureCompanionBodies() []companion.Body {
	stoneFull, _ := core.ItemMaxDurability(core.ItemStonePickaxe)
	ironFull, _ := core.ItemMaxDurability(core.ItemIronPickaxe)
	high := companion.Body{
		ID: fixtureCompanionID(2), Dimension: core.Overworld,
		Position: [3]float32{-12.5, 70, 3.25}, Yaw: 1.25, Pitch: -0.5,
	}
	high.Inventory.Hotbar.Selected = 4
	high.Inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
	high.Inventory.Hotbar.Slots[4] = core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: stoneFull}
	high.Inventory.Backpack[0] = core.ItemStack{Item: core.ItemOakLog, Count: 7}

	low := companion.Body{
		ID: fixtureCompanionID(1), Dimension: core.Overworld,
		Position: [3]float32{8.5, 65, -9.75}, Yaw: -2.5, Pitch: 0.75,
	}
	low.Inventory.Hotbar.Selected = 2
	low.Inventory.Hotbar.Slots[2] = core.ItemStack{Item: core.ItemGlass, Count: 12}
	low.Inventory.Backpack[7] = core.ItemStack{Item: core.ItemIronPickaxe, Count: 1, Durability: ironFull}
	low.Inventory.Backpack[core.BackpackSlots-1] = core.ItemStack{Item: core.ItemDirt, Count: 5}
	return []companion.Body{high, low}
}

func TestCompanionCodecV1RoundTripAndGolden(t *testing.T) {
	// v1 golden 字节零改动：编码端只写当前 schema（v4），v1 路径由冻结的
	// golden 驱动只读迁移验证。
	path := filepath.Join("testdata", "companions-v1.bin")
	golden, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(golden) != 32+2*companionRecordLength {
		t.Fatalf("v1 golden 长度=%d，想要 %d", len(golden), 32+2*companionRecordLength)
	}
	if schema := binary.LittleEndian.Uint32(golden[8:12]); schema != 1 {
		t.Fatalf("v1 golden schema=%d，想要 1", schema)
	}
	got, err := Decode(golden)
	if err != nil {
		t.Fatal(err)
	}
	wantRecords := []companion.Body{fixtureCompanionBodies()[1], fixtureCompanionBodies()[0]}
	if got.Revision != 19 || !reflect.DeepEqual(got.Records, wantRecords) {
		t.Fatalf("v1 golden decode=%+v，想要 revision=19 records=%+v", got, wantRecords)
	}
	if got.Queues != nil {
		t.Fatalf("v1 golden 携带任务域=%+v，想要空", got.Queues)
	}

	// 同一载荷的首次保存必须写出当前 schema（v4）：记录 = v1 身体 + flags 字节。
	input := fixtureCompanionBodies()
	before := append([]companion.Body(nil), input...)
	encoded, err := Encode(CompanionSave{Revision: 19, Records: input})
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != 32+2*(companionRecordLength+1) {
		t.Fatalf("无任务双记录长度=%d，想要 %d", len(encoded), 32+2*(companionRecordLength+1))
	}
	if !reflect.DeepEqual(input, before) {
		t.Fatalf("编码修改调用者 records：got=%+v want=%+v", input, before)
	}
	if schema := binary.LittleEndian.Uint32(encoded[8:12]); schema != CurrentSchema {
		t.Fatalf("首次保存 schema=%d，想要 %d", schema, CurrentSchema)
	}
	migrated, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if migrated.Revision != 19 || !reflect.DeepEqual(migrated.Records, wantRecords) ||
		migrated.Queues != nil {
		t.Fatalf("迁移写 v4 后 decode=%+v", migrated)
	}
}

func TestCompanionCodecCurrentSchemaRoundTripsSwordItems(t *testing.T) {
	want := fixtureCompanionBodies()[0]
	stacks := [...]core.ItemStack{
		{Item: core.ItemWoodenSword, Count: 1, Durability: 58},
		{Item: core.ItemStoneSword, Count: 1, Durability: 130},
		{Item: core.ItemIronSword, Count: 1, Durability: 249},
		{Item: core.ItemBrokenWoodenSword, Count: 1},
		{Item: core.ItemBrokenStoneSword, Count: 1},
		{Item: core.ItemBrokenIronSword, Count: 1},
	}
	copy(want.Inventory.Hotbar.Slots[:], stacks[:])

	encoded, err := encodeCompanions(CompanionSave{Revision: 29, Records: []companion.Body{want}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeCompanions(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Records) != 1 || got.Records[0].Inventory != want.Inventory {
		t.Fatalf("伙伴剑物品往返 = %+v，想要 %+v", got.Records, want.Inventory)
	}
}

func TestCompanionCodecAcceptsMaximumStoredRecords(t *testing.T) {
	records := make([]companion.Body, companion.MaxStored)
	for index := range records {
		records[index].ID = fixtureCompanionID(byte(index))
	}
	encoded, err := Encode(CompanionSave{Revision: 23, Records: records})
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != 32+companion.MaxStored*(companionRecordLength+1) {
		t.Fatalf(
			"64 条无任务记录长度=%d，想要 %d",
			len(encoded), 32+companion.MaxStored*(companionRecordLength+1),
		)
	}
	got, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != 23 || len(got.Records) != 64 || !reflect.DeepEqual(got.Records, records) {
		t.Fatalf("64 条记录 decode=%+v", got)
	}
	if _, err := Decode(append(encoded, 0)); !errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("trailing byte decode error=%v，想要 storagedef.ErrCorrupt", err)
	}
}

func TestCompanionCodecRejectsCRCTruncationFutureVersionAndOversizedRecords(t *testing.T) {
	valid, err := Encode(CompanionSave{Revision: 7, Records: fixtureCompanionBodies()})
	if err != nil {
		t.Fatal(err)
	}
	badFloat := func(offset int) []byte {
		payload := bytes.Clone(valid)
		binary.LittleEndian.PutUint32(payload[offset:], math.Float32bits(float32(math.NaN())))
		repairCompanionCRC(payload)
		return payload
	}
	tests := []struct {
		name    string
		payload func() []byte
		want    error
	}{
		{"magic", func() []byte { p := bytes.Clone(valid); p[0] ^= 1; return p }, storagedef.ErrCorrupt},
		{"old envelope", func() []byte { p := bytes.Clone(valid); binary.LittleEndian.PutUint32(p[4:], 0); return p }, storagedef.ErrCorrupt},
		{"future envelope", func() []byte { p := bytes.Clone(valid); binary.LittleEndian.PutUint32(p[4:], 2); return p }, storagedef.ErrFutureVersion},
		{"old schema", func() []byte { p := bytes.Clone(valid); binary.LittleEndian.PutUint32(p[8:], 0); return p }, storagedef.ErrCorrupt},
		{"future schema", func() []byte {
			p := bytes.Clone(valid)
			binary.LittleEndian.PutUint32(p[8:], CurrentSchema+1)
			return p
		}, storagedef.ErrFutureVersion},
		{"zero revision", func() []byte { p := bytes.Clone(valid); clear(p[12:20]); repairCompanionCRC(p); return p }, storagedef.ErrCorrupt},
		{"payload length", func() []byte { p := bytes.Clone(valid); binary.LittleEndian.PutUint32(p[24:], 1); return p }, storagedef.ErrCorrupt},
		{"CRC", func() []byte { p := bytes.Clone(valid); p[28] ^= 1; return p }, storagedef.ErrCorrupt},
		{"truncation", func() []byte { return bytes.Clone(valid[:len(valid)-1]) }, storagedef.ErrCorrupt},
		{"trailing byte", func() []byte { return append(bytes.Clone(valid), 0) }, storagedef.ErrCorrupt},
		{"invalid ID", func() []byte { p := bytes.Clone(valid); clear(p[32:48]); repairCompanionCRC(p); return p }, storagedef.ErrCorrupt},
		{"invalid dimension", func() []byte {
			p := bytes.Clone(valid)
			binary.LittleEndian.PutUint32(p[48:], 1)
			repairCompanionCRC(p)
			return p
		}, storagedef.ErrCorrupt},
		{"position", func() []byte { return badFloat(52) }, storagedef.ErrCorrupt},
		{"yaw", func() []byte { return badFloat(64) }, storagedef.ErrCorrupt},
		{"pitch", func() []byte { return badFloat(68) }, storagedef.ErrCorrupt},
		{"pitch outside range", func() []byte {
			p := bytes.Clone(valid)
			binary.LittleEndian.PutUint32(p[68:], math.Float32bits(2))
			repairCompanionCRC(p)
			return p
		}, storagedef.ErrCorrupt},
		{"selected slot", func() []byte { p := bytes.Clone(valid); p[72] = core.HotbarSlots; repairCompanionCRC(p); return p }, storagedef.ErrCorrupt},
		{"invalid inventory", func() []byte {
			p := bytes.Clone(valid)
			binary.LittleEndian.PutUint16(p[73:], 4242)
			p[75] = 1
			repairCompanionCRC(p)
			return p
<<<<<<< HEAD:internal/storage/companion_codec_test.go
		}, ErrCorrupt},
		{"item sentinel", func() []byte {
			p := bytes.Clone(valid)
			binary.LittleEndian.PutUint16(p[73:], uint16(core.ItemIDMax))
			p[75] = 1
			repairCompanionCRC(p)
			return p
		}, ErrCorrupt},
=======
		}, storagedef.ErrCorrupt},
>>>>>>> origin/main:internal/storage/companion/companion_codec_test.go
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decode(tc.payload())
			if !errors.Is(err, tc.want) {
				t.Fatalf("decode error=%v，想要 %v", err, tc.want)
			}
		})
	}

	oversized := make([]byte, 32)
	copy(oversized, "MCAI")
	binary.LittleEndian.PutUint32(oversized[4:], 1)
	binary.LittleEndian.PutUint32(oversized[8:], 1)
	binary.LittleEndian.PutUint64(oversized[12:], 1)
	binary.LittleEndian.PutUint32(oversized[20:], companion.MaxStored+1)
	binary.LittleEndian.PutUint32(oversized[24:], (companion.MaxStored+1)*221)
	_, err = Decode(oversized)
	if !errors.Is(err, storagedef.ErrCorrupt) || !strings.Contains(err.Error(), "count") {
		t.Fatalf("oversized count error=%v，想要分配前 count 门禁", err)
	}

	tooMany := make([]companion.Body, companion.MaxStored+1)
	for i := range tooMany {
		tooMany[i] = fixtureCompanionBodies()[0]
	}
	if _, err := Encode(CompanionSave{Revision: 1, Records: tooMany}); !errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("encode 65 records error=%v，想要 storagedef.ErrCorrupt", err)
	}
	if _, err := Encode(CompanionSave{Records: fixtureCompanionBodies()[:1]}); !errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("encode zero revision error=%v，想要 storagedef.ErrCorrupt", err)
	}
	invalidBody := fixtureCompanionBodies()[0]
	invalidBody.Position[0] = float32(math.Inf(1))
	if _, err := Encode(CompanionSave{Revision: 1, Records: []companion.Body{invalidBody}}); !errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("encode invalid body error=%v，想要 storagedef.ErrCorrupt", err)
	}
}

func TestCompanionCodecRejectsDuplicateOrUnsortedIDs(t *testing.T) {
	valid, err := Encode(CompanionSave{Revision: 3, Records: fixtureCompanionBodies()})
	if err != nil {
		t.Fatal(err)
	}
	duplicate := bytes.Clone(valid)
	copy(duplicate[32+221:32+221+16], duplicate[32:48])
	repairCompanionCRC(duplicate)
	unsorted := bytes.Clone(valid)
	first := bytes.Clone(unsorted[32:48])
	copy(unsorted[32:48], unsorted[32+221:32+221+16])
	copy(unsorted[32+221:32+221+16], first)
	repairCompanionCRC(unsorted)
	for name, payload := range map[string][]byte{"duplicate": duplicate, "unsorted": unsorted} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(payload); !errors.Is(err, storagedef.ErrCorrupt) {
				t.Fatalf("decode error=%v，想要 storagedef.ErrCorrupt", err)
			}
		})
	}

	input := fixtureCompanionBodies()
	input[1].ID = input[0].ID
	if _, err := Encode(CompanionSave{Revision: 1, Records: input}); !errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("encode duplicate error=%v，想要 storagedef.ErrCorrupt", err)
	}
}

func TestCompanionCodecDoesNotPersistNameTaskOrPersona(t *testing.T) {
	encoded, err := Encode(CompanionSave{Revision: 5, Records: fixtureCompanionBodies()[:1]})
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != 32+companionRecordLength+1 {
		t.Fatalf("单记录文件长度=%d，想要固定 %d；任务区缺席时只追加 flags 字节", len(encoded), 32+companionRecordLength+1)
	}
	for _, forbidden := range [][]byte{[]byte("阿木"), []byte("挖石头"), []byte("persona")} {
		if bytes.Contains(encoded, forbidden) {
			t.Fatalf("v1 存档包含禁止字段 %q", forbidden)
		}
	}
}

func TestCompanionCodecV1PreservesWornToolDurability(t *testing.T) {
	body := fixtureCompanionBodies()[0]
	body.Inventory.Hotbar.Slots[4].Durability = 73
	body.Inventory.Backpack[3] = core.ItemStack{Item: core.ItemIronPickaxe, Count: 1, Durability: 149}
	encoded, err := Encode(CompanionSave{Revision: 9, Records: []companion.Body{body}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Records) != 1 || got.Records[0].Inventory != body.Inventory {
		t.Fatalf("磨损工具往返 inventory=%+v，想要 %+v", got.Records, body.Inventory)
	}
}

func repairCompanionCRC(payload []byte) {
	hasher := crc32.New(crc32.MakeTable(crc32.Castagnoli))
	_, _ = hasher.Write(payload[8:28])
	_, _ = hasher.Write(payload[32:])
	binary.LittleEndian.PutUint32(payload[28:], hasher.Sum32())
}
