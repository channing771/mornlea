package mesh_test

import (
	"math"
	"math/rand"
	"testing"
	"unsafe"

	"github.com/channing771/mornlea/internal/assets"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/mesh"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/world"
)

func TestQuadPackRemainsEightBytes(t *testing.T) {
	if got := unsafe.Sizeof(mesh.Quad{}.Pack()); got != 8 {
		t.Fatalf("Quad.Pack 大小 = %d，想要 8", got)
	}
}

func TestQuadPackRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 100000; i++ {
		mat := uint16(rng.Intn(65536))
		// 植物 material 只允许出现在 face 6/7 上（quad.go 的双向约束），随机的
		// 轴向面必须避开那 8 个值，否则 Pack 会按约定拒绝。植物 quad 自身的往返
		// 由 TestPlantQuadsSurviveTheUploadRoundTrip 覆盖。
		if mesh.PlantMaterial(mat) {
			mat = mesh.PlantMaterialLast + 1
		}
		want := mesh.Quad{
			X:     uint8(rng.Intn(16)),
			Y:     uint8(rng.Intn(16)),
			Z:     uint8(rng.Intn(16)),
			W:     uint8(rng.Intn(16) + 1),
			H:     uint8(rng.Intn(16) + 1),
			Face:  mesh.Face(rng.Intn(6)),
			Mat:   mat,
			AO:    uint8(rng.Intn(256)),
			Light: uint8(rng.Intn(256)),
		}
		if got := mesh.UnpackQuad(want.Pack()); got != want {
			t.Fatalf("往返不一致:\n实际 %+v\n期望 %+v", got, want)
		}
	}
}

// TestQuadPackRoundTripCarriesCornerHeights 对四个角高度的全部合法组合穷举往返。
//
// 取值域是「顶面顶点的 7..=15」加「非顶面顶点的 0」；角 2 必须非零，否则整条
// quad 会被判成普通 quad（判别规则见 UnpackQuad）。任一角写错位偏移、或高位
// 那两个角在解包时被丢掉，往返都会对不上——后者正是上传路径上的数据丢失。
func TestQuadPackRoundTripCarriesCornerHeights(t *testing.T) {
	values := []uint8{0, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	checked := 0
	for _, c0 := range values {
		for _, c1 := range values {
			for _, c2 := range values {
				if c2 == 0 {
					continue
				}
				for _, c3 := range values {
					want := mesh.Quad{
						X: 1, Y: 2, Z: 3, W: 1, H: 1,
						Face: mesh.FacePosY, Mat: 0xBEEF, AO: 0x5A, Light: 0xA5,
						Corners: [4]uint8{c0, c1, c2, c3},
					}
					packed := want.Pack()
					if got := mesh.UnpackQuad(packed); got != want {
						t.Fatalf("往返不一致:\n实际 %+v\n期望 %+v", got, want)
					}
					// quad 实例格式 MUST 保持 8 字节：bit 63 必须仍然空着。
					if packed>>63 != 0 {
						t.Fatalf("corners=%v 占用了 bit 63: %#016x", want.Corners, packed)
					}
					checked++
				}
			}
		}
	}
	if want := len(values) * len(values) * (len(values) - 1) * len(values); checked != want {
		t.Fatalf("穷举组合数=%d，想要 %d", checked, want)
	}
}

// TestQuadPackRejectsOutOfRangeCorner 锁定 Go 与 Rust 的 pack 断言对称：
// 角高度只有 4 bit，越界值会串进相邻字段，两侧都必须当场炸而不是静默写坏。
func TestQuadPackRejectsOutOfRangeCorner(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("角高度 16 未被拒绝")
		}
	}()
	mesh.Quad{X: 1, Y: 1, Z: 1, W: 1, H: 1, Corners: [4]uint8{0, 0, 16, 0}}.Pack()
}

func TestQuadPackFitsIn55Bits(t *testing.T) {
	full := mesh.Quad{
		X: 15, Y: 15, Z: 15, W: 16, H: 16,
		Face: 5, Mat: 0xFFFF, AO: 0xFF, Light: 0xFF,
	}
	if v := full.Pack(); v>>55 != 0 {
		t.Fatalf("打包用满了高位: %#016x，第 55 位以上应为空", v)
	}
}

