package client

import (
	"reflect"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// takeAllReady 依次取出就绪堆的全部键，供顺序断言复用。
func takeAllReady(ready *readySectionHeap) []core.SectionKey {
	keys := make([]core.SectionKey, 0, ready.Len())
	for {
		key, ok := ready.Take()
		if !ok {
			return keys
		}
		keys = append(keys, key)
	}
}

// TestReadySectionHeapOrdersNearestFirstThenLexicographic 钉住复合键契约：
// 维度仍是最外层（其他维度的键即使距离更近也排在当前维度之后），同维度内
// 按到视图中心的水平平方距离升序，距离平局回落到既有字典序（X、Z、Y）。
// 同一区块列内的高低区段共享水平距离，先后完全由字典序的 Y 次序决定。
func TestReadySectionHeapOrdersNearestFirstThenLexicographic(t *testing.T) {
	ready := newReadySectionHeap()
	ready.SetCenter(ViewCenter{Dimension: core.Overworld, Chunk: core.ChunkPos{}})
	near := core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{}}
	tieLow := core.SectionKey{
		Dimension: core.Overworld,
		Pos:       core.SectionPos{X: 0, Y: 0, Z: 1},
	}
	tieHigh := core.SectionKey{
		Dimension: core.Overworld,
		Pos:       core.SectionPos{X: 0, Y: 3, Z: 1},
	}
	tieEast := core.SectionKey{
		Dimension: core.Overworld,
		Pos:       core.SectionPos{X: 1},
	}
	far := core.SectionKey{
		Dimension: core.Overworld,
		Pos:       core.SectionPos{X: 3, Z: 4},
	}
	otherDimension := core.SectionKey{Dimension: core.Depths, Pos: core.SectionPos{}}
	for _, key := range []core.SectionKey{far, tieEast, near, otherDimension, tieHigh, tieLow, near} {
		ready.Add(key)
	}
	if ready.Add(near) {
		t.Fatal("重复 Add 返回 true")
	}
	want := []core.SectionKey{near, tieLow, tieHigh, tieEast, far, otherDimension}
	if got := takeAllReady(&ready); !reflect.DeepEqual(got, want) {
		t.Fatalf("Take 顺序 = %+v，想要 %+v", got, want)
	}
}

// TestReadySectionHeapOrderIndependentOfInsertionOrder 钉住「出队序与插入
// 顺序无关」：同一键集按多种顺序插入，出队序列必须逐位一致。
func TestReadySectionHeapOrderIndependentOfInsertionOrder(t *testing.T) {
	keys := []core.SectionKey{
		{Dimension: core.Overworld, Pos: core.SectionPos{X: -2}},
		{Dimension: core.Overworld, Pos: core.SectionPos{}},
		{Dimension: core.Overworld, Pos: core.SectionPos{X: 5, Z: -1}},
		{Dimension: core.Overworld, Pos: core.SectionPos{X: 1, Y: 2}},
		{Dimension: core.Overworld, Pos: core.SectionPos{Z: -3}},
	}
	orders := [][]int{
		{0, 1, 2, 3, 4},
		{4, 3, 2, 1, 0},
		{2, 0, 4, 1, 3},
	}
	var want []core.SectionKey
	for run, order := range orders {
		ready := newReadySectionHeap()
		ready.SetCenter(ViewCenter{Dimension: core.Overworld, Chunk: core.ChunkPos{}})
		for _, index := range order {
			ready.Add(keys[index])
		}
		got := takeAllReady(&ready)
		if run == 0 {
			want = got
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("插入顺序 %v 的 Take 顺序 = %+v，想要 %+v", order, got, want)
		}
	}
}

// TestReadySectionHeapRecentersWhenCenterMoves 钉住中心移动后的惰性重排：
// 按旧中心排序的待烘焙键，在 SetCenter 到新中心后必须按新中心距离出队。
func TestReadySectionHeapRecentersWhenCenterMoves(t *testing.T) {
	ready := newReadySectionHeap()
	west := core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{}}
	east := core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{X: 8}}
	ready.SetCenter(ViewCenter{Dimension: core.Overworld, Chunk: core.ChunkPos{X: -1}})
	ready.Add(west)
	ready.Add(east)
	if got := takeAllReady(&ready); !reflect.DeepEqual(got, []core.SectionKey{west, east}) {
		t.Fatalf("旧中心下 Take 顺序 = %+v，想要 [west east]", got)
	}

	ready = newReadySectionHeap()
	ready.SetCenter(ViewCenter{Dimension: core.Overworld, Chunk: core.ChunkPos{X: -1}})
	ready.Add(west)
	ready.Add(east)
	ready.SetCenter(ViewCenter{Dimension: core.Overworld, Chunk: core.ChunkPos{X: 9}})
	if got := takeAllReady(&ready); !reflect.DeepEqual(got, []core.SectionKey{east, west}) {
		t.Fatalf("新中心下 Take 顺序 = %+v，想要 [east west]", got)
	}
}

func TestReadySectionHeapRemoveMaintainsIndexes(t *testing.T) {
	ready := newReadySectionHeap()
	left := core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{X: -1}}
	middle := core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{}}
	right := core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{X: 1}}
	for _, key := range []core.SectionKey{middle, right, left} {
		ready.Add(key)
	}
	if !ready.Remove(middle) || ready.Remove(middle) {
		t.Fatal("Remove 未精确报告存在性")
	}
	if !ready.Add(middle) || !ready.Remove(left) {
		t.Fatal("删除后重新添加或交换索引失败")
	}
	first, ok := ready.Take()
	if !ok || first != middle {
		t.Fatalf("首项 = %+v,%v，想要 middle", first, ok)
	}
	second, ok := ready.Take()
	if !ok || second != right {
		t.Fatalf("次项 = %+v,%v，想要 right", second, ok)
	}
}

// TestMesherSchedulePrefersNearestToViewCenter 经 `Schedule` 调用面钉住中心
// 传递：同帧内两个可网格化的脏段，靠近视图中心的那个必须先被投递；下一帧
// 中心移动到另一侧后，投递优先级随之反转。
func TestMesherSchedulePrefersNearestToViewCenter(t *testing.T) {
	mesher := newUnstartedMesherForBackpressureTest(2)
	mesher.jobs <- mesherJob{}
	mirror := NewMirror()
	applyAirChunkForBackpressureTest(t, mirror, core.ChunkPos{X: -3})
	applyAirChunkForBackpressureTest(t, mirror, core.ChunkPos{X: 9})
	west := core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{X: -3}}
	east := core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{X: 9}}
	mesher.MarkDirty(west, east)

	mesher.Schedule(mirror, ViewCenter{
		Dimension: core.Overworld,
		Chunk:     core.ChunkPos{X: 9},
	}, 4096)
	<-mesher.jobs
	if job := <-mesher.jobs; job.key != east {
		t.Fatalf("近处优先的首个 job = %+v，想要 %+v", job.key, east)
	}

	// 模拟 worker 侧消费后重新入队（与背压基准同一手法），保留两段待烘焙。
	mesher.mu.Lock()
	delete(mesher.queued, east)
	mesher.enqueueReadyLocked(east)
	mesher.mu.Unlock()
	mesher.jobs <- mesherJob{}
	mesher.Schedule(mirror, ViewCenter{
		Dimension: core.Overworld,
		Chunk:     core.ChunkPos{X: -3},
	}, 4096)
	<-mesher.jobs
	if job := <-mesher.jobs; job.key != west {
		t.Fatalf("中心移动后的首个 job = %+v，想要 %+v", job.key, west)
	}
}
