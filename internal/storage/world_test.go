package storage

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

// TestDoorRoundTrip 验证门方块（下半 62..69 + 上半 70）在 chunk schema v9 下的
// 编解码往返一致性。BlockID 以 uint16 直接存于调色板/直存容器，无需 schema 升版或迁移。
func TestDoorRoundTrip(t *testing.T) {
	key := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: 5, Z: -2}}
	chunk := world.NewChunk(key.Pos)

	// 双格门：下半 + 上半垂直相邻，覆盖全部 8 个下半态各配一个上半。
	lowerIDs := []core.BlockID{
		core.DoorLowerSouthClosed, core.DoorLowerSouthOpen,
		core.DoorLowerWestClosed, core.DoorLowerWestOpen,
		core.DoorLowerNorthClosed, core.DoorLowerNorthOpen,
		core.DoorLowerEastClosed, core.DoorLowerEastOpen,
	}
	for i, id := range lowerIDs {
		x := i * 2
		z := 0
		yLower := int32(64)
		chunk.SetBlock(x, yLower, z, id)
		chunk.SetBlock(x, yLower+1, z, core.DoorUpper)
	}
	// 额外：孤立上半与孤立下半亦需单独往返正确
	chunk.SetBlock(15, 70, 15, core.DoorUpper)
	chunk.SetBlock(15, 71, 15, core.DoorLowerSouthClosed)

	encoded, err := encodeChunkPayload(ChunkSave{Key: key, Revision: 42, Chunk: chunk})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := decodeChunkPayload(key, 42, encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Schema != 9 || got.Migrated {
		t.Fatalf("schema=%d migrated=%v，想要 9/false", got.Schema, got.Migrated)
	}
	if got.Chunk.Hash() != chunk.Hash() {
		t.Fatalf("hash 不一致：门方块往返丢失")
	}
	for i, id := range lowerIDs {
		x := i * 2
		z := 0
		yLower := int32(64)
		if gotID := got.Chunk.BlockAt(x, yLower, z); gotID != id {
			t.Fatalf("lower[%d] (%d) 往返 = %d，想要 %d", i, id, gotID, id)
		}
		if gotID := got.Chunk.BlockAt(x, yLower+1, z); gotID != core.DoorUpper {
			t.Fatalf("upper[%d] 往返 = %d，想要 %d", i, gotID, core.DoorUpper)
		}
	}
	if gotID := got.Chunk.BlockAt(15, 70, 15); gotID != core.DoorUpper {
		t.Fatalf("孤立上半往返 = %d，想要 %d", gotID, core.DoorUpper)
	}
	if gotID := got.Chunk.BlockAt(15, 71, 15); gotID != core.DoorLowerSouthClosed {
		t.Fatalf("孤立下半往返 = %d，想要 %d", gotID, core.DoorLowerSouthClosed)
	}
	// 确定性：二次编码字节一致
	second, err := encodeChunkPayload(ChunkSave{Key: key, Revision: 42, Chunk: chunk})
	if err != nil {
		t.Fatalf("second encode: %v", err)
	}
	if string(encoded) != string(second) {
		t.Fatal("同块二次编码不一致")
	}
}

// TestDoorBlockIDsRoundTripExhaustive 穷举 9 个门 ID 各自单独成段时的往返。
func TestDoorBlockIDsRoundTripExhaustive(t *testing.T) {
	key := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: -3, Z: 7}}
	for id := core.DoorLowerSouthClosed; id <= core.DoorUpper; id++ {
		chunk := world.NewChunk(key.Pos)
		chunk.SetBlock(0, core.MinY, 0, id)
		encoded, err := encodeChunkPayload(ChunkSave{Key: key, Revision: 1, Chunk: chunk})
		if err != nil {
			t.Fatalf("门 %d 编码失败: %v", id, err)
		}
		got, err := decodeChunkPayload(key, 1, encoded)
		if err != nil {
			t.Fatalf("门 %d 解码失败: %v", id, err)
		}
		if gotID := got.Chunk.BlockAt(0, core.MinY, 0); gotID != id {
			t.Fatalf("门 %d 往返 = %d", id, gotID)
		}
	}
}
