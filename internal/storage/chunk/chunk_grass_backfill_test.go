package chunk

// 本文件是 natural-grass-generation 变更「已保存区块不回填自然短草」的
// 存档层黑盒证据:升级前已保存(不含 ShortGrassID)的 chunk schema v9
// 区块,经 Decode/Encode 反复往返后全部方块逐格不变,任何环节都不会凭空
// 写入 ShortGrassID。存档层不依赖 worldgen(依赖方向由 archcheck 强制),
// "加载不触发生成"的服务端证据见 internal/server 的同主题测试。

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/channing771/mornlea/internal/storage/region"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
	"github.com/channing771/mornlea/packages/shared/worldgen"
)

// preUpgradeSavedChunk 构造一份升级前已保存的区块:用生产生成器生成同种子
// 世界,再把自然短草归一为空气——这正是升级前程序会写出的字节内容
// (既有地形、矿石、橡树与门控关闭的空气层全部保留)。
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

// assertNoShortGrass 逐格确认区块不含短草,防止归一化夹具本身失效。
func assertNoShortGrass(t *testing.T, chunk *world.Chunk, context string) {
	t.Helper()
	for y := int32(core.MinY); y < core.MaxY; y++ {
		for z := 0; z < core.SectionSize; z++ {
			for x := 0; x < core.SectionSize; x++ {
				if got := chunk.BlockAt(x, y, z); got == core.ShortGrassID {
					t.Fatalf("%s (%d,%d,%d) 出现 ShortGrassID", context, x, y, z)
				}
			}
		}
	}
}

// assertBlocksEqual 逐格比较两个区块的全部方块。
func assertBlocksEqual(t *testing.T, want, got *world.Chunk, context string) {
	t.Helper()
	for y := int32(core.MinY); y < core.MaxY; y++ {
		for z := 0; z < core.SectionSize; z++ {
			for x := 0; x < core.SectionSize; x++ {
				if want.BlockAt(x, y, z) != got.BlockAt(x, y, z) {
					t.Fatalf("%s (%d,%d,%d) 方块 %d != %d", context, x, y, z,
						got.BlockAt(x, y, z), want.BlockAt(x, y, z))
				}
			}
		}
	}
}

// TestPreUpgradeSavedChunkRoundTripsWithoutShortGrassBackfill 钉住存档层
// 兼容证据:升级前保存的 v9 区块加载→保存→再加载,全部方块逐格不变,
// 任何往返都不会向其中补种 ShortGrassID,schema 保持 v9。
func TestPreUpgradeSavedChunkRoundTripsWithoutShortGrassBackfill(t *testing.T) {
	key := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: 0, Z: 0}}
	preUpgrade := preUpgradeSavedChunk(key.Pos)
	assertNoShortGrass(t, preUpgrade, "夹具本身")

	// 升级前的保存字节:模拟旧程序写出的 schema v9 记录。
	saved, err := Encode(ChunkSave{Key: key, Revision: 5, Chunk: preUpgrade})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := Decode(key, 5, saved)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Schema != 9 || loaded.Migrated {
		t.Fatalf("schema=%d migrated=%v，想要 9/false", loaded.Schema, loaded.Migrated)
	}
	assertBlocksEqual(t, preUpgrade, loaded.Chunk, "首次加载")
	assertNoShortGrass(t, loaded.Chunk, "首次加载")

	// 再次保存后再加载:方块逐格不变,短草仍不出现。
	resaved, err := Encode(ChunkSave{Key: key, Revision: 6, Chunk: loaded.Chunk})
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := Decode(key, 6, resaved)
	if err != nil {
		t.Fatal(err)
	}
	assertBlocksEqual(t, preUpgrade, reloaded.Chunk, "重存再加载")
	assertNoShortGrass(t, reloaded.Chunk, "重存再加载")
}

// TestPreUpgradeSavedChunkLoadsFromRegionWithoutBackfill 用 Region 容器
// (落盘记录层)重复同一证据:升级前区块经真实落盘与读回,方块逐格不变。
func TestPreUpgradeSavedChunkLoadsFromRegionWithoutBackfill(t *testing.T) {
	key := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: 1, Z: -2}}
	preUpgrade := preUpgradeSavedChunk(key.Pos)

	regionKey, _ := region.RegionFor(key)
	path := filepath.Join(
		t.TempDir(), "dimensions", "0", "regions",
		fmt.Sprintf("r.%d.%d.region", regionKey.X, regionKey.Z),
	)
	// `CreateRegion` 要在同目录落原子临时文件,先补齐父目录。
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	container, err := CreateRegion(context.Background(), path, regionKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = container.Close() })
	if _, err := container.Save(context.Background(), []ChunkSave{{
		Key: key, Revision: 3, Chunk: preUpgrade,
	}}); err != nil {
		t.Fatal(err)
	}
	stored, err := container.Load(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	assertBlocksEqual(t, preUpgrade, stored.Chunk, "region 读回")
	assertNoShortGrass(t, stored.Chunk, "region 读回")
}
