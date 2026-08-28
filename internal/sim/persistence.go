package sim

import "github.com/channing771/mornlea/internal/core"

func (engine *Engine) PersistenceSnapshots(maxChunks int, maxBytes int, mode SaveMode) []ChunkSaveSnapshot {
	return engine.realm.PersistenceSnapshots(maxChunks, maxBytes, mode)
}

// TouchChunkForTest 直接递增一个已加载区块的 revision，仅供测试把经由
// SetChunkChestForTest/SetChunkFurnaceForTest 等原始状态覆写标记为脏。
func (engine *Engine) TouchChunkForTest(key core.ChunkKey) {
	dimension := engine.dimensions[key.Dimension]
	if dimension == nil {
		return
	}
	if record, ok := dimension.Records[key.Pos]; ok {
		record.Revision++
	}
}

func (engine *Engine) ApplyPersisted(acks []PersistedChunk) {
	engine.realm.ApplyPersisted(acks)
}

func (engine *Engine) FailPersistence(snapshots []ChunkSaveSnapshot) {
	engine.realm.FailPersistence(snapshots)
}

func (engine *Engine) PersistenceStats() PersistenceStats {
	return engine.realm.PersistenceStats()
}
