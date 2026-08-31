package client

import (
	"fmt"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

const (
	renderWorldBatchHeaderBytes   = 24
	renderWorldRecordHeaderBytes  = 32
	renderWorldMaxBatchBytes      = 4 * 1024 * 1024
	renderWorldMaxRecords         = 4096
	renderWorldColumnPayloadBytes = core.SectionSize * core.SectionSize * 2
)

// RenderWorldUpdateKind 标识一条 MRW1 render world 更新的语义。
type RenderWorldUpdateKind uint8

const (
	RenderWorldSectionUpsert    RenderWorldUpdateKind = 1
	RenderWorldColumnUpsert     RenderWorldUpdateKind = 2
	RenderWorldSectionTombstone RenderWorldUpdateKind = 3
	RenderWorldColumnTombstone  RenderWorldUpdateKind = 4
	RenderWorldReset            RenderWorldUpdateKind = 5
)

// RenderWorldUpdate 是一条尚未编码的 render world 更新值。
type RenderWorldUpdate struct {
	Kind     RenderWorldUpdateKind
	Key      core.SectionKey
	Revision uint64
	Snapshot world.ContainerSnapshot
	Heights  world.HeightMap
}

// RenderWorldBatch 是同一 epoch 内原子编码的一组 render world 更新。
type RenderWorldBatch struct {
	Epoch   uint64
	Updates []RenderWorldUpdate
}

// EncodeRenderWorldBatch 把完整验证后的值批编码为 MRW1 小端字节。
func EncodeRenderWorldBatch(batch RenderWorldBatch) ([]byte, error) {
	if batch.Epoch == 0 {
		return nil, fmt.Errorf("client: render world batch has zero epoch")
	}
	if len(batch.Updates) == 0 || len(batch.Updates) > renderWorldMaxRecords {
		return nil, fmt.Errorf("client: render world record count %d is invalid", len(batch.Updates))
	}

	total := renderWorldBatchHeaderBytes
	for index, update := range batch.Updates {
		payloadBytes, err := validateRenderWorldUpdate(index, update)
		if err != nil {
			return nil, err
		}
		total, err = checkedRenderWorldAdd(total, renderWorldRecordHeaderBytes)
		if err != nil {
			return nil, err
		}
		total, err = checkedRenderWorldAdd(total, payloadBytes)
		if err != nil {
			return nil, err
		}
		if total > renderWorldMaxBatchBytes {
			return nil, fmt.Errorf("client: render world batch length %d exceeds %d", total, renderWorldMaxBatchBytes)
		}
	}

	encoded := make([]byte, 0, total)
	encoded = append(encoded, 'M', 'R', 'W', '1')
	encoded = appendRenderWorldU16(encoded, 1)
	encoded = appendRenderWorldU16(encoded, 0)
	encoded = appendRenderWorldU64(encoded, batch.Epoch)
	encoded = appendRenderWorldU32(encoded, uint32(len(batch.Updates)))
	encoded = appendRenderWorldU32(encoded, 0)
	for _, update := range batch.Updates {
		encoded = appendRenderWorldUpdate(encoded, update)
	}
	return encoded, nil
}

// BuildRenderWorldChunkBatch 从一个完整区块生成其全部 section 和 height map 更新。
func BuildRenderWorldChunkBatch(
	epoch uint64,
	dimension core.DimensionID,
	revision uint64,
	chunk *world.Chunk,
) (RenderWorldBatch, error) {
	if epoch == 0 {
		return RenderWorldBatch{}, fmt.Errorf("client: render world batch has zero epoch")
	}
	if chunk == nil {
		return RenderWorldBatch{}, fmt.Errorf("client: render world chunk is nil")
	}

	batch := RenderWorldBatch{
		Epoch:   epoch,
		Updates: make([]RenderWorldUpdate, 0, core.SectionsPerChunk+1),
	}
	for sectionIndex := 0; sectionIndex < core.SectionsPerChunk; sectionIndex++ {
		section := chunk.Section(sectionIndex)
		if section == nil || section.Blocks == nil {
			return RenderWorldBatch{}, fmt.Errorf("client: chunk section %d has nil blocks", sectionIndex)
		}
		batch.Updates = append(batch.Updates, RenderWorldUpdate{
			Kind: RenderWorldSectionUpsert,
			Key: core.SectionKey{
				Dimension: dimension,
				Pos: core.SectionPos{
					X: chunk.Pos.X,
					Y: int32(sectionIndex),
					Z: chunk.Pos.Z,
				},
			},
			Revision: revision,
			Snapshot: section.Blocks.Snapshot(),
		})
	}
	batch.Updates = append(batch.Updates, RenderWorldUpdate{
		Kind: RenderWorldColumnUpsert,
		Key: core.SectionKey{
			Dimension: dimension,
			Pos:       core.SectionPos{X: chunk.Pos.X, Z: chunk.Pos.Z},
		},
		Revision: revision,
		Heights:  chunk.Heights(),
	})
	return batch, nil
}

func validateRenderWorldUpdate(index int, update RenderWorldUpdate) (int, error) {
	switch update.Kind {
	case RenderWorldSectionUpsert:
		if err := validateRenderWorldSectionKey(update.Key); err != nil {
			return 0, err
		}
		if update.Heights != (world.HeightMap{}) {
			return 0, fmt.Errorf("client: section update has heights")
		}
		return renderWorldSectionPayloadBytes(update.Snapshot)

	case RenderWorldColumnUpsert:
		if update.Key.Pos.Y != 0 {
			return 0, fmt.Errorf("client: column update has section Y %d", update.Key.Pos.Y)
		}
		if !renderWorldSnapshotIsZero(update.Snapshot) {
			return 0, fmt.Errorf("client: column update has snapshot")
		}
		return renderWorldColumnPayloadBytes, nil

	case RenderWorldSectionTombstone:
		if err := validateRenderWorldSectionKey(update.Key); err != nil {
			return 0, err
		}
		if !renderWorldSnapshotIsZero(update.Snapshot) || update.Heights != (world.HeightMap{}) {
			return 0, fmt.Errorf("client: section tombstone has payload")
		}
		return 0, nil

	case RenderWorldColumnTombstone:
		if update.Key.Pos.Y != 0 {
			return 0, fmt.Errorf("client: column tombstone has section Y %d", update.Key.Pos.Y)
		}
		if !renderWorldSnapshotIsZero(update.Snapshot) || update.Heights != (world.HeightMap{}) {
			return 0, fmt.Errorf("client: column tombstone has payload")
		}
		return 0, nil

	case RenderWorldReset:
		if index != 0 {
			return 0, fmt.Errorf("client: render world reset is not first")
		}
		if update.Key != (core.SectionKey{}) || update.Revision != 0 ||
			!renderWorldSnapshotIsZero(update.Snapshot) || update.Heights != (world.HeightMap{}) {
			return 0, fmt.Errorf("client: render world reset has fields")
		}
		return 0, nil

	default:
		return 0, fmt.Errorf("client: unknown render world update kind %d", update.Kind)
	}
}

func validateRenderWorldSectionKey(key core.SectionKey) error {
	if key.Pos.Y < 0 || key.Pos.Y >= int32(core.SectionsPerChunk) {
		return fmt.Errorf("client: section update has section Y %d", key.Pos.Y)
	}
	return nil
}

func renderWorldSnapshotIsZero(snapshot world.ContainerSnapshot) bool {
	return snapshot.Kind == world.StorageSingle && snapshot.Single == 0 && snapshot.Bits == 0 &&
		snapshot.Palette == nil && snapshot.Packed == nil
}

func renderWorldSectionPayloadBytes(snapshot world.ContainerSnapshot) (int, error) {
	switch snapshot.Kind {
	case world.StorageSingle:
		if snapshot.Bits != 0 || len(snapshot.Palette) != 0 || len(snapshot.Packed) != 0 {
			return 0, fmt.Errorf("client: single snapshot has compressed payload")
		}
		if !core.RegisteredBlock(snapshot.Single) {
			return 0, fmt.Errorf("client: single block ID %d is unregistered", snapshot.Single)
		}
		return 8, nil

	case world.StorageIndexed:
		if snapshot.Single != 0 {
			return 0, fmt.Errorf("client: indexed snapshot has single value")
		}
		if snapshot.Bits != 4 && snapshot.Bits != 8 {
			return 0, fmt.Errorf("client: indexed snapshot has invalid bits %d", snapshot.Bits)
		}
		if len(snapshot.Palette) == 0 || len(snapshot.Palette) > 1<<snapshot.Bits {
			return 0, fmt.Errorf("client: indexed palette length %d is invalid", len(snapshot.Palette))
		}
		wordCount := renderWorldWordsForBits(snapshot.Bits)
		if len(snapshot.Packed) != wordCount {
			return 0, fmt.Errorf("client: indexed packed length %d, want %d", len(snapshot.Packed), wordCount)
		}
		for paletteIndex, id := range snapshot.Palette {
			if !core.RegisteredBlock(id) {
				return 0, fmt.Errorf("client: indexed palette ID %d is unregistered", id)
			}
			for _, earlier := range snapshot.Palette[:paletteIndex] {
				if id == earlier {
					return 0, fmt.Errorf("client: indexed palette has duplicate block ID %d", id)
				}
			}
		}
		for blockIndex := 0; blockIndex < core.BlocksPerSection; blockIndex++ {
			if renderWorldReadPacked(snapshot.Packed, snapshot.Bits, blockIndex) >= uint32(len(snapshot.Palette)) {
				return 0, fmt.Errorf("client: indexed palette slot at block %d exceeds palette", blockIndex)
			}
		}
		return checkedRenderWorldPayload(8, len(snapshot.Palette), 2, len(snapshot.Packed), 8)

	case world.StorageDirect:
		if snapshot.Single != 0 || len(snapshot.Palette) != 0 {
			return 0, fmt.Errorf("client: direct snapshot has palette or single value")
		}
		if snapshot.Bits != 15 {
			return 0, fmt.Errorf("client: direct snapshot has invalid bits %d", snapshot.Bits)
		}
		if len(snapshot.Packed) != renderWorldWordsForBits(15) {
			return 0, fmt.Errorf("client: direct packed length %d, want %d", len(snapshot.Packed), renderWorldWordsForBits(15))
		}
		for wordIndex, word := range snapshot.Packed {
			if word>>60 != 0 {
				return 0, fmt.Errorf("client: direct packed word %d has unused high bits", wordIndex)
			}
			for slot := 0; slot < 4; slot++ {
				id := core.BlockID((word >> uint(slot*15)) & 0x7fff)
				if !core.RegisteredBlock(id) {
					return 0, fmt.Errorf("client: direct block ID %d is unregistered", id)
				}
			}
		}
		return checkedRenderWorldPayload(8, 0, 2, len(snapshot.Packed), 8)

	default:
		return 0, fmt.Errorf("client: unknown snapshot storage %d", snapshot.Kind)
	}
}

func renderWorldWordsForBits(bits uint8) int {
	return core.BlocksPerSection / (64 / int(bits))
}

func renderWorldReadPacked(words []uint64, bits uint8, blockIndex int) uint32 {
	perWord := 64 / int(bits)
	shift := uint((blockIndex % perWord) * int(bits))
	return uint32((words[blockIndex/perWord] >> shift) & (uint64(1)<<bits - 1))
}

func checkedRenderWorldPayload(
	base, firstCount, firstBytes, secondCount, secondBytes int,
) (int, error) {
	first, err := checkedRenderWorldMultiply(firstCount, firstBytes)
	if err != nil {
		return 0, err
	}
	second, err := checkedRenderWorldMultiply(secondCount, secondBytes)
	if err != nil {
		return 0, err
	}
	total, err := checkedRenderWorldAdd(base, first)
	if err != nil {
		return 0, err
	}
	return checkedRenderWorldAdd(total, second)
}

func checkedRenderWorldAdd(left, right int) (int, error) {
	if left < 0 || right < 0 || left > int(^uint(0)>>1)-right {
		return 0, fmt.Errorf("client: render world encoded size overflows")
	}
	return left + right, nil
}

func checkedRenderWorldMultiply(left, right int) (int, error) {
	if left < 0 || right < 0 || (left != 0 && right > int(^uint(0)>>1)/left) {
		return 0, fmt.Errorf("client: render world encoded size overflows")
	}
	return left * right, nil
}

func appendRenderWorldUpdate(encoded []byte, update RenderWorldUpdate) []byte {
	storage, bits, payloadBytes := renderWorldUpdateMetadata(update)
	encoded = append(encoded, byte(update.Kind), storage, bits, 0)
	encoded = appendRenderWorldU32(encoded, uint32(update.Key.Dimension))
	encoded = appendRenderWorldU32(encoded, uint32(update.Key.Pos.X))
	encoded = appendRenderWorldU32(encoded, uint32(update.Key.Pos.Y))
	encoded = appendRenderWorldU32(encoded, uint32(update.Key.Pos.Z))
	encoded = appendRenderWorldU64(encoded, update.Revision)
	encoded = appendRenderWorldU32(encoded, uint32(payloadBytes))

	switch update.Kind {
	case RenderWorldSectionUpsert:
		encoded = appendRenderWorldSectionPayload(encoded, update.Snapshot)
	case RenderWorldColumnUpsert:
		for _, height := range update.Heights {
			encoded = appendRenderWorldU16(encoded, uint16(height))
		}
	}
	return encoded
}

func renderWorldUpdateMetadata(update RenderWorldUpdate) (storage, bits uint8, payloadBytes int) {
	switch update.Kind {
	case RenderWorldSectionUpsert:
		switch update.Snapshot.Kind {
		case world.StorageIndexed:
			return 1, update.Snapshot.Bits, 8 + len(update.Snapshot.Palette)*2 + len(update.Snapshot.Packed)*8
		case world.StorageDirect:
			return 2, update.Snapshot.Bits, 8 + len(update.Snapshot.Packed)*8
		default:
			return 0, 0, 8
		}
	case RenderWorldColumnUpsert:
		return 0, 0, renderWorldColumnPayloadBytes
	default:
		return 0, 0, 0
	}
}

func appendRenderWorldSectionPayload(encoded []byte, snapshot world.ContainerSnapshot) []byte {
	encoded = appendRenderWorldU16(encoded, uint16(snapshot.Single))
	encoded = appendRenderWorldU16(encoded, uint16(len(snapshot.Palette)))
	encoded = appendRenderWorldU16(encoded, uint16(len(snapshot.Packed)))
	encoded = appendRenderWorldU16(encoded, 0)
	for _, id := range snapshot.Palette {
		encoded = appendRenderWorldU16(encoded, uint16(id))
	}
	for _, word := range snapshot.Packed {
		encoded = appendRenderWorldU64(encoded, word)
	}
	return encoded
}

func appendRenderWorldU16(encoded []byte, value uint16) []byte {
	return append(encoded, byte(value), byte(value>>8))
}

func appendRenderWorldU32(encoded []byte, value uint32) []byte {
	return append(encoded, byte(value), byte(value>>8), byte(value>>16), byte(value>>24))
}

func appendRenderWorldU64(encoded []byte, value uint64) []byte {
	return append(encoded,
		byte(value), byte(value>>8), byte(value>>16), byte(value>>24),
		byte(value>>32), byte(value>>40), byte(value>>48), byte(value>>56),
	)
}
