package client_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

const (
	mrw1BatchHeaderBytes  = 24
	mrw1RecordHeaderBytes = 32
)

func TestBuildRenderWorldChunkBatchEncodesAllSectionsAndHeightMap(t *testing.T) {
	chunk := world.NewChunk(core.ChunkPos{X: -2, Z: 3})
	chunk.SetBlock(1, core.MinY, 2, core.StoneID)
	batch, err := client.BuildRenderWorldChunkBatch(1, core.Overworld, 17, chunk)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(batch.Updates), core.SectionsPerChunk+1; got != want {
		t.Fatalf("更新数=%d, want %d", got, want)
	}
	for section := 0; section < core.SectionsPerChunk; section++ {
		update := batch.Updates[section]
		if update.Kind != client.RenderWorldSectionUpsert {
			t.Fatalf("section %d kind=%d", section, update.Kind)
		}
		if got, want := update.Key, (core.SectionKey{
			Dimension: core.Overworld,
			Pos:       core.SectionPos{X: -2, Y: int32(section), Z: 3},
		}); got != want {
			t.Fatalf("section %d key=%+v, want %+v", section, got, want)
		}
		if update.Revision != 17 {
			t.Fatalf("section %d revision=%d", section, update.Revision)
		}
		if update.Heights != (world.HeightMap{}) {
			t.Fatalf("section %d has heights", section)
		}
	}
	column := batch.Updates[core.SectionsPerChunk]
	if column.Kind != client.RenderWorldColumnUpsert || column.Key.Pos.Y != 0 {
		t.Fatalf("column=%+v", column)
	}
	if !isZeroSnapshot(column.Snapshot) {
		t.Fatalf("column has snapshot=%+v", column.Snapshot)
	}
	if got, want := column.Heights, chunk.Heights(); got != want {
		t.Fatalf("column heights=%v, want %v", got, want)
	}

	encoded, err := client.EncodeRenderWorldBatch(batch)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(encoded[:4]); got != "MRW1" {
		t.Fatalf("magic=%q", got)
	}
	if got := binary.LittleEndian.Uint16(encoded[4:6]); got != 1 {
		t.Fatalf("layout version=%d", got)
	}
	if got := binary.LittleEndian.Uint64(encoded[8:16]); got != 1 {
		t.Fatalf("epoch=%d", got)
	}
	if got, want := binary.LittleEndian.Uint32(encoded[16:20]), uint32(core.SectionsPerChunk+1); got != want {
		t.Fatalf("record count=%d, want %d", got, want)
	}

	records := parseMRW1Records(t, encoded)
	if got, want := len(records), core.SectionsPerChunk+1; got != want {
		t.Fatalf("encoded records=%d, want %d", got, want)
	}
	for section := 0; section < core.SectionsPerChunk; section++ {
		record := records[section]
		if record.kind != uint8(client.RenderWorldSectionUpsert) || record.y != int32(section) {
			t.Fatalf("record %d kind/y=%d/%d", section, record.kind, record.y)
		}
		if record.dimension != int32(core.Overworld) || record.x != -2 || record.z != 3 || record.revision != 17 {
			t.Fatalf("record %d header=%+v", section, record)
		}
	}
	columnRecord := records[core.SectionsPerChunk]
	if columnRecord.kind != uint8(client.RenderWorldColumnUpsert) || len(columnRecord.payload) != 512 {
		t.Fatalf("column record=%+v", columnRecord)
	}
	if got := int16(binary.LittleEndian.Uint16(columnRecord.payload[(2*16+1)*2:])); got != core.MinY {
		t.Fatalf("encoded height=%d, want %d", got, core.MinY)
	}
}

func TestBuildRenderWorldChunkBatchRejectsNilChunk(t *testing.T) {
	if _, err := client.BuildRenderWorldChunkBatch(1, core.Overworld, 0, nil); err == nil {
		t.Fatal("want nil chunk error")
	}
}

