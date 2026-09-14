package server

import (
	"sync"
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/shared/core"
)

// TestStepObservesChunkStreamingAcquireAndReady 钉死 streaming 指标族的服务端
// 观测点：每个权威 tick 把本 tick 的订阅装载请求（BeginLoading 侧 `Acquire`）
// 与就绪区块（`Ready`，含存档装载与生成两条路径的终点）原样交给
// `StreamingObserver`，供 benchmark 探针在观测点之间换算加载时延与已加载
// 区块数。nil 观察者保持零开销缺省。
func TestStepObservesChunkStreamingAcquireAndReady(t *testing.T) {
	config := hostTestConfig()
	// 声明视距 32 被服务端上界（ViewRadius 3）钳制为生效半径 3：订阅并集
	// 收敛为出生中心 ±3 的 7×7 方形，恰好 49 个区块，断言有确定终值。
	config.ViewRadius = 3
	var mu sync.Mutex
	acquired := make(map[core.ChunkKey]struct{})
	ready := make(map[core.ChunkKey]struct{})
	config.StreamingObserver = func(acquireKeys, readyKeys []core.ChunkKey) {
		mu.Lock()
		defer mu.Unlock()
		for _, key := range acquireKeys {
			acquired[key] = struct{}{}
		}
		for _, key := range readyKeys {
			ready[key] = struct{}{}
		}
	}
	host, _ := startHostWithConfig(t, config, newHostTestStore())
	login := startMemoryLogin(t, host, playerIdentity(21))
	waitReady(t, host, login)

	want := make(map[core.ChunkKey]struct{}, 49)
	for dz := int32(-3); dz <= 3; dz++ {
		for dx := int32(-3); dx <= 3; dx++ {
			want[core.ChunkKey{
				Dimension: core.Overworld,
				Pos:       core.ChunkPos{X: dx, Z: dz},
			}] = struct{}{}
		}
	}
	deadline := time.Now().Add(waitDeadline)
	for {
		mu.Lock()
		readyCount := len(ready)
		mu.Unlock()
		if readyCount == len(want) {
			break
		}
		if time.Now().After(deadline) {
			mu.Lock()
			t.Fatalf("流式就绪区块未在期限内收敛: ready=%d want=%d", len(ready), len(want))
		}
		time.Sleep(time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	for key := range want {
		if _, ok := ready[key]; !ok {
			t.Fatalf("订阅方形内区块 %+v 未出现在 Ready 观测", key)
		}
		if _, ok := acquired[key]; !ok {
			t.Fatalf("就绪区块 %+v 缺少前置 Acquire 观测（时延无法配对）", key)
		}
	}
	if len(acquired) != len(want) {
		t.Fatalf("Acquire 观测=%d want=%d（不应有方形外的额外装载）", len(acquired), len(want))
	}
}
