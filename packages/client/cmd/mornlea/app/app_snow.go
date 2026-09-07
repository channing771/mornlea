//go:build darwin

package app

// app_snow.go：踩雪音效与踢雪尘的本地派生（spec「踩雪音效与踢雪粒子本地
// 呈现」）。两条呈现都以本地预测状态与只读世界镜像为唯一输入，不新增协议、
// 不回写权威；触发只针对本地玩家（第一人称听感/脚部粒子），其他实体不做。

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/audio"
	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
)

const (
	// snowStepStride 是踩雪音效的位移门控（design 总表：水平位移每累计
	// ≥0.8 格播放一次）：限频即位移门控本身，离开雪面/静止自然静默。取值
	// 大于权威脚印步长 0.6——脚印逐格削雪、音效按步频出声，两者不必锁步。
	snowStepStride = float32(0.8)
	// snowStepTeleportBlocks 是位移累计的跳变重锚阈值：跨过视为传送/和解，
	// 不累计也不出声（与编码器摆动跟踪的传送门限同一量级）。
	snowStepTeleportBlocks = float32(8)
)

// snowStepFeedback 是本地玩家的踩雪步频累计器：内部基线持有上次观测的脚位，
// 水平位移逐帧累计，跨过 `snowStepStride` 时采样落足格——为雪层才出声。全部
// 字段只在帧循环内读写，不落盘、不进协议；会话重置与权威 reset 经 `Reset`
// 清零基线。
type snowStepFeedback struct {
	hasPos bool
	pos    mgl32.Vec3
	travel float32
}

// Reset 丢弃会话、重生或权威重置前的位置基线与累计里程。
func (feedback *snowStepFeedback) Reset() {
	*feedback = snowStepFeedback{}
}

// ObserveStride 观测一帧本地脚位并判定是否播放踩雪音。边界语义沿权威脚印
// 判定（snow_footprint.go）同形：
//   - 不在地面：既不累计也不清零——空中位移不是「落足移动」，已累计里程
//     保留到落地后继续；
//   - 跳变超过传送门限：只重锚基线，不累计不出声；
//   - 跨过步长：清零累计并采样落足格，落足格为雪层且方块视图就绪才返回
//     true——缺块宁可漏响也不凭空出声（与 `MirrorCollisionSource.BlockIDAt`
//     的回退契约同向）；非雪面同样清零累计，跨雪/干地边界不会积攒出迟响。
func (feedback *snowStepFeedback) ObserveStride(
	position mgl32.Vec3,
	onGround bool,
	footBlock core.BlockID,
	footLoaded bool,
) bool {
	if !feedback.hasPos {
		feedback.hasPos = true
		feedback.pos = position
		return false
	}
	dx := position.X() - feedback.pos.X()
	dz := position.Z() - feedback.pos.Z()
	feedback.pos = position
	horizontal := float32(math.Sqrt(float64(dx*dx + dz*dz)))
	if horizontal > snowStepTeleportBlocks {
		return false
	}
	if !onGround {
		return false
	}
	feedback.travel += horizontal
	if feedback.travel < snowStepStride {
		return false
	}
	feedback.travel = 0
	return footLoaded && core.IsSnowLayer(footBlock)
}

// deriveSnowKick 从一帧本地预测前后状态派生踢雪尘驱动输入：疾跑进行中
// （疾跑意图 + 前移 + 落足，与 `physics.Step` 的疾跑门控同三条，浸没位预测器
// 侧不可见、由「雪层与水体不共生」的生成条件兜底）或落地边沿（上一帧不在
// 地面、本帧在地面，与权威踩踏收集同一次判定），且落足格为雪层才有事件；
// 其余情形（含晴天干地、普通行走、视图未就绪）恒零值输入，装配侧零值不重锚，
// 上一事件的扬尘按寿命自然老化。
func deriveSnowKick(
	before, after physics.State,
	sprintIntent bool,
	moveZ int8,
	footBlock core.BlockID,
	footLoaded bool,
) render.SnowKickInput {
	landed := !before.OnGround && after.OnGround
	sprinting := sprintIntent && moveZ > 0 && after.OnGround
	if !landed && !sprinting {
		return render.SnowKickInput{}
	}
	if !footLoaded || !core.IsSnowLayer(footBlock) {
		return render.SnowKickInput{}
	}
	return render.SnowKickInput{Kick: true, Feet: after.Position}
}

// feetBlockPos 把脚部世界坐标量化为落足格：floor 不减 `physics.GroundProbe`，
// 与权威脚印判定同一几何——雪层零碰撞，实体由下方承载方块支撑、脚部恰在
// 雪层格内。
func feetBlockPos(position mgl32.Vec3) core.BlockPos {
	return core.BlockPos{
		X: int32(math.Floor(float64(position.X()))),
		Y: int32(math.Floor(float64(position.Y()))),
		Z: int32(math.Floor(float64(position.Z()))),
	}
}

// observeSnowFeedback 在本地预测推进后观测踩雪呈现：读一次落足格（步频判定
// 与踢雪门控共用同一次方块查询），步频跨过门控且落足格为雪层时播放
// `CueSnowStep`，并把踢雪驱动输入写进 `frameSnowKick` 供 `RenderFrame` 的
// 降水实例流装配消费。全景/无头路径不经此处，`frameSnowKick` 保持零值。
func (a *Application) observeSnowFeedback(before, after physics.State, control client.Control) {
	source := client.MirrorCollisionSource{Mirror: a.mirror, Dimension: core.Overworld}
	footBlock, loaded := source.BlockIDAt(feetBlockPos(after.Position))
	if a.snowStep.ObserveStride(after.Position, after.OnGround, footBlock, loaded) {
		a.playLocalCue(audio.CueSnowStep)
	}
	a.frameSnowKick = deriveSnowKick(before, after, control.Sprinting, control.MoveZ, footBlock, loaded)
}
