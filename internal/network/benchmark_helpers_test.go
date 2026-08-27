package network

import "github.com/channing771/mornlea/internal/core"

var benchmarkPayload []byte

func worstLegalBenchmarkSnapshot() ChunkSnapshot {
	sections := make([]SectionData, core.SectionsPerChunk)
	valuesPerWord := 64 / 15
	words := (core.BlocksPerSection + valuesPerWord - 1) / valuesPerWord
	for section := range sections {
		packed := make([]uint64, words)
		for index := 0; index < core.BlocksPerSection; index++ {
			value := uint64((section + index) % int(core.MossyCobblestoneID+1))
			packed[index/valuesPerWord] |= value << uint((index%valuesPerWord)*15)
		}
		sections[section] = SectionData{Y: int32(section), Storage: SectionDirect, Bits: 15, Packed: packed}
	}
	return ChunkSnapshot{Dimension: core.Overworld, Chunk: core.ChunkPos{X: -3, Z: 7}, Revision: 19, Sections: sections}
}
