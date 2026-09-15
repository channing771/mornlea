package client

import (
	"container/heap"

	"github.com/channing771/mornlea/packages/shared/core"
)

type readySectionHeap struct {
	keys    []core.SectionKey
	indexes map[core.SectionKey]int
	// center 是当前排序中心；只在持有 mesher.mu 的路径上读写
	// （Add/Remove/Take/SetCenter），worker 消费路径不直接触碰。
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

// SetCenter 同步视图中心：区块粒度比较，未变化时只是一次结构比较；变化时
// 整体重建堆一次（O(n)）。中心由 `Schedule` 每帧传入，相机在区块内的小幅
// 漂移不会触发重建，跨界频率远低于帧率，因此不需要每帧 O(n log n) 全量
// 重排。调用方必须已持有 mesher.mu。
func (ready *readySectionHeap) SetCenter(center ViewCenter) {
	if ready.center == center {
		return
	}
	ready.center = center
	heap.Init(ready)
}

// readySectionLess 是就绪堆的复合键比较：维度仍是最外层（其他维度的键即使
// 距离更近也排在当前维度之后），同维度内按到视图中心的水平平方距离升序，
// 平局回落到既有 sectionKeyLess 字典序。距离只取水平分量并与上传调度
// `FlushUploads` 的近处优先键对齐：同一区块列内的高低区段共享水平距离，
// 先后由字典序的 Y 次序兜底，排序因此不依赖相机高度，中心只需区块粒度。
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

// sectionChunkDistance2 返回区段所在区块到视图中心的水平平方距离。
// 差值与平方都在 int64 上累加：ready 队列只容纳服务端订阅范围内已加载
// 区块的区段，到中心的距离实际以视距为上界，int64 余量巨大；这里采用
// 「先拓宽再相减」的写法，比渲染侧参考实现（int32 求差后再转 int64）更
// 安全，即使未来放宽坐标域也不会在求差或平方上溢出。
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
