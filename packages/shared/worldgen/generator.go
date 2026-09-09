// Package worldgen 生成地形。
//
// 本包必须是确定性的:同种子 + 同区块坐标 = 完全相同的输出(spec §4.3)。
// 自 rust-engine-worldgen 起,噪声求值、地表分层、矿石与橡树的全部计算由
// Rust `mornlea_engine` 独占生产;本包只保留 seed→perm 表播种(Go
// `math/rand` 语义)、`MGW1` 请求编码、native 调用与结果解码,没有生产
// Go fallback。生成语义的确定性由生产黑盒测试兜底:`GenerateChunk` 与
// `BaseBlockAt` 两种出口的一致性断言、区块输出的 golden 字节锁以及服务端
// e2e parity 共同构成回归网。
package worldgen

import (
	"encoding/binary"
	"math/rand"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/nativeabi"
	"github.com/channing771/mornlea/packages/shared/world"
)

// `MGW1` ABI 编码常量,必须与 engine `worldgen.rs` 的布局逐字一致:
// header = magic(4) + layout version(4) + seed(8) + min_y(4) + max_y(4) +
// 材料表 15×u16(30) + perm 512×u8(512)。
//
// engine ABI v4 起材料表末项是 water,它正当占用 v3 预留的 reserved 槽
// (偏移 50),layout version 1 → 2。engine ABI v10 再在末尾追加第 15 项
// short_grass(偏移 52,perm 后移到偏移 54),header 564 → 566 字节,
// layout version 2 → 3——它是独立于 ABI 版本号的带内第二道混装防线。
//
// 不再保留空槽是刻意选择,不是漏了:新增一个 reserved 槽本身就要把 perm 往后
// 挪,而 reserved 的意义是推迟这个代价、不是提前支付;何况下一次扩字段必然
// 同样改动材料表布局、必然升 ABI 版本,混装在版本号校验那一步就被挡住。
const (
	worldgenMagic       = "MGW1"
	worldgenLayout      = 3
	worldgenHeaderBytes = 566
	// worldgenChunkOutputBytes 是 dense `[y−min_y][lz][lx]` 布局的
	// 16×16×(MaxY−MinY) 个 u16。
	worldgenChunkOutputBytes = core.SectionSize * core.SectionSize * (core.MaxY - core.MinY) * 2
	// probe 记录:mode(4) + wx/wy/wz(12) 输入,height(4)+block(2)+reserved(2) 输出。
	worldgenProbeRecordBytes       = 16
	worldgenProbeOutputRecordBytes = 8

	// probe 查询模式,与 engine 侧约定一致。
	probeModeHeight  = 0
	probeModeTerrain = 1
	probeModeBase    = 2

	// depthsSeedSalt 是 `Depths` 维度的种子盐:位模式即 `0x9E3779B97F4A7C15`
	// (经典 64 位黄金比例常数)。字面量 `0x9E3779B97F4A7C15` 超出 int64
	// 上界、不能直接写进 `^` 运算,故以其二进制补码相反数
	// `-0x61C8864680B583EB` 表达,两者位模式相同。
	depthsSeedSalt = int64(-0x61C8864680B583EB)
)

// Generator 按种子生成地形。
//
// New 之后 header 只读共享,每次调用使用独立缓冲,可并发调用。生成器常驻
// 双维 header(各 566 字节):主世界 header 与 `Depths` header 只在种子盐上
// 不同,engine 侧零改动,复用同一套 `nativeabi` 调用。
type Generator struct {
	// primary 是构造时声明的主维度,只决定 `Header` 返回哪一维的 header;
	// 按维查询(`GenerateChunk`/`HeightAt` 等)一律以传入的 `dim` 为准,
	// 不受 `primary` 影响,任何实例都能回答任意维度,不存在半初始化实例。
	primary core.DimensionID
	// overworld 是主世界种子的预编码 `MGW1` header,与历史版本逐字节一致。
	overworld []byte
	// depths 是盐化种子的预编码 `MGW1` header,只服务 `Depths` 查询。
	depths []byte
}

// `dimSeed` 按维度派生世界种子:主世界保持基础种子(既有世界逐字节不变),
// `Depths` 异或固定盐后独立演化;未知维度回落主世界(值域由协议层收紧,
// 此处保持全函数、不在热路径抛错)。
func dimSeed(base int64, dim core.DimensionID) int64 {
	if dim == core.Depths {
		return base ^ depthsSeedSalt
	}
	return base
}

