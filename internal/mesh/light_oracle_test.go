package mesh_test

import (
	"sort"
	"testing"

	"github.com/channing771/mornlea/internal/assets"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/mesh"
	"github.com/channing771/mornlea/internal/world"
)

const (
	lightMin    = -core.SectionSize
	lightSide   = 3 * core.SectionSize
	lightVolume = lightSide * lightSide * lightSide
	skyMask     = uint8(0xf0)
	blockMask   = uint8(0x0f)
)

var lightDirections = [...]struct{ x, y, z int }{
	{-1, 0, 0}, {1, 0, 0},
	{0, -1, 0}, {0, 1, 0},
	{0, 0, -1}, {0, 0, 1},
}

// goLightScratch 保存一次区段网格化复用的有界光照传播 oracle 状态。
type goLightScratch struct {
	levels [lightVolume]uint8
	queue  [lightVolume]uint32
	tail   int
}

// newGoLightScratch 创建固定容量的光照传播 oracle scratch。
func newGoLightScratch() *goLightScratch { return new(goLightScratch) }

func lightIndex(x, y, z int) int {
	return ((x-lightMin)*lightSide+(y-lightMin))*lightSide + z - lightMin
}

func (s *goLightScratch) at(x, y, z int) uint8 {
	if x < lightMin || x >= lightMin+lightSide ||
		y < lightMin || y >= lightMin+lightSide ||
		z < lightMin || z >= lightMin+lightSide {
		return 0
	}
	return s.levels[lightIndex(x, y, z)]
}

func (s *goLightScratch) enqueue(index int) {
	if s.tail == len(s.queue) {
		panic("mesh: 光照内部队列溢出")
	}
	s.queue[s.tail] = uint32(index)
	s.tail++
}

func (s *goLightScratch) build(n *world.Neighborhood, reg mesh.Registry) {
	clear(s.levels[:])
	s.tail = 0
	s.buildSky(n, reg, reg.MeshSnapshot())
	s.tail = 0
	s.buildBlock(n, reg)
}

func (s *goLightScratch) buildSky(
	n *world.Neighborhood,
	reg mesh.Registry,
	snapshot mesh.RegistrySnapshot,
) {
	for x := lightMin; x < lightMin+lightSide; x++ {
		for z := lightMin; z < lightMin+lightSide; z++ {
			direct := false
			for y := lightMin + lightSide - 1; y >= lightMin; y-- {
				if n.SkyLight(x, y, z) == 15 {
					direct = true
				}
				if !direct {
					continue
				}
				id := n.At(x, y, z)
				if !oracleRegistryContains(snapshot, id) || reg.Opaque(id) ||
					id != world.AirID && !core.IsPlant(id) {
					direct = false
					continue
				}
				index := lightIndex(x, y, z)
				s.levels[index] = skyMask
				s.enqueue(index)
			}
		}
	}

	// 按亮度降序分桶、每桶先零衰减后有衰减，与 Rust build_sky 同结构。
	// 这是「每格至多入队一次」的来源，容量恰好 lightVolume 才够用；推导见 light.rs。
	start, end := 0, s.tail
	for start < s.tail {
		deferred := s.spreadSky(n, reg, snapshot, start, end, false)
		nextEnd := s.tail
		if deferred {
			s.spreadSky(n, reg, snapshot, start, end, true)
		}
		start, end = end, nextEnd
	}
}

