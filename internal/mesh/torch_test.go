package mesh

import (
	"encoding/binary"
	"slices"
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

// modelTagRegistry 返回一个除 Model 恒为 tag 外与 internalTestRegistry 完全
// 一致的 reader，专门喂 model tag 的域校验。
type modelTagRegistry uint8

func (modelTagRegistry) Opaque(id world.BlockID) bool { return internalTestRegistry{}.Opaque(id) }
func (modelTagRegistry) FaceVisible(id, adjacent world.BlockID) bool {
	return internalTestRegistry{}.FaceVisible(id, adjacent)
}
func (modelTagRegistry) Material(id world.BlockID, f Face) uint16 {
	return internalTestRegistry{}.Material(id, f)
}
func (modelTagRegistry) Emission(id world.BlockID) uint8 {
	return internalTestRegistry{}.Emission(id)
}
func (modelTagRegistry) FluidHeight(id world.BlockID) uint8 {
	return internalTestRegistry{}.FluidHeight(id)
}
func (modelTagRegistry) LightAttenuation(id world.BlockID) uint8 {
	return internalTestRegistry{}.LightAttenuation(id)
}
func (modelTagRegistry) BlockTopRaw(id world.BlockID) uint8 {
	return internalTestRegistry{}.BlockTopRaw(id)
}
func (r modelTagRegistry) Model(world.BlockID) uint8 { return uint8(r) }
func (modelTagRegistry) MeshSnapshot() RegistrySnapshot {
	panic("modelTagRegistry.MeshSnapshot 不应被调用")
}

// TestBuildRegistrySnapshotModelTagDomain 把 model tag 的封闭集合钉在快照层：
// 0（默认）、1..5（火把五形态）与 6（床八形态共用的单值床几何）合法，7 起
// 的未知值被拒。Rust 侧 `RegistryView::validate` 同口径拒绝，这里提前给出
// 可读错误。
func TestBuildRegistrySnapshotModelTagDomain(t *testing.T) {
	for _, tag := range []uint8{0, 1, 2, 3, 4, 5, 6} {
		if _, err := BuildRegistrySnapshot(
			[]world.BlockID{core.AirID, core.BarrierID},
			modelTagRegistry(tag),
		); err != nil {
			t.Fatalf("model tag=%d 被拒绝：%v", tag, err)
		}
	}
	for _, tag := range []uint8{7, 8, 255} {
		if _, err := BuildRegistrySnapshot(
			[]world.BlockID{core.AirID, core.BarrierID},
			modelTagRegistry(tag),
		); err == nil {
			t.Fatalf("model tag=%d 未被拒绝", tag)
		}
	}
}

// TestEncodeNativeInputWritesModelByteAtOffsetNineteen 锁定 registry entry 第
// 20 字节：model 追加在条目末尾（offset 19），既有 19 字节布局逐字节不变。
// 长度算式 16 + 27*4096*2 + 9 + 9*256*2 + 3*20 + 3*8 = 225901 是「model 字节
// 真的被写进去了」的算术证据（上一版 3 条 ×19 字节时是 225898）。
func TestEncodeNativeInputWritesModelByteAtOffsetNineteen(t *testing.T) {
	snapshot := RegistrySnapshot{
		Blocks: []BlockProperties{
			{ID: core.AirID, Materials: [6]uint16{1, 2, 3, 4, 5, 6}},
			{ID: core.BarrierID, Opaque: true, Materials: [6]uint16{10, 11, 12, 13, 14, 15}},
			{ID: 40002, Model: 3, Materials: [6]uint16{20, 21, 22, 23, 24, 25}},
		},
		Visibility: []uint64{2, 5, 1},
	}
	dst := make([]byte, 300000)
	length, err := encodeNativeInput(dst, fullyLoadedAirNeighborhood(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if length != 225901 {
		t.Fatalf("input length=%d，想要 225901", length)
	}
	const registryOffset = 225817
	// 前两条条目的 model 字节必须是默认 0：证明写入没有跨条目串位。
	for entryIndex := 0; entryIndex < 2; entryIndex++ {
		if got := dst[registryOffset+entryIndex*20+19]; got != 0 {
			t.Fatalf("registry 条目 %d 的 model=%d，想要默认 0", entryIndex, got)
		}
	}
	// 第三条驮 model=3：offset 19 恰在 blockTopRaw（offset 18）之后。
	if got := dst[registryOffset+2*20+19]; got != 3 {
		t.Fatalf("third registry model=%d，想要 3", got)
	}
	// 既有布局锚点不随 model 追加而移动：第三条 id 与 blockTopRaw 前的
	// lightAttenuation 仍在原偏移。
	if got := binary.LittleEndian.Uint16(dst[registryOffset+40:]); got != 40002 {
		t.Fatalf("third registry ID=%d，想要 40002", got)
	}
	if got := dst[registryOffset+40+17]; got != 0 {
		t.Fatalf("third registry lightAttenuation=%d，想要 0", got)
	}
	// visibility 紧随条目表之后，行距随 20 字节条目步长平移。
	if got := binary.LittleEndian.Uint64(dst[registryOffset+3*20+8:]); got != 5 {
		t.Fatalf("visibility row 1=%d，想要 5", got)
	}
}

// TestEncodeNativeInputRejectsUnknownModelTags 是上一条的编码侧镜像：
// 未通过 BuildRegistrySnapshot 预检的手工快照同样在编码期被拒绝。
func TestEncodeNativeInputRejectsUnknownModelTags(t *testing.T) {
	for _, tag := range []uint8{7, 8, 255} {
		snapshot := RegistrySnapshot{
			Blocks: []BlockProperties{
				{ID: core.AirID},
				{ID: core.BarrierID, Opaque: true, Model: tag},
			},
			Visibility: []uint64{0, 0},
		}
		if _, err := encodeNativeInput(make([]byte, 300000), fullyLoadedAirNeighborhood(), snapshot); err == nil {
			t.Fatalf("model tag=%d 未被编码器拒绝", tag)
		}
	}
}

// torchFixtureMaterial 是火把夹具共用的材质层号：刻意取植物区间
// [PlantMaterialFirst, PlantMaterialLast] 与耕地区间之外，证明火把 quad 的
// 判别完全依赖 registry model tag、不与任何 material 区间判别串味。
const torchFixtureMaterial uint16 = 90

// torchModelRegistry 是带 model tag 的火把网格夹具：五种火把形态分别登记
// model 1..5（与方块编号 71..75 同序），材质共用 torchFixtureMaterial，
// 发光 14 与 core.BlockEmission 的生产取值一致。
type torchModelRegistry struct{ internalTestRegistry }

func (torchModelRegistry) Material(id world.BlockID, _ Face) uint16 {
	if core.IsTorch(id) {
		return torchFixtureMaterial
	}
	return 0
}

func (torchModelRegistry) Emission(id world.BlockID) uint8 {
	if core.IsTorch(id) {
		return 14
	}
	return 0
}

// Model 实现可选的 ModelReader 接口：未实现该接口的 registry（例如尚未
// 登记火把材质的 assets.Registry）整体回落默认 tag 0，既有快照零改动。
func (torchModelRegistry) Model(id world.BlockID) uint8 {
	switch id {
	case core.TorchStandingID:
		return 1
	case core.TorchWallPosXID:
		return 2
	case core.TorchWallNegXID:
		return 3
	case core.TorchWallPosZID:
		return 4
	case core.TorchWallNegZID:
		return 5
	default:
		return 0
	}
}

func (r torchModelRegistry) MeshSnapshot() RegistrySnapshot {
	snapshot, err := BuildRegistrySnapshot([]world.BlockID{
		core.AirID,
		core.BarrierID,
		core.StoneID,
		core.TorchStandingID,
		core.TorchWallPosXID,
		core.TorchWallNegXID,
		core.TorchWallPosZID,
		core.TorchWallNegZID,
	}, r)
	if err != nil {
		panic(err)
	}
	return snapshot
}

// torchQuadsAtCell 取出落在某一格上的全部火把夹具 quad。
func torchQuadsAtCell(quads []Quad, x, y, z uint8) []Quad {
	var result []Quad
	for _, quad := range quads {
		if quad.Mat == torchFixtureMaterial && quad.X == x && quad.Y == y && quad.Z == z {
			result = append(result, quad)
		}
	}
	return result
}

// TestNativeMeshTorchQuadsRoundTripThroughGoUnpack 让五形态火把真的过一次
// FFI 并经 UnpackQuad 解包：落地 4 条居中双面交叉斜面、墙面 3 条贴面外倾，
// 材质与光值（火把自身发光 14 经光照 BFS 衰减到上方格的 13，叠加满天空光
// 15）无损回读。任一环节丢位——编码漏写 model 字节、Rust 拒绝 20 字节条
// 目、解包误读角高度——这里都会先于任何渲染路径变红。
func TestNativeMeshTorchQuadsRoundTripThroughGoUnpack(t *testing.T) {
	n := fullyLoadedAirNeighborhood()
	n.Center.Blocks.Set(8, 8, 8, core.TorchStandingID)
	n.Center.Blocks.Set(4, 8, 8, core.TorchWallPosXID)
	n.Center.Blocks.Set(8, 8, 4, core.TorchWallPosZID)
	n.Center.Blocks.Set(12, 8, 12, core.TorchWallNegXID)
	n.Center.Blocks.Set(12, 8, 4, core.TorchWallNegZID)

	quads := MeshSection(n, torchModelRegistry{}, NewLightScratch())

	// 落地：4 条，两条对角面 × 正背各一，居中竖直窄柱（交叉斜面是 8 字节
	// quad 格式里唯一的格内居中表达，视觉窄度由材质 alpha 收窄）。
	standing := torchQuadsAtCell(quads, 8, 8, 8)
	if len(standing) != 4 {
		t.Fatalf("落地火把 quad=%d 条，想要 4", len(standing))
	}
	wantFaces := []Face{FacePlantDiagA, FacePlantDiagA, FacePlantDiagB, FacePlantDiagB}
	wantBacks := []bool{false, true, false, true}
	for i, quad := range standing {
		if quad.Face != wantFaces[i] || quad.Back != wantBacks[i] {
			t.Fatalf("落地 quad[%d] face/back=%v/%v，想要 %v/%v", i, quad.Face, quad.Back, wantFaces[i], wantBacks[i])
		}
		if quad.W != 1 || quad.H != 1 {
			t.Fatalf("落地 quad[%d] 被贪心合并成 %dx%d", i, quad.W, quad.H)
		}
		if quad.Corners != [4]uint8{} {
			t.Fatalf("落地 quad[%d] 不应带角高度：%v", i, quad.Corners)
		}
		if quad.Mat != torchFixtureMaterial {
			t.Fatalf("落地 quad[%d] material=%d，想要 %d", i, quad.Mat, torchFixtureMaterial)
		}
		if quad.Light != 0xfd {
			t.Fatalf("落地 quad[%d] light=%#x，想要上方格 0xfd（sky 15 | block 13）", i, quad.Light)
		}
	}

	// 墙面 +X（支撑在 −X 侧）：两片 ±Z 斜板顶缘向 +X 抬高 + 一片 −X 贴面帽。
	wallPosX := torchQuadsAtCell(quads, 4, 8, 8)
	if len(wallPosX) != 3 {
		t.Fatalf("墙 +X 火把 quad=%d 条，想要 3", len(wallPosX))
	}
	wantWallPosX := []struct {
		face    Face
		corners [4]uint8
	}{
		{FaceNegZ, [4]uint8{0, 0, 13, 8}},
		{FacePosZ, [4]uint8{0, 0, 13, 8}},
		{FaceNegX, [4]uint8{}},
	}
	for i, want := range wantWallPosX {
		if wallPosX[i].Face != want.face {
			t.Fatalf("墙 +X quad[%d] face=%v，想要 %v", i, wallPosX[i].Face, want.face)
		}
		if wallPosX[i].Corners != want.corners {
			t.Fatalf("墙 +X quad[%d] corners=%v，想要 %v", i, wallPosX[i].Corners, want.corners)
		}
		if wallPosX[i].W != 1 || wallPosX[i].H != 1 || wallPosX[i].Back {
			t.Fatalf("墙 +X quad[%d] 尺寸/正背非法：%+v", i, wallPosX[i])
		}
		if wallPosX[i].Light != 0xfd {
			t.Fatalf("墙 +X quad[%d] light=%#x，想要 0xfd", i, wallPosX[i].Light)
		}
	}

	// 墙面 +Z（支撑在 −Z 侧）：两片 ±X 斜板 + 一片 −Z 贴面帽。
	wallPosZ := torchQuadsAtCell(quads, 8, 8, 4)
	if len(wallPosZ) != 3 {
		t.Fatalf("墙 +Z 火把 quad=%d 条，想要 3", len(wallPosZ))
	}
	for i, want := range []struct {
		face    Face
		corners [4]uint8
	}{
		{FaceNegX, [4]uint8{0, 8, 13, 0}},
		{FacePosX, [4]uint8{0, 8, 13, 0}},
		{FaceNegZ, [4]uint8{}},
	} {
		if wallPosZ[i].Face != want.face || wallPosZ[i].Corners != want.corners {
			t.Fatalf("墙 +Z quad[%d]=%+v，想要 face=%v corners=%v", i, wallPosZ[i], want.face, want.corners)
		}
	}

	// 墙面 −X / −Z：倾斜方向镜像（远离支撑一侧的顶缘角更高）。
	wallNegX := torchQuadsAtCell(quads, 12, 8, 12)
	if len(wallNegX) != 3 {
		t.Fatalf("墙 −X 火把 quad=%d 条，想要 3", len(wallNegX))
	}
	if wallNegX[0].Corners != [4]uint8{0, 0, 8, 13} || wallNegX[1].Corners != [4]uint8{0, 0, 8, 13} {
		t.Fatalf("墙 −X 斜板 corners=%v / %v，想要 [0 0 8 13]", wallNegX[0].Corners, wallNegX[1].Corners)
	}
	if wallNegX[2].Face != FacePosX {
		t.Fatalf("墙 −X 贴面帽 face=%v，想要 FacePosX", wallNegX[2].Face)
	}
	wallNegZ := torchQuadsAtCell(quads, 12, 8, 4)
	if len(wallNegZ) != 3 {
		t.Fatalf("墙 −Z 火把 quad=%d 条，想要 3", len(wallNegZ))
	}
	if wallNegZ[0].Corners != [4]uint8{0, 13, 8, 0} || wallNegZ[1].Corners != [4]uint8{0, 13, 8, 0} {
		t.Fatalf("墙 −Z 斜板 corners=%v / %v，想要 [0 13 8 0]", wallNegZ[0].Corners, wallNegZ[1].Corners)
	}
	if wallNegZ[2].Face != FacePosZ {
		t.Fatalf("墙 −Z 贴面帽 face=%v，想要 FacePosZ", wallNegZ[2].Face)
	}

	// 火把之间不合并、也不并走相邻方块的合并：这里没有相邻火把，退一步
	// 断言火把 quad 没有溢出到别的格子（坐标过滤已经隐含），并保留石头
	// 对照——中心区段补一块石头后它必须照常出满 6 面（不透明孤立块）。
	n.Center.Blocks.Set(1, 1, 1, core.StoneID)
	quadsWithStone := MeshSection(n, torchModelRegistry{}, NewLightScratch())
	stone := 0
	for _, quad := range quadsWithStone {
		if quad.X == 1 && quad.Y == 1 && quad.Z == 1 && quad.Mat == 0 {
			stone++
		}
	}
	if stone != 6 {
		t.Fatalf("对照石头出面=%d，想要 6（火把几何不得影响既有方块路径）", stone)
	}
	if !slices.EqualFunc(torchQuadsAtCell(quads, 8, 8, 8), torchQuadsAtCell(quadsWithStone, 8, 8, 8), func(a, b Quad) bool { return a.Pack() == b.Pack() }) {
		t.Fatal("加入对照石头后火把 quad 发生变化")
	}
}
