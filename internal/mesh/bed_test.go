package mesh

import (
	"slices"
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

// bedFixtureMaterial 是床夹具共用的材质层号：与火把夹具同理，刻意取植物区间
// 与耕地区间之外，证明床 quad 的判别完全依赖 registry model tag。
const bedFixtureMaterial uint16 = 91

// bedModelRegistry 是带 model tag 的床网格夹具：床尾南向形态登记保留的
// model 6，材质共用 bedFixtureMaterial，非不透明、不发光（与生产侧
// assets.Registry 对床的登记一致）。
type bedModelRegistry struct{ internalTestRegistry }

func (bedModelRegistry) Opaque(id world.BlockID) bool {
	if core.IsBed(id) {
		return false
	}
	return internalTestRegistry{}.Opaque(id)
}

func (bedModelRegistry) FaceVisible(id, adjacent world.BlockID) bool {
	if core.IsBed(id) {
		return !internalTestRegistry{}.Opaque(adjacent)
	}
	return internalTestRegistry{}.FaceVisible(id, adjacent)
}

func (bedModelRegistry) Material(id world.BlockID, _ Face) uint16 {
	if core.IsBed(id) {
		return bedFixtureMaterial
	}
	return 0
}

// Model 实现可选的 ModelReader 接口：床八形态统一登记保留 tag 6。
func (bedModelRegistry) Model(id world.BlockID) uint8 {
	if core.IsBed(id) {
		return 6
	}
	return 0
}

func (r bedModelRegistry) MeshSnapshot() RegistrySnapshot {
	snapshot, err := BuildRegistrySnapshot([]world.BlockID{
		core.AirID,
		core.BarrierID,
		core.StoneID,
		core.BedFootSouthID,
	}, r)
	if err != nil {
		panic(err)
	}
	return snapshot
}

// bedQuadsAtCell 取出落在某一格上的全部床夹具 quad。
func bedQuadsAtCell(quads []Quad, x, y, z uint8) []Quad {
	var result []Quad
	for _, quad := range quads {
		if quad.Mat == bedFixtureMaterial && quad.X == x && quad.Y == y && quad.Z == z {
			result = append(result, quad)
		}
	}
	return result
}

// TestNativeMeshBedQuadsRoundTripThroughGoUnpack 让保留 model tag 6 的床真的
// 过一次 FFI 并经 UnpackQuad 解包：单格床几何为 5 条 quad——四面半高侧板
// （顶缘角高度 8，即 (8+1)/16 = 9/16，与 physics 的床碰撞体同线）+ 一片平顶。
// 面次序与角高度逐条钉死；顶缘高度写错（9/16 变满格或更矮）任一格都会红。
// 不出底面：床必贴支撑放置，底面与支撑块顶面共面，无条件发射只会 z-fighting。
func TestNativeMeshBedQuadsRoundTripThroughGoUnpack(t *testing.T) {
	n := fullyLoadedAirNeighborhood()
	n.Center.Blocks.Set(8, 8, 8, core.BedFootSouthID)

	quads := MeshSection(n, bedModelRegistry{}, NewLightScratch())

	bed := bedQuadsAtCell(quads, 8, 8, 8)
	if len(bed) != 5 {
		t.Fatalf("床 quad=%d 条，想要 5（四侧板 + 平顶）", len(bed))
	}
	want := []struct {
		face    Face
		corners [4]uint8
	}{
		{FaceNegX, [4]uint8{0, 8, 8, 0}},
		{FacePosX, [4]uint8{0, 8, 8, 0}},
		{FaceNegZ, [4]uint8{0, 0, 8, 8}},
		{FacePosZ, [4]uint8{0, 0, 8, 8}},
		{FacePosY, [4]uint8{8, 8, 8, 8}},
	}
	for i, w := range want {
		if bed[i].Face != w.face {
			t.Fatalf("床 quad[%d] face=%v，想要 %v", i, bed[i].Face, w.face)
		}
		if bed[i].Corners != w.corners {
			t.Fatalf("床 quad[%d] corners=%v，想要 %v", i, bed[i].Corners, w.corners)
		}
		if bed[i].W != 1 || bed[i].H != 1 || bed[i].Back {
			t.Fatalf("床 quad[%d] 尺寸/正背非法：%+v", i, bed[i])
		}
		if bed[i].Mat != bedFixtureMaterial {
			t.Fatalf("床 quad[%d] material=%d，想要 %d", i, bed[i].Mat, bedFixtureMaterial)
		}
		if bed[i].AO != 0xff {
			t.Fatalf("床 quad[%d] ao=%#x，想要格内几何记满的 0xff", i, bed[i].AO)
		}
	}
	// 五条 quad 光照一致（取自床顶上方格，光照模型本身不因床改变，不在此
	// 钉绝对值）。
	for i := 1; i < len(bed); i++ {
		if bed[i].Light != bed[0].Light {
			t.Fatalf("床 quad[%d] light=%#x 与首条 %#x 不一致", i, bed[i].Light, bed[0].Light)
		}
	}
	// 床格不得再走默认立方体路径：除床几何外，本格不允许出现任何其他 quad。
	var others []Quad
	for _, quad := range quads {
		if quad.X == 8 && quad.Y == 8 && quad.Z == 8 && quad.Mat != bedFixtureMaterial {
			others = append(others, quad)
		}
	}
	if len(others) != 0 {
		t.Fatalf("床格漏出 %d 条非床 quad：model 豁免失效 %+v", len(others), others)
	}
	// 保留石头对照：床几何不得影响既有方块路径（石头照常出满 6 面）。
	n.Center.Blocks.Set(1, 1, 1, core.StoneID)
	quadsWithStone := MeshSection(n, bedModelRegistry{}, NewLightScratch())
	stone := 0
	for _, quad := range quadsWithStone {
		if quad.X == 1 && quad.Y == 1 && quad.Z == 1 && quad.Mat == 0 {
			stone++
		}
	}
	if stone != 6 {
		t.Fatalf("对照石头出面=%d，想要 6（床几何不得影响既有方块路径）", stone)
	}
	if !slices.EqualFunc(bedQuadsAtCell(quads, 8, 8, 8), bedQuadsAtCell(quadsWithStone, 8, 8, 8), func(a, b Quad) bool { return a.Pack() == b.Pack() }) {
		t.Fatal("加入对照石头后床 quad 发生变化")
	}
}
