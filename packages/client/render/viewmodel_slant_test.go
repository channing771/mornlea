package render

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件锁定第一人称双手的斜持姿态：双手自左下/右下屏角斜向入画，长轴向
// 画面中心倾斜而非竖直柱状，右手为主手（更靠中心、位置更高），准星常空。

// viewmodelSlantDegreesOf 从实例矩阵还原手臂长轴与竖直方向的夹角（度）：
// 实例几何以局部原点为中心、`Y` 列即长轴方向（含缩放），零位姿下根为单位
// 阵，世界轴即相机轴，可直接读倾斜。
func viewmodelSlantDegreesOf(out []byte, index int) float64 {
	base := index * avatarInstanceBytes
	// 实例矩阵列主序：第 1 列（元素 4/5/6）即手臂长轴方向，不是第 1 行。
	axis := mgl32.Vec3{
		math.Float32frombits(uint32Of(out, base+4*4)),
		math.Float32frombits(uint32Of(out, base+5*4)),
		math.Float32frombits(uint32Of(out, base+6*4)),
	}
	length := axis.Len()
	if length == 0 {
		return 0
	}
	cos := axis[1] / length
	if cos > 1 {
		cos = 1
	}
	if cos < -1 {
		cos = -1
	}
	return math.Acos(float64(cos)) * 180 / math.Pi
}

func uint32Of(out []byte, offset int) uint32 {
	return uint32(out[offset]) | uint32(out[offset+1])<<8 |
		uint32(out[offset+2])<<16 | uint32(out[offset+3])<<24
}

// viewmodelInstanceCorner 把局部立方体的角点经实例矩阵变为世界点：局部半
// 边长恒为 0.5（缩放进矩阵），角点符号各取正负。
func viewmodelInstanceCorner(out []byte, index int, sx, sy, sz float32) mgl32.Vec3 {
	base := index * avatarInstanceBytes
	var columns [4]mgl32.Vec4
	for column := range 4 {
		for row := range 4 {
			columns[column][row] = math.Float32frombits(
				uint32Of(out, base+(column*4+row)*4))
		}
	}
	local := mgl32.Vec4{sx * 0.5, sy * 0.5, sz * 0.5, 1}
	x := columns[0][0]*local[0] + columns[1][0]*local[1] + columns[2][0]*local[2] + columns[3][0]*local[3]
	y := columns[0][1]*local[0] + columns[1][1]*local[1] + columns[2][1]*local[2] + columns[3][1]*local[3]
	z := columns[0][2]*local[0] + columns[1][2]*local[1] + columns[2][2]*local[2] + columns[3][2]*local[3]
	w := columns[0][3]*local[0] + columns[1][3]*local[1] + columns[2][3]*local[2] + columns[3][3]*local[3]
	return mgl32.Vec3{x / w, y / w, z / w}
}

// TestViewmodelSlantAngleInSpecRange 锁定斜持倾角落在契约区间：手臂长轴与
// 竖直方向的夹角必须在 20°–35° 之间，左右手都要满足。
func TestViewmodelSlantAngleInSpecRange(t *testing.T) {
	input := viewmodelTestInput(core.PlayerID{51}, core.ItemStack{}, 10)
	out := (&ViewmodelEncoder{}).EncodeViewmodelInstances(nil, input)
	if len(out) != 2*avatarInstanceBytes {
		t.Fatalf("中立实例数 = %d，想要 2（左右手）", len(out)/avatarInstanceBytes)
	}
	for index := range 2 {
		if degrees := viewmodelSlantDegreesOf(out, index); degrees < 20 || degrees > 35 {
			t.Fatalf("第 %d 只手倾角 = %.1f°，想要 20°–35°", index, degrees)
		}
	}
}

