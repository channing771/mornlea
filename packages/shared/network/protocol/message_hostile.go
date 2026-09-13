package protocol

import (
	"errors"
	"fmt"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 夜行者与掷骨者两类敌怪共享的三类 S→C 消息（`HostileSpawn`/`HostileState`/
// `HostileDespawn`）的固定 wire 契约。上限与 record 步长由
// `TestHostileMessagesWireLimitsAreFrozen` 的字节推导锁死：全服同时存在的
// 敌怪数量上限当前为 64（权威侧 sim 的 `maxHostiles` 同值契约，两类 kind
// 共享同一预算），任何一侧单独调整都必须同步另一侧并更新 golden。
const (
	// MaxHostileRecords 是单包承载的 record 数上限，也是协议层拒绝 count>64
	// 的依据；count 为 0 同样拒绝——空批次没有可观察语义。`Hostile*`.Validate
	// 与编解码侧解码共用。
	MaxHostileRecords = 64
	// HostileSpawnWireBytes 是单条 spawn record 的固定编码长度：u64 ID +
	// i32 dimension + 3×f32 position + f32 yaw + u8 health + u8 kind = 30。
	HostileSpawnWireBytes = 30
	// HostileStateWireBytes 是单条 state record 的固定编码长度：u64 ID +
	// 3×f32 position + 3×f32 velocity + f32 yaw + u8 health + u8 kind = 38。
	HostileStateWireBytes = 38
	// HostileDespawnWireBytes 是单条 despawn record 的固定编码长度：只携带
	// u64 ID = 8。
	HostileDespawnWireBytes = 8
	// HostileSpawnMaxWireBytes/HostileStateMaxWireBytes/HostileDespawnMaxWireBytes
	// 是三类载荷（u64 tick + u8 count + records）的固定 wire 上限，供解码端
	// 在分配前做总量截断拒绝。
	HostileSpawnMaxWireBytes   = 9 + MaxHostileRecords*HostileSpawnWireBytes
	HostileStateMaxWireBytes   = 9 + MaxHostileRecords*HostileStateWireBytes
	HostileDespawnMaxWireBytes = 9 + MaxHostileRecords*HostileDespawnWireBytes
)

// HostileKindNightwalker/HostileKindBoneThrower 是敌怪 kind 字节的全部合法
// 取值：0 表示夜行者（近战追击），1 表示掷骨者（远程投掷骨刺）。kind 以
// pure-append 方式位于 spawn/state record 尾部，与 wire 同形数值的引擎域与
// 存档侧共享该映射；新增敌怪类别必须同步扩展两侧值域并升版协议。
const (
	HostileKindNightwalker uint8 = 0
	HostileKindBoneThrower uint8 = 1
)

// validHostileKind 报告 kind 是否落在敌怪值域 {0,1} 内。值域判定集中在此处，
// spawn 与 state 两条 record 校验共用，避免两处各写一份造成域定义漂移。
func validHostileKind(kind uint8) bool {
	return kind == HostileKindNightwalker || kind == HostileKindBoneThrower
}

// HostileSpawnRecord 是一只敌怪的出生事实：ID 非零，位置与朝向有限，生命
// 落在 1..core.MaxHealth。维度当前恒为 `core.Overworld`。`Kind` 是 record
// 尾部 pure-append 的敌怪类别字节（值域 {0,1}，越界拒绝）。
type HostileSpawnRecord struct {
	ID        uint64
	Dimension core.DimensionID
	Position  mgl32.Vec3
	Yaw       float32
	Health    uint8
	Kind      uint8
}

func (record HostileSpawnRecord) validate() error {
	if record.ID == 0 {
		return errors.New("network: hostile spawn ID is zero")
	}
	if record.Dimension != core.Overworld {
		return fmt.Errorf("network: hostile spawn dimension %d is invalid", record.Dimension)
	}
	if !finiteVec3(record.Position) || !finite32(record.Yaw) {
		return errors.New("network: hostile spawn pose is not finite")
	}
	if record.Health == 0 || record.Health > core.MaxHealth {
		return fmt.Errorf("network: hostile spawn health %d outside 1..%d", record.Health, core.MaxHealth)
	}
	if !validHostileKind(record.Kind) {
		return fmt.Errorf("network: hostile spawn kind %d is invalid", record.Kind)
	}
	return nil
}

// HostileSpawn 在敌怪（任一 kind）进入某会话已订阅 chunk 时发布其完整
// 身体。record 按 ID 严格升序，每 tick 至多一包。
type HostileSpawn struct {
	ServerTick uint64
	Spawns     []HostileSpawnRecord
}

func (HostileSpawn) serverMessage() {}
func (HostileSpawn) serverPacket()  {}

// Validate 验证批次数量、每条记录与 ID 严格升序；任何一条不成立都整体拒绝。
func (spawn HostileSpawn) Validate() error {
	if len(spawn.Spawns) < 1 || len(spawn.Spawns) > MaxHostileRecords {
		return fmt.Errorf("network: hostile spawn count is outside 1..%d", MaxHostileRecords)
	}
	for index := range spawn.Spawns {
		if err := spawn.Spawns[index].validate(); err != nil {
			return fmt.Errorf("network: hostile spawn %d: %w", index, err)
		}
		if index > 0 && spawn.Spawns[index-1].ID >= spawn.Spawns[index].ID {
			return errors.New("network: hostile spawns are not strictly sorted")
		}
	}
	return nil
}

// HostileStateRecord 是一只敌怪在一个权威 tick 的身体状态。`Kind` 与 spawn
// record 尾部字节同域同义（值域 {0,1}，越界拒绝）：客户端镜像在 despawn 前
// 不得依赖 state 缺席而丢失类别呈现。
type HostileStateRecord struct {
	ID       uint64
	Position mgl32.Vec3
	Velocity mgl32.Vec3
	Yaw      float32
	Health   uint8
	Kind     uint8
}

func (record HostileStateRecord) validate() error {
	if record.ID == 0 {
		return errors.New("network: hostile state ID is zero")
	}
	if !finiteVec3(record.Position) || !finiteVec3(record.Velocity) || !finite32(record.Yaw) {
		return errors.New("network: hostile state is not finite")
	}
	if record.Health == 0 || record.Health > core.MaxHealth {
		return fmt.Errorf("network: hostile state health %d outside 1..%d", record.Health, core.MaxHealth)
	}
	if !validHostileKind(record.Kind) {
		return fmt.Errorf("network: hostile state kind %d is invalid", record.Kind)
	}
	return nil
}

// HostileState 是按 ID 严格升序的有界敌怪状态批次，逐 tick 发布给已订阅
// 会话。与 spawn 不同，state 不携带维度：维度变化必然先经过 despawn/spawn
// 对，客户端镜像按 ID 维持身体即可。
type HostileState struct {
	ServerTick uint64
	States     []HostileStateRecord
}

func (HostileState) serverMessage() {}
func (HostileState) serverPacket()  {}

// Validate 验证批次数量、每条记录与 ID 严格升序；任何一条不成立都整体拒绝。
func (state HostileState) Validate() error {
	if len(state.States) < 1 || len(state.States) > MaxHostileRecords {
		return fmt.Errorf("network: hostile state count is outside 1..%d", MaxHostileRecords)
	}
	for index := range state.States {
		if err := state.States[index].validate(); err != nil {
			return fmt.Errorf("network: hostile state %d: %w", index, err)
		}
		if index > 0 && state.States[index-1].ID >= state.States[index].ID {
			return errors.New("network: hostile states are not strictly sorted")
		}
	}
	return nil
}

// HostileDespawn 在敌怪离开订阅范围或死亡时按 ID 移除客户端可见身体。
// record 只携带 ID，按 ID 严格升序，每 tick 至多一包。
type HostileDespawn struct {
	ServerTick uint64
	IDs        []uint64
}

func (HostileDespawn) serverMessage() {}
func (HostileDespawn) serverPacket()  {}

// Validate 验证批次数量与 ID 严格升序且非零；任何一条不成立都整体拒绝。
func (despawn HostileDespawn) Validate() error {
	if len(despawn.IDs) < 1 || len(despawn.IDs) > MaxHostileRecords {
		return fmt.Errorf("network: hostile despawn count is outside 1..%d", MaxHostileRecords)
	}
	for index, id := range despawn.IDs {
		if id == 0 {
			return fmt.Errorf("network: hostile despawn %d ID is zero", index)
		}
		if index > 0 && despawn.IDs[index-1] >= id {
			return errors.New("network: hostile despawns are not strictly sorted")
		}
	}
	return nil
}
