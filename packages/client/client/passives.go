package client

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

var ErrPassiveProtocol = errors.New("passive protocol error")

// MaxPassives 是客户端被动牛镜像的固定容量，与服务端全服上限（权威侧 sim
// 的 32 头上限同源契约）一致；超出容量的 spawn 按稳定规则忽略，不驱逐既
// 有身体。
const MaxPassives = 32

// PassivePresentation 是一头被动牛的只读呈现值：位置与朝向已经过与远端
// 玩家/伙伴/夜行者相同的时间边界插值，生命是最近一次权威 state 的直读值，
// 放牧位是同一批次 state 的吃草瞬态直读（置位即低头呈现的唯一事实来源）。
type PassivePresentation struct {
	ID        uint64
	Dimension core.DimensionID
	Position  mgl32.Vec3
	Yaw       float32
	Health    uint8
	Grazing   bool
}

// passivePresentationState 是一头被动牛的客户端镜像：身体事实 latest-wins，
// 移动呈现复用 `remoteActor` 的既有时间边界（不预测生命、伤害、放牧位或出生
// 位置——它们只随权威消息到达）。
type passivePresentationState struct {
	health  uint8
	grazing bool
	remoteActor
}

// Passives 是权威被动牛的固定容量 latest-wins 镜像。它由客户端主线程独占；
// 镜像层拒绝（消息校验失败）返回错误，镜像层丢弃（未知 ID、过期 tick、
// 容量溢出、重复 spawn）按稳定规则静默处理——被动牛是纯权威事实，客户端
// 没有任何可失配的本地预测，健壮性优先于会话终结。
type Passives struct {
	values map[uint64]*passivePresentationState
}

// ApplySpawn 建立（或按稳定规则忽略）一头被动牛的身体。同 ID 已有身体时
// 忽略重复 spawn（既有镜像保持不变）；镜像已满时同样忽略新个体。
func (passives *Passives) ApplySpawn(spawn network.PassiveSpawn) error {
	if err := spawn.Validate(); err != nil {
		return passiveProtocolError("PassiveSpawn: %v", err)
	}
	if passives.values == nil {
		passives.values = make(map[uint64]*passivePresentationState, MaxPassives)
	}
	for _, record := range spawn.Spawns {
		if _, exists := passives.values[record.ID]; exists {
			continue
		}
		if len(passives.values) >= MaxPassives {
			continue
		}
		state := &passivePresentationState{health: record.Health}
		state.pushSnapshot(remoteSnapshot{
			tick:      spawn.ServerTick,
			dimension: record.Dimension,
			position:  record.Position,
			yaw:       record.Yaw,
		}, true)
		passives.values[record.ID] = state
	}
	return nil
}

// ApplyStates 只接受 `ServerTick` 更新的状态：未知 ID 的记录丢弃且不隐式
// 造实体，过期（不比镜像新）的记录丢弃并保持既有值，其余记录按批次 tick
// 更新身体、生命与放牧位（出生批次不带瞬态，新身体默认非放牧；放牧字节经
// `Validate` 已限 0/1，这里按置位语义直读）。
func (passives *Passives) ApplyStates(states network.PassiveState) error {
	if err := states.Validate(); err != nil {
		return passiveProtocolError("PassiveState: %v", err)
	}
	for _, update := range states.States {
		state, exists := passives.values[update.ID]
		if !exists {
			continue
		}
		if states.ServerTick <= state.lastTick {
			continue
		}
		state.health = update.Health
		state.grazing = update.Grazing == 1
		state.pushSnapshot(remoteSnapshot{
			tick:      states.ServerTick,
			dimension: state.dimension,
			position:  update.Position,
			yaw:       update.Yaw,
		}, false)
	}
	return nil
}

// ApplyDespawn 移除一头被动牛的镜像；未知 ID 的 despawn 丢弃。
func (passives *Passives) ApplyDespawn(despawn network.PassiveDespawn) error {
	if err := despawn.Validate(); err != nil {
		return passiveProtocolError("PassiveDespawn: %v", err)
	}
	for _, id := range despawn.IDs {
		delete(passives.values, id)
	}
	return nil
}

// Advance 推进全部被动牛的呈现插值。
func (passives *Passives) Advance(elapsed time.Duration) {
	for _, state := range passives.values {
		state.advance(elapsed)
	}
}

// AppendPresentations 追加全部被动牛的插值后呈现（按 ID 升序，帧与帧之间
// 的顺序确定），复用调用方切片。
func (passives *Passives) AppendPresentations(dst []PassivePresentation) []PassivePresentation {
	for id, state := range passives.values {
		dst = append(dst, PassivePresentation{
			ID:        id,
			Dimension: state.dimension,
			Position:  state.position,
			Yaw:       state.yaw,
			Health:    state.health,
			Grazing:   state.grazing,
		})
	}
	slices.SortFunc(dst, func(left, right PassivePresentation) int {
		switch {
		case left.ID < right.ID:
			return -1
		case left.ID > right.ID:
			return 1
		default:
			return 0
		}
	})
	return dst
}

// Reset 清空镜像（重连时调用）。
func (passives *Passives) Reset() {
	clear(passives.values)
}

func passiveProtocolError(format string, arguments ...any) error {
	return fmt.Errorf("%w: %s", ErrPassiveProtocol, fmt.Sprintf(format, arguments...))
}
