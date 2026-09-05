package render

import (
	"math"
	"slices"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// deathPartCenter 返回某部件变换后单位立方体的包围盒中心。
func deathPartCenter(part avatarPart) mgl32.Vec3 {
	bounds := transformedUnitCubeBounds(part.transform)
	return bounds.min.Add(bounds.max.Sub(bounds.min).Mul(0.5))
}

// TestPassiveDeathPhaseIsPureTickFunction 锁定死亡相位的确定性：红闪插值与
// 侧倒角度是 `(despawn tick, 牛 ID, 当前 tick)` 的纯函数，不读墙钟；同输入
// 两次重放逐帧相同，不同个体相位可辨。
func TestPassiveDeathPhaseIsPureTickFunction(t *testing.T) {
	first := make([][2]float32, 0, 21)
	second := make([][2]float32, 0, 21)
	for now := uint64(100); now <= 120; now++ {
		roll, flash := PassiveDeathPhase(100, 7, now)
		first = append(first, [2]float32{roll, flash})
		roll, flash = PassiveDeathPhase(100, 7, now)
		second = append(second, [2]float32{roll, flash})
	}
	if !slices.Equal(first, second) {
		t.Fatal("同 tick 序列两次重放的死亡相位不一致，想要逐帧相同")
	}
	if roll, flash := PassiveDeathPhase(100, 7, 100); roll != 0 || flash != 0 {
		t.Fatalf("死亡当 tick 相位=(%v,%v)，想要零值（与变更前逐字节一致）", roll, flash)
	}
	if roll, _ := PassiveDeathPhase(100, 7, 120); roll != float32(math.Pi/2) {
		t.Fatalf("保留末 tick 侧倒=%v，想要 90°（π/2）", roll)
	}
	if roll, _ := PassiveDeathPhase(100, 7, 130); roll != float32(math.Pi/2) {
		t.Fatalf("超保留期侧倒=%v，想要钳制在 90°", roll)
	}
	same := true
	for now := uint64(100); now <= 120; now++ {
		leftRoll, leftFlash := PassiveDeathPhase(100, 7, now)
		rightRoll, rightFlash := PassiveDeathPhase(100, 9, now)
		if leftRoll != rightRoll || leftFlash != rightFlash {
			same = false
			break
		}
	}
	if same {
		t.Fatal("同 tick 死亡的两头牛相位恒等，想要 ID 参与派生、避免整齐划一")
	}
}

// TestPassiveDyingAvatarTipsSidewaysAndReddens 锁定死亡呈现的几何与颜色：
// 保留末 tick 的身体相对活体侧倒（躯干中心侧移且下沉）、颜色向红插值，材质
// 层（牛皮/牛头）不变。
func TestPassiveDyingAvatarTipsSidewaysAndReddens(t *testing.T) {
	position := mgl32.Vec3{1, 2, 3}
	live := buildAvatarParts(nil, []Avatar{{Key: PassiveEntityKey(7), Position: position, Yaw: 0.5}})
	roll, flash := PassiveDeathPhase(100, 7, 120)
	if flash <= 0 {
		t.Fatalf("保留末 tick 红闪=%v，想要正值", flash)
	}
	dying := buildAvatarParts(nil, []Avatar{{Key: PassiveEntityKey(7), Position: position, Yaw: 0.5, Roll: roll, Flash: flash}})
	if len(dying) != len(live) {
		t.Fatalf("死亡部件=%d，想要与活体相同的 %d", len(dying), len(live))
	}
	for index := range dying {
		if dying[index].material != live[index].material {
			t.Fatalf("部件 %d 材质=%d，想要与活体相同的 %d", index, dying[index].material, live[index].material)
		}
		if dying[index].material == avatarMaterialSolid {
			t.Fatalf("部件 %d 走纯色分支，想要保持牛皮/牛头贴图采样", index)
		}
		got, want := dying[index].color, live[index].color
		if !(got[0] >= want[0] && got[1] < want[1] && got[2] < want[2]) {
			t.Fatalf("部件 %d 颜色=%v，想要相对活体 %v 向红插值", index, got, want)
		}
	}
	// 躯干是部件下标 1（下标 0 为牛头）：侧倒绕面朝轴（X）滚转，躯干中心应
	// 侧移且下沉。
	liveCenter, dyingCenter := deathPartCenter(live[1]), deathPartCenter(dying[1])
	lateral := math.Abs(float64(dyingCenter.X()-liveCenter.X())) + math.Abs(float64(dyingCenter.Z()-liveCenter.Z()))
	if lateral < 0.2 {
		t.Fatalf("死亡躯干中心=%v，想要相对活体 %v 明显侧移", dyingCenter, liveCenter)
	}
	if dyingCenter.Y() >= liveCenter.Y() {
		t.Fatalf("死亡躯干中心高度=%v，想要低于活体 %v（侧倒下沉）", dyingCenter.Y(), liveCenter.Y())
	}
}

// TestPassiveAvatarWithoutDeathKeepsGrazingPose 锁定零值分支：非死亡身体
// （`Roll`/`Flash` 零值）的头部俯仰仍由放牧位独占，死亡通道不干扰吃草位姿。
func TestPassiveAvatarWithoutDeathKeepsGrazingPose(t *testing.T) {
	position := mgl32.Vec3{1, 2, 3}
	standing := buildAvatarParts(nil, []Avatar{{Key: PassiveEntityKey(7), Position: position}})
	grazing := buildAvatarParts(nil, []Avatar{{Key: PassiveEntityKey(7), Position: position, Pitch: PassiveGrazeHeadPitch(true)}})
	if got, want := deathPartCenter(grazing[1]), deathPartCenter(standing[1]); !got.ApproxEqualThreshold(want, 1e-4) {
		t.Fatalf("放牧躯干中心=%v，想要与常态一致的 %v", got, want)
	}
	// 牛头俯仰是绕头心的自转（中心不动、面朝下压），按既有先例查面朝方向。
	standingFacing := transformedDirection(standing[0].transform, mgl32.Vec3{1, 0, 0})
	grazingFacing := transformedDirection(grazing[0].transform, mgl32.Vec3{1, 0, 0})
	if standingFacing.ApproxEqualThreshold(grazingFacing, 1e-4) {
		t.Fatalf("放牧头面朝=%v，想要相对常态 %v 下压", grazingFacing, standingFacing)
	}
}