// spreadSky 放松 queue[start:end) 这一个桶，返回是否推迟过有衰减的邻居。
// attenuating 为 false 时只处理零衰减邻居（扣 1），为 true 时只处理有衰减邻居（扣 >= 2）。
func (s *goLightScratch) spreadSky(
	n *world.Neighborhood,
	reg mesh.Registry,
	snapshot mesh.RegistrySnapshot,
	start, end int,
	attenuating bool,
) bool {
	deferred := false
	for slot := start; slot < end; slot++ {
		index := int(s.queue[slot])
		z := index%lightSide + lightMin
		index /= lightSide
		y := index%lightSide + lightMin
		x := index/lightSide + lightMin
		current := s.at(x, y, z) >> 4
		if current <= 1 {
			continue
		}
		// best 是本格能给出的最好结果（扣减恰好为 1），先拿它剪枝再查表。
		best := current - 1
		for _, direction := range lightDirections {
			nx, ny, nz := x+direction.x, y+direction.y, z+direction.z
			if nx < lightMin || nx >= lightMin+lightSide ||
				ny < lightMin || ny >= lightMin+lightSide ||
				nz < lightMin || nz >= lightMin+lightSide {
				continue
			}
			next := lightIndex(nx, ny, nz)
			if s.levels[next]>>4 >= best {
				continue
			}
			id := n.At(nx, ny, nz)
			if !oracleRegistryContains(snapshot, id) || reg.Opaque(id) {
				continue
			}
			attenuation := reg.LightAttenuation(id)
			if (attenuation != 0) != attenuating {
				deferred = deferred || !attenuating
				continue
			}
			// 每格扣减 = 固定的 1 + 目标方块的额外衰减，六个方向同一公式。
			step := 1 + attenuation
			if current <= step {
				continue
			}
			candidate := current - step
			if s.levels[next]>>4 >= candidate {
				continue
			}
			s.levels[next] = s.levels[next]&blockMask | candidate<<4
			s.enqueue(next)
		}
	}
	return deferred
}

func oracleRegistryContains(snapshot mesh.RegistrySnapshot, id world.BlockID) bool {
	index := sort.Search(len(snapshot.Blocks), func(index int) bool {
		return snapshot.Blocks[index].ID >= id
	})
	return index < len(snapshot.Blocks) && snapshot.Blocks[index].ID == id
}

func (s *goLightScratch) buildBlock(n *world.Neighborhood, reg mesh.Registry) {
	var sourceCounts [16]int
	for x := lightMin; x < lightMin+lightSide; x++ {
		for y := lightMin; y < lightMin+lightSide; y++ {
			for z := lightMin; z < lightMin+lightSide; z++ {
				level := reg.Emission(n.At(x, y, z))
				if level == 0 {
					continue
				}
				if level > 15 {
					panic("mesh: 方块发光等级超过 15")
				}
				sourceCounts[level]++
			}
		}
	}

	// 与 Rust 一样按最终亮度降序推进：先前桶已经穷尽所有更高候选，同一格第一次
	// 入队就是最终值。sourceCounts 是固定 16 档，queue 仍只有精确的 lightVolume。
	start := 0
	for level := uint8(15); level > 0; level-- {
		if sourceCounts[level] != 0 {
			for x := lightMin; x < lightMin+lightSide; x++ {
				for y := lightMin; y < lightMin+lightSide; y++ {
					for z := lightMin; z < lightMin+lightSide; z++ {
						if reg.Emission(n.At(x, y, z)) != level {
							continue
						}
						index := lightIndex(x, y, z)
						if s.levels[index]&blockMask >= level {
							continue
						}
						s.levels[index] = s.levels[index]&skyMask | level
						s.enqueue(index)
					}
				}
			}
		}

		end := s.tail
		for slot := start; slot < end; slot++ {
			index := int(s.queue[slot])
			z := index%lightSide + lightMin
			index /= lightSide
			y := index%lightSide + lightMin
			x := index/lightSide + lightMin
			if got := s.at(x, y, z) & blockMask; got != level {
				panic("mesh: 方块光分桶亮度不一致")
			}
			if level <= 1 {
				continue
			}
			candidate := level - 1
			for _, direction := range lightDirections {
				nx, ny, nz := x+direction.x, y+direction.y, z+direction.z
				if nx < lightMin || nx >= lightMin+lightSide ||
					ny < lightMin || ny >= lightMin+lightSide ||
					nz < lightMin || nz >= lightMin+lightSide {
					continue
				}
				next := lightIndex(nx, ny, nz)
				id := n.At(nx, ny, nz)
				if s.levels[next]&blockMask >= candidate || id != world.AirID && !core.IsPlant(id) {
					continue
				}
				s.levels[next] = s.levels[next]&skyMask | candidate
				s.enqueue(next)
			}
		}
		start = end
	}
}

