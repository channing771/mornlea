package world_test

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand"
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

// chunkHashOracle 是缓冲编码落地前的旧逐体素实现，保留作对拍 oracle：
// 每体素一次 `Get` 与一次 2 字节小写入。其摘要必须与现行 `Chunk.Hash`
// 逐字节一致——这是哈希缓冲化的等价性契约。
func chunkHashOracle(c *world.Chunk) [sha256.Size]byte {
	hash := sha256.New()
	var encoded [2]byte
	for sectionIndex := 0; sectionIndex < core.SectionsPerChunk; sectionIndex++ {
		section := c.Section(sectionIndex)
		for y := 0; y < core.SectionSize; y++ {
			for z := 0; z < core.SectionSize; z++ {
				for x := 0; x < core.SectionSize; x++ {
					binary.LittleEndian.PutUint16(
						encoded[:],
						uint16(section.Blocks.Get(x, y, z)),
					)
					_, _ = hash.Write(encoded[:])
				}
			}
		}
	}
	var sum [sha256.Size]byte
	hash.Sum(sum[:0])
	return sum
}

// maxRepresentableBlockID 是直接态 15 位槽可存的全局 ID 上界（含）。
// 超出该域的 ID 不是容器可表达的合法内容，边界值取空气 0 与该上界。
const maxRepresentableBlockID = world.BlockID(1<<15 - 1)

// sectionVarieties 把区段分别逼进调色板容器的全部形态：
// 0 保持全空气单值态；1 整段同值后经 `Compact` 退回非空气单值态；
// 2..16 落在 4 位索引态；17..256 落在 8 位索引态；257 以上升级为直接态。
var sectionVarieties = []int{0, 1, 2, 16, 17, 200, 256, 257, 2000}

// sectionWorldY 把区段索引与区段内局部 Y 合成世界 Y。
func sectionWorldY(section, y int) int32 {
	return int32(core.MinY + section*core.SectionSize + y)
}

// distinctBlockIDs 生成 count 个互不相同且在直接态可表达范围内的 block ID。
// boundary 为真时把空气（0）与直接态上界（32767）掺入取值池，覆盖边界值。
func distinctBlockIDs(rng *rand.Rand, count int, boundary bool) []world.BlockID {
	seen := make(map[world.BlockID]struct{}, count)
	if boundary {
		seen[world.AirID] = struct{}{}
		seen[maxRepresentableBlockID] = struct{}{}
	}
	for len(seen) < count {
		seen[world.BlockID(1+rng.Intn(int(maxRepresentableBlockID)-1))] = struct{}{}
	}
	pool := make([]world.BlockID, 0, count)
	for id := range seen {
		pool = append(pool, id)
	}
	// 打乱取值池：palette 槽位按首次写入顺序分配，池序随机化即随机 palette。
	rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	return pool
}

// fillRandomSection 用随机写入把第 section 个区段填成目标形态的内容，
// variety 语义见 sectionVarieties。随机写入顺序保证 palette 槽位分配随机；
// 末段补写保证每个取值至少出现一次——只有实际用量到位，17/257 个不同值
// 才会真正触发位宽升级与直接态升级。
func fillRandomSection(rng *rand.Rand, c *world.Chunk, section int, variety int, boundary bool) {
	if variety == 0 {
		return
	}
	if variety == 1 {
		id := world.BlockID(1 + rng.Intn(int(maxRepresentableBlockID)-1))
		for y := 0; y < core.SectionSize; y++ {
			for z := 0; z < core.SectionSize; z++ {
				for x := 0; x < core.SectionSize; x++ {
					c.SetBlock(x, sectionWorldY(section, y), z, id)
				}
			}
		}
		c.Section(section).Blocks.Compact()
		return
	}
	pool := distinctBlockIDs(rng, variety, boundary)
	for i := 0; i < core.BlocksPerSection; i++ {
		id := pool[rng.Intn(len(pool))]
		c.SetBlock(i&core.SectionMask, sectionWorldY(section, i>>8), i>>4&core.SectionMask, id)
	}
	for i, id := range pool {
		c.SetBlock(i&core.SectionMask, sectionWorldY(section, i>>8), i>>4&core.SectionMask, id)
	}
}

