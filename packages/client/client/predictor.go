package client

import (
	"errors"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/physics"
)

const (
	predictionHistoryCapacity = 256
	maxPredictionSteps        = 5
)

// Control 是渲染帧提供给固定步预测器的当前控制意图。
type Control struct {
	MoveX  int8
	MoveZ  int8
	Jump   bool
	Yaw    float32
	Pitch  float32
	Mining bool
	// Eating 是本帧是否请求进食。它由 app 按「手持食物 + 使用键按住」置位，
	// 逐固定步原样上行给服务端；客户端**不做任何本地预测**——既不扣食物也不
	// 改饥饿值，界面显示的永远是服务端确认回来的那个数。
	Eating bool
	// Sprinting 是本帧是否请求疾跑。门控见 sprint spec：地面+前移+非浸没+饥饿≥6 时
	// 才在物理侧提升目标速度，客户端只上行意图，服务端与预测侧各自按权威/镜像饥饿门控。
	Sprinting bool
}

// ReconcileResult 描述权威状态和解对视角的影响。
type ReconcileResult struct {
	ResetView bool
	Yaw       float32
	Pitch     float32
}

type predictedInput struct {
	sequence uint64
	input    physics.Input
}

// Predictor 使用与服务端共享的固定步物理维护本地玩家预测状态。
type Predictor struct {
	ready               bool
	dimension           core.DimensionID
	current, previous   physics.State
	accumulator         time.Duration
	history             []predictedInput
	lastServerTick      uint64
	maxSentInput        uint64
	suspended           bool
	suspendSequence     uint64
	suspendInputSent    bool
	displayOffset       mgl32.Vec3
	correctionRemaining time.Duration
	health              uint8
	oxygen              uint16
	hunger              uint8
	saturationZero      bool
	// weather 是最近确认的权威天气。它与生命值、饥饿值同为纯镜像值：
	// 只由服务端确认写入，客户端不预测、不随机、不按墙钟自选，呈现层
	// 经 Weather 查询，供降水/天空/亮度表现消费。
	weather core.WeatherKind
	// season/seasonProgress/temperature 是最近确认的权威季节三字段（协议
	// v37 起随玩家状态同步），与 weather 同一镜像纪律：只由服务端确认写入、
	// 仅前进 `ServerTick` 可改写，客户端绝不按本地世界时间外插——季节是
	// 「世界时间+seed 偏移」的服务端派生量，回退/重放场景下本地推算会与
	// 权威派生漂移，镜像值始终持有服务端算出的那一份。
	season         core.Season
	seasonProgress uint8
	// temperature 是服务端对玩家所在高度按共享公式求得的权威观察温度
	// （int8 摄氏度）。呈现层需要任意高度/粒子位置的局部温度时经共享公式
	// 对镜像输入另行求值，而不是改写这个单点观察值。
	temperature int8
	// eyeInFluid 是最近一次浸没判定给出的眼睛浸没标志。它由 stepWithSubmersion
	// 与权威状态和解共同写入，是水下视觉唯一的判定来源。见 EyeInFluid。
	eyeInFluid bool
}

// NewPredictor 创建具有固定历史容量的未就绪预测器。
func NewPredictor() *Predictor {
	return &Predictor{
		history: make([]predictedInput, 0, predictionHistoryCapacity),
	}
}

// Begin 从第一条有限且 Ready 的权威状态开始预测。
func (p *Predictor) Begin(message network.PlayerState) error {
	state := physics.State{
		Position: message.Position,
		Velocity: message.Velocity,
		OnGround: message.OnGround,
	}
	if !message.Ready {
		return errors.New("client: cannot begin prediction before player is ready")
	}
	if !physics.ValidState(state) || !finiteFloat32(message.Yaw) ||
		!finiteFloat32(message.Pitch) {
		return errors.New("client: cannot begin prediction from non-finite state")
	}
	if !core.ValidHealth(message.Health) {
		return errors.New("client: cannot begin prediction from invalid health")
	}
	if !core.ValidOxygen(message.Oxygen) {
		return errors.New("client: cannot begin prediction from invalid oxygen")
	}
	if !core.ValidHunger(message.Hunger) {
		return errors.New("client: cannot begin prediction from invalid hunger")
	}
	if message.WeatherKind > core.WeatherThunder {
		return errors.New("client: cannot begin prediction from invalid weather")
	}
	// 季节是四值枚举（协议 v37 起随状态同步）：合法值域与协议 `Validate`
	// 同为 0..冬，越界值与天气同形在首帧处拒绝，不进镜像。
	if message.Season > core.SeasonWinter {
		return errors.New("client: cannot begin prediction from invalid season")
	}

	p.ready = true
	p.dimension = message.Dimension
	p.current = state
	p.previous = state
	p.accumulator = 0
	p.history = p.history[:0]
	p.lastServerTick = message.ServerTick
	p.maxSentInput = message.LastInputSequence
	p.suspended = false
	p.suspendSequence = 0
	p.suspendInputSent = false
	p.displayOffset = mgl32.Vec3{}
	p.correctionRemaining = 0
	p.health = message.Health
	p.oxygen = message.Oxygen
	p.hunger = message.Hunger
	p.saturationZero = message.SaturationZero
	p.weather = message.WeatherKind
	p.season = message.Season
	p.seasonProgress = message.SeasonProgress
	p.temperature = message.Temperature
	// Begin 只有权威位置、没有方块视图，浸没标志留待第一次固定步或和解算出。
	p.eyeInFluid = false
	return nil
}

