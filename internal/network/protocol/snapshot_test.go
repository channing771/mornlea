package protocol_test

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network/protocol"
)

func TestChunkSnapshotValidatesCanonicalSections(t *testing.T) {
	snapshot := validChunkSnapshot()
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("合法快照被拒绝: %v", err)
	}
	if got, want := snapshot.PayloadBytes(), core.SectionsPerChunk*2; got != want {
		t.Fatalf("PayloadBytes = %d，想要 %d", got, want)
	}

	indexed := protocol.SectionData{
		Y:       0,
		Storage: protocol.SectionIndexed,
		Bits:    4,
		Palette: []core.BlockID{core.AirID, core.StoneID},
		Packed:  make([]uint64, 256),
	}
	if err := indexed.Validate(); err != nil {
		t.Fatalf("合法 indexed section 被拒绝: %v", err)
	}
	if got, want := indexed.PayloadBytes(), 2*2+256*8; got != want {
		t.Fatalf("indexed PayloadBytes = %d，想要 %d", got, want)
	}
}

func TestChunkSnapshotRejectsMalformedSections(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*protocol.ChunkSnapshot)
	}{
		{
			name: "section count",
			mutate: func(snapshot *protocol.ChunkSnapshot) {
				snapshot.Sections = snapshot.Sections[:23]
			},
		},
		{
			name: "duplicate and missing Y",
			mutate: func(snapshot *protocol.ChunkSnapshot) {
				snapshot.Sections[23].Y = 22
			},
		},
		{
			name: "out of order Y",
			mutate: func(snapshot *protocol.ChunkSnapshot) {
				snapshot.Sections[0], snapshot.Sections[1] =
					snapshot.Sections[1], snapshot.Sections[0]
			},
		},
		{
			name: "unknown storage",
			mutate: func(snapshot *protocol.ChunkSnapshot) {
				snapshot.Sections[0].Storage = protocol.SectionStorage(99)
			},
		},
		{
			name: "illegal bits",
			mutate: func(snapshot *protocol.ChunkSnapshot) {
				snapshot.Sections[0] = validIndexedSection(0)
				snapshot.Sections[0].Bits = 5
			},
		},
		{
			name: "wrong packed words",
			mutate: func(snapshot *protocol.ChunkSnapshot) {
				snapshot.Sections[0] = validIndexedSection(0)
				snapshot.Sections[0].Packed = snapshot.Sections[0].Packed[:255]
			},
		},
		{
			name: "indexed slot outside palette",
			mutate: func(snapshot *protocol.ChunkSnapshot) {
				snapshot.Sections[0] = validIndexedSection(0)
				snapshot.Sections[0].Palette = snapshot.Sections[0].Palette[:1]
				snapshot.Sections[0].Packed[0] = 1
			},
		},
		{
			name: "block ID exceeds 15 bits",
			mutate: func(snapshot *protocol.ChunkSnapshot) {
				snapshot.Sections[0].Single = core.BlockID(1 << 15)
			},
		},
		{
			name: "direct unused high bits",
			mutate: func(snapshot *protocol.ChunkSnapshot) {
				snapshot.Sections[0] = protocol.SectionData{
					Y:       0,
					Storage: protocol.SectionDirect,
					Bits:    15,
					Packed:  make([]uint64, 1024),
				}
				snapshot.Sections[0].Packed[0] = uint64(1) << 63
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := validChunkSnapshot()
			tc.mutate(&snapshot)
			if err := snapshot.Validate(); err == nil {
				t.Fatal("想要快照验证错误")
			}
		})
	}
}