func TestEncodeRenderWorldBatchEncodesCompactStorage(t *testing.T) {
	single := snapshotForStorage(t, world.StorageSingle)
	indexed4 := snapshotForStorage(t, world.StorageIndexed)
	indexed8 := snapshotForIndexedBits(t, 8)
	direct := snapshotForStorage(t, world.StorageDirect)

	batch := client.RenderWorldBatch{
		Epoch: 9,
		Updates: []client.RenderWorldUpdate{
			sectionUpdate(0, single),
			sectionUpdate(1, indexed4),
			sectionUpdate(2, indexed8),
			sectionUpdate(3, direct),
		},
	}
	encoded, err := client.EncodeRenderWorldBatch(batch)
	if err != nil {
		t.Fatal(err)
	}
	records := parseMRW1Records(t, encoded)
	for index, want := range []struct {
		storage uint8
		bits    uint8
		words   uint16
	}{
		{storage: 0, bits: 0, words: 0},
		{storage: 1, bits: 4, words: 256},
		{storage: 1, bits: 8, words: 512},
		{storage: 2, bits: 15, words: 1024},
	} {
		record := records[index]
		if record.storage != want.storage || record.bits != want.bits {
			t.Fatalf("record %d storage/bits=%d/%d, want %d/%d", index, record.storage, record.bits, want.storage, want.bits)
		}
		if got := binary.LittleEndian.Uint16(record.payload[4:6]); got != want.words {
			t.Fatalf("record %d packed words=%d, want %d", index, got, want.words)
		}
	}
	if got := binary.LittleEndian.Uint64(records[3].payload[8:16]); got != direct.Packed[0] {
		t.Fatalf("direct first packed word=%#x, want %#x", got, direct.Packed[0])
	}
}