// TestDoorQuad 验证门材质与薄板几何快照（关贴边、开旋转 90°）。
//
// 材质：DoorMaterial=55 紧接植物区间 31..54，单值且非植物、独立于 Workbench。
// 几何：关闭厚 3/16 贴方向边，开启旋转 90° 到左铰链边；上半无碰撞。
// 同时验证门 quad 的 Pack/Unpack 往返与 mesh 快照出面（非不透明薄板对空气可见）。
func TestDoorQuad(t *testing.T) {
	if mesh.DoorMaterial != 55 {
		t.Fatalf("DoorMaterial=%d，想要 55", mesh.DoorMaterial)
	}
	if mesh.PlantMaterial(mesh.DoorMaterial) {
		t.Fatalf("DoorMaterial %d 落进植物区间", mesh.DoorMaterial)
	}
	if !mesh.IsDoorMaterial(mesh.DoorMaterial) {
		t.Fatal("IsDoorMaterial 对 DoorMaterial 返回 false")
	}
	if mesh.PlantMaterial(assets.LayerDoor) {
		t.Fatalf("LayerDoor %d 落进植物区间", assets.LayerDoor)
	}

	// 资产层一致性
	reg := assets.NewRegistry()
	if got := reg.Material(core.DoorLowerSouthClosed, mesh.FacePosY); got != mesh.DoorMaterial {
		t.Fatalf("门下半材质=%d，想要 DoorMaterial %d", got, mesh.DoorMaterial)
	}
	if got := reg.Material(core.DoorUpper, mesh.FaceNegX); got != mesh.DoorMaterial {
		t.Fatalf("门上半材质=%d，想要 DoorMaterial %d", got, mesh.DoorMaterial)
	}
	if reg.Opaque(core.DoorLowerSouthClosed) || reg.Opaque(core.DoorUpper) {
		t.Fatal("门不应是不透明方块（薄板）")
	}
	if reg.BlockTopRaw(core.DoorLowerSouthClosed) != 0 || reg.BlockTopRaw(core.DoorUpper) != 0 {
		t.Fatal("门 BlockTopRaw 必须为 0 不下沉")
	}
	// FaceVisible：薄板对空气可见
	if !reg.FaceVisible(core.DoorLowerSouthClosed, core.AirID) {
		t.Fatal("门朝向空气的面应可见")
	}

	// 碰撞薄板快照：关贴边、开旋转，厚度 3/16
	const thickness = float32(3.0 / 16.0)
	const eps = float32(1e-6)
	closed := map[core.BlockID][2]float32{
		core.DoorLowerSouthClosed: {1 - thickness, 1}, // Z 高边
		core.DoorLowerWestClosed:  {0, thickness},      // X 低边
		core.DoorLowerNorthClosed: {0, thickness},      // Z 低边
		core.DoorLowerEastClosed:  {1 - thickness, 1},  // X 高边
	}
	open := map[core.BlockID][2]float32{
		core.DoorLowerSouthOpen: {1 - thickness, 1}, // 南开→东
		core.DoorLowerWestOpen:  {1 - thickness, 1}, // 西开→南（Z）
		core.DoorLowerNorthOpen: {0, thickness},      // 北开→西
		core.DoorLowerEastOpen:  {0, thickness},      // 东开→北（Z）
	}
	for id, want := range closed {
		boxes := physics.BlockCollisionBoxes(id, true)
		if !boxes.Loaded || boxes.Count != 1 {
			t.Fatalf("关闭门 %d 碰撞 Loaded=%v Count=%d 想要 true/1", id, boxes.Loaded, boxes.Count)
		}
		b := boxes.Boxes[0]
		dir := core.DoorDir(id)
		var gotMin, gotMax float32
		if dir == 0 || dir == 2 {
			gotMin = b.Min.Z()
			gotMax = b.Max.Z()
		} else {
			gotMin = b.Min.X()
			gotMax = b.Max.X()
		}
		if math.Abs(float64(gotMin-want[0])) > float64(eps) || math.Abs(float64(gotMax-want[1])) > float64(eps) {
			t.Fatalf("关闭门 %d 方向 %d 薄板位置 min=%.4f max=%.4f 想要 %.4f/%.4f", id, dir, gotMin, gotMax, want[0], want[1])
		}
		if b.Min.Y() != 0 || b.Max.Y() != 1 {
			t.Fatalf("关闭门 %d Y 范围错误: %+v", id, b)
		}
	}
	for id, want := range open {
		boxes := physics.BlockCollisionBoxes(id, true)
		if !boxes.Loaded || boxes.Count != 1 {
			t.Fatalf("开启门 %d 碰撞 Loaded=%v Count=%d 想要 true/1", id, boxes.Loaded, boxes.Count)
		}
		b := boxes.Boxes[0]
		dir := core.DoorDir(id)
		// 开启时旋转 90°：南→X, 西→Z, 北→X, 东→Z
		var gotMin, gotMax float32
		if dir == 0 || dir == 2 {
			// 南/北开时薄边在 X
			gotMin = b.Min.X()
			gotMax = b.Max.X()
		} else {
			gotMin = b.Min.Z()
			gotMax = b.Max.Z()
		}
		if math.Abs(float64(gotMin-want[0])) > float64(eps) || math.Abs(float64(gotMax-want[1])) > float64(eps) {
			t.Fatalf("开启门 %d 方向 %d 薄板位置 min=%.4f max=%.4f 想要 %.4f/%.4f", id, dir, gotMin, gotMax, want[0], want[1])
		}
	}
	// 关与开同一方向的薄边必须不同（旋转）
	for _, dir := range []struct{ closed, open core.BlockID }{
		{core.DoorLowerSouthClosed, core.DoorLowerSouthOpen},
		{core.DoorLowerWestClosed, core.DoorLowerWestOpen},
		{core.DoorLowerNorthClosed, core.DoorLowerNorthOpen},
		{core.DoorLowerEastClosed, core.DoorLowerEastOpen},
	} {
		c := physics.BlockCollisionBoxes(dir.closed, true).Boxes[0]
		o := physics.BlockCollisionBoxes(dir.open, true).Boxes[0]
		if c.Min == o.Min && c.Max == o.Max {
			t.Fatalf("方向 %d 关与开碰撞相同，未旋转", core.DoorDir(dir.closed))
		}
	}
	// 上半无碰撞
	if boxes := physics.BlockCollisionBoxes(core.DoorUpper, true); !boxes.Loaded || boxes.Count != 0 {
		t.Fatalf("DoorUpper 碰撞 Loaded=%v Count=%d 想要 true/0", boxes.Loaded, boxes.Count)
	}

	// Mesh 快照：孤立门在空旷区段中应产生 DoorMaterial 的 quad，且 Pack 往返无损
	airSection := world.NewSection()
	for _, id := range []core.BlockID{
		core.DoorLowerSouthClosed, core.DoorLowerSouthOpen, core.DoorUpper,
	} {
		center := world.NewSection()
		center.Blocks.Set(8, 8, 8, id)
		n := &world.Neighborhood{Center: center, SectionY: 4}
		for cx := 0; cx < 3; cx++ {
			for cy := 0; cy < 3; cy++ {
				for cz := 0; cz < 3; cz++ {
					n.Around[cx][cy][cz] = airSection
				}
			}
		}
		quads := mesh.MeshSection(n, reg, mesh.NewLightScratch())
		found := false
		for _, q := range quads {
			if q.Mat == mesh.DoorMaterial {
				found = true
				if got := mesh.UnpackQuad(q.Pack()); got != q {
					t.Fatalf("门 %d quad 往返不一致: %+v vs %+v", id, got, q)
				}
				if q.Pack()>>63 != 0 {
					t.Fatalf("门 %d quad 占用 bit63", id)
				}
			}
		}
		if !found {
			t.Fatalf("门 %d 未产生 DoorMaterial quad，实际 %d 条: %+v", id, len(quads), quads)
		}
	}
}
