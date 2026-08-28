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
		for pos, record := range dimension.records {
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
		}
		if ack.Revision == record.Revision {
			record.NeedsRewrite = false
		}
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
	}
}

func (state *State) PersistenceStats() PersistenceStats {
	var stats PersistenceStats
	for dimensionID, dimension := range state.dimensions {
		for pos, record := range dimension.records {
			if record.Dirty() && record.Chunk != nil {
				stats.DirtyChunks++
				stats.EstimatedBytes += int64(estimateChunkBytes(record.Chunk))
			}
			key := core.ChunkKey{Dimension: dimensionID, Pos: pos}
			if inFlight, exists := state.persistenceInFlight(key, record); exists {
				stats.InFlightChunks++
				stats.EstimatedBytes += int64(inFlight.estimatedBytes)
			}
			if record.UnloadRequested {
				stats.UnloadWaiting++
			}
		}
	}
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