func TestEncodeRenderWorldBatchMatchesLiteralMRW1Bytes(t *testing.T) {
	encoded, err := client.EncodeRenderWorldBatch(client.RenderWorldBatch{
		Epoch: 0x1122334455667788,
		Updates: []client.RenderWorldUpdate{
			{Kind: client.RenderWorldReset},
			{
				Kind: client.RenderWorldSectionUpsert,
				Key: core.SectionKey{
					Dimension: -2,
					Pos:       core.SectionPos{X: 0x01020304, Y: 3, Z: -4},
				},
				Revision: 0x0102030405060708,
				Snapshot: world.ContainerSnapshot{
					Kind:   world.StorageSingle,
					Single: core.StoneID,
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		'M', 'R', 'W', '1', 0x01, 0x00, 0x00, 0x00,
		0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11,
		0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,

		0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,

		0x01, 0x00, 0x00, 0x00, 0xfe, 0xff, 0xff, 0xff,
		0x04, 0x03, 0x02, 0x01, 0x03, 0x00, 0x00, 0x00,
		0xfc, 0xff, 0xff, 0xff, 0x08, 0x07, 0x06, 0x05,
		0x04, 0x03, 0x02, 0x01, 0x08, 0x00, 0x00, 0x00,

		0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(encoded, want) {
		t.Fatalf("encoded=% x, want % x", encoded, want)
	}
}

func TestEncodeRenderWorldBatchRejectsOtherwiseValidOversizeBatch(t *testing.T) {
	const recordCount = 510
	direct := snapshotForStorage(t, world.StorageDirect)
	updates := make([]client.RenderWorldUpdate, recordCount)
	for index := range updates {
		updates[index] = client.RenderWorldUpdate{
			Kind:     client.RenderWorldSectionUpsert,
			Key:      core.SectionKey{Pos: core.SectionPos{Y: int32(index % core.SectionsPerChunk)}},
			Revision: uint64(index + 1),
			Snapshot: direct,
		}
	}
	encoded, err := client.EncodeRenderWorldBatch(client.RenderWorldBatch{
		Epoch:   1,
		Updates: updates,
	})
	if err == nil {
		t.Fatal("want 4 MiB validation error")
	}
	if encoded != nil {
		t.Fatalf("oversize batch returned %d bytes", len(encoded))
	}
}

func TestEncodeRenderWorldBatchEncodesResetAndTombstones(t *testing.T) {
	const (
		minI32 = int32(-1 << 31)
		maxI32 = int32(1<<31 - 1)
	)
	encoded, err := client.EncodeRenderWorldBatch(client.RenderWorldBatch{
		Epoch: 12,
		Updates: []client.RenderWorldUpdate{
			{Kind: client.RenderWorldReset},
			{
				Kind:     client.RenderWorldSectionTombstone,
				Key:      core.SectionKey{Dimension: core.DimensionID(minI32), Pos: core.SectionPos{X: maxI32, Y: core.SectionsPerChunk - 1, Z: minI32}},
				Revision: 4,
			},
			{
				Kind:     client.RenderWorldColumnTombstone,
				Key:      core.SectionKey{Dimension: core.DimensionID(maxI32), Pos: core.SectionPos{X: minI32, Z: maxI32}},
				Revision: 5,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint16(encoded[6:8]) != 0 || binary.LittleEndian.Uint32(encoded[20:24]) != 0 {
		t.Fatal("batch reserved fields are nonzero")
	}
	records := parseMRW1Records(t, encoded)
	if got := records[0]; got.kind != uint8(client.RenderWorldReset) || got.storage != 0 || got.bits != 0 ||
		got.dimension != 0 || got.x != 0 || got.y != 0 || got.z != 0 || got.revision != 0 || len(got.payload) != 0 {
		t.Fatalf("reset record=%+v", got)
	}
	if got := records[1]; got.kind != uint8(client.RenderWorldSectionTombstone) || got.dimension != minI32 ||
		got.x != maxI32 || got.y != core.SectionsPerChunk-1 || got.z != minI32 || got.revision != 4 || len(got.payload) != 0 {
		t.Fatalf("section tombstone=%+v", got)
	}
	if got := records[2]; got.kind != uint8(client.RenderWorldColumnTombstone) || got.dimension != maxI32 ||
		got.x != minI32 || got.y != 0 || got.z != maxI32 || got.revision != 5 || len(got.payload) != 0 {
		t.Fatalf("column tombstone=%+v", got)
	}
}

func TestEncodeRenderWorldBatchRejectsInvalidValues(t *testing.T) {
	valid := snapshotForStorage(t, world.StorageSingle)
	direct := snapshotForStorage(t, world.StorageDirect)
	indexed := snapshotForStorage(t, world.StorageIndexed)

	tests := []struct {
		name  string
		batch client.RenderWorldBatch
	}{
		{
			name:  "zero epoch",
			batch: client.RenderWorldBatch{Updates: []client.RenderWorldUpdate{sectionUpdate(0, valid)}},
		},
		{
			name: "reset is not first",
			batch: client.RenderWorldBatch{Epoch: 1, Updates: []client.RenderWorldUpdate{
				sectionUpdate(0, valid),
				{Kind: client.RenderWorldReset},
			}},
		},
		{
			name: "column y is nonzero",
			batch: client.RenderWorldBatch{Epoch: 1, Updates: []client.RenderWorldUpdate{{
				Kind: client.RenderWorldColumnUpsert,
				Key:  core.SectionKey{Pos: core.SectionPos{Y: 1}},
			}}},
		},
		{
			name: "section y is out of range",
			batch: client.RenderWorldBatch{Epoch: 1, Updates: []client.RenderWorldUpdate{func() client.RenderWorldUpdate {
				update := sectionUpdate(core.SectionsPerChunk, valid)
				return update
			}()}},
		},
		{
			name: "direct packed high bits",
			batch: client.RenderWorldBatch{Epoch: 1, Updates: []client.RenderWorldUpdate{func() client.RenderWorldUpdate {
				direct.Packed[0] |= uint64(1) << 60
				return sectionUpdate(0, direct)
			}()}},
		},
		{
			name: "indexed slot is out of range",
			batch: client.RenderWorldBatch{Epoch: 1, Updates: []client.RenderWorldUpdate{func() client.RenderWorldUpdate {
				indexed.Palette = indexed.Palette[:1]
				indexed.Packed[0] = 1
				return sectionUpdate(0, indexed)
			}()}},
		},
		{
			name:  "too many records",
			batch: tooManyRenderWorldUpdates(),
		},
		{
			name: "section unused heights",
			batch: client.RenderWorldBatch{Epoch: 1, Updates: []client.RenderWorldUpdate{func() client.RenderWorldUpdate {
				update := sectionUpdate(0, valid)
				update.Heights[0] = 1
				return update
			}()}},
		},
		{
			name: "column unused snapshot",
			batch: client.RenderWorldBatch{Epoch: 1, Updates: []client.RenderWorldUpdate{{
				Kind:     client.RenderWorldColumnUpsert,
				Snapshot: world.ContainerSnapshot{Palette: []core.BlockID{}},
			}}},
		},
		{
			name: "reset has fields",
			batch: client.RenderWorldBatch{Epoch: 1, Updates: []client.RenderWorldUpdate{{
				Kind:     client.RenderWorldReset,
				Revision: 1,
			}}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := client.EncodeRenderWorldBatch(tc.batch)
			if err == nil {
				t.Fatal("want validation error")
			}
			if encoded != nil {
				t.Fatalf("invalid batch returned %d bytes", len(encoded))
			}
		})
	}
}

func sectionUpdate(y int32, snapshot world.ContainerSnapshot) client.RenderWorldUpdate {
	return client.RenderWorldUpdate{
		Kind:     client.RenderWorldSectionUpsert,
		Key:      core.SectionKey{Dimension: -7, Pos: core.SectionPos{X: -2, Y: y, Z: 3}},
		Revision: 19,
		Snapshot: snapshot,
	}
}

func snapshotForStorage(t *testing.T, want world.StorageKind) world.ContainerSnapshot {
	t.Helper()
	chunk := world.NewChunk(core.ChunkPos{})
	blocks := chunk.Section(0).Blocks
	switch want {
	case world.StorageSingle:
		blocks.Set(0, 0, 0, core.StoneID)
		blocks.Set(0, 0, 0, core.AirID)
		chunk.Compact()
	case world.StorageIndexed:
		blocks.Set(0, 0, 0, core.StoneID)
		chunk.Compact()
	case world.StorageDirect:
		chunk.Compact()
		for index := 0; index < 257; index++ {
			setContainerIndex(blocks, index, core.BlockID(index+1))
		}
		for index := 0; index < 257; index++ {
			setContainerIndex(blocks, index, core.BlockID(index%2+1))
		}
	default:
		t.Fatalf("unknown storage %d", want)
	}
	snapshot := blocks.Snapshot()
	if snapshot.Kind != want {
		t.Fatalf("storage=%d, want %d", snapshot.Kind, want)
	}
	if _, err := world.NewPalettedContainerFromSnapshot(snapshot); err != nil {
		t.Fatalf("snapshot is not valid: %v", err)
	}
	return snapshot
}

func snapshotForIndexedBits(t *testing.T, bits uint8) world.ContainerSnapshot {
	t.Helper()
	chunk := world.NewChunk(core.ChunkPos{})
	blocks := chunk.Section(0).Blocks
	for index := 0; index < 17; index++ {
		setContainerIndex(blocks, index, core.BlockID(index+1))
	}
	chunk.Compact()
	snapshot := blocks.Snapshot()
	if snapshot.Kind != world.StorageIndexed || snapshot.Bits != bits {
		t.Fatalf("storage/bits=%d/%d, want indexed/%d", snapshot.Kind, snapshot.Bits, bits)
	}
	return snapshot
}

func setContainerIndex(blocks *world.PalettedContainer, index int, id core.BlockID) {
	blocks.Set(index&core.SectionMask, index>>8, index>>4&core.SectionMask, id)
}

func tooManyRenderWorldUpdates() client.RenderWorldBatch {
	updates := make([]client.RenderWorldUpdate, 4097)
	for index := range updates {
		updates[index] = client.RenderWorldUpdate{
			Kind: client.RenderWorldSectionTombstone,
			Key:  core.SectionKey{Pos: core.SectionPos{Y: int32(index % core.SectionsPerChunk)}},
		}
	}
	return client.RenderWorldBatch{Epoch: 1, Updates: updates}
}

func isZeroSnapshot(snapshot world.ContainerSnapshot) bool {
	return snapshot.Kind == world.StorageSingle && snapshot.Single == 0 && snapshot.Bits == 0 &&
		snapshot.Palette == nil && snapshot.Packed == nil
}

type mrw1Record struct {
	kind, storage, bits uint8
	dimension, x, y, z  int32
	revision            uint64
	payload             []byte
}

func parseMRW1Records(t *testing.T, encoded []byte) []mrw1Record {
	t.Helper()
	if len(encoded) < mrw1BatchHeaderBytes {
		t.Fatalf("encoded batch too short: %d", len(encoded))
	}
	count := int(binary.LittleEndian.Uint32(encoded[16:20]))
	data := encoded[mrw1BatchHeaderBytes:]
	records := make([]mrw1Record, 0, count)
	for range count {
		if len(data) < mrw1RecordHeaderBytes {
			t.Fatal("truncated record header")
		}
		payloadLength := int(binary.LittleEndian.Uint32(data[28:32]))
		if payloadLength > len(data)-mrw1RecordHeaderBytes {
			t.Fatal("truncated record payload")
		}
		records = append(records, mrw1Record{
			kind:      data[0],
			storage:   data[1],
			bits:      data[2],
			dimension: int32(binary.LittleEndian.Uint32(data[4:8])),
			x:         int32(binary.LittleEndian.Uint32(data[8:12])),
			y:         int32(binary.LittleEndian.Uint32(data[12:16])),
			z:         int32(binary.LittleEndian.Uint32(data[16:20])),
			revision:  binary.LittleEndian.Uint64(data[20:28]),
			payload:   data[mrw1RecordHeaderBytes : mrw1RecordHeaderBytes+payloadLength],
		})
		data = data[mrw1RecordHeaderBytes+payloadLength:]
	}
	if len(data) != 0 {
		t.Fatalf("trailing batch data: %d", len(data))
	}
	return records
}
