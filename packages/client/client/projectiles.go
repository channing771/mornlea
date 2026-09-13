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

var ErrProjectileProtocol = errors.New("projectile protocol error")

// MaxProjectiles 是客户端投射物镜像的固定容量，与 wire 侧
// `network.MaxProjectileRecords` 的 128 record 上限同源契约；超出容量的
// spawn 按稳定规则忽略，不驱逐既有个体。
const MaxProjectiles = 128

// ProjectilePresentation 是一枚投射物的只读呈现值：位置与速度均来自权威
// 事实——位置已经过与夜行者相同的时间边界插值，速度是最近一次权威估计
// （spawn 初速直读、state 按位置差分重估），客户端绝不本地预测弹道。
type ProjectilePresentation struct {
	ID        uint64
	Kind      uint8
	Dimension core.DimensionID
	Position  mgl32.Vec3
	Velocity  mgl32.Vec3
}

// projectilePresentationState 是一枚投射物的客户端镜像：身体事实
// latest-wins，移动呈现复用 `remoteActor` 的既有时间边界。速度只是取向
// 估计（差分自权威位置序列），不参与插值。
type projectilePresentationState struct {
	kind     uint8
	velocity mgl32.Vec3
	remoteActor
}

// Projectiles 是权威投射物的固定容量 latest-wins 镜像，纪律与
// `Hostiles` 同形：镜像层拒绝（消息校验失败）返回错误，镜像层丢弃（未知
// ID、过期 tick、容量溢出、重复 spawn）按稳定规则静默处理。投射物是纯
// 权威瞬态实体，客户端没有任何可失配的本地预测。
type Projectiles struct {
	values map[uint64]*projectilePresentationState
}

// ApplySpawn 建立（或按稳定规则忽略）一枚投射物的镜像。同 ID 已有镜像时
// 忽略重复 spawn（既有镜像保持不变）；镜像已满时同样忽略新个体。速度取
// spawn record 的初速直读。
func (projectiles *Projectiles) ApplySpawn(spawn network.ProjectileSpawn) error {
	if err := spawn.Validate(); err != nil {
		return projectileProtocolError("ProjectileSpawn: %v", err)
	}
	if projectiles.values == nil {
		projectiles.values = make(map[uint64]*projectilePresentationState, MaxProjectiles)
	}
	for _, record := range spawn.Spawns {
		if _, exists := projectiles.values[record.ID]; exists {
			continue
		}
		if len(projectiles.values) >= MaxProjectiles {
			continue
		}
		state := &projectilePresentationState{kind: record.Kind, velocity: record.Velocity}
		state.pushSnapshot(remoteSnapshot{
			tick:      spawn.ServerTick,
			dimension: record.Dimension,
			position:  record.Position,
		}, true)
		projectiles.values[record.ID] = state
	}
	return nil
}

// ApplyStates 只接受 `ServerTick` 更新的状态：未知 ID 的记录丢弃且不隐式
// 造实体，过期（不比镜像新）的记录丢弃并保持既有值，其余记录按批次 tick
// 更新位置。速度按位置差分重估（位移除以 tick 差）；零位移的更新保持上一
// 次估计——悬停瞬间的取向不退回未知，呈现侧因此拿到稳定的取向输入。
func (projectiles *Projectiles) ApplyStates(states network.ProjectileState) error {
	if err := states.Validate(); err != nil {
		return projectileProtocolError("ProjectileState: %v", err)
	}
	for _, update := range states.States {
		state, exists := projectiles.values[update.ID]
		if !exists {
			continue
		}
		if states.ServerTick <= state.lastTick {
			continue
		}
		delta := update.Position.Sub(state.position)
		if delta.LenSqr() > 0 {
			state.velocity = delta.Mul(1 / float32(states.ServerTick-state.lastTick))
		}
		state.pushSnapshot(remoteSnapshot{
			tick:      states.ServerTick,
			dimension: state.dimension,
			position:  update.Position,
		}, false)
	}
	return nil
}

// ApplyDespawn 移除一枚投射物的镜像；未知 ID 的 despawn 丢弃。
func (projectiles *Projectiles) ApplyDespawn(despawn network.ProjectileDespawn) error {
	if err := despawn.Validate(); err != nil {
		return projectileProtocolError("ProjectileDespawn: %v", err)
	}
	for _, id := range despawn.IDs {
		delete(projectiles.values, id)
	}
	return nil
}

// Advance 推进全部投射物的呈现插值。
func (projectiles *Projectiles) Advance(elapsed time.Duration) {
	for _, state := range projectiles.values {
		state.advance(elapsed)
	}
}

// AppendPresentations 追加全部投射物的插值后呈现（按 ID 升序，帧与帧之间
// 的顺序确定），复用调用方切片。
func (projectiles *Projectiles) AppendPresentations(dst []ProjectilePresentation) []ProjectilePresentation {
	for id, state := range projectiles.values {
		dst = append(dst, ProjectilePresentation{
			ID:        id,
			Kind:      state.kind,
			Dimension: state.dimension,
			Position:  state.position,
			Velocity:  state.velocity,
		})
	}
	slices.SortFunc(dst, func(left, right ProjectilePresentation) int {
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
func (projectiles *Projectiles) Reset() {
	clear(projectiles.values)
}

func projectileProtocolError(format string, arguments ...any) error {
	return fmt.Errorf("%w: %s", ErrProjectileProtocol, fmt.Sprintf(format, arguments...))
}
