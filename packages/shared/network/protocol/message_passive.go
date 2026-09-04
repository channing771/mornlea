package protocol

import (
	"errors"
	"fmt"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 被动牛三类 S→C 消息（`PassiveSpawn`/`PassiveState`/`PassiveDespawn`）的
// 固定 wire 契约。字段面与 `contract.PassiveMob` 对齐：spawn 携带出生时的
// 完整身体，state 携带逐 tick 的身体状态，despawn 只携带 ID。上限与 record
// 步长由 `TestPassiveMessagesWireLimitsAreFrozen` 的字节推导锁死：单包至多
// 64 条记录（协议上界，与权威侧全服 32 头的容量是两层概念——解码接受 64，
// 服务端实际发布与客户端镜像按 32 收敛），任何一侧单独调整都必须同步另一
// 侧并更新 golden。
const (
	// MaxPassiveRecords 是单包承载的 record 数上限，也是协议层拒绝 count>64
	// 的依据；count 为 0 同样拒绝——空批次没有可观察语义。`Passive*`.Validate
	// 与编解码侧解码共用。
	MaxPassiveRecords = 64
	// PassiveSpawnWireBytes 是单条 spawn record 的固定编码长度：u64 ID +
	// i32 dimension + 3×f32 position + f32 yaw + u8 health = 29。
	PassiveSpawnWireBytes = 29
	// PassiveStateWireBytes 是单条 state record 的固定编码长度：u64 ID +
	// 3×f32 position + 3×f32 velocity + f32 yaw + u8 health = 37。
	PassiveStateWireBytes = 37
	// PassiveDespawnWireBytes 是单条 despawn record 的固定编码长度：只携带
	// u64 ID = 8。
	PassiveDespawnWireBytes = 8
	// PassiveSpawnMaxWireBytes/PassiveStateMaxWireBytes/PassiveDespawnMaxWireBytes
	// 是三类载荷（u64 tick + u8 count + records）的固定 wire 上限，供解码端
	// 在分配前做总量截断拒绝。
	PassiveSpawnMaxWireBytes   = 9 + MaxPassiveRecords*PassiveSpawnWireBytes
	PassiveStateMaxWireBytes   = 9 + MaxPassiveRecords*PassiveStateWireBytes
	PassiveDespawnMaxWireBytes = 9 + MaxPassiveRecords*PassiveDespawnWireBytes
)

// PassiveSpawnRecord 是一头被动牛的出生事实：ID 非零，位置与朝向有限，
// 生命落在 1..core.MaxHealth。维度当前恒为 `core.Overworld`。
type PassiveSpawnRecord struct {
	ID        uint64
	Dimension core.DimensionID
	Position  mgl32.Vec3
	Yaw       float32
	Health    uint8
}

func (record PassiveSpawnRecord) validate() error {
	if record.ID == 0 {
		return errors.New("network: passive spawn ID is zero")
	}
	if record.Dimension != core.Overworld {
		return fmt.Errorf("network: passive spawn dimension %d is invalid", record.Dimension)
	}
	if !finiteVec3(record.Position) || !finite32(record.Yaw) {
		return errors.New("network: passive spawn pose is not finite")
	}
	if record.Health == 0 || record.Health > core.MaxHealth {
		return fmt.Errorf("network: passive spawn health %d outside 1..%d", record.Health, core.MaxHealth)
	}
	return nil
}

// PassiveSpawn 在被动牛进入某会话已订阅 chunk 时发布其完整身体。record 按
// ID 严格升序，每 tick 至多一包。
type PassiveSpawn struct {
	ServerTick uint64
	Spawns     []PassiveSpawnRecord
}

func (PassiveSpawn) serverMessage() {}
func (PassiveSpawn) serverPacket()  {}

// Validate 验证批次数量、每条记录与 ID 严格升序；任何一条不成立都整体拒绝。
func (spawn PassiveSpawn) Validate() error {
	if len(spawn.Spawns) < 1 || len(spawn.Spawns) > MaxPassiveRecords {
		return fmt.Errorf("network: passive spawn count is outside 1..%d", MaxPassiveRecords)
	}
	for index := range spawn.Spawns {
		if err := spawn.Spawns[index].validate(); err != nil {
			return fmt.Errorf("network: passive spawn %d: %w", index, err)
		}
		if index > 0 && spawn.Spawns[index-1].ID >= spawn.Spawns[index].ID {
			return errors.New("network: passive spawns are not strictly sorted")
		}
	}
	return nil
}

// PassiveStateRecord 是一头被动牛在一个权威 tick 的身体状态。
type PassiveStateRecord struct {
	ID       uint64
	Position mgl32.Vec3
	Velocity mgl32.Vec3
	Yaw      float32
	Health   uint8
}

func (record PassiveStateRecord) validate() error {
	if record.ID == 0 {
		return errors.New("network: passive state ID is zero")
	}
	if !finiteVec3(record.Position) || !finiteVec3(record.Velocity) || !finite32(record.Yaw) {
		return errors.New("network: passive state is not finite")
	}
	if record.Health == 0 || record.Health > core.MaxHealth {
		return fmt.Errorf("network: passive state health %d outside 1..%d", record.Health, core.MaxHealth)
	}
	return nil
}

// PassiveState 是按 ID 严格升序的有界被动牛状态批次，逐 tick 发布给已订阅
// 会话。与 spawn 不同，state 不携带维度：维度变化必然先经过 despawn/spawn
// 对，客户端镜像按 ID 维持身体即可。
type PassiveState struct {
	ServerTick uint64
	States     []PassiveStateRecord
}

func (PassiveState) serverMessage() {}
func (PassiveState) serverPacket()  {}

// Validate 验证批次数量、每条记录与 ID 严格升序；任何一条不成立都整体拒绝。
func (state PassiveState) Validate() error {
	if len(state.States) < 1 || len(state.States) > MaxPassiveRecords {
		return fmt.Errorf("network: passive state count is outside 1..%d", MaxPassiveRecords)
	}
	for index := range state.States {
		if err := state.States[index].validate(); err != nil {
			return fmt.Errorf("network: passive state %d: %w", index, err)
		}
		if index > 0 && state.States[index-1].ID >= state.States[index].ID {
			return errors.New("network: passive states are not strictly sorted")
		}
	}
	return nil
}

// PassiveDespawn 在被动牛离开订阅范围或死亡时按 ID 移除客户端可见身体。
// record 只携带 ID，按 ID 严格升序，每 tick 至多一包。
type PassiveDespawn struct {
	ServerTick uint64
	IDs        []uint64
}

func (PassiveDespawn) serverMessage() {}
func (PassiveDespawn) serverPacket()  {}

// Validate 验证批次数量与 ID 严格升序且非零；任何一条不成立都整体拒绝。
func (despawn PassiveDespawn) Validate() error {
	if len(despawn.IDs) < 1 || len(despawn.IDs) > MaxPassiveRecords {
		return fmt.Errorf("network: passive despawn count is outside 1..%d", MaxPassiveRecords)
	}
	for index, id := range despawn.IDs {
		if id == 0 {
			return fmt.Errorf("network: passive despawn %d ID is zero", index)
		}
		if index > 0 && despawn.IDs[index-1] >= id {
			return errors.New("network: passive despawns are not strictly sorted")
		}
	}
	return nil
}