// New 创建一个地形生成器。
//
// perm 表播种保持 Go `math/rand` 语义:这是既有世界的确定性来源,
// 不迁入 Rust,保证相同 seed 在迁移前后产生相同世界。
//
// 参数 fluidEnabled 是顶层配置同名键的取值,门控海平面注水。门控在 Go 侧
// 以"材料表 water 字段填什么编号"的形式实现(design D6):关闭时填 core.AirID,
// engine 的注水步就退化为把空气写回空气,生成结果与未引入流体的基线逐位一致;
// engine 侧因此没有任何开关分支。短草无门控:它只在注水结算后仍为空气的
// 命中草地列写入,编号恒取 core.ShortGrassID。
func New(seed int64, fluidEnabled bool) *Generator {
	return NewForDimension(seed, fluidEnabled, core.Overworld)
}

// NewForDimension 创建以 `dim` 为主维度的地形生成器。
//
// 双维 header 常驻(内存 1KB 量级):单维调用方(如某维出生扫描的探针)用它
// 自文档化意图,但实例仍能回答任意维度的查询。`dim` 只决定 `Header` 返回
// 哪一维,不限制查询维度。
func NewForDimension(seed int64, fluidEnabled bool, dim core.DimensionID) *Generator {
	if dim != core.Depths {
		dim = core.Overworld
	}
	return &Generator{
		primary:   dim,
		overworld: buildHeader(dimSeed(seed, core.Overworld), fluidEnabled),
		depths:    buildHeader(dimSeed(seed, core.Depths), fluidEnabled),
	}
}

// buildHeader 预编码单一种子的 566 字节 `MGW1` 公共 header(seed、材料表、
// perm),由 `New` 家族按维度种子各调一次。
func buildHeader(seed int64, fluidEnabled bool) []byte {
	header := make([]byte, worldgenHeaderBytes)
	copy(header[:4], worldgenMagic)
	binary.LittleEndian.PutUint32(header[4:8], worldgenLayout)
	binary.LittleEndian.PutUint64(header[8:16], uint64(seed))
	minY, maxY := int32(core.MinY), int32(core.MaxY)
	binary.LittleEndian.PutUint32(header[16:20], uint32(minY))
	binary.LittleEndian.PutUint32(header[20:24], uint32(maxY))
	// 门控编码:关闭时 water 取 air 编号,engine 侧注水随之成为空操作。
	water := core.AirID
	if fluidEnabled {
		water = core.WaterSourceID
	}
	// 材料表编码顺序与 engine `Materials` 字段顺序逐字对应;末项
	// short_grass 参与装饰写入,不享受 water == air 的门控豁免。
	for index, id := range [15]core.BlockID{
		core.AirID, core.StoneID, core.DirtID, core.GrassID, core.BedrockID,
		core.SnowBlockID, core.SandID, core.ClayID, core.GravelID,
		core.IronOreID, core.CoalOreID, core.OakLogID, core.LeavesID,
		water, core.ShortGrassID,
	} {
		binary.LittleEndian.PutUint16(header[24+index*2:26+index*2], uint16(id))
	}
	perm := permTable(seed)
	// perm 从偏移 54 开始:24..54 恰好是 15 项材料表,末项 short_grass
	// 占 52..54。
	copy(header[54:], perm[:])
	return header
}

// permTable 用给定种子构造 512 项 Perlin 置换表(0..255 重复两遍)。
//
// 必须保持 Go `math/rand` 的 NewSource+Shuffle 语义:任何改动都会
// 改变既有世界。
func permTable(seed int64) [512]byte {
	base := make([]int, 256)
	for i := range base {
		base[i] = i
	}
	rng := rand.New(rand.NewSource(seed))
	rng.Shuffle(256, func(i, j int) { base[i], base[j] = base[j], base[i] })
	var perm [512]byte
	for i := 0; i < 512; i++ {
		perm[i] = byte(base[i&255])
	}
	return perm
}

// Header 返回生成器主维度的预编码 566 字节 `MGW1` 公共 header(seed、
// 材料表、perm)。`New` 构造的主维度即主世界,历史调用方(远环壳生成、菜单
// 全景)语义不变;`NewForDimension` 构造的返回其声明维度。返回切片与生成器
// 内部共享，构造后只读——调用方不得修改，否则会同时污染近环 worldgen 与
// 所有共享方。远环壳生成(internal/lod)用同一种子构造的 header 与近环逐字
// 节一致，保证同一世界的近环与远环地形来自同一份确定性输入。
func (g *Generator) Header() []byte {
	return g.headerFor(g.primary)
}