type oracleCountingRegistry struct {
	opaqueQueries   int
	emissionQueries int
}

func (r *oracleCountingRegistry) Opaque(world.BlockID) bool {
	r.opaqueQueries++
	return false
}

func (*oracleCountingRegistry) FaceVisible(world.BlockID, world.BlockID) bool { return false }
func (*oracleCountingRegistry) Material(world.BlockID, mesh.Face) uint16      { return 0 }
func (r *oracleCountingRegistry) Emission(world.BlockID) uint8 {
	r.emissionQueries++
	return 0
}
func (*oracleCountingRegistry) FluidHeight(world.BlockID) uint8      { return 0 }
func (*oracleCountingRegistry) LightAttenuation(world.BlockID) uint8 { return 0 }
func (*oracleCountingRegistry) BlockTopRaw(world.BlockID) uint8      { return 0 }

func (*oracleCountingRegistry) MeshSnapshot() mesh.RegistrySnapshot {
	return oracleCountingMeshSnapshot
}

var oracleCountingMeshSnapshot = mesh.RegistrySnapshot{
	Blocks: []mesh.BlockProperties{{ID: core.AirID}, {ID: core.BarrierID, Opaque: true}},
}

type oracleOverbrightRegistry struct{ testRegistry }

func (oracleOverbrightRegistry) Emission(id world.BlockID) uint8 {
	if id == core.LightBlockID {
		return 16
	}
	return 0
}

func fullyLoadedAirNeighborhoodOracle() *world.Neighborhood {
	n := &world.Neighborhood{Center: world.NewSection(), SectionY: 8}
	for dx := range n.Around {
		for dy := range n.Around[dx] {
			for dz := range n.Around[dx][dy] {
				n.Around[dx][dy][dz] = world.NewSection()
			}
		}
	}
	for dx := range n.HeightsPresent {
		for dz := range n.HeightsPresent[dx] {
			n.HeightsPresent[dx][dz] = true
		}
	}
	return n
}

func fillOracleSection(section *world.Section, id world.BlockID) {
	for y := 0; y < core.SectionSize; y++ {
		for z := 0; z < core.SectionSize; z++ {
			for x := 0; x < core.SectionSize; x++ {
				section.Blocks.Set(x, y, z, id)
			}
		}
	}
}

func TestGoLightOracleExactCapacityAndStableBuildDoesNotAllocate(t *testing.T) {
	if got, want := len(new(goLightScratch).levels), 48*48*48; got != want {
		t.Fatalf("levels=%d，想要 %d", got, want)
	}
	if got, want := len(new(goLightScratch).queue), 48*48*48; got != want {
		t.Fatalf("queue=%d，想要 %d", got, want)
	}
	n := fullyLoadedAirNeighborhoodOracle()
	registry := assets.NewRegistry()
	scratch := newGoLightScratch()
	scratch.build(n, registry)
	if got := testing.AllocsPerRun(100, func() { scratch.build(n, registry) }); got != 0 {
		t.Fatalf("稳定传播分配=%v，想要 0", got)
	}
}

func TestGoLightOracleDoesNotSampleSettledNeighbors(t *testing.T) {
	n := fullyLoadedAirNeighborhoodOracle()
	reg := new(oracleCountingRegistry)

	newGoLightScratch().build(n, reg)

	if got, want := reg.opaqueQueries, lightVolume; got != want {
		t.Fatalf("稳定全直射输入的不透明查询=%d，想要仅种子扫描的 %d", got, want)
	}
}

