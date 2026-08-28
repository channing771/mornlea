package codec

import (
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network/protocol"
)

var benchmarkPayload []byte

func worstLegalBenchmarkSnapshot() protocol.ChunkSnapshot {
	sections := make([]protocol.SectionData, core.SectionsPerChunk)
	valuesPerWord := 64 / 15
	words := (core.BlocksPerSection + valuesPerWord - 1) / valuesPerWord
	for section := range sections {
		packed := make([]uint64, words)
		for index := 0; index < core.BlocksPerSection; index++ {
			value := uint64((section + index) % int(core.MossyCobblestoneID+1))
			packed[index/valuesPerWord] |= value << uint((index%valuesPerWord)*15)
		}
		sections[section] = protocol.SectionData{Y: int32(section), Storage: protocol.SectionDirect, Bits: 15, Packed: packed}
	}
	return protocol.ChunkSnapshot{Dimension: core.Overworld, Chunk: core.ChunkPos{X: -3, Z: 7}, Revision: 19, Sections: sections}
}
