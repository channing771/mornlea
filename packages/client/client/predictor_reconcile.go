package client

import (
	"errors"
	"math"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// ApplyPlayerState 应用更新的权威玩家状态并重放尚未确认的输入。
func (p *Predictor) ApplyPlayerState(
	message network.PlayerState,
	source physics.WorldSource,
) (ReconcileResult, error) {
	if message.ServerTick <= p.lastServerTick {
		return ReconcileResult{}, nil
	}
	authority, err := validatePlayerState(message, p.maxSentInput)
	if err != nil {
		return ReconcileResult{}, err
	}

	if !message.Ready {
		p.clearForNotReady(message)
		return ReconcileResult{}, nil
	}
	if !p.ready || message.Reset || message.Dimension != p.dimension {
		if err := p.Begin(message); err != nil {
			return ReconcileResult{}, err
		}
		return ReconcileResult{
			ResetView: true,
			Yaw:       message.Yaw,
			Pitch:     message.Pitch,
		}, nil
	}
	if p.suspended && (!p.suspendInputSent || message.LastInputSequence < p.suspendSequence) {
		p.lastServerTick = message.ServerTick
		return ReconcileResult{}, nil
	}

	oldDisplayed := p.presentationPositionNoAdvance()
	oldPredicted := p.current.Position
	oldCorrectionRemaining := p.correctionRemaining
	p.current = authority
	p.previous = authority
	// 权威位置到手后立刻按它重算一次浸没标志：历史为空（刚同步、或挂起后清空）
	// 时下面的重放循环一步都不会跑，水下视觉不能因此停在上一次预测位置的旧标志上。
	_, p.eyeInFluid = physics.SubmersionFlags(authority.Position, source)
	p.health = message.Health
	p.oxygen = message.Oxygen
	p.hunger = message.Hunger
	p.saturationZero = message.SaturationZero
	p.weather = message.WeatherKind
	p.season = message.Season
	p.seasonProgress = message.SeasonProgress
	p.temperature = message.Temperature
	if p.suspended {
		p.history = p.history[:0]
		p.accumulator = 0
		p.suspended = false
		p.suspendSequence = 0
		p.suspendInputSent = false
	} else {
		p.dropAcknowledged(message.LastInputSequence)
		for _, entry := range p.history {
			p.previous = p.current
			p.current = p.stepWithSubmersion(p.current, entry.input, source)
		}
	}
	p.lastServerTick = message.ServerTick

	errorDistance := p.current.Position.Sub(oldPredicted).Len()
	switch {
	case errorDistance >= 0.5:
		p.displayOffset = mgl32.Vec3{}
		p.correctionRemaining = 0
	case errorDistance >= 1.0/128:
		p.displayOffset = oldDisplayed.Sub(p.interpolatedPosition())
		p.correctionRemaining = 100 * time.Millisecond
	default:
		p.displayOffset = oldDisplayed.Sub(p.interpolatedPosition())
		if p.displayOffset == (mgl32.Vec3{}) {
			p.correctionRemaining = 0
			break
		}
		remainingStep := max(time.Duration(0), physics.FixedDelta-p.accumulator)
		p.correctionRemaining = max(oldCorrectionRemaining, remainingStep)
		if p.correctionRemaining <= 0 {
			p.displayOffset = mgl32.Vec3{}
		}
	}
	return ReconcileResult{}, nil
}

func (p *Predictor) clearForNotReady(message network.PlayerState) {
	p.ready = false
	p.dimension = message.Dimension
	p.current = physics.State{}
	p.previous = physics.State{}
	p.accumulator = 0
	p.history = p.history[:0]
	p.lastServerTick = message.ServerTick
	p.maxSentInput = message.LastInputSequence
	p.suspended = false
	p.suspendSequence = 0
	p.suspendInputSent = false
	p.displayOffset = mgl32.Vec3{}
	p.correctionRemaining = 0
	p.health = 0
	// 与生命值同法清零：Ready 之间的会话不共享镜像值，留着旧饥饿值会让下一次
	// 就绪前的一帧显示上一条会话的读数。
	p.hunger = 0
	p.saturationZero = false
	// 天气与饥饿值同为会话级镜像值：回到默认晴天，不把上一会话的雨/雷暴
	// 漏到下一会话就绪前的呈现里（未就绪时查询门本就关闭，这里只清存储）。
	p.weather = core.WeatherClear
	// 季节三字段同理清回默认春始零进度/0℃：新会话的权威季节只等第一条
	// 就绪状态带来，不留上一会话的季节相位。
	p.season = core.SeasonSpring
	p.seasonProgress = 0
	p.temperature = 0
}

func validatePlayerState(message network.PlayerState, maxSentInput uint64) (physics.State, error) {
	state := physics.State{
		Position: message.Position,
		Velocity: message.Velocity,
		OnGround: message.OnGround,
	}
	if message.Dimension != core.Overworld {
		return physics.State{}, errors.New("client: player state has unknown dimension")
	}
	if !physics.ValidState(state) || !finiteFloat32(message.Yaw) ||
		!finiteFloat32(message.Pitch) {
		return physics.State{}, errors.New("client: player state contains non-finite value")
	}
	if !core.ValidHealth(message.Health) {
		return physics.State{}, errors.New("client: player state has out-of-range health")
	}
	if !core.ValidOxygen(message.Oxygen) {
		return physics.State{}, errors.New("client: player state has out-of-range oxygen")
	}
	if !core.ValidHunger(message.Hunger) {
		return physics.State{}, errors.New("client: player state has out-of-range hunger")
	}
	// 天气是服务端权威三态：合法值域与协议 `Validate` 同为 0..雷暴，
	// 越界值在这里与首帧处都被拒绝，不进镜像。
	if message.WeatherKind > core.WeatherThunder {
		return physics.State{}, errors.New("client: player state has out-of-range weather")
	}
	// 季节是四值枚举：合法值域与协议 `Validate` 同为 0..冬，协议编解码层
	// 已拒一遍，这里照天气形态做镜像侧的纵深拒绝。进度（u8）与温度（i8）
	// 全域合法，不设子域拒绝。
	if message.Season > core.SeasonWinter {
		return physics.State{}, errors.New("client: player state has out-of-range season")
	}
	const maxPitch = float32(math.Pi/2 - 0.01)
	if message.Pitch < -maxPitch || message.Pitch > maxPitch {
		return physics.State{}, errors.New("client: player state has invalid pitch")
	}
	if message.LastInputSequence > maxSentInput {
		return physics.State{}, errors.New("client: player state acknowledges unsent input")
	}
	if message.Reset && !message.Ready {
		return physics.State{}, errors.New("client: reset player state is not ready")
	}
	return state, nil
}
