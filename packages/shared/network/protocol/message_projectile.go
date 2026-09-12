package protocol

import (
	"errors"
	"fmt"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 投射物三类 S→C 消息（`ProjectileSpawn`/`ProjectileState`/`ProjectileDespawn`）
// 的固定 wire 契约。投射物是服务端权威的瞬态实体，全服同时在飞上限 128 条；
// 上限与 record 步长由 `TestProjectileMessagesWireLimitsAreFrozen`（codec 侧
// 同名族）的字节推导锁死，任何一侧单独调整都必须同步另一侧并更新 golden。
const (
	// MaxProjectileRecords 是单包承载的 record 数上限，也是协议层拒绝
	// count>128 的依据；count 为 0 同样拒绝——空批次没有可观察语义。
	// `Projectile*`.Validate 与编解码侧解码共用。
	MaxProjectileRecords = 128
	// ProjectileSpawnWireBytes 是单条 spawn record 的固定编码长度：u64 ID +
	// u8 kind + i32 dimension + 3×f32 position + 3×f32 velocity = 37。
	ProjectileSpawnWireBytes = 37
	// ProjectileStateWireBytes 是单条 state record 的固定编码长度：u64 ID +
	// 3×f32 position = 20。与 spawn 不同，state 不携带维度与弹种：两者在
	// 投射物的整个生命周期不变，客户端镜像按 ID 在 spawn 时记牢即可。
	ProjectileStateWireBytes = 20
	// ProjectileDespawnWireBytes 是单条 despawn record 的固定编码长度：只携带
	// u64 ID = 8。
	ProjectileDespawnWireBytes = 8
	// ProjectileSpawnMaxWireBytes/ProjectileStateMaxWireBytes/ProjectileDespawnMaxWireBytes
	// 是三类载荷（u64 tick + u8 count + records）的固定 wire 上限，供解码端
	// 在分配前做总量截断拒绝。
	ProjectileSpawnMaxWireBytes   = 9 + MaxProjectileRecords*ProjectileSpawnWireBytes
	ProjectileStateMaxWireBytes   = 9 + MaxProjectileRecords*ProjectileStateWireBytes
	ProjectileDespawnMaxWireBytes = 9 + MaxProjectileRecords*ProjectileDespawnWireBytes
)

// ProjectileKindShard/ProjectileKindArrow 是弹种 kind 字节的全部合法取值：
// 0 表示骨刺（远程敌怪发射），1 表示箭（玩家弓发射）。引擎域与存档侧按
// 同一数值映射消费这两个值，协议层是数值的权威定义点；新增弹种必须同步
// 扩展两侧值域并升版协议。
const (
	ProjectileKindShard uint8 = 0
	ProjectileKindArrow uint8 = 1
)

// validProjectileKind 报告 kind 是否落在弹种值域 {0,1} 内。值域判定集中在
// 此处，`ProjectileSpawnRecord.validate` 与后续弹种消费者共用，避免双侧
// 各写一份造成域定义漂移。
func validProjectileKind(kind uint8) bool {
	return kind == ProjectileKindShard || kind == ProjectileKindArrow
}

// ProjectileSpawnRecord 是一条投射物的出生事实：ID 非零，位置与速度有限，
// 维度取玩家可达的两个合法维度之一（玩家弓在两个维度都可发射，敌怪骨刺
// 当前只在主世界生成——wire 层不做「弹种×维度」组合策略，该策略属权威 sim）。
type ProjectileSpawnRecord struct {
	ID        uint64
	Kind      uint8
	Dimension core.DimensionID
	Position  mgl32.Vec3
	Velocity  mgl32.Vec3
}

func (record ProjectileSpawnRecord) validate() error {
	if record.ID == 0 {
		return errors.New("network: projectile spawn ID is zero")
	}
	if !validProjectileKind(record.Kind) {
		return fmt.Errorf("network: projectile spawn kind %d is invalid", record.Kind)
	}
	if record.Dimension != core.Overworld && record.Dimension != core.Depths {
		return fmt.Errorf("network: projectile spawn dimension %d is invalid", record.Dimension)
	}
	if !finiteVec3(record.Position) || !finiteVec3(record.Velocity) {
		return errors.New("network: projectile spawn pose is not finite")
	}
	return nil
}

// ProjectileSpawn 在投射物进入某会话已订阅 chunk 时发布其完整飞行状态。
// record 按 ID 严格升序，每 tick 至多一包。
type ProjectileSpawn struct {
	ServerTick uint64
	Spawns     []ProjectileSpawnRecord
}

func (ProjectileSpawn) serverMessage() {}
func (ProjectileSpawn) serverPacket()  {}

// Validate 验证批次数量、每条记录与 ID 严格升序；任何一条不成立都整体拒绝。
func (spawn ProjectileSpawn) Validate() error {
	if len(spawn.Spawns) < 1 || len(spawn.Spawns) > MaxProjectileRecords {
		return fmt.Errorf("network: projectile spawn count is outside 1..%d", MaxProjectileRecords)
	}
	for index := range spawn.Spawns {
		if err := spawn.Spawns[index].validate(); err != nil {
			return fmt.Errorf("network: projectile spawn %d: %w", index, err)
		}
		if index > 0 && spawn.Spawns[index-1].ID >= spawn.Spawns[index].ID {
			return errors.New("network: projectile spawns are not strictly sorted")
		}
	}
	return nil
}

// ProjectileStateRecord 是一条投射物在一个权威 tick 的飞行位置。
type ProjectileStateRecord struct {
	ID       uint64
	Position mgl32.Vec3
}

func (record ProjectileStateRecord) validate() error {
	if record.ID == 0 {
		return errors.New("network: projectile state ID is zero")
	}
	if !finiteVec3(record.Position) {
		return errors.New("network: projectile state is not finite")
	}
	return nil
}

// ProjectileState 是按 ID 严格升序的有界投射物位置批次，逐 tick 发布给已
// 订阅会话，供客户端做 latest-wins 镜像与插值呈现。
type ProjectileState struct {
	ServerTick uint64
	States     []ProjectileStateRecord
}

func (ProjectileState) serverMessage() {}
func (ProjectileState) serverPacket()  {}

// Validate 验证批次数量、每条记录与 ID 严格升序；任何一条不成立都整体拒绝。
func (state ProjectileState) Validate() error {
	if len(state.States) < 1 || len(state.States) > MaxProjectileRecords {
		return fmt.Errorf("network: projectile state count is outside 1..%d", MaxProjectileRecords)
	}
	for index := range state.States {
		if err := state.States[index].validate(); err != nil {
			return fmt.Errorf("network: projectile state %d: %w", index, err)
		}
		if index > 0 && state.States[index-1].ID >= state.States[index].ID {
			return errors.New("network: projectile states are not strictly sorted")
		}
	}
	return nil
}

// ProjectileDespawn 在投射物命中、寿命耗尽、离开全部订阅区或被上限挤占时
// 按 ID 移除客户端可见实体。record 只携带 ID，按 ID 严格升序，每 tick 至多
// 一包。
type ProjectileDespawn struct {
	ServerTick uint64
	IDs        []uint64
}

func (ProjectileDespawn) serverMessage() {}
func (ProjectileDespawn) serverPacket()  {}

// Validate 验证批次数量与 ID 严格升序且非零；任何一条不成立都整体拒绝。
func (despawn ProjectileDespawn) Validate() error {
	if len(despawn.IDs) < 1 || len(despawn.IDs) > MaxProjectileRecords {
		return fmt.Errorf("network: projectile despawn count is outside 1..%d", MaxProjectileRecords)
	}
	for index, id := range despawn.IDs {
		if id == 0 {
			return fmt.Errorf("network: projectile despawn %d ID is zero", index)
		}
		if index > 0 && despawn.IDs[index-1] >= id {
			return errors.New("network: projectile despawns are not strictly sorted")
		}
	}
	return nil
}
