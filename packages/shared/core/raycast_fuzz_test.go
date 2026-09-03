package core

// raycast_fuzz_test.go：`RaycastBlocks` 的纯性质网与其确定性孪生用例。
//
// 差分 Go oracle（旧 DDA 副本）已随 change drop-go-test-oracles 删除，正确性
// 不再靠「与冻结副本逐位一致」把守，改由互补的几层共同覆盖（design D2）：
//
//   - `TestRaycastInputCursorAndRecordLayoutV1` 钉死 input/cursor/record 的
//     ABI 字节布局；
//   - raycast_native_test.go 的行为锁钉死回调传播、batch 边界与并发一致性；
//   - 本文件的 `FuzzRaycastBlocks` 对任意有限输入验证五条不变量：有限输入
//     无错、命中距离有界、命中格实心、命中点位于命中格单位立方内、命中面是
//     进入面。
//
// 后两条几何不变量是 oracle 删除后的补位：floor/int32 回绕算术错、距离与格
// 失步、报告陈旧格、多步漂移这类缺陷都会表现为「命中点不在其声称的命中格立方
// 内」或「进入面法线与方向点积为正」，无需任何参考实现即可发现。覆盖分工：
// 纯遍历序回归不在其锁定面内——序错时每条 record 仍几何自洽，遍历序由
// `TestRaycastBlocksUsesXYZTiePriority` 与 Rust 侧严格 XYZ 平局序测试兜底。
// 紧随其后的两个确定性测试是同一性质的固定输入孪生，保证性质本身可离线复核，
// fuzz 语料意外退化时先在这里暴露。

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func FuzzRaycastBlocks(f *testing.F) {
	f.Add(
		float32(0.5), float32(1.5), float32(2.5),
		float32(1), float32(0), float32(0), float32(6),
	)
	f.Add(
		float32(-10.25), float32(-3.5), float32(-7.75),
		float32(-1), float32(0.5), float32(-0.25), float32(32),
	)

	f.Fuzz(func(
		t *testing.T,
		ox, oy, oz, dx, dy, dz, maxDistance float32,
	) {
		values := [...]float32{ox, oy, oz, dx, dy, dz, maxDistance}
		for _, value := range values {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				t.Skip()
			}
		}
		if ox < -1024 || ox > 1024 ||
			oy < -1024 || oy > 1024 ||
			oz < -1024 || oz > 1024 ||
			maxDistance < 0.01 || maxDistance > 32 {
			t.Skip()
		}
		length := math.Hypot(math.Hypot(float64(dx), float64(dy)), float64(dz))
		if length < 1e-6 {
			t.Skip()
		}

		solid := func(p BlockPos) (bool, error) {
			return (p.X*31+p.Y*17+p.Z*13)&15 == 0, nil
		}
		hit, ok, err := RaycastBlocks(
			mgl32.Vec3{ox, oy, oz},
			mgl32.Vec3{dx, dy, dz},
			maxDistance,
			solid,
		)
		if err != nil {
			t.Fatalf("有限且有效的输入返回错误: %v", err)
		}
		if !ok {
			return
		}
		if hit.Distance < 0 || hit.Distance > maxDistance {
			t.Fatalf("命中距离 %f 不在 [0,%f]", hit.Distance, maxDistance)
		}
		occupied, err := solid(hit.Block)
		if err != nil || !occupied {
			t.Fatalf("返回的方块 %+v 不是实心", hit.Block)
		}

		// 不变量一：命中点位于命中格单位立方内（含边界面）。`RayHit.Point`
		// 由 origin + 方向×距离 重构，本条锁定「格与距离失步」类缺陷：floor/
		// int32 回绕算术错、跨格后未按新格重算边界、报告陈旧格或多步漂移，都会
		// 让局部坐标越出 [0,1]。它不锁纯遍历序回归（序错时每条 record 仍几何
		// 自洽，覆盖分工见文件头）；轴向恰好多推一步会落在共享面上（local=0/1）
		// 而不越界，掠射对角的漂移也可低于容差。浮点误差链只有几个 ulp
		// （语料坐标上限约 2048，ulp 约 2.4e-4），1e-2 容差的作用是把舍入
		// 噪声挡在外面而非放大信号。
		const cubeTolerance = float32(1e-2)
		blockAxes := [3]int32{hit.Block.X, hit.Block.Y, hit.Block.Z}
		for axis := range 3 {
			local := hit.Point[axis] - float32(blockAxes[axis])
			if local < -cubeTolerance || local > 1+cubeTolerance {
				t.Fatalf("Point[%d]=%v 相对命中格 %+v 的局部坐标 %v 越出 [0,1]",
					axis, hit.Point[axis], hit.Block, local)
			}
		}

		// 不变量二：命中面必须是进入面。face 编码偶数项外法线朝负轴、奇数项朝
		// 正轴（与 `decodeRaycastRecord` 的 face&1 校验同约定），进入面的外法线
		// 与归一化方向点积必 < 0；face 换成对面或轴错位都会让点积转正，无需
		// 参考实现即可发现。起点即实心的命中没有几何进入面，以 `BlockFaceNone`
		// 表达并豁免本不变量（不变量一对其依然成立：点即起点，在本格内）。
		if hit.Face != BlockFaceNone {
			normalized := mgl32.Vec3{dx, dy, dz}.Mul(float32(1 / length))
			if dot := faceNormalSign(hit.Face) * normalized[int(hit.Face/2)]; dot >= 0 {
				t.Fatalf("Face=%d 外法线与归一化方向点积 %v 不为负，命中格 %+v",
					hit.Face, dot, hit.Block)
			}
		}
	})
}

