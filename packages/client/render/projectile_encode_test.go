//go:build darwin

package render

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// projectileAxisOf 解码实例 mat4 的第 column 列（列主序，每列 4 个 f32，
// 前三行）。
func projectileAxisOf(stream []byte, column int) mgl32.Vec3 {
	offset := column * 16
	value := mgl32.Vec3{}
	for axis := 0; axis < 3; axis++ {
		bits := uint32(stream[offset+axis*4]) | uint32(stream[offset+axis*4+1])<<8 |
			uint32(stream[offset+axis*4+2])<<16 | uint32(stream[offset+axis*4+3])<<24
		value[axis] = math.Float32frombits(bits)
	}
	return value
}

// projectileColorAt 解码实例 64..80 字节的 RGBA 颜色。
func projectileColorAt(stream []byte) [4]float32 {
	var color [4]float32
	for channel := 0; channel < 4; channel++ {
		bits := uint32(stream[64+channel*4]) | uint32(stream[65+channel*4])<<8 |
			uint32(stream[66+channel*4])<<16 | uint32(stream[67+channel*4])<<24
		color[channel] = math.Float32frombits(bits)
	}
	return color
}

// projectileMaterialBits 解码实例 80..84 字节的材质槽（原始位型：纯色哨兵
// 是 ^uint32(0)，按 f32 位型读是 NaN，必须按位比较）。
func projectileMaterialBits(stream []byte) uint32 {
	return uint32(stream[80]) | uint32(stream[81])<<8 | uint32(stream[82])<<16 | uint32(stream[83])<<24
}

// TestProjectileEncoderUsesAvatarLayoutWithSentinelMaterial 锁定投射物实例
// 布局：96 字节/实例与 avatar 逐字节同布局，材质槽是纯色哨兵，颜色按弹种
// 取固定调色（骨刺骨白、箭深棕），垫充区显式写零。
func TestProjectileEncoderUsesAvatarLayoutWithSentinelMaterial(t *testing.T) {
	encoder := &InstanceEncoder{}
	stream := encoder.EncodeProjectileInstances(nil, []Projectile{{
		Kind: ProjectileKindShard, Position: mgl32.Vec3{3, 4, 5}, Velocity: mgl32.Vec3{0, 0, -22},
	}})
	if got := len(stream); got != avatarInstanceBytes {
		t.Fatalf("实例流=%d 字节，想要单实例 %d 字节", got, avatarInstanceBytes)
	}
	if got := projectileMaterialBits(stream); got != avatarMaterialSolid {
		t.Fatalf("材质=%d，想要纯色哨兵 %d", got, avatarMaterialSolid)
	}
	if got := projectileColorAt(stream); got != projectileShardColor {
		t.Fatalf("骨刺颜色=%v，想要 %v", got, projectileShardColor)
	}
	for index := 84; index < avatarInstanceBytes; index++ {
		if stream[index] != 0 {
			t.Fatalf("垫充区字节 %d 非零", index)
		}
	}
	// 箭实例取深棕箭杆调色。
	arrow := encoder.EncodeProjectileInstances(nil, []Projectile{{
		Kind: ProjectileKindArrow, Position: mgl32.Vec3{}, Velocity: mgl32.Vec3{30, 0, 0},
	}})
	if got := projectileColorAt(arrow); got != projectileArrowColor {
		t.Fatalf("箭颜色=%v，想要 %v", got, projectileArrowColor)
	}
	// 骨白与深棕两类调色必须可辨：亮度差大于阈值。
	bright := projectileShardColor
	dark := projectileArrowColor
	if bright[0]-dark[0] < 0.3 {
		t.Fatalf("骨刺 %v 与箭 %v 的调色不可辨", projectileShardColor, projectileArrowColor)
	}
}

