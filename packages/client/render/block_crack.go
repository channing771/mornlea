package render

import (
	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// blockCrackExpand 是裂纹 overlay 相对单位方块各面向外扩的世界单位数：
// 合成边长 1.002，与选框 0.003 的外扩同属防深度冲突（z-fight）的外扩族，
// 同时满足「外扩不超过 0.005」的呈现契约；Rust crack pass 以深度只读 +
// CompareLessEqual 配合此外扩完成叠加。
const blockCrackExpand float32 = 0.001

// blockCrackInstanceBytes 是裂纹实例的定长字节数，与 Rust 侧 crack pass 的
// 实例布局跨语言约定一致：0..63 是 mat4（列主序 little-endian f32 × 16），
// 64..68 是 atlas 层号 f32（`assets.LayerCrack0 + stage`），68..80 零填充。
// 裂纹是独立的单实例 overlay 契约：avatar 实例扩至 96 字节后此处不再与
// `avatarInstanceBytes` 同源，保持 80 字节不变。
const blockCrackInstanceBytes = 80

// BlockCrackStageNone 是 BlockCrackStage 的无效阶段哨兵：requiredTicks 为 0
// 或其他无法构成有效进度的输入时返回，呈现层据此隐藏裂纹而不是猜测阶段。
const BlockCrackStageNone = -1

// BlockCrackStage 把权威采集进度映射为 10 个离散裂纹阶段（0..9）：
// `min(9, floor(clamp(progressTicks/requiredTicks, 0, 1) × 10))`。阶段是
// 呈现概念，只由权威进度驱动（进度停滞时阶段逐帧稳定，绝不随帧数自增）。
//
// 浮点边界语义：除法与乘法都按 float32 求值，进度恰落在 1/10 的整数倍
// 边界时（如 3/30、9/30、15/30），f32 结果稳定落在精确整数值上，阶段按
// 「进入新阶段」取整——这是 `floor(p×10)` 在 IEEE-754 舍入下的自然行为，
// 十进制边界不得依赖超出 f32 精度的十进制定义。进度比例饱和到 1（含
// progress 超过 required 的钳制）时必须得到最重的第 9 阶段。
func BlockCrackStage(progressTicks, requiredTicks uint16) int {
	if requiredTicks == 0 {
		return BlockCrackStageNone
	}
	ratio := float32(progressTicks) / float32(requiredTicks)
	if ratio < 0 {
		ratio = 0
	} else if ratio > 1 {
		ratio = 1
	}
	return min(9, int(ratio*10))
}

// BlockCrack 是当前帧的采掘裂纹 overlay 输入：单一可复用实例（常驻容量
// 恰为 1），以 Visible 表达状态切换，不创建/销毁任何渲染对象。
type BlockCrack struct {
	Visible  bool
	Position core.BlockPos
	// Stage 是 BlockCrackStage 的产出；约定合法区间 0..9，越界按不可见处理。
	Stage int
}

// buildBlockCrackPart 构建覆盖目标方块完整包围盒的 overlay 变换：单位立方体
// 先平移到方块原点、再平移 0.5 到方块中心、最后各向外扩 blockCrackExpand，
// 六面都呈现同一阶段的裂纹。
func buildBlockCrackPart(position core.BlockPos) avatarPart {
	root := mgl32.Translate3D(float32(position.X), float32(position.Y), float32(position.Z))
	side := 1 + 2*blockCrackExpand
	return avatarCuboid(root, mgl32.Vec3{0.5, 0.5, 0.5}, mgl32.Vec3{side, side, side}, [4]float32{})
}

// valid 报告阶段是否可用于编码：只在 0..9 区间内与 atlas 层号一一对应。
func (crack BlockCrack) valid() bool {
	return crack.Visible && crack.Stage >= 0 && crack.Stage <= 9
}