func TestSectionDataRejectsUnknownBlockEveryStorage(t *testing.T) {
	// 未注册编号一律用独占哨兵 core.BlockIDMax 表达：写死具体编号（历史上写过
	// MossyCobblestoneID+1、WaterLevel7ID+1）会在追加新方块时静默变成已注册。
	unknown := core.BlockIDMax
	tests := []struct {
		name    string
		section protocol.SectionData
	}{
		{
			name: "single",
			section: protocol.SectionData{
				Storage: protocol.SectionSingle,
				Single:  unknown,
			},
		},
		{
			name: "indexed",
			section: protocol.SectionData{
				Storage: protocol.SectionIndexed,
				Bits:    4,
				Palette: []core.BlockID{core.AirID, unknown},
				Packed:  make([]uint64, 256),
			},
		},
		{
			name: "direct",
			section: protocol.SectionData{
				Storage: protocol.SectionDirect,
				Bits:    15,
				Packed:  append([]uint64{uint64(unknown)}, make([]uint64, 1023)...),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.section.Validate(); err == nil {
				t.Fatalf("未注册方块 %d 被接受", unknown)
			}
		})
	}
}

func TestBlockChangesValidateRevisionPositionAndOrder(t *testing.T) {
	valid := protocol.BlockChanges{
		Dimension:    core.Overworld,
		Chunk:        core.ChunkPos{X: -2, Z: 1},
		BaseRevision: 3,
		NewRevision:  4,
		Changes: []protocol.BlockChange{
			{
				Position: core.BlockPos{X: -32, Y: core.MinY, Z: 16},
				Block:    core.StoneID,
			},
			{
				Position: core.BlockPos{X: -31, Y: core.MinY, Z: 16},
				Block:    core.DirtID,
			},
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("合法增量被拒绝: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*protocol.BlockChanges)
	}{
		{
			name: "revision gap",
			mutate: func(changes *protocol.BlockChanges) {
				changes.NewRevision = 5
			},
		},
		{
			name: "wrong chunk",
			mutate: func(changes *protocol.BlockChanges) {
				changes.Changes[0].Position.X = -33
			},
		},
		{
			name: "outside world height",
			mutate: func(changes *protocol.BlockChanges) {
				changes.Changes[0].Position.Y = core.MaxY
			},
		},
		{
			name: "not strictly index sorted",
			mutate: func(changes *protocol.BlockChanges) {
				changes.Changes[0], changes.Changes[1] =
					changes.Changes[1], changes.Changes[0]
			},
		},
		{
			name: "unregistered block ID",
			mutate: func(changes *protocol.BlockChanges) {
				// 未注册编号的独占哨兵，见上。
				changes.Changes[0].Block = core.BlockIDMax
			},
		},
		{
			name: "too many changes",
			mutate: func(changes *protocol.BlockChanges) {
				changes.Changes = make([]protocol.BlockChange, 4097)
				for index := range changes.Changes {
					changes.Changes[index] = protocol.BlockChange{
						Position: core.BlockPos{
							X: int32(index % core.SectionSize),
							Y: core.MinY + int32(index/(core.SectionSize*core.SectionSize)),
							Z: int32((index / core.SectionSize) % core.SectionSize),
						},
						Block: core.StoneID,
					}
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			changes := valid
			changes.Changes = append([]protocol.BlockChange(nil), valid.Changes...)
			tc.mutate(&changes)
			if err := changes.Validate(); err == nil {
				t.Fatal("想要增量验证错误")
			}
		})
	}
}

func validChunkSnapshot() protocol.ChunkSnapshot {
	sections := make([]protocol.SectionData, core.SectionsPerChunk)
	for y := range sections {
		sections[y] = protocol.SectionData{
			Y:       int32(y),
			Storage: protocol.SectionSingle,
			Single:  core.AirID,
		}
	}
	return protocol.ChunkSnapshot{
		Dimension: core.Overworld,
		Chunk:     core.ChunkPos{X: -2, Z: 1},
		Revision:  1,
		Sections:  sections,
	}
}

func validIndexedSection(y int32) protocol.SectionData {
	return protocol.SectionData{
		Y:       y,
		Storage: protocol.SectionIndexed,
		Bits:    4,
		Palette: []core.BlockID{core.AirID, core.StoneID},
		Packed:  make([]uint64, 256),
	}
}