func TestGoLightOracleWorstCaseMultipleSourcesFitsExactQueue(t *testing.T) {
	n := fullyLoadedAirNeighborhoodOracle()
	fillOracleSection(n.Center, core.LightBlockID)
	for dx := range n.Around {
		for dy := range n.Around[dx] {
			for dz, section := range n.Around[dx][dy] {
				if dx == 1 && dy == 1 && dz == 1 {
					continue
				}
				fillOracleSection(section, core.LightBlockID)
			}
		}
	}

	scratch := newGoLightScratch()
	scratch.build(n, testRegistry{})
	if got, want := scratch.tail, 48*48*48; got != want {
		t.Fatalf("全邻域多光源入队=%d，想要精确容量 %d", got, want)
	}
}

func TestGoLightOracleBuildScansEachCellOnceForEmission(t *testing.T) {
	n := fullyLoadedAirNeighborhoodOracle()
	reg := new(oracleCountingRegistry)

	newGoLightScratch().build(n, reg)

	if got, want := reg.emissionQueries, 48*48*48; got != want {
		t.Fatalf("Emission 扫描=%d，想要精确 %d", got, want)
	}
}

func TestGoLightOracleReusesQueueBetweenSkyAndBlockPasses(t *testing.T) {
	n := fullyLoadedAirNeighborhoodOracle()
	n.Center.Blocks.Set(8, 8, 8, core.LightBlockID)
	scratch := newGoLightScratch()

	scratch.build(n, testRegistry{})

	if got, want := scratch.tail, 4089; got != want {
		t.Fatalf("方块光 pass 入队=%d，想要复用清空后的队列得到 %d", got, want)
	}
	if got := scratch.at(9, 8, 8) & 0x0f; got != 14 {
		t.Fatalf("光源相邻方块光=%d，想要 14", got)
	}
}

func TestGoLightOracleRejectsEmissionAboveFifteen(t *testing.T) {
	n := fullyLoadedAirNeighborhoodOracle()
	n.Center.Blocks.Set(8, 8, 8, core.LightBlockID)
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("Emission=16 未触发 panic")
		}
	}()
	newGoLightScratch().build(n, oracleOverbrightRegistry{})
}

func TestMeshSectionPlantBlockLightMatchesGoOracle(t *testing.T) {
	const floorY int32 = 64
	registry := assets.NewRegistry()
	for _, tt := range []struct {
		name  string
		plant world.BlockID
	}{
		{"既有作物", core.WheatStage0ID},
		{"短草", core.ShortGrassID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			n, localY := blockLightCorridor(t, floorY, map[core.BlockPos]world.BlockID{
				{X: 0, Y: floorY + 1, Z: 8}: core.LightBlockID,
				{X: 1, Y: floorY + 1, Z: 8}: tt.plant,
			})
			quads, oracle := meshSectionLightMatchesGoOracle(t, n, registry)
			if got := oracle.at(1, localY+1, 8) & blockMask; got != 14 {
				t.Fatalf("Go oracle 植物格方块光=%d，想要 14", got)
			}
			if got := oracle.at(2, localY+1, 8) & blockMask; got != 13 {
				t.Fatalf("Go oracle 植物后空气方块光=%d，想要 13", got)
			}

			if got := blockLight(topFaceLightAt(t, quads, 1, localY, 8)); got != 14 {
				t.Fatalf("Rust packed 植物格方块光=%d，Go oracle=14", got)
			}
			if got := blockLight(topFaceLightAt(t, quads, 2, localY, 8)); got != 13 {
				t.Fatalf("Rust packed 植物后空气方块光=%d，Go oracle=13", got)
			}
		})
	}
}

