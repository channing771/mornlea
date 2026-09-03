package chunk

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/channing771/mornlea/packages/server/storage/region"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/worldgen"
)

var (
	benchmarkEncodedPayload []byte
	benchmarkDecodedChunk   StoredChunk
)

func BenchmarkChunkEncode(b *testing.B) {
	key := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: -2, Z: 3}}
	save := ChunkSave{
		Key: key, Revision: 1,
		Chunk: worldgen.New(42, false).GenerateChunk(key.Pos),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		payload, err := Encode(save)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkEncodedPayload = payload
	}
}

func BenchmarkChunkDecode(b *testing.B) {
	key := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: -2, Z: 3}}
	save := ChunkSave{
		Key: key, Revision: 1,
		Chunk: worldgen.New(42, false).GenerateChunk(key.Pos),
	}
	payload, err := Encode(save)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		decoded, err := Decode(key, save.Revision, payload)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkDecodedChunk = StoredChunk{
			Key: key, Revision: decoded.Revision, Chunk: decoded.Chunk,
		}
	}
}

// 以下两个基准保持基线函数名不变，但夹具为 chunk 域内最小装配：直接组合
// region 记录层容器（CreateRegion/Save、OpenRegion/Load/Close）复现原
// DiskStore 基准的落盘与冷加载负载。域包测试禁止反向导入根包，无法再经
// DiskStore 编排；计时数值只记录不设门槛。

func BenchmarkDiskStoreSave32(b *testing.B) {
	ctx := context.Background()
	generator := worldgen.New(42, false)
	saves := make([]ChunkSave, 32)
	for index := range saves {
		key := core.ChunkKey{
			Dimension: core.Overworld,
			Pos:       core.ChunkPos{X: int32(index), Z: 0},
		}
		saves[index] = ChunkSave{Key: key, Chunk: generator.GenerateChunk(key.Pos)}
	}
	regionKey, _ := region.RegionFor(saves[0].Key)
	path := filepath.Join(
		b.TempDir(), "dimensions", "0", "regions",
		fmt.Sprintf("r.%d.%d.region", regionKey.X, regionKey.Z),
	)
	// 基准沿用的 dimensions/0/regions 层级在生产由 `DiskStore` 落盘前代建，
	// 域内直装配不含这一步；而 `CreateRegion` 要在同目录落原子临时文件，
	// 父目录缺失会直接 ENOENT，故先补齐目录再创建。
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		b.Fatal(err)
	}
	opened, err := CreateRegion(ctx, path, regionKey)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for iteration := range b.N {
		for index := range saves {
			saves[index].Revision = uint64(iteration + 1)
		}
		if _, err := opened.Save(ctx, saves); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if err := opened.Close(); err != nil {
		b.Fatal(err)
	}
}

func BenchmarkDiskStoreColdLoad(b *testing.B) {
	ctx := context.Background()
	key := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: -2, Z: 3}}
	regionKey, _ := region.RegionFor(key)
	path := filepath.Join(
		b.TempDir(), "dimensions", "0", "regions",
		fmt.Sprintf("r.%d.%d.region", regionKey.X, regionKey.Z),
	)
	// 与 Save32 同理：域内直装配不经 world_files 布局，region 父目录
	// 须由基准自行补齐，否则 `CreateRegion` 的原子临时文件无处落盘。
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		b.Fatal(err)
	}
	created, err := CreateRegion(ctx, path, regionKey)
	if err != nil {
		b.Fatal(err)
	}
	if _, err := created.Save(ctx, []ChunkSave{{
		Key: key, Revision: 1,
		Chunk: worldgen.New(42, false).GenerateChunk(key.Pos),
	}}); err != nil {
		b.Fatal(err)
	}
	if err := created.Close(); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		opened, err := OpenRegion(ctx, path, regionKey)
		if err != nil {
			b.Fatal(err)
		}
		loaded, loadErr := opened.Load(ctx, key)
		closeErr := opened.Close()
		if loadErr != nil {
			b.Fatal(loadErr)
		}
		if closeErr != nil {
			b.Fatal(closeErr)
		}
		benchmarkDecodedChunk = loaded
	}
}
