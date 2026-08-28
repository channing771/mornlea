package protocol

import (
	"errors"

	"github.com/channing771/mornlea/internal/core"
)

type ForgetChunks struct {
	Dimension core.DimensionID
	Chunks    []core.ChunkPos
}

func (ForgetChunks) serverMessage() {}
func (ForgetChunks) serverPacket()  {}

func (forget ForgetChunks) Validate() error {
	if len(forget.Chunks) < 1 || len(forget.Chunks) > 4096 {
		return errors.New("network: forget chunks count is outside 1..4096")
	}
	seen := make(map[core.ChunkPos]struct{}, len(forget.Chunks))
	for _, chunk := range forget.Chunks {
		if _, duplicate := seen[chunk]; duplicate {
			return errors.New("network: forget chunks contains a duplicate chunk")
		}
		seen[chunk] = struct{}{}
	}
	return nil
}

// MaxChunkBlockIndex 是区块内方块索引的排他上限；`ItemDrop.validate` 的
// BlockIndex 值域检查与掉落物编解码测试共用。
const MaxChunkBlockIndex = core.SectionsPerChunk * core.BlocksPerSection