func TestMeshSectionBlockLightBlockersMatchGoOracle(t *testing.T) {
	const floorY int32 = 64
	registry := assets.NewRegistry()
	for _, tt := range []struct {
		name    string
		blocker world.BlockID
	}{
		{"玻璃", core.GlassID},
		{"水", core.WaterSourceID},
		{"石头", core.StoneID},
		{"未知方块", world.BlockID(60000)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			n, localY := blockLightCorridor(t, floorY, map[core.BlockPos]world.BlockID{
				{X: 0, Y: floorY + 1, Z: 8}: core.LightBlockID,
				{X: 1, Y: floorY + 1, Z: 8}: tt.blocker,
			})
			quads, oracle := meshSectionLightMatchesGoOracle(t, n, registry)
			if got := oracle.at(1, localY+1, 8) & blockMask; got != 0 {
				t.Fatalf("Go oracle 让方块光进入%s格：%d", tt.name, got)
			}
			if got := oracle.at(2, localY+1, 8) & blockMask; got != 0 {
				t.Fatalf("Go oracle 让方块光穿过%s：%d", tt.name, got)
			}
			if got := blockLight(topFaceLightAt(t, quads, 2, localY, 8)); got != 0 {
				t.Fatalf("Rust packed 让方块光穿过%s：%d", tt.name, got)
			}
		})
	}
}

func TestMeshSectionMissingNeighborBlockLightMatchesGoOracle(t *testing.T) {
	const floorY int32 = 64
	registry := assets.NewRegistry()
	n, localY := blockLightCorridor(t, floorY, map[core.BlockPos]world.BlockID{
		{X: -1, Y: floorY + 1, Z: 8}: core.LightBlockID,
	})
	loaded, loadedOracle := meshSectionLightMatchesGoOracle(t, n, registry)
	if got := loadedOracle.at(0, localY+1, 8) & blockMask; got != 14 {
		t.Fatalf("已加载邻区的 Go oracle 边界方块光=%d，想要 14", got)
	}
	if got := blockLight(topFaceLightAt(t, loaded, 0, localY, 8)); got != 14 {
		t.Fatalf("已加载邻区的 Rust packed 边界方块光=%d，想要 14", got)
	}

	n.Around[0][1][1] = nil
	missing, missingOracle := meshSectionLightMatchesGoOracle(t, n, registry)
	if got := missingOracle.at(0, localY+1, 8) & blockMask; got != 0 {
		t.Fatalf("缺失邻区后的 Go oracle 边界方块光=%d，想要 0", got)
	}
	if got := blockLight(topFaceLightAt(t, missing, 0, localY, 8)); got != 0 {
		t.Fatalf("缺失邻区后的 Rust packed 边界方块光=%d，想要 0", got)
	}
}

func TestMeshSectionPlantDirectSkyLightMatchesGoOracle(t *testing.T) {
	registry := assets.NewRegistry()
	for _, tt := range []struct {
		name  string
		plant world.BlockID
	}{
		{"既有作物", core.WheatStage0ID},
		{"短草", core.ShortGrassID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			n, localY := realHeightPlantSkyNeighborhood(t, tt.plant)

			quads, oracle := meshSectionLightMatchesGoOracle(t, n, registry)
			if got := oracle.at(8, localY+1, 8) >> 4; got != 15 {
				t.Fatalf("Go oracle 植物格直射天空光=%d，想要 15", got)
			}
			if got := oracle.at(10, localY+1, 8) >> 4; got != 15 {
				t.Fatalf("Go oracle 植物正下方天空光=%d，想要 15", got)
			}
			if got := skyLight(topFaceLightAt(t, quads, 8, localY, 8)); got != 15 {
				t.Fatalf("Rust packed 植物格直射天空光=%d，想要 15", got)
			}
			if got := skyLight(topFaceLightAt(t, quads, 10, localY, 8)); got != 15 {
				t.Fatalf("Rust packed 植物正下方天空光=%d，想要 15", got)
			}
		})
	}
}

