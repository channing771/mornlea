package client

import (
	"container/heap"

	"github.com/channing771/mornlea/packages/shared/core"
)

type readySectionHeap struct {
	keys    []core.SectionKey
	indexes map[core.SectionKey]int
	// `center` is the current ordering origin. Only paths holding `mesher.mu`
	// (`Add`, `Remove`, `Take`, and `SetCenter`) may access it; workers do not.
	center ViewCenter
}

func newReadySectionHeap() readySectionHeap {
	return readySectionHeap{indexes: make(map[core.SectionKey]int)}
}

func (ready *readySectionHeap) Len() int { return len(ready.keys) }
func (ready *readySectionHeap) Less(i, j int) bool {
	return readySectionLess(ready.keys[i], ready.keys[j], ready.center)
}
func (ready *readySectionHeap) Swap(i, j int) {
	ready.keys[i], ready.keys[j] = ready.keys[j], ready.keys[i]
	ready.indexes[ready.keys[i]] = i
	ready.indexes[ready.keys[j]] = j
}
func (ready *readySectionHeap) Push(value any) {
	key := value.(core.SectionKey)
	ready.indexes[key] = len(ready.keys)
	ready.keys = append(ready.keys, key)
}
func (ready *readySectionHeap) Pop() any {
	last := len(ready.keys) - 1
	key := ready.keys[last]
	ready.keys[last] = core.SectionKey{}
	ready.keys = ready.keys[:last]
	delete(ready.indexes, key)
	return key
}
func (ready *readySectionHeap) Add(key core.SectionKey) bool {
	if ready.indexes == nil {
		ready.indexes = make(map[core.SectionKey]int)
	}
	if _, exists := ready.indexes[key]; exists {
		return false
	}
	heap.Push(ready, key)
	return true
}
func (ready *readySectionHeap) Remove(key core.SectionKey) bool {
	index, exists := ready.indexes[key]
	if !exists {
		return false
	}
	heap.Remove(ready, index)
	return true
}
func (ready *readySectionHeap) Take() (core.SectionKey, bool) {
	if ready.Len() == 0 {
		return core.SectionKey{}, false
	}
	return heap.Pop(ready).(core.SectionKey), true
}

// `SetCenter` synchronizes the chunk-scale view center. An unchanged center costs
// one structural comparison; a changed center rebuilds the heap once in O(n).
// `Schedule` supplies the value each frame, so camera motion within a chunk does
// not rebuild the heap and chunk-boundary crossings remain much less frequent
// than frames. The caller must hold `mesher.mu`.
func (ready *readySectionHeap) SetCenter(center ViewCenter) {
	if ready.center == center {
		return
	}
	ready.center = center
	heap.Init(ready)
}

// `readySectionLess` compares the ready heap's compound key. Dimension remains
// outermost, even when another dimension has a closer key. Within the center's
// dimension, horizontal squared distance sorts ascending, with the existing
// `sectionKeyLess` order breaking ties. Using only horizontal distance matches
// the nearest-first key in `FlushUploads`: vertical sections in one chunk
// column share a distance and fall back to lexical `Y`, so camera height cannot
// affect ordering and the center needs only chunk granularity.
func readySectionLess(left, right core.SectionKey, center ViewCenter) bool {
	if left.Dimension != right.Dimension {
		return left.Dimension < right.Dimension
	}
	if left.Dimension == center.Dimension {
		leftDistance := sectionChunkDistance2(left.Pos, center.Chunk)
		rightDistance := sectionChunkDistance2(right.Pos, center.Chunk)
		if leftDistance != rightDistance {
			return leftDistance < rightDistance
		}
	}
	return sectionKeyLess(left, right)
}

// `sectionChunkDistance2` returns the horizontal squared distance from a
// section's chunk to the view center. Differences and squares use `int64`.
// Ready entries are loaded sections inside the server subscription radius, so
// view distance bounds the real values with ample headroom. Widening before
// subtraction is safer than the renderer reference implementation and avoids
// overflow if the coordinate domain expands later.
func sectionChunkDistance2(pos core.SectionPos, center core.ChunkPos) int64 {
	dx := int64(pos.X) - int64(center.X)
	dz := int64(pos.Z) - int64(center.Z)
	return dx*dx + dz*dz
}

func sectionKeyLess(left, right core.SectionKey) bool {
	if left.Dimension != right.Dimension {
		return left.Dimension < right.Dimension
	}
	if left.Pos.X != right.Pos.X {
		return left.Pos.X < right.Pos.X
	}
	if left.Pos.Z != right.Pos.Z {
		return left.Pos.Z < right.Pos.Z
	}
	return left.Pos.Y < right.Pos.Y
}
