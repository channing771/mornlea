package chunk

import (
	"fmt"

	"github.com/klauspost/compress/zstd"

	"github.com/channing771/mornlea/packages/server/storage/region"
	"github.com/channing771/mornlea/packages/server/storage/storagedef"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

const (
	// currentChunkSchema 为 v9：追加 8 个流体方块编号。payload 布局与 v8 完全相同，
	// 版本号只用于让旧程序拒绝含流体的记录。
	currentChunkSchema uint32 = 9
	// 压缩载荷上界由 region 包的 region.MaxCompressedChunk 承载（bank 校验与
	// 信封编码共用同一常量，避免两包各自漂移）。
	maxDecodedChunk = 2 << 20

	chunkEnvelopeVersion uint32 = 1
	compressionZstd      uint32 = 1
)

var (
	chunkEnvelopeMagic = [4]byte{'C', 'H', 'N', 'K'}
	chunkLogicalMagic  = [4]byte{'M', 'C', 'G', 'C'}
)

type DecodedPayload struct {
	Key      core.ChunkKey
	Revision uint64
	Schema   uint32
	Chunk    *world.Chunk
	// Migrated 表示记录读自旧 schema，需要在下次正常保存时改写。
	Migrated bool
}

// Encode serializes one full chunk as a bounded, versioned zstd envelope.
func Encode(save ChunkSave) ([]byte, error) {
	if save.Chunk == nil {
		return nil, fmt.Errorf("%w: nil chunk", storagedef.ErrCorrupt)
	}
	if save.Revision == 0 {
		return nil, fmt.Errorf("%w: zero revision", storagedef.ErrCorrupt)
	}
	if save.Chunk.Pos != save.Key.Pos {
		return nil, fmt.Errorf("%w: chunk position does not match key", storagedef.ErrCorrupt)
	}

	logical, err := encodeLogicalChunk(save)
	if err != nil {
		return nil, err
	}
	if len(logical) > maxDecodedChunk {
		return nil, fmt.Errorf("%w: decoded chunk exceeds %d bytes", storagedef.ErrCorrupt, maxDecodedChunk)
	}

	encoder, err := zstd.NewWriter(nil,
		zstd.WithEncoderConcurrency(1),
		zstd.WithEncoderCRC(true),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: create zstd encoder: %v", storagedef.ErrCorrupt, err)
	}
	defer encoder.Close()

	compressed := encoder.EncodeAll(logical, nil)
	if len(compressed) > region.MaxCompressedChunk {
		return nil, fmt.Errorf("%w: compressed chunk exceeds %d bytes", storagedef.ErrCorrupt, region.MaxCompressedChunk)
	}

	payload := make([]byte, 0, 44+len(compressed))
	payload = append(payload, chunkEnvelopeMagic[:]...)
	payload = appendU32(payload, chunkEnvelopeVersion)
	payload = appendU32(payload, currentChunkSchema)
	payload = appendU32(payload, uint32(save.Key.Dimension))
	payload = appendU32(payload, uint32(save.Key.Pos.X))
	payload = appendU32(payload, uint32(save.Key.Pos.Z))
	payload = appendU64(payload, save.Revision)
	payload = appendU32(payload, compressionZstd)
	payload = appendU32(payload, uint32(len(logical)))
	payload = appendU32(payload, uint32(len(compressed)))
	payload = append(payload, compressed...)
	return payload, nil
}

// Decode verifies the envelope and reconstructs an independent chunk.
func Decode(
	wantKey core.ChunkKey,
	wantRevision uint64,
	payload []byte,
) (DecodedPayload, error) {
	if wantRevision == 0 {
		return DecodedPayload{}, fmt.Errorf("%w: zero requested revision", storagedef.ErrCorrupt)
	}

	envelope := byteDecoder{data: payload}
	if err := envelope.magic(chunkEnvelopeMagic); err != nil {
		return DecodedPayload{}, corrupt("envelope magic", err)
	}
	version, err := envelope.u32()
	if err != nil {
		return DecodedPayload{}, corrupt("envelope version", err)
	}
	if version != chunkEnvelopeVersion {
		if version > chunkEnvelopeVersion {
			return DecodedPayload{}, fmt.Errorf("%w: envelope version %d", storagedef.ErrFutureVersion, version)
		}
		return DecodedPayload{}, fmt.Errorf("%w: unsupported envelope version %d", storagedef.ErrCorrupt, version)
	}
	schema, err := envelope.u32()
	if err != nil {
		return DecodedPayload{}, corrupt("envelope schema", err)
	}
	if schema > currentChunkSchema {
		return DecodedPayload{}, fmt.Errorf("%w: chunk schema %d", storagedef.ErrFutureVersion, schema)
	}
	if schema < oldestChunkSchema {
		return DecodedPayload{}, fmt.Errorf("%w: unsupported chunk schema %d", storagedef.ErrCorrupt, schema)
	}
	key, err := decodeKey(&envelope)
	if err != nil {
		return DecodedPayload{}, corrupt("envelope key", err)
	}
	revision, err := envelope.u64()
	if err != nil {
		return DecodedPayload{}, corrupt("envelope revision", err)
	}
	if revision == 0 {
		return DecodedPayload{}, fmt.Errorf("%w: zero revision", storagedef.ErrCorrupt)
	}
	if key != wantKey || revision != wantRevision {
		return DecodedPayload{}, fmt.Errorf("%w: envelope key or revision does not match request", storagedef.ErrCorrupt)
	}
	compression, err := envelope.u32()
	if err != nil {
		return DecodedPayload{}, corrupt("compression ID", err)
	}
	if compression != compressionZstd {
		return DecodedPayload{}, fmt.Errorf("%w: unknown compression ID %d", storagedef.ErrCorrupt, compression)
	}
	decodedLength, err := envelope.u32()
	if err != nil {
		return DecodedPayload{}, corrupt("decoded length", err)
	}
	if decodedLength > maxDecodedChunk {
		return DecodedPayload{}, fmt.Errorf("%w: decoded length %d exceeds limit", storagedef.ErrCorrupt, decodedLength)
	}
	compressedLength, err := envelope.u32()
	if err != nil {
		return DecodedPayload{}, corrupt("compressed length", err)
	}
	if compressedLength > region.MaxCompressedChunk {
		return DecodedPayload{}, fmt.Errorf("%w: compressed length %d exceeds limit", storagedef.ErrCorrupt, compressedLength)
	}
	if uint64(envelope.remaining()) != uint64(compressedLength) {
		return DecodedPayload{}, fmt.Errorf("%w: compressed length does not match envelope", storagedef.ErrCorrupt)
	}
	compressed, err := envelope.take(int(compressedLength))
	if err != nil {
		return DecodedPayload{}, corrupt("compressed bytes", err)
	}

	decoder, err := zstd.NewReader(nil,
		zstd.WithDecoderConcurrency(1),
		zstd.WithDecoderMaxMemory(maxDecodedChunk),
	)
	if err != nil {
		return DecodedPayload{}, fmt.Errorf("%w: create zstd decoder: %v", storagedef.ErrCorrupt, err)
	}
	decompressed, err := decoder.DecodeAll(compressed, make([]byte, 0, int(decodedLength)))
	decoder.Close()
	if err != nil {
		return DecodedPayload{}, fmt.Errorf("%w: decompress chunk: %v", storagedef.ErrCorrupt, err)
	}
	if len(decompressed) != int(decodedLength) {
		return DecodedPayload{}, fmt.Errorf("%w: decoded length does not match envelope", storagedef.ErrCorrupt)
	}

	dto, err := decodeLogicalChunk(key, revision, schema, decompressed)
	if err != nil {
		return DecodedPayload{}, err
	}
	dto, migrated, err := migrateChunk(schema, dto)
	if err != nil {
		return DecodedPayload{}, err
	}
	chunk, err := chunkFromDTO(dto)
	if err != nil {
		return DecodedPayload{}, err
	}
	return DecodedPayload{
		Key: key, Revision: revision, Schema: currentChunkSchema,
		Chunk: chunk, Migrated: migrated,
	}, nil
}

func chunkFromDTO(dto chunkDTO) (*world.Chunk, error) {
	chunk := world.NewChunk(dto.Key.Pos)
	for index, snapshot := range dto.Sections {
		container, err := world.NewPalettedContainerFromSnapshot(snapshot)
		if err != nil {
			return nil, fmt.Errorf("%w: section %d: %v", storagedef.ErrCorrupt, index, err)
		}
		chunk.Section(index).Blocks = container
	}
	// section 是直接装入的，派生高度表需要一次性重建。
	chunk.RebuildHeights()
	for slot, drop := range dto.Drops {
		chunk.SetDrop(slot, drop)
	}
	for slot, furnace := range dto.Furnaces {
		chunk.SetFurnace(slot, furnace)
	}
	for slot, chest := range dto.Chests {
		chunk.SetChest(slot, chest)
	}
	if err := validateFurnaceSlots(chunk); err != nil {
		return nil, fmt.Errorf("%w: %v", storagedef.ErrCorrupt, err)
	}
	if err := validateChestSlots(chunk); err != nil {
		return nil, fmt.Errorf("%w: %v", storagedef.ErrCorrupt, err)
	}
	return chunk, nil
}
