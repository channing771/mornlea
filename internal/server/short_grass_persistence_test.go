package server

// 本文件是 natural-grass-generation 变更「已保存区块不回填自然短草」的
// 服务端黑盒证据:升级前已保存(不含 ShortGrassID)的区块被服务器获取时
// 完全不触碰生成器;加载进权威 realm 的方块逐格等于存档内容;再次落盘
// 后读回仍逐格不变且不含短草。存档层证据见 internal/storage/chunk 的
// 同主题测试。

import (
	"context"
	"testing"

	"github.com/channing771/mornlea/internal/sim/contract"
	"github.com/channing771/mornlea/internal/storage"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
	"github.com/channing771/mornlea/packages/shared/worldgen"
)

// preUpgradeSavedChunk 构造升级前已保存的区块:生产生成器生成同种子世界后
// 把自然短草归一为空气——这正是升级前程序写出的内容,作为已保存语料。
func preUpgradeSavedChunk(pos core.ChunkPos) *world.Chunk {
	chunk := worldgen.New(42, false).GenerateChunk(pos)
	for y := int32(core.MinY); y < core.MaxY; y++ {
		for z := 0; z < core.SectionSize; z++ {
			for x := 0; x < core.SectionSize; x++ {
				if chunk.BlockAt(x, y, z) == core.ShortGrassID {
					chunk.SetBlock(x, y, z, core.AirID)
				}
			}
		}
	}
	chunk.Compact()
	return chunk
}

// TestSavedChunkAcquiresWithoutGeneratorAndShortGrassBackfill 钉住加载路径:
// 存档命中时生成器零调用,权威区块逐格等于存档,重存后字节内容仍逐格不变。
// 区块取观察者中心(采集夹具 ViewRadius=0,只有中心区块会被请求)。
func TestSavedChunkAcquiresWithoutGeneratorAndShortGrassBackfill(t *testing.T) {
	key := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{}}
	preUpgrade := preUpgradeSavedChunk(key.Pos)

	store := storage.NewMemory(storage.Metadata{FormatVersion: 3, Seed: 42})
	if _, err := store.SaveBatch(context.Background(), []storage.ChunkSave{{
		Key: key, Revision: 7, Chunk: preUpgrade,
	}}); err != nil {
		t.Fatal(err)
	}
	generator := &countingGenerator{}
	running := newAcquireServer(t, store, generator)

	stepUntilServer(t, running, func(result contract.TickResult) bool {
		info, ok := running.ChunkInfo(key.Dimension, key.Pos)
		return ok && info.State == contract.ChunkReady
	})
	if calls := generator.Calls(); calls != 0 {
		t.Fatalf("存档命中仍调用了生成器 %d 次", calls)
	}

	loaded, _, ok := running.CloneReadyChunkForTest(key)
	if !ok {
		t.Fatal("权威区块未就绪")
	}
	for y := int32(core.MinY); y < core.MaxY; y++ {
		for z := 0; z < core.SectionSize; z++ {
			for x := 0; x < core.SectionSize; x++ {
				if got, want := loaded.BlockAt(x, y, z), preUpgrade.BlockAt(x, y, z); got != want {
					t.Fatalf("加载后 (%d,%d,%d) 方块 %d != 存档 %d", x, y, z, got, want)
				}
				if got := loaded.BlockAt(x, y, z); got == core.ShortGrassID {
					t.Fatalf("加载后 (%d,%d,%d) 被补种 ShortGrassID", x, y, z)
				}
			}
		}
	}

	// 显式标脏触发重存,然后读回:逐格不变,短草仍不出现。
	running.TouchChunkForTest(key)
	shutdownServerForTest(t, running)
	stored, err := store.LoadChunk(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	for y := int32(core.MinY); y < core.MaxY; y++ {
		for z := 0; z < core.SectionSize; z++ {
			for x := 0; x < core.SectionSize; x++ {
				if got, want := stored.Chunk.BlockAt(x, y, z), preUpgrade.BlockAt(x, y, z); got != want {
					t.Fatalf("重存后 (%d,%d,%d) 方块 %d != 存档 %d", x, y, z, got, want)
				}
				if got := stored.Chunk.BlockAt(x, y, z); got == core.ShortGrassID {
					t.Fatalf("重存后 (%d,%d,%d) 被补种 ShortGrassID", x, y, z)
				}
			}
		}
	}
}