// TestViewmodelArmsLeanTowardCenter 锁定倾斜方向与主手布局：左手顶端偏右、
// 右手顶端偏左（向画面中心），右手更靠中心且位置更高。
func TestViewmodelArmsLeanTowardCenter(t *testing.T) {
	input := viewmodelTestInput(core.PlayerID{51}, core.ItemStack{}, 10)
	out := (&ViewmodelEncoder{}).EncodeViewmodelInstances(nil, input)
	topX := func(index int) float32 {
		base := index * avatarInstanceBytes
		return math.Float32frombits(uint32Of(out, base+4*4))
	}
	if got := topX(0); got <= 0 {
		t.Fatalf("左手长轴顶端 x 分量 = %.3f，想要偏右（>0）", got)
	}
	if got := topX(1); got >= 0 {
		t.Fatalf("右手长轴顶端 x 分量 = %.3f，想要偏左（<0）", got)
	}
	// 零位姿下世界坐标即相机坐标：右手理应更靠中心（|x| 更小）且更高。
	left, right := decodedPartCenter(out, 0), decodedPartCenter(out, 1)
	if abs32(right[0]) >= abs32(left[0]) {
		t.Fatalf("右手 |x|=%.3f，想要小于左手 |x|=%.3f（主手更靠中心）", right[0], left[0])
	}
	if right[1] <= left[1] {
		t.Fatalf("右手 y=%.3f，想要高于左手 y=%.3f", right[1], left[1])
	}
}

func abs32(value float32) float32 {
	if value < 0 {
		return -value
	}
	return value
}

// assertViewmodelCrosshairClear 断言准星像素不被双手覆盖：逐实例把 8 角点
// 投影为屏面凸包，原点落在任一凸包内（留一像素级余量）即失败。长条持物
// 的顶端可以越过中心线（《我的世界》式持握本就如此），只要细长剪影偏在
// 一侧、准星像素本身是空的即算通过。
func assertViewmodelCrosshairClear(t *testing.T, out []byte) {
	t.Helper()
	count := len(out) / avatarInstanceBytes
	viewProj := viewmodelProjectionViewProj(mgl32.Vec3{}, 0, 0)
	for index := range count {
		points := make([]mgl32.Vec2, 0, 8)
		for _, sx := range []float32{-1, 1} {
			for _, sy := range []float32{-1, 1} {
				for _, sz := range []float32{-1, 1} {
					world := viewmodelInstanceCorner(out, index, sx, sy, sz)
					ndc, w := projectToNDC(viewProj, world)
					if w <= 0 {
						t.Fatalf("实例 %d 角点落在相机后方（w=%.2f）", index, w)
					}
					points = append(points, mgl32.Vec2{ndc[0], ndc[1]})
				}
			}
		}
		if viewmodelOriginInConvexHull(points) {
			t.Fatalf("实例 %d 的投影覆盖准星像素，想要留空", index)
		}
	}
}

// viewmodelOriginInConvexHull 用单调链求凸包再判原点是否在其内部：凸包退
// 化为线段或点时不可能覆盖准星，直接通过。
func viewmodelOriginInConvexHull(points []mgl32.Vec2) bool {
	sorted := append([]mgl32.Vec2(nil), points...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && (sorted[j-1][0] > sorted[j][0] ||
			(sorted[j-1][0] == sorted[j][0] && sorted[j-1][1] > sorted[j][1])); j-- {
			sorted[j-1], sorted[j] = sorted[j], sorted[j-1]
		}
	}
	cross := func(o, a, b mgl32.Vec2) float32 {
		return (a[0]-o[0])*(b[1]-o[1]) - (a[1]-o[1])*(b[0]-o[0])
	}
	var lower, upper []mgl32.Vec2
	for _, p := range sorted {
		for len(lower) >= 2 && cross(lower[len(lower)-2], lower[len(lower)-1], p) <= 0 {
			lower = lower[:len(lower)-1]
		}
		lower = append(lower, p)
	}
	for i := len(sorted) - 1; i >= 0; i-- {
		p := sorted[i]
		for len(upper) >= 2 && cross(upper[len(upper)-2], upper[len(upper)-1], p) <= 0 {
			upper = upper[:len(upper)-1]
		}
		upper = append(upper, p)
	}
	hull := append(lower[:len(lower)-1], upper[:len(upper)-1]...)
	if len(hull) < 3 {
		return false
	}
	// 凸多边形同侧判定：原点与包内任一点恒在每条边的同侧。
	const margin = float32(0.01)
	sign := float32(0)
	for i := range hull {
		a, b := hull[i], hull[(i+1)%len(hull)]
		edge, toOrigin := mgl32.Vec2{b[0] - a[0], b[1] - a[1]}, mgl32.Vec2{-a[0], -a[1]}
		value := edge[0]*toOrigin[1] - edge[1]*toOrigin[0]
		if value > margin {
			if sign < 0 {
				return false
			}
			sign = 1
		} else if value < -margin {
			if sign > 0 {
				return false
			}
			sign = -1
		}
	}
	return true
}