func realHeightPlantSkyNeighborhood(t *testing.T, plant world.BlockID) (*world.Neighborhood, int) {
	t.Helper()
	chunks := make(map[core.ChunkPos]*world.Chunk, 9)
	for cx := int32(-1); cx <= 1; cx++ {
		for cz := int32(-1); cz <= 1; cz++ {
			pos := core.ChunkPos{X: cx, Z: cz}
			chunks[pos] = world.NewChunk(pos)
		}
	}
	const floorY int32 = 64
	center := chunks[core.ChunkPos{}]
	center.SetBlock(8, floorY, 8, core.StoneID)
	center.SetBlock(8, floorY+1, 8, plant)
	center.SetBlock(10, floorY, 8, core.StoneID)
	center.SetBlock(10, floorY+2, 8, plant)
	n := world.NeighborhoodAt(
		func(pos core.ChunkPos) *world.Chunk { return chunks[pos] },
		core.ChunkPos{},
		core.BlockPos{Y: floorY}.SectionIndex(),
	)
	if n == nil {
		t.Fatal("NeighborhoodAt 返回 nil")
	}
	return n, int(floorY-core.MinY) & core.SectionMask
}

func TestMeshSectionUnknownBlockStopsSkyLightMatchesGoOracle(t *testing.T) {
	n, localY := realHeightUnknownSkyCorridor(t)
	quads, oracle := meshSectionLightMatchesGoOracle(t, n, assets.NewRegistry())
	if got := oracle.at(0, localY+1, 8) >> 4; got != 0 {
		t.Fatalf("Go oracle 让天空光穿过未知方块：%d", got)
	}
	if got := skyLight(topFaceLightAt(t, quads, 0, localY, 8)); got != 0 {
		t.Fatalf("Rust packed 让天空光穿过未知方块：%d", got)
	}
}

func realHeightUnknownSkyCorridor(t *testing.T) (*world.Neighborhood, int) {
	t.Helper()
	chunks := make(map[core.ChunkPos]*world.Chunk, 9)
	for cx := int32(-1); cx <= 1; cx++ {
		for cz := int32(-1); cz <= 1; cz++ {
			pos := core.ChunkPos{X: cx, Z: cz}
			chunk := world.NewChunk(pos)
			for lz := 0; lz < core.SectionSize; lz++ {
				worldZ := int(cz)*core.SectionSize + lz
				for lx := 0; lx < core.SectionSize; lx++ {
					worldX := int(cx)*core.SectionSize + lx
					chunk.SetBlock(lx, 64, lz, core.StoneID)
					if worldZ != 8 {
						chunk.SetBlock(lx, 65, lz, core.StoneID)
					}
					if worldX != 0 || worldZ != 8 {
						chunk.SetBlock(lx, 66, lz, core.StoneID)
					}
				}
			}
			chunks[pos] = chunk
		}
	}
	chunks[core.ChunkPos{}].SetBlock(0, 66, 8, core.BlockIDMax)
	n := world.NeighborhoodAt(
		func(pos core.ChunkPos) *world.Chunk { return chunks[pos] },
		core.ChunkPos{},
		core.BlockPos{Y: 64}.SectionIndex(),
	)
	if n == nil {
		t.Fatal("NeighborhoodAt 返回 nil")
	}
	return n, int(64-core.MinY) & core.SectionMask
}

func TestMeshSectionPlantBlockLightMixedSourcesFitFixedQueue(t *testing.T) {
	registry := assets.NewRegistry()
	for _, tt := range []struct {
		name  string
		plant world.BlockID
	}{
		{"既有作物", core.WheatStage0ID},
		{"短草", core.ShortGrassID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			n := fullMixedSourceNeighborhood(tt.plant)
			quads, oracle := meshSectionLightMatchesGoOracle(t, n, registry)
			if got := oracle.tail; got != lightVolume {
				t.Fatalf("混合光源入队=%d，想要固定容量 %d", got, lightVolume)
			}
			if got := oracle.at(lightMin, lightMin, lightMin) & blockMask; got != 14 {
				t.Fatalf("植物格方块光=%d，想要 14", got)
			}
			if len(quads) != 0 {
				t.Fatalf("全包围中心区段生成 %d 条 quad，想要 0", len(quads))
			}
		})
	}
}

