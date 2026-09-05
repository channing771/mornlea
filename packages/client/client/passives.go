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
// 死亡保留体（`Dying` 置位）的位置与朝向是死亡 tick 的冻结值，`DeathTick` 是
// 其 despawn 的权威 tick（渲染侧死亡相位的唯一事实来源）。
type PassivePresentation struct {
	ID        uint64
	Dimension core.DimensionID
	Position  mgl32.Vec3
	Yaw       float32
	Health    uint8
	Grazing   bool
	Dying     bool
	DeathTick uint64
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
//
// 死亡保留体住 `dying` 表：死亡原因的 despawn 把身体从 `values` 转入本表并
// 冻结位姿，保留进度由见过的最大权威 tick 推进，满 20 tick 移除；保留体与
// 活体共用 32 上限语义。
type Passives struct {
	values  map[uint64]*passivePresentationState
	dying   map[uint64]PassivePresentation
	maxTick uint64
}

// passiveDeathRetention 是死亡保留的权威 tick 数：保留体在 T+19 仍在、T+20
// 移除，数值由死亡呈现规格锁定。
const passiveDeathRetention = 20

// observeTick 记录见过的最大权威 tick 并推进死亡保留：保留进度满
// `passiveDeathRetention` 的保留体移除，后续渲染不再出现。
func (passives *Passives) observeTick(tick uint64) {
	if tick > passives.maxTick {
		passives.maxTick = tick
	}
	for id, kept := range passives.dying {
		if passives.maxTick-kept.DeathTick >= passiveDeathRetention {
			delete(passives.dying, id)
		}
	}
}

// ApplySpawn 建立（或按稳定规则忽略）一头被动牛的身体。同 ID 已有活体或
// 正处死亡保留时忽略重复 spawn（既有镜像与保留态保持不变）；镜像已满
// （活体与保留体合计）时同样忽略新个体。
func (passives *Passives) ApplySpawn(spawn network.PassiveSpawn) error {
	if err := spawn.Validate(); err != nil {
		return passiveProtocolError("PassiveSpawn: %v", err)
	}
	passives.observeTick(spawn.ServerTick)
	if passives.values == nil {
		passives.values = make(map[uint64]*passivePresentationState, MaxPassives)
	}
	for _, record := range spawn.Spawns {
		if _, exists := passives.values[record.ID]; exists {
			continue
		}
		if _, kept := passives.dying[record.ID]; kept {
			continue
		}
		if len(passives.values)+len(passives.dying) >= MaxPassives {
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
	passives.observeTick(states.ServerTick)
	for _, update := range states.States {
		// 死亡保留体不再接受权威更新：死亡后无 state，位姿保持冻结。
		if _, kept := passives.dying[update.ID]; kept {
			continue
		}
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

// ApplyDespawn 按原因位移除一头被动牛的镜像：消失原因立即移除活体；死亡
// 原因把活体转入死亡保留（冻结当前位姿，满 20 tick 后移除）；未知 ID 的
// despawn 丢弃。
func (passives *Passives) ApplyDespawn(despawn network.PassiveDespawn) error {
	if err := despawn.Validate(); err != nil {
		return passiveProtocolError("PassiveDespawn: %v", err)
	}
	passives.observeTick(despawn.ServerTick)
	for _, record := range despawn.Despawns {
		if record.Reason == network.PassiveDespawnDied {
			state, exists := passives.values[record.ID]
			if !exists {
				continue
			}
			if passives.dying == nil {
				passives.dying = make(map[uint64]PassivePresentation, MaxPassives)
			}
			passives.dying[record.ID] = PassivePresentation{
				ID:        record.ID,
				Dimension: state.dimension,
				Position:  state.position,
				Yaw:       state.yaw,
				Health:    state.health,
				Grazing:   state.grazing,
				Dying:     true,
				DeathTick: despawn.ServerTick,
			}
			delete(passives.values, record.ID)
			continue
		}
		delete(passives.values, record.ID)
	}
	return nil
}

// Advance 推进全部被动牛的呈现插值。
func (passives *Passives) Advance(elapsed time.Duration) {
	for _, state := range passives.values {
		state.advance(elapsed)
	}
}

// AppendPresentations 追加全部被动牛的插值后呈现（含死亡保留体，按 ID
// 升序，帧与帧之间的顺序确定），复用调用方切片。
func (passives *Passives) AppendPresentations(dst []PassivePresentation) []PassivePresentation {
	passives.observeTick(passives.maxTick)
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
	for _, kept := range passives.dying {
		dst = append(dst, kept)
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

// Reset 清空镜像（含死亡保留，重连时调用）。
func (passives *Passives) Reset() {
	clear(passives.values)
	clear(passives.dying)
	passives.maxTick = 0
}

func passiveProtocolError(format string, arguments ...any) error {
	return fmt.Errorf("%w: %s", ErrPassiveProtocol, fmt.Sprintf(format, arguments...))
}