// TestChunkHashMatchesVoxelwiseOracle 钉住缓冲编码与逐体素 oracle 的摘要
// 等价：随机化区块覆盖单值/索引/直接三态、随机 palette 顺序、空气与直接态
// 上界两个边界 block ID，以及 `Compact` 重打包前后的形态差异。
func TestChunkHashMatchesVoxelwiseOracle(t *testing.T) {
	for seed := int64(1); seed <= 8; seed++ {
		seed := seed
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			chunk := world.NewChunk(core.ChunkPos{X: int32(seed%3 - 1), Z: int32(seed%3 - 1)})
			for section := 0; section < core.SectionsPerChunk; section++ {
				variety := sectionVarieties[section%len(sectionVarieties)]
				fillRandomSection(rng, chunk, section, variety, section%2 == 0)
			}
			// 偶数 seed 追加整体 Compact：重打包会收窄位宽、重排 palette，
			// 摘要仍必须与 oracle 一致。
			if seed%2 == 0 {
				chunk.Compact()
			}
			if got, want := chunk.Hash(), chunkHashOracle(chunk); got != want {
				t.Fatalf("缓冲编码摘要与逐体素 oracle 不一致（seed=%d）", seed)
			}
		})
	}
}

// refillShuffled 读取 source 的全部方块内容，按打乱的体素顺序写入新区块：
// 内容一一相同，但 palette 槽位按重填序列中的首次出现重新分配，
// 位打包内容因此与 source 的排列互不相同。
func refillShuffled(rng *rand.Rand, pos core.ChunkPos, source *world.Chunk) *world.Chunk {
	rebuilt := world.NewChunk(pos)
	order := rng.Perm(core.SectionsPerChunk * core.BlocksPerSection)
	for _, flat := range order {
		section := flat / core.BlocksPerSection
		local := flat % core.BlocksPerSection
		x, y, z := local&core.SectionMask, local>>8, local>>4&core.SectionMask
		rebuilt.SetBlock(x, sectionWorldY(section, y), z, source.BlockAt(x, sectionWorldY(section, y), z))
	}
	return rebuilt
}

// TestChunkHashIgnoresPaletteArrangementAndBitPacking 钉住摘要只由逻辑方块值
// 决定：内容一一相同的两个区块，在 palette 槽位分配、位宽与存储形态互不相同
// （经 `Compact` 与乱序重填构造）时摘要必须相同。
func TestChunkHashIgnoresPaletteArrangementAndBitPacking(t *testing.T) {
	t.Run("IndexedBitWidth", func(t *testing.T) {
		rng := rand.New(rand.NewSource(11))
		pos := core.ChunkPos{X: 1, Z: 2}
		source := world.NewChunk(pos)
		const section = 5
		values := distinctBlockIDs(rng, 17, false)

		// 确定性铺入 16 个值（palette 恰满 4 位索引态），再随机覆盖若干体素。
		for i := 0; i < 16; i++ {
			source.SetBlock(i&core.SectionMask, sectionWorldY(section, 0), i>>4, values[i])
		}
		for i := 0; i < 512; i++ {
			source.SetBlock(
				rng.Intn(core.SectionSize),
				sectionWorldY(section, rng.Intn(core.SectionSize)),
				rng.Intn(core.SectionSize),
				values[rng.Intn(16)],
			)
		}
		// 第 17 个值写入三个体素触发 4→8 位升级，随即全部改写回已有值：
		// 内容只余 16 种，容器停留在 8 位索引态且 palette 仍含 17 槽。
		for _, v := range [][3]int{{3, 3, 3}, {7, 7, 7}, {11, 11, 11}} {
			source.SetBlock(v[0], sectionWorldY(section, v[1]), v[2], values[16])
		}
		for _, v := range [][3]int{{3, 3, 3}, {7, 7, 7}, {11, 11, 11}} {
			source.SetBlock(v[0], sectionWorldY(section, v[1]), v[2], values[rng.Intn(16)])
		}

		// 乱序重填同内容：16 种值从头分配 palette，停留在 4 位索引态。
		rebuilt := refillShuffled(rng, pos, source)
		if source.Hash() != rebuilt.Hash() {
			t.Fatal("同内容、8 位与 4 位索引态排列互异的区块摘要不同")
		}
	})

	t.Run("DirectStorage", func(t *testing.T) {
		rng := rand.New(rand.NewSource(23))
		pos := core.ChunkPos{X: -4, Z: 9}
		source := world.NewChunk(pos)
		values := distinctBlockIDs(rng, 400, true)
		// 覆盖全部体素且实际用量超过 256，区段升级为直接态；
		// 取值池含空气与直接态上界两个边界 ID。
		for i := 0; i < core.BlocksPerSection; i++ {
			source.SetBlock(
				i&core.SectionMask,
				sectionWorldY(2, i>>8),
				i>>4&core.SectionMask,
				values[i%len(values)],
			)
		}
		rebuilt := refillShuffled(rng, pos, source)
		if source.Hash() != rebuilt.Hash() {
			t.Fatal("同内容、位打包排列互异的直接态区块摘要不同")
		}
	})

	t.Run("UniformContentShape", func(t *testing.T) {
		pos := core.ChunkPos{X: 6, Z: -8}
		indexed := world.NewChunk(pos)
		// 全段写成同一非空气值但不 Compact：内容统一，容器停留在索引态。
		for y := 0; y < core.SectionSize; y++ {
			for z := 0; z < core.SectionSize; z++ {
				for x := 0; x < core.SectionSize; x++ {
					indexed.SetBlock(x, sectionWorldY(7, y), z, core.StoneID)
				}
			}
		}
		compacted := indexed.Clone()
		compacted.Compact()

		if _, ok := indexed.Section(7).Blocks.IsUniform(); ok {
			t.Fatal("未 Compact 的统一内容区段不应处于单值态")
		}
		if id, ok := compacted.Section(7).Blocks.IsUniform(); !ok || id != core.StoneID {
			t.Fatal("Compact 后统一内容区段应退回单值态")
		}
		if indexed.Hash() != compacted.Hash() {
			t.Fatal("同内容、索引态与单值态的区块摘要不同")
		}
	})
}