// State 返回当前预测物理状态以及预测器是否已就绪。
func (p *Predictor) State() (physics.State, bool) {
	return p.current, p.ready
}

// Health 返回只读镜像持有的权威生命值以及预测器是否已就绪。
// 生命值只接受服务端确认值，客户端不对其做任何预测。
func (p *Predictor) Health() (uint8, bool) {
	return p.health, p.ready
}

// Oxygen 返回只读镜像持有的权威氧气以及预测器是否已就绪。
// 同生命值：氧气是权威值，客户端只镜像不推算——本地即便算得出眼睛浸没标志，
// 也绝不据此自行增减氧气，否则界面会显示一个服务端并不认可的数值。
func (p *Predictor) Oxygen() (uint16, bool) {
	return p.oxygen, p.ready
}

// Hunger 返回只读镜像持有的权威饥饿值以及预测器是否已就绪。
// 同生命值与氧气：饥饿值只接受服务端确认值，客户端**不做任何预测**——
// 本地既不推进疲劳也不结算进食，界面显示的永远是服务端认可的那个数。
func (p *Predictor) Hunger() (uint8, bool) {
	return p.hunger, p.ready
}

// SaturationZero 返回饱和度归零提示位以及预测器是否已就绪。
// 同饥饿值：零提示位只接受服务端确认值，客户端不据本地饱和度推导。
func (p *Predictor) SaturationZero() (bool, bool) {
	return p.saturationZero, p.ready
}

// Weather 返回只读镜像持有的权威天气以及预测器是否已就绪。
// 同饥饿值：天气只接受服务端确认值，客户端不做任何本地随机或墙钟自选；
// 旧或重复 `ServerTick` 的状态由和解入口的去重门挡掉，不会回退已确认值。
func (p *Predictor) Weather() (core.WeatherKind, bool) {
	return p.weather, p.ready
}

// Season 返回只读镜像持有的权威季节以及预测器是否已就绪。
// 同天气：季节只接受服务端确认值，客户端不按本地世界时间外插；旧或重复
// `ServerTick` 的状态由和解入口的去重门挡掉，不会回退已确认季节。
func (p *Predictor) Season() (core.Season, bool) {
	return p.season, p.ready
}

// SeasonProgress 返回只读镜像持有的季内进度量化值（0..255）以及预测器
// 是否已就绪。接受纪律同 `Season`：仅前进 `ServerTick` 可改写。
func (p *Predictor) SeasonProgress() (uint8, bool) {
	return p.seasonProgress, p.ready
}

// Temperature 返回只读镜像持有的、玩家所在位置的权威观察温度（int8 摄氏
// 度）以及预测器是否已就绪。客户端不本地外插温度；呈现侧的任意高度局部
// 温度另经共享公式求值，本镜像只承载服务端对玩家单点的权威观察。
func (p *Predictor) Temperature() (int8, bool) {
	return p.temperature, p.ready
}

// EyeInFluid 报告最近一次浸没判定认为相机所在格是不是流体。
//
// 它返回的就是驱动水中物理、并与服务端氧气结算同源的那一个标志（两侧从各自的
// 方块镜像用同一个 physics.SubmersionFlags 算出，见任务组 5 的权威/预测一致性）。
// 水下视觉读它而不是另算一遍：规格要求这两处判定 MUST NOT 存在第二套。
//
// 预测器未就绪时恒为 false——没有权威状态就没有相机位置可谈。
func (p *Predictor) EyeInFluid() bool {
	return p.ready && p.eyeInFluid
}

// HistoryLen 返回尚未被权威状态确认的输入数量。
func (p *Predictor) HistoryLen() int {
	return len(p.history)
}

// Suspended 报告预测是否因历史达到容量而暂停。
func (p *Predictor) Suspended() bool {
	return p.suspended
}
