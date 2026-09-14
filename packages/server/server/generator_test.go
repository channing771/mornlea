package server_test

import (
	"sync"
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/server/server"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/world"
)

func TestGeneratorWorkerPanicIsolated(t *testing.T) {
	panicAt := core.ChunkPos{X: 1, Z: 1}
	generator := &panicGenerator{panicAt: panicAt}
	config := server.DefaultConfig(7)
	config.ViewRadius = 1
	config.Workers = 2
	_, endpoint := network.NewMemoryPair(64)
	running := newMemoryAttachedWorldForExternalTest(config, endpoint, generator)
	t.Cleanup(func() { shutdownExternalServerForTest(t, running) })

	ready := make(map[core.ChunkPos]struct{})
	deadline := time.Now().Add(waitDeadline)
	for time.Now().Before(deadline) {
		result := running.Step()
		for _, key := range result.Ready {
			ready[key.Pos] = struct{}{}
		}
		if generator.callsFor(panicAt) >= 2 &&
			generator.callsFor(core.ChunkPos{X: -1, Z: -1}) > 0 &&
			len(ready) >= 5 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf(
		"panic 未隔离: panicCalls=%d otherCalls=%d ready=%d",
		generator.callsFor(panicAt),
		generator.callsFor(core.ChunkPos{X: -1, Z: -1}),
		len(ready),
	)
}

type panicGenerator struct {
	mu      sync.Mutex
	panicAt core.ChunkPos
	calls   map[core.ChunkPos]int
}

func (generator *panicGenerator) GenerateChunk(_ core.DimensionID, pos core.ChunkPos) *world.Chunk {
	generator.mu.Lock()
	if generator.calls == nil {
		generator.calls = make(map[core.ChunkPos]int)
	}
	generator.calls[pos]++
	generator.mu.Unlock()
	if pos == generator.panicAt {
		panic("injected generator panic")
	}
	return world.NewChunk(pos)
}

func (generator *panicGenerator) BaseBlockAt(core.BlockPos) core.BlockID {
	return core.AirID
}

func (generator *panicGenerator) callsFor(pos core.ChunkPos) int {
	generator.mu.Lock()
	defer generator.mu.Unlock()
	return generator.calls[pos]
}

// TestTerrainProbeCarriesDimension 锁定高度探针的维度归属:默认探针读主世界
// 高度图,按维探针读本维高度图——传送落点的新维出生扫描依赖该语义,而不是
// 主世界高度。
func TestTerrainProbeCarriesDimension(t *testing.T) {
	const seed int64 = 42
	overworld := server.NewTerrainProbe(seed)
	explicit := server.NewTerrainProbeForDimension(seed, core.Overworld)
	depths := server.NewTerrainProbeForDimension(seed, core.Depths)
	diverged := false
	for x := int32(-48); x < 48; x += 7 {
		for z := int32(-48); z < 48; z += 11 {
			if overworld.HeightAt(x, z) != explicit.HeightAt(x, z) {
				t.Fatalf("默认探针与主世界探针在 (%d,%d) 高度不一致", x, z)
			}
			if overworld.HeightAt(x, z) != depths.HeightAt(x, z) {
				diverged = true
			}
		}
	}
	if !diverged {
		t.Fatal("采样区内双维探针高度全一致,探针未携带维度")
	}
}