func fullMixedSourceNeighborhood(plant world.BlockID) *world.Neighborhood {
	n := &world.Neighborhood{Center: world.NewSection(), SectionY: 8}
	fillOracleSection(n.Center, core.LightBlockID)
	for dx := range n.Around {
		for dy := range n.Around[dx] {
			for dz := range n.Around[dx][dy] {
				section := world.NewSection()
				fillOracleSection(section, core.LightBlockID)
				n.Around[dx][dy][dz] = section
			}
		}
	}
	corner := n.Around[0][0][0]
	corner.Blocks.Set(0, 0, 0, plant)
	corner.Blocks.Set(0, 0, 1, core.TorchStandingID)
	return n
}

func TestMeshSectionPlantPropagatedSkyLightMatchesGoOracle(t *testing.T) {
	registry := assets.NewRegistry()
	for _, tt := range []struct {
		name  string
		plant world.BlockID
	}{
		{"既有作物", core.WheatStage0ID},
		{"短草", core.ShortGrassID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			n, localY := propagatedSkyWorld(t, 0, nil)
			n.Center.Blocks.Set(1, localY+1, 8, tt.plant)
			quads, oracle := meshSectionLightMatchesGoOracle(t, n, registry)
			if got := oracle.at(1, localY+1, 8) >> 4; got != 14 {
				t.Fatalf("Go oracle 植物格派生天空光=%d，想要 14", got)
			}
			if got := oracle.at(2, localY+1, 8) >> 4; got != 13 {
				t.Fatalf("Go oracle 植物后空气天空光=%d，想要 13", got)
			}
			if got := skyLight(topFaceLightAt(t, quads, 1, localY, 8)); got != 14 {
				t.Fatalf("Rust packed 植物格派生天空光=%d，想要 14", got)
			}
			if got := skyLight(topFaceLightAt(t, quads, 2, localY, 8)); got != 13 {
				t.Fatalf("Rust packed 植物后空气天空光=%d，想要 13", got)
			}
		})
	}
}

// meshSectionLightMatchesGoOracle 逐条 quad、逐个被覆盖单元对照 native packed
// 光照与 Go oracle。轴向面采样相邻格，植物交叉斜面采样正上方格。
func meshSectionLightMatchesGoOracle(
	t *testing.T,
	n *world.Neighborhood,
	registry mesh.Registry,
) ([]mesh.Quad, *goLightScratch) {
	t.Helper()
	oracle := newGoLightScratch()
	oracle.build(n, registry)
	quads := mesh.MeshSection(n, registry, mesh.NewLightScratch())
	for index, quad := range quads {
		if quad.Face.Plant() {
			want := oracle.at(int(quad.X), int(quad.Y)+1, int(quad.Z))
			if quad.Light != want {
				t.Fatalf("quad[%d] 植物面 packed light=%#x，Go oracle=%#x", index, quad.Light, want)
			}
			continue
		}
		axis := quad.Face.Axis()
		u, v := (axis+1)%3, (axis+2)%3
		step := -1
		if quad.Face.Positive() {
			step = 1
		}
		for dv := 0; dv < int(quad.H); dv++ {
			for du := 0; du < int(quad.W); du++ {
				cell := [3]int{int(quad.X), int(quad.Y), int(quad.Z)}
				cell[u] += du
				cell[v] += dv
				cell[axis] += step
				want := oracle.at(cell[0], cell[1], cell[2])
				if quad.Light != want {
					t.Fatalf("quad[%d] face=%d cell=(%d,%d,%d) packed light=%#x，Go oracle=%#x", index, quad.Face, cell[0], cell[1], cell[2], quad.Light, want)
				}
			}
		}
	}
	return quads, oracle
}