// TestProjectileEncoderOrientsLongAxisAlongVelocity 锁定取向纪律：实例长轴
// （局部 +Y）指向速度方向，基右手正交；零速度是确定性回退（单位基，不引
// 用上一帧状态）。
func TestProjectileEncoderOrientsLongAxisAlongVelocity(t *testing.T) {
	encoder := &InstanceEncoder{}
	stream := encoder.EncodeProjectileInstances(nil, []Projectile{{
		Kind: ProjectileKindArrow, Position: mgl32.Vec3{1, 2, 3}, Velocity: mgl32.Vec3{0, 0, -30},
	}})
	localY := projectileAxisOf(stream, 1).Normalize()
	localX := projectileAxisOf(stream, 0)
	localZ := projectileAxisOf(stream, 2)
	if localY != (mgl32.Vec3{0, 0, -1}) {
		t.Fatalf("局部 +Y=%v，想要速度方向 (0,0,-1)", localY)
	}
	if math.Abs(float64(localX.Dot(localY))) > 1e-5 || math.Abs(float64(localY.Dot(localZ))) > 1e-5 {
		t.Fatalf("基不正交：X=%v Y=%v Z=%v", localX, localY, localZ)
	}
	if localX.Cross(localY).Dot(localZ) < 0 {
		t.Fatalf("基非右手：X=%v Y=%v Z=%v", localX, localY, localZ)
	}
	// 平移列携带插值后位置（缩放只作用在方向列上）。
	if got := projectileAxisOf(stream, 3); got != (mgl32.Vec3{1, 2, 3}) {
		t.Fatalf("平移列=%v，想要位置 (1,2,3)", got)
	}

	// 垂直向上的速度：近平行参考轴时换轴，仍产出正交右手基。
	vertical := encoder.EncodeProjectileInstances(nil, []Projectile{{
		Kind: ProjectileKindShard, Position: mgl32.Vec3{}, Velocity: mgl32.Vec3{0, 22, 0},
	}})
	if got := projectileAxisOf(vertical, 1).Normalize(); got != (mgl32.Vec3{0, 1, 0}) {
		t.Fatalf("垂直发射的局部 +Y=%v，想要 (0,1,0)", got)
	}
	if axisX, axisZ := projectileAxisOf(vertical, 0), projectileAxisOf(vertical, 2); axisX.Dot(axisZ) != 0 {
		t.Fatalf("垂直发射的基不正交：X=%v Z=%v", axisX, axisZ)
	}

	// 零速度：确定性回退为单位基（与位置平移复合），不引用上一帧。
	degenerate := encoder.EncodeProjectileInstances(nil, []Projectile{{
		Kind: ProjectileKindArrow, Position: mgl32.Vec3{7, 8, 9}, Velocity: mgl32.Vec3{},
	}})
	if got := projectileAxisOf(degenerate, 1).Normalize(); got != (mgl32.Vec3{0, 1, 0}) {
		t.Fatalf("零速度回退的局部 +Y=%v，想要单位基 (0,1,0)", got)
	}
	if got := projectileAxisOf(degenerate, 0).Normalize(); got != (mgl32.Vec3{1, 0, 0}) {
		t.Fatalf("零速度回退的局部 +X=%v，想要 (1,0,0)", got)
	}
	if got := projectileAxisOf(degenerate, 3); got != (mgl32.Vec3{7, 8, 9}) {
		t.Fatalf("零速度回退的平移列=%v，想要位置 (7,8,9)", got)
	}
}

// TestProjectileEncoderCapsSegmentBudgetAtOneHundredTwentyEight 锁定段预算：
// 每帧至多 128 实例（镜像容量同源），超出部分按输入顺序丢弃尾部，帧内
// 字节恒不超过 128×96。
func TestProjectileEncoderCapsSegmentBudgetAtOneHundredTwentyEight(t *testing.T) {
	if MaxProjectileInstances != 128 {
		t.Fatalf("MaxProjectileInstances=%d，想要 128", MaxProjectileInstances)
	}
	encoder := &InstanceEncoder{}
	projectiles := make([]Projectile, MaxProjectileInstances+3)
	for index := range projectiles {
		projectiles[index] = Projectile{
			Kind:     ProjectileKindShard,
			Position: mgl32.Vec3{float32(index), 0, 0},
			Velocity: mgl32.Vec3{0, 0, -1},
		}
	}
	stream := encoder.EncodeProjectileInstances(nil, projectiles)
	if got, want := len(stream), MaxProjectileInstances*avatarInstanceBytes; got != want {
		t.Fatalf("段长=%d 字节，想要 %d", got, want)
	}
	// 保留的是输入顺序的前 128 枚：首实例位置 x=0、末实例位置 x=127。
	if first := projectileAxisOf(stream[:96], 3); first.X() != 0 {
		t.Fatalf("保留首枚位置 x=%v，想要 0", first.X())
	}
	tail := stream[(MaxProjectileInstances-1)*96:]
	if last := projectileAxisOf(tail, 3); last.X() != float32(MaxProjectileInstances-1) {
		t.Fatalf("保留末枚位置 x=%v，想要 %d", last.X(), MaxProjectileInstances-1)
	}

	// 空输入返回空流（本帧无投射物）。
	if got := encoder.EncodeProjectileInstances(nil, nil); len(got) != 0 {
		t.Fatalf("空输入产出 %d 字节，想要空流", len(got))
	}
}