// benchmarkChunkHashSink 防止编译器把哈希计算优化掉。
var benchmarkChunkHashSink [sha256.Size]byte

// newBenchmarkChunks 构造三种代表性区块：全空气（多数真实区段的单值形态）、
// 四种形态混合（单值/4 位/8 位/直接）与全直接态（最重的逐体素解包路径）。
// 固定种子保证两个基准函数对拍同一批区块。
func newBenchmarkChunks() (uniform, mixed, direct *world.Chunk) {
	rng := rand.New(rand.NewSource(7))
	uniform = world.NewChunk(core.ChunkPos{})
	mixed = world.NewChunk(core.ChunkPos{})
	varieties := []int{0, 8, 100, 600}
	for section := 0; section < core.SectionsPerChunk; section++ {
		fillRandomSection(rng, mixed, section, varieties[section%len(varieties)], false)
	}
	direct = world.NewChunk(core.ChunkPos{})
	for section := 0; section < core.SectionsPerChunk; section++ {
		fillRandomSection(rng, direct, section, 600, false)
	}
	return uniform, mixed, direct
}

func BenchmarkChunkHash(b *testing.B) {
	uniform, mixed, direct := newBenchmarkChunks()
	b.Run("UniformAir", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			benchmarkChunkHashSink = uniform.Hash()
		}
	})
	b.Run("MixedSections", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			benchmarkChunkHashSink = mixed.Hash()
		}
	})
	b.Run("DenseDirect", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			benchmarkChunkHashSink = direct.Hash()
		}
	})
}

// BenchmarkChunkHashVoxelwiseOracle 对旧逐体素实现取数，作缓冲编码前后的
// 对照参考；数值只记录，不构成门禁。
func BenchmarkChunkHashVoxelwiseOracle(b *testing.B) {
	uniform, mixed, direct := newBenchmarkChunks()
	b.Run("UniformAir", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			benchmarkChunkHashSink = chunkHashOracle(uniform)
		}
	})
	b.Run("MixedSections", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			benchmarkChunkHashSink = chunkHashOracle(mixed)
		}
	})
	b.Run("DenseDirect", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			benchmarkChunkHashSink = chunkHashOracle(direct)
		}
	})
}
