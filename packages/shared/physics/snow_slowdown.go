package physics

import (
	"github.com/channing771/mornlea/packages/shared/core"
)

const (
	// snowLayerSlowdownMinTier 是触发厚雪减速的最低雪层档位。spec「厚雪减速且
	// 预测一致」钉死 ≥3 档减速、≤2 档恒等：3、4 档的呈现高度（4/16、5/16）
	// 已没过脚踝，值得付出步频代价；1、2 档只是薄霜。
	snowLayerSlowdownMinTier = 3

	// snowLayerSlowdownFactor 是厚雪上水平移动目标速度的固定因子 0.7。spec 把
	// 数值写死为可观察契约：减速是确定性的同表判定，权威与预测两侧读的是同
	// 一个常量，任何调参都会制造橡皮筋回拉。
	snowLayerSlowdownFactor = float32(0.7)
)

// FootBlockSource 查询世界方块的原始 ID，供落足格采样使用。
//
// 它与 CollisionSource、FluidSource 刻意分开：CollisionSource 只交付碰撞盒，
// 而雪层全档零碰撞、与空气同形状，碰撞视图里拿不到任何可判定的信息；减速
// 需要的是「这一格是什么方块」。三个接口各自只暴露自己需要的那一份方块
// 视图，服务端 dimensionCollisionSource 与客户端 MirrorCollisionSource 同时
// 实现全部三者。只实现 CollisionSource 的来源（如各测试的盒世界）不参与
// 减速——见 BlockIDAt 的回退契约。
type FootBlockSource interface {
	// BlockIDAt 返回该格当前的方块 ID。第二个返回值报告该格的方块视图是否
	// 可用：未加载、已失同步的区块 MUST 返回 (0, false)——厚雪减速宁可漏判
	// 也不能凭空捏造，否则客户端会在区块尚未到达时预测出一段服务端不存在
	// 的慢速物理。超出世界高度的格 MUST 返回 (core.AirID, true)。
	BlockIDAt(core.BlockPos) (core.BlockID, bool)
}

// applySnowLayerSlowdown 在积分前按落足格采样厚雪，压低本固定步的水平目标
// 速度。
//
// 权威模拟与客户端预测都经 physics.Step/StepWithTunables 进入这里，减速判定
// 因此同表自动一致——两侧唯一允许不同的是 FootBlockSource 背后的方块镜像，
// 与 physics.SubmersionFlags 的纪律相同；任何一侧私自实现第二套判定都会制造
// 持续纠偏。落足格取「脚部所在格」：floor 不减 GroundProbe，与脚印判定同一
// 几何语义——雪层零碰撞，实体由下方承载方块支撑、脚部恰在雪层格内。
//
// 实现落在 Go 侧目标速度层：缩放的是本步的 WalkSpeed 快照，Rust 积分从
// walk_speed 头字段派生 movement_target（疾跑倍率乘在其后），因此非疾跑与
// 疾跑目标同步 ×0.7，sweep bounds、ABI 编码与 Rust 积分读的是同一个缩放值，
// StepInput 布局与 engine ABI 均不变。门控三条：不在地面（空中位移不是落足
// 移动）、无水平移动意图（静止不查询）、落足格非 ≥3 档雪层——任一不满足
// 时恒等返回，来源不实现 FootBlockSource 或方块视图未就绪同样安全回退为
// 不减速。采样是单格常数工作量，无遍历、无分配。
func applySnowLayerSlowdown(
	state State,
	input Input,
	source CollisionSource,
	tunables Tunables,
) Tunables {
	if !state.OnGround || (input.MoveX == 0 && input.MoveZ == 0) {
		return tunables
	}
	sampler, ok := source.(FootBlockSource)
	if !ok {
		return tunables
	}
	block, loaded := sampler.BlockIDAt(core.BlockPos{
		X: collisionCheckedFloor(state.Position.X()),
		Y: collisionCheckedFloor(state.Position.Y()),
		Z: collisionCheckedFloor(state.Position.Z()),
	})
	if !loaded {
		return tunables
	}
	tier, isLayer := core.SnowLayerTier(block)
	if !isLayer || tier < snowLayerSlowdownMinTier {
		return tunables
	}
	tunables.WalkSpeed *= snowLayerSlowdownFactor
	return tunables
}
