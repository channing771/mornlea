//go:build darwin

package render

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

// 弹种在呈现侧的镜像域与固定调色。kind 数值与 wire 侧 kind 字节同值映射
// （0=骨刺、1=箭，协议层是数值的权威定义点）；呈现侧只消费值域分支配色
// 与形体，新增弹种必须先扩协议再同步此处。
const (
	// ProjectileKindShard 表示骨刺（远程敌怪发射）：小号骨白棱刺。
	ProjectileKindShard uint8 = 0
	// ProjectileKindArrow 表示箭（玩家弓发射）：深棕细长箭杆。
	ProjectileKindArrow uint8 = 1
	// MaxProjectileInstances 是投射物段的单帧实例上限，与客户端镜像容量
	// （128）及 Rust 侧 projectile pass 的常驻容量同源；超出部分按输入顺
	// 序丢弃尾部，帧内字节恒不超过 128×96。
	MaxProjectileInstances = 128
)

// projectileShardColor 是骨刺的固定调色：骨白（pale bone，≈ #DED4B8）——
// 高亮度、暖色偏黄的低饱和白，与夜行者的暗青、掷骨者的躯干同族可辨又更
// 亮，飞行中的小目标在多数背景下可读。原创数值，不取自任何第三方资源。
var projectileShardColor = [4]float32{0.87, 0.83, 0.72, 1}

// projectileArrowColor 是箭的固定调色：深棕（dark wood shaft，≈ #5C3D21）
// ——原木箭杆的暗棕，与骨白骨刺一眼可辨，与掉落木制品同族但不共用槽位。
var projectileArrowColor = [4]float32{0.36, 0.24, 0.13, 1}

// 投射物形体尺寸（格）：单 cuboid 实例，长轴沿局部 +Y（速度方向）。
// 骨刺短而略厚，箭更长更细；尺寸是呈现侧固定契约，不进任何线上协议。
var (
	projectileShardSize = mgl32.Vec3{0.09, 0.34, 0.09}
	projectileArrowSize = mgl32.Vec3{0.06, 0.52, 0.06}
)

// projectileBasisFromVelocity 由速度方向构造正交右手基：局部 +Y（长轴）
// 指向速度方向。参考轴取世界 +Y，速度与参考轴接近平行（|dot| > 0.9）时
// 换用 +X，保证任意方向都有稳定的横向参考。退化输入（零向量或非有限分
// 量）取单位基——确定性回退，不引用上一帧状态。
func projectileBasisFromVelocity(velocity mgl32.Vec3) mgl32.Mat4 {
	degenerate := !finiteVec3(velocity) || velocity.LenSqr() < 1e-12
	if degenerate {
		return mgl32.Ident4()
	}
	forward := velocity.Normalize()
	reference := mgl32.Vec3{0, 1, 0}
	if float32(math.Abs(float64(forward.Dot(reference)))) > 0.9 {
		reference = mgl32.Vec3{1, 0, 0}
	}
	side := reference.Cross(forward).Normalize()
	localZ := side.Cross(forward)
	// 列主序：列 0 是局部 +X、列 1 是长轴 +Y（速度方向）、列 2 是 +Z。
	return mgl32.Mat4{
		side[0], side[1], side[2], 0,
		forward[0], forward[1], forward[2], 0,
		localZ[0], localZ[1], localZ[2], 0,
		0, 0, 0, 1,
	}
}

// finiteVec3 报告三分量是否全部有限（NaN/Inf 视为非法取向输入）。
func finiteVec3(value mgl32.Vec3) bool {
	for _, component := range value {
		if math.IsNaN(float64(component)) || math.IsInf(float64(component), 0) {
			return false
		}
	}
	return true
}

// Projectile 是一枚投射物的插值后呈现输入：位置与速度均来自客户端镜像
// （速度为权威差分估计），编码器只做取向与配色，不预测弹道。
type Projectile struct {
	Kind     uint8
	Position mgl32.Vec3
	Velocity mgl32.Vec3
}

// buildProjectileParts 追加一枚投射物的单 cuboid 部件：变换 = 位置平移 ·
// 速度取向基 · 形体缩放（列主序左乘，平移列不被基旋转）；材质走纯色哨
// 兵，颜色按弹种取固定调色。
func buildProjectileParts(dst []avatarPart, projectile Projectile) []avatarPart {
	basis := projectileBasisFromVelocity(projectile.Velocity)
	size := projectileShardSize
	color := projectileShardColor
	if projectile.Kind == ProjectileKindArrow {
		size, color = projectileArrowSize, projectileArrowColor
	}
	root := mgl32.Translate3D(
		projectile.Position[0], projectile.Position[1], projectile.Position[2],
	).Mul4(basis)
	return append(dst, avatarPart{
		transform: root.Mul4(mgl32.Scale3D(size[0], size[1], size[2])),
		color:     color,
		material:  avatarMaterialSolid,
	})
}