// headerFor 取某维度的预编码 header;未知维度回落主世界(与 `dimSeed` 同策)。
func (g *Generator) headerFor(dim core.DimensionID) []byte {
	if dim == core.Depths {
		return g.depths
	}
	return g.overworld
}

// probe 执行一条单点查询,返回 8 字节结果记录。
func (g *Generator) probe(dim core.DimensionID, mode uint32, x, y, z int32) []byte {
	input := make([]byte, 0, worldgenHeaderBytes+4+worldgenProbeRecordBytes)
	input = append(input, g.headerFor(dim)...)
	input = binary.LittleEndian.AppendUint32(input, 1)
	input = binary.LittleEndian.AppendUint32(input, mode)
	input = binary.LittleEndian.AppendUint32(input, uint32(x))
	input = binary.LittleEndian.AppendUint32(input, uint32(y))
	input = binary.LittleEndian.AppendUint32(input, uint32(z))
	output := make([]byte, worldgenProbeOutputRecordBytes)
	nativeabi.WorldgenProbe(input, output)
	return output
}

// HeightAt 返回指定维度世界坐标 (wx,wz) 处最高实心方块的 Y。
func (g *Generator) HeightAt(dim core.DimensionID, wx, wz int32) int32 {
	output := g.probe(dim, probeModeHeight, wx, 0, wz)
	return int32(binary.LittleEndian.Uint32(output[0:4]))
}

// TerrainBlockAt 返回不叠加橡树结构时指定世界位置的确定性地形方块。
//
// 它保持纯地形语义：即使 fluidEnabled 开启，海平面以下的空气格在这里仍返回
// core.AirID，注水只作用于 BaseBlockAt 与 GenerateChunk。这个不对称是必需的
// ——BaseBlockAt 以"地形非空即早返回"的方式叠加橡树，若地形层就把空气改写成
// 水，早返回会吞掉橡树分支，海平面以下的树会整棵消失。
func (g *Generator) TerrainBlockAt(dim core.DimensionID, pos core.BlockPos) core.BlockID {
	output := g.probe(dim, probeModeTerrain, pos.X, pos.Y, pos.Z)
	return core.BlockID(binary.LittleEndian.Uint16(output[4:6]))
}

// BaseBlockAt 返回不应用会话修改时指定世界位置的确定性方块。
//
// 它是含注水与自然短草的完整生成语义，与 GenerateChunk 逐格一致：
// 地形 → 橡树 → 海水 → 短草 四层，fluidEnabled 开启时海平面及其以下最终
// 仍为空气的格返回 core.WaterSourceID；树与海水结算后仍为空气、且判定
// 命中的草地表面正上方格返回 core.ShortGrassID。需要纯地形结果的调用方
// 用 TerrainBlockAt(地形与高度语义忽略装饰短草)。
func (g *Generator) BaseBlockAt(dim core.DimensionID, pos core.BlockPos) core.BlockID {
	output := g.probe(dim, probeModeBase, pos.X, pos.Y, pos.Z)
	return core.BlockID(binary.LittleEndian.Uint16(output[4:6]))
}

// GenerateChunk 生成一个完整区块。
//
// 一次 native 调用产出 dense 数组;Go 侧只把非 air 方块写入 chunk,
// 与旧实现"地形只写到地表高度、树只写原木/树叶"的写入集合一致,
// palette 构建路径保持不变。
func (g *Generator) GenerateChunk(dim core.DimensionID, pos core.ChunkPos) *world.Chunk {
	input := make([]byte, 0, worldgenHeaderBytes+8)
	input = append(input, g.headerFor(dim)...)
	input = binary.LittleEndian.AppendUint32(input, uint32(pos.X))
	input = binary.LittleEndian.AppendUint32(input, uint32(pos.Z))
	dense := make([]byte, worldgenChunkOutputBytes)
	nativeabi.WorldgenChunk(input, dense)

	c := world.NewChunk(pos)
	offset := 0
	for y := int32(core.MinY); y < core.MaxY; y++ {
		for lz := 0; lz < core.SectionSize; lz++ {
			for lx := 0; lx < core.SectionSize; lx++ {
				id := core.BlockID(binary.LittleEndian.Uint16(dense[offset : offset+2]))
				offset += 2
				if id != core.AirID {
					c.SetBlock(lx, y, lz, id)
				}
			}
		}
	}
	c.Compact()
	return c
}
