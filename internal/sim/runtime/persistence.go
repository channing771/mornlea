package runtime

import (
	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/packages/shared/core"
)

func (engine *Engine) PersistenceSnapshots(maxChunks int, maxBytes int, mode SaveMode) []ChunkSaveSnapshot {
	snapshots := engine.realm.PersistenceSnapshots(maxChunks, maxBytes, realm.SaveMode(mode))
	result := make([]ChunkSaveSnapshot, len(snapshots))
	for index, snapshot := range snapshots {
		result[index] = ChunkSaveSnapshot{
			Key:            snapshot.Key,
			Revision:       snapshot.Revision,
			EstimatedBytes: snapshot.EstimatedBytes,
			Chunk:          snapshot.Chunk,
		}
	}
	return result
}

// TouchChunkForTest 直接递增一个已加载区块的 revision，仅供测试把经由
// SetChunkChestForTest/SetChunkFurnaceForTest 等原始状态覆写标记为脏。
func (engine *Engine) TouchChunkForTest(key core.ChunkKey) {
	dimension := engine.dimension(key.Dimension)
	if dimension == nil {
		return
	}
	dimension.Touch(key.Pos)
}

func (engine *Engine) ApplyPersisted(acks []PersistedChunk) {
	converted := make([]realm.PersistedChunk, len(acks))
	for index, ack := range acks {
		converted[index] = realm.PersistedChunk{Key: ack.Key, Revision: ack.Revision}
	}
	engine.realm.ApplyPersisted(converted)
}

func (engine *Engine) FailPersistence(snapshots []ChunkSaveSnapshot) {
	converted := make([]realm.ChunkSaveSnapshot, len(snapshots))
	for index, snapshot := range snapshots {
		converted[index] = realm.ChunkSaveSnapshot{
			Key:            snapshot.Key,
			Revision:       snapshot.Revision,
			EstimatedBytes: snapshot.EstimatedBytes,
			Chunk:          snapshot.Chunk,
		}
	}
	engine.realm.FailPersistence(converted)
}

func (engine *Engine) PersistenceStats() PersistenceStats {
	stats := engine.realm.PersistenceStats()
	return PersistenceStats{
		DirtyChunks:    stats.DirtyChunks,
		EstimatedBytes: stats.EstimatedBytes,
		InFlightChunks: stats.InFlightChunks,
		UnloadWaiting:  stats.UnloadWaiting,
	}
}
