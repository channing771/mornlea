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

var ErrHostileProtocol = errors.New("hostile protocol error")

// MaxHostiles 是客户端夜行者镜像的固定容量，与服务端全服上限（wire 侧
// `network` 的 64 record 上限同源契约）一致；超出容量的 spawn 按稳定规则
// 忽略，不驱逐既有身体。
const MaxHostiles = 64

// HostilePresentation 是一只夜行者的只读呈现值：位置与朝向已经过与远端
// 玩家/伙伴相同的时间边界插值，生命是最近一次权威 state 的直读值。
type HostilePresentation struct {
	ID        uint64
	Dimension core.DimensionID
	Position  mgl32.Vec3
	Yaw       float32
	Health    uint8
}

// hostilePresentationState 是一只夜行者的客户端镜像：身体事实 latest-wins，
// 移动呈现复用 `remoteActor` 的既有时间边界（不预测生命、伤害、冷却或出生
// 位置——它们只随权威消息到达）。
type hostilePresentationState struct {
	health uint8
	remoteActor
}

// Hostiles 是权威夜行者的固定容量 latest-wins 镜像。它由客户端主线程独占；
// 镜像层拒绝（消息校验失败）返回错误，镜像层丢弃（未知 ID、过期 tick、
// 容量溢出、重复 spawn）按稳定规则静默处理——夜行者是纯权威事实，客户端
// 没有任何可失配的本地预测，健壮性优先于会话终结。
type Hostiles struct {
	values map[uint64]*hostilePresentationState
}

// ApplySpawn 建立（或按稳定规则忽略）一只夜行者的身体。同 ID 已有身体时
// 忽略重复 spawn（既有镜像保持不变）；镜像已满时同样忽略新个体。
func (hostiles *Hostiles) ApplySpawn(spawn network.HostileSpawn) error {
	if err := spawn.Validate(); err != nil {
		return hostileProtocolError("HostileSpawn: %v", err)
	}
	if hostiles.values == nil {
		hostiles.values = make(map[uint64]*hostilePresentationState, MaxHostiles)
	}
	for _, record := range spawn.Spawns {
		if _, exists := hostiles.values[record.ID]; exists {
			continue
		}
		if len(hostiles.values) >= MaxHostiles {
			continue
		}
		state := &hostilePresentationState{health: record.Health}
		state.pushSnapshot(remoteSnapshot{
			tick:      spawn.ServerTick,
			dimension: record.Dimension,
			position:  record.Position,
			yaw:       record.Yaw,
		}, true)
		hostiles.values[record.ID] = state
	}
	return nil
}

// ApplyStates 只接受 `ServerTick` 更新的状态：未知 ID 的记录丢弃且不隐式
// 造实体，过期（不比镜像新）的记录丢弃并保持既有值，其余记录按批次 tick
// 更新身体与生命。
func (hostiles *Hostiles) ApplyStates(states network.HostileState) error {
	if err := states.Validate(); err != nil {
		return hostileProtocolError("HostileState: %v", err)
	}
	for _, update := range states.States {
		state, exists := hostiles.values[update.ID]
		if !exists {
			continue
		}
		if states.ServerTick <= state.lastTick {
			continue
		}
		state.health = update.Health
		state.pushSnapshot(remoteSnapshot{
			tick:      states.ServerTick,
			dimension: state.dimension,
			position:  update.Position,
			yaw:       update.Yaw,
		}, false)
	}
	return nil
}

// ApplyDespawn 移除一只夜行者的镜像；未知 ID 的 despawn 丢弃。
func (hostiles *Hostiles) ApplyDespawn(despawn network.HostileDespawn) error {
	if err := despawn.Validate(); err != nil {
		return hostileProtocolError("HostileDespawn: %v", err)
	}
	for _, id := range despawn.IDs {
		delete(hostiles.values, id)
	}
	return nil
}

// Advance 推进全部夜行者的呈现插值。
func (hostiles *Hostiles) Advance(elapsed time.Duration) {
	for _, state := range hostiles.values {
		state.advance(elapsed)
	}
}

// AppendPresentations 追加全部夜行者的插值后呈现（按 ID 升序，帧与帧之间
// 的顺序确定），复用调用方切片。
func (hostiles *Hostiles) AppendPresentations(dst []HostilePresentation) []HostilePresentation {
	for id, state := range hostiles.values {
		dst = append(dst, HostilePresentation{
			ID:        id,
			Dimension: state.dimension,
			Position:  state.position,
			Yaw:       state.yaw,
			Health:    state.health,
		})
	}
	slices.SortFunc(dst, func(left, right HostilePresentation) int {
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
func (hostiles *Hostiles) Reset() {
	clear(hostiles.values)
}

func hostileProtocolError(format string, arguments ...any) error {
	return fmt.Errorf("%w: %s", ErrHostileProtocol, fmt.Sprintf(format, arguments...))
}
