package realm

import (
	"fmt"
	"sort"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

type SaveMode uint8

const (
	SaveUrgent SaveMode = iota
	SaveAll
)

type ChunkSaveSnapshot struct {
	Key            core.ChunkKey
	Revision       uint64
	EstimatedBytes int
	Chunk          *world.Chunk
}

type PersistedChunk struct {
	Key      core.ChunkKey
	Revision uint64
}

type PersistenceStats struct {
	DirtyChunks    int
	EstimatedBytes int64
	InFlightChunks int
	UnloadWaiting  int
}

type persistenceCandidate struct {
	key    core.ChunkKey
	record *ChunkRecord
}

type persistenceInFlight struct {
	revision       uint64
	estimatedBytes int
}

func (state *State) PersistenceSnapshots(maxChunks int, maxBytes int, mode SaveMode) []ChunkSaveSnapshot {
	candidates := make([]persistenceCandidate, 0)
	for dimensionID, dimension := range state.dimensions {
		// 候选收集只迭代脏区块索引；索引成员资格由 refreshRecord 与记录同生命
		// 周期维护，迭代时仍复验原有全部过滤条件，保证与全量扫描语义一致。
		for pos := range dimension.dirtyIndex {
			record := dimension.records[pos]
			key := core.ChunkKey{Dimension: dimensionID, Pos: pos}
			_, inFlight := state.persistenceInFlight(key, record)
			if record.Chunk == nil || !record.Dirty() || inFlight || mode == SaveUrgent && !record.UnloadRequested {
				continue
			}
			candidates = append(candidates, persistenceCandidate{key: key, record: record})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if left.record.UnloadRequested != right.record.UnloadRequested {
			return left.record.UnloadRequested
		}
		return chunkKeyLess(left.key, right.key)
	})

	if state.inFlightSaves == nil {
		state.inFlightSaves = make(map[core.ChunkKey]persistenceInFlight)
	}
	snapshots := make([]ChunkSaveSnapshot, 0, min(maxChunks, len(candidates)))
	estimatedBytes := 0
	for _, candidate := range candidates {
		estimate := estimateChunkBytes(candidate.record.Chunk)
		if len(snapshots) > 0 && (len(snapshots) >= maxChunks || estimatedBytes+estimate > maxBytes) {
			break
		}
		clone := candidate.record.Chunk.Clone()
		candidate.record.SaveInFlightRevision = candidate.record.Revision
		state.inFlightSaves[candidate.key] = persistenceInFlight{revision: candidate.record.Revision, estimatedBytes: estimate}
		// 派发只置 SaveInFlightRevision 与在途条目，不影响 Dirty() 贡献；对
		// EstimatedBytes 的增量维护是累加在途估算字节，与脏估算双计入。
		state.inFlightEstimatedBytes += int64(estimate)
		snapshots = append(snapshots, ChunkSaveSnapshot{
			Key: candidate.key, Revision: candidate.record.Revision, EstimatedBytes: estimate, Chunk: clone,
		})
		estimatedBytes += estimate
	}
	return snapshots
}

func (state *State) ApplyPersisted(acks []PersistedChunk) {
	for _, ack := range acks {
		dimension := state.dimensions[ack.Key.Dimension]
		if dimension == nil {
			continue
		}
		record := dimension.records[ack.Key.Pos]
		if record == nil {
			continue
		}
		if ack.Revision > record.Revision {
			panic(fmt.Sprintf("sim: persisted revision %d exceeds current revision %d at %+v", ack.Revision, record.Revision, ack.Key))
		}
		record.PersistedRevision = max(record.PersistedRevision, ack.Revision)
		inFlight, exists := state.persistenceInFlight(ack.Key, record)
		if exists && inFlight.revision == ack.Revision {
			record.SaveInFlightRevision = 0
			delete(state.inFlightSaves, ack.Key)
			state.inFlightEstimatedBytes -= int64(inFlight.estimatedBytes)
		}
		if ack.Revision == record.Revision {
			record.NeedsRewrite = false
		}
		dimension.refreshRecord(ack.Key.Pos, record)
		dimension.deleteCleanUnloading(ack.Key.Pos)
	}
}

func (state *State) FailPersistence(snapshots []ChunkSaveSnapshot) {
	for _, snapshot := range snapshots {
		dimension := state.dimensions[snapshot.Key.Dimension]
		if dimension == nil {
			continue
		}
		record := dimension.records[snapshot.Key.Pos]
		if record == nil {
			continue
		}
		inFlight, exists := state.persistenceInFlight(snapshot.Key, record)
		if !exists || inFlight.revision != snapshot.Revision {
			continue
		}
		record.SaveInFlightRevision = 0
		delete(state.inFlightSaves, snapshot.Key)
		// 失败结算只清在途标记与条目，Dirty() 贡献不受影响，无需刷新记录。
		state.inFlightEstimatedBytes -= int64(inFlight.estimatedBytes)
	}
}

// PersistenceStats 是 O(维度数) 的增量聚合读取：不触碰任何区块记录，成本与已
// 加载区块数解耦。EstimatedBytes 同时含各维度脏区块的当前估算与在途快照估算
// （「脏且在途」双计入的现行语义）；InFlightChunks 取在途条目数——条目与记录
// 标记由派发/结算路径成对维护，一致性由 persistenceInFlight 的 panic 检查兜底。
func (state *State) PersistenceStats() PersistenceStats {
	var stats PersistenceStats
	for _, dimension := range state.dimensions {
		stats.DirtyChunks += dimension.dirtyChunks
		stats.EstimatedBytes += dimension.dirtyEstimatedBytes
		stats.UnloadWaiting += dimension.unloadWaiting
	}
	stats.EstimatedBytes += state.inFlightEstimatedBytes
	stats.InFlightChunks = len(state.inFlightSaves)
	return stats
}

func estimateChunkBytes(chunk *world.Chunk) int {
	return chunk.PayloadBytes()
}

func (state *State) persistenceInFlight(key core.ChunkKey, record *ChunkRecord) (persistenceInFlight, bool) {
	inFlight, exists := state.inFlightSaves[key]
	marked := record.SaveInFlightRevision != 0
	if exists != marked || exists && inFlight.revision != record.SaveInFlightRevision {
		panic(fmt.Sprintf("sim: inconsistent persistence in-flight state at %+v", key))
	}
	return inFlight, exists
}