// faceNormalSign 返回 `BlockFace` 外法线沿自身轴向的符号：偶数编号面（NegX/
// NegY/NegZ）为 −1，奇数编号面（PosX/PosY/PosZ）为 +1，与生产
// `decodeRaycastRecord` 的 face&1 步进方向校验共享同一约定。fuzz 体与确定性
// 孪生用例经它复用同一份 face→法线换算，避免两处手写漂移。
func faceNormalSign(face BlockFace) float32 {
	if face%2 == 1 {
		return 1
	}
	return -1
}

func TestRaycastBlocksHitPointStaysInHitCellCube(t *testing.T) {
	for _, tc := range []struct {
		name              string
		origin, direction mgl32.Vec3
		target            BlockPos
		face              BlockFace
	}{
		{
			// 斜向进入且交点落在格内部：局部坐标必须严格处于开区间内。
			name:      "interior point on positive diagonal",
			origin:    mgl32.Vec3{0.5, 0.5, 0.5},
			direction: mgl32.Vec3{1, 0.5, 0},
			target:    BlockPos{X: 1},
			face:      BlockFaceNegX,
		},
		{
			// 负轴向进入：x 分量恰落在上边界，局部坐标必须容许到 1。
			name:      "boundary point at one on negative axis",
			origin:    mgl32.Vec3{3.5, 0.5, 0.5},
			direction: mgl32.Vec3{-1, 0, 0},
			target:    BlockPos{X: 1},
			face:      BlockFacePosX,
		},
		{
			// 下行斜线按 tMax 序依次跨 x（2.25→2.0）、跨 y（1.5→1.0）、再跨 x
			// 进入末格：验证多次步进后交点仍在末格立方内。
			name:      "descending diagonal after multiple steps",
			origin:    mgl32.Vec3{2.25, 1.5, 3.5},
			direction: mgl32.Vec3{-1, -0.5, 0},
			target:    BlockPos{X: 0, Y: 0, Z: 3},
			face:      BlockFacePosX,
		},
		{
			// 起点即实心：`RayHit.Point` 恒等于 origin，局部坐标是起点的小数部分。
			name:      "origin inside solid pins point to origin",
			origin:    mgl32.Vec3{1.25, 2.5, 3.75},
			direction: mgl32.Vec3{0, 1, 0},
			target:    BlockPos{X: 1, Y: 2, Z: 3},
			face:      BlockFaceNone,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hit, ok, err := RaycastBlocks(tc.origin, tc.direction, 8, func(p BlockPos) (bool, error) {
				return p == tc.target, nil
			})
			if err != nil || !ok || hit.Block != tc.target || hit.Face != tc.face {
				t.Fatalf("hit=%+v ok=%v err=%v，想要 Block=%+v Face=%d", hit, ok, err, tc.target, tc.face)
			}
			const cubeTolerance = float32(1e-2)
			targetAxes := [3]int32{tc.target.X, tc.target.Y, tc.target.Z}
			for axis := range 3 {
				local := hit.Point[axis] - float32(targetAxes[axis])
				if local < -cubeTolerance || local > 1+cubeTolerance {
					t.Fatalf("Point[%d]=%v 局部坐标 %v 越出 [0,1]", axis, hit.Point[axis], local)
				}
			}
		})
	}
}

func TestRaycastBlocksHitFaceOpposesRayDirection(t *testing.T) {
	for _, tc := range []struct {
		name      string
		direction mgl32.Vec3
		target    BlockPos
		face      BlockFace
	}{
		{name: "positive X", direction: mgl32.Vec3{1, 0, 0}, target: BlockPos{X: 3}, face: BlockFaceNegX},
		{name: "negative X", direction: mgl32.Vec3{-1, 0, 0}, target: BlockPos{X: -2}, face: BlockFacePosX},
		{name: "positive Y", direction: mgl32.Vec3{0, 1, 0}, target: BlockPos{Y: 3}, face: BlockFaceNegY},
		{name: "negative Y", direction: mgl32.Vec3{0, -1, 0}, target: BlockPos{Y: -2}, face: BlockFacePosY},
		{name: "positive Z", direction: mgl32.Vec3{0, 0, 1}, target: BlockPos{Z: 3}, face: BlockFaceNegZ},
		{name: "negative Z", direction: mgl32.Vec3{0, 0, -1}, target: BlockPos{Z: -2}, face: BlockFacePosZ},
		{
			// XYZ 平局优先级下 z 最后跨格：斜向命中经 (1,1,0) 跨 z=1 进入，
			// 进入面是 NegZ——期望面逐行显式钉死，点积只作方向性复核。
			name:      "diagonal enters lowest-priority face",
			direction: mgl32.Vec3{1, 1, 1},
			target:    BlockPos{X: 1, Y: 1, Z: 1},
			face:      BlockFaceNegZ,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hit, ok, err := RaycastBlocks(mgl32.Vec3{0.5, 0.5, 0.5}, tc.direction, 8, func(p BlockPos) (bool, error) {
				return p == tc.target, nil
			})
			if err != nil || !ok || hit.Block != tc.target || hit.Face != tc.face {
				t.Fatalf("hit=%+v ok=%v err=%v，想要 Block=%+v Face=%d", hit, ok, err, tc.target, tc.face)
			}
			length := math.Hypot(math.Hypot(float64(tc.direction[0]), float64(tc.direction[1])), float64(tc.direction[2]))
			normalized := tc.direction.Mul(float32(1 / length))
			if dot := faceNormalSign(hit.Face) * normalized[int(hit.Face/2)]; dot >= 0 {
				t.Fatalf("Face=%d 外法线与归一化方向点积 %v 不为负", hit.Face, dot)
			}
		})
	}
}