// TestViewmodelCrosshairClearNeutral 锁定中立持握下准星未被遮挡：空手与持
// 物两态逐实例投影都不覆盖原点。
func TestViewmodelCrosshairClearNeutral(t *testing.T) {
	empty := viewmodelTestInput(core.PlayerID{51}, core.ItemStack{}, 10)
	assertViewmodelCrosshairClear(t, (&ViewmodelEncoder{}).EncodeViewmodelInstances(nil, empty))
	armed := viewmodelTestInput(core.PlayerID{51},
		core.ItemStack{Item: core.ItemIronSword, Count: 1, Durability: 125}, 10)
	assertViewmodelCrosshairClear(t, (&ViewmodelEncoder{}).EncodeViewmodelInstances(nil, armed))
}

// TestViewmodelCrosshairClearAtSwingPeak 锁定挥动峰值仍不挡准星：以最大摆
// 幅档（工具 0.7 弧度）正负峰值直接装配，投影依旧留空准星。
func TestViewmodelCrosshairClearAtSwingPeak(t *testing.T) {
	input := viewmodelTestInput(core.PlayerID{51},
		core.ItemStack{Item: core.ItemIronSword, Count: 1, Durability: 125}, 10)
	for _, angle := range []float32{0.7, -0.7} {
		parts := buildViewmodelParts(nil, input, angle)
		dst := growEncodeBuffer(nil, len(parts)*avatarInstanceBytes)
		encodeAvatarPartsInto(dst, parts)
		assertViewmodelCrosshairClear(t, dst)
	}
}

// TestViewmodelHeldItemSitsForwardOfHand 锁定持物装在手的前方（朝相机一侧）：
// 中立持握下持物中心在相机空间比右手中心更靠近相机（`z` 更大），刃面不被
// 手臂遮挡才有像素可读性；左右手臂本身不参与本断言。
func TestViewmodelHeldItemSitsForwardOfHand(t *testing.T) {
	for _, stack := range []core.ItemStack{
		{Item: core.ItemIronPickaxe, Count: 1, Durability: 125},
		{Item: core.ItemDirt, Count: 1},
	} {
		input := viewmodelTestInput(core.PlayerID{51}, stack, 10)
		out := (&ViewmodelEncoder{}).EncodeViewmodelInstances(nil, input)
		if len(out) != 3*avatarInstanceBytes {
			t.Fatalf("持物实例数 = %d，想要 3（左右手 + 持物）", len(out)/avatarInstanceBytes)
		}
		hand := decodedPartCenter(out, 1)
		held := decodedPartCenter(out, 2)
		// 零位姿下世界坐标即相机坐标（相机朝 `-Z`）：持物 `z` 必须显著更大。
		if held[2]-hand[2] < 0.05 {
			t.Fatalf("持物 %v 相对手臂前移 = %.3f，想要至少 0.05（装在手的前方）",
				stack.Item, held[2]-hand[2])
		}
	}
}

// TestViewmodelArmRootsOutsideScreenCorners 锁定臂根落在屏角之外：手臂底端
// 经投影必须在屏幕之外（下边缘之下），只留前臂入画。
func TestViewmodelArmRootsOutsideScreenCorners(t *testing.T) {
	input := viewmodelTestInput(core.PlayerID{51}, core.ItemStack{}, 10)
	out := (&ViewmodelEncoder{}).EncodeViewmodelInstances(nil, input)
	viewProj := viewmodelProjectionViewProj(mgl32.Vec3{}, 0, 0)
	for _, index := range []int{0, 1} {
		bottom := viewmodelInstanceCorner(out, index, 0, -1, 0)
		ndc, w := projectToNDC(viewProj, bottom)
		if w <= 0 {
			t.Fatalf("第 %d 只手臂根在相机后方（w=%.2f）", index, w)
		}
		if ndc[1] >= -1 {
			t.Fatalf("第 %d 只手臂根 NDC=(%.2f,%.2f)，想要落在屏底之外（y<-1）",
				index, ndc[0], ndc[1])
		}
	}
}
