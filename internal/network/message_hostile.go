package network

import (
	"errors"
	"fmt"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
)

// 夜行者三类 S→C 消息（`HostileSpawn`/`HostileState`/`HostileDespawn`）的
// 固定 wire 契约。上限与 record 步长由 `TestHostileMessagesWireLimitsAreFrozen`
// 的字节推导锁死：全服同时存在的夜行者数量上限当前为 64（权威侧 sim 的
// `maxHostiles` 同值契约），任何一侧单独调整都必须同步另一侧并更新 golden。
const (
	// maxHostileRecords 是单包承载的 record 数上限，也是协议层拒绝 count>64
	// 的依据；count 为 0 同样拒绝——空批次没有可观察语义。
	maxHostileRecords = 64
	// hostileSpawnWireBytes 是单条 spawn record 的固定编码长度：u64 ID +
	// i32 dimension + 3×f32 position + f32 yaw + u8 health = 29。
	hostileSpawnWireBytes = 29
	// hostileStateWireBytes 是单条 state record 的固定编码长度：u64 ID +
	// 3×f32 position + 3×f32 velocity + f32 yaw + u8 health = 37。
	hostileStateWireBytes = 37
	// hostileDespawnWireBytes 是单条 despawn record 的固定编码长度：只携带
	// u64 ID = 8。
	hostileDespawnWireBytes = 8
	// hostileSpawnMaxWireBytes/hostileStateMaxWireBytes/hostileDespawnMaxWireBytes
	// 是三类载荷（u64 tick + u8 count + records）的固定 wire 上限，供解码端
	// 在分配前做总量截断拒绝。
	hostileSpawnMaxWireBytes   = 9 + maxHostileRecords*hostileSpawnWireBytes
	hostileStateMaxWireBytes   = 9 + maxHostileRecords*hostileStateWireBytes
	hostileDespawnMaxWireBytes = 9 + maxHostileRecords*hostileDespawnWireBytes
)

// HostileSpawnRecord 是一只夜行者的出生事实：ID 非零，位置与朝向有限，
// 生命落在 1..core.MaxHealth。维度当前恒为 `core.Overworld`。
type HostileSpawnRecord struct {
	ID        uint64
	Dimension core.DimensionID
	Position  mgl32.Vec3
	Yaw       float32
	Health    uint8
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
	return nil
}

// HostileSpawn 在夜行者进入某会话已订阅 chunk 时发布其完整身体。record 按
// ID 严格升序，每 tick 至多一包。
type HostileSpawn struct {
	ServerTick uint64
	Spawns     []HostileSpawnRecord
}

func (HostileSpawn) serverMessage() {}
func (HostileSpawn) serverPacket()  {}

// Validate 验证批次数量、每条记录与 ID 严格升序；任何一条不成立都整体拒绝。
func (spawn HostileSpawn) Validate() error {
	if len(spawn.Spawns) < 1 || len(spawn.Spawns) > maxHostileRecords {
		return fmt.Errorf("network: hostile spawn count is outside 1..%d", maxHostileRecords)
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

// HostileStateRecord 是一只夜行者在一个权威 tick 的身体状态。
type HostileStateRecord struct {
	ID       uint64
	Position mgl32.Vec3
	Velocity mgl32.Vec3
	Yaw      float32
	Health   uint8
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
	return nil
}

// HostileState 是按 ID 严格升序的有界夜行者状态批次，逐 tick 发布给已订阅
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
	if len(state.States) < 1 || len(state.States) > maxHostileRecords {
		return fmt.Errorf("network: hostile state count is outside 1..%d", maxHostileRecords)
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

// HostileDespawn 在夜行者离开订阅范围或死亡时按 ID 移除客户端可见身体。
// record 只携带 ID，按 ID 严格升序，每 tick 至多一包。
type HostileDespawn struct {
	ServerTick uint64
	IDs        []uint64
}

func (HostileDespawn) serverMessage() {}
func (HostileDespawn) serverPacket()  {}

// Validate 验证批次数量与 ID 严格升序且非零；任何一条不成立都整体拒绝。
func (despawn HostileDespawn) Validate() error {
	if len(despawn.IDs) < 1 || len(despawn.IDs) > maxHostileRecords {
		return fmt.Errorf("network: hostile despawn count is outside 1..%d", maxHostileRecords)
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

func encodeHostileSpawn(e *byteEncoder, spawn HostileSpawn) {
	e.u64(spawn.ServerTick)
	e.u8(uint8(len(spawn.Spawns)))
	for _, record := range spawn.Spawns {
		e.u64(record.ID)
		e.i32(int32(record.Dimension))
		for _, value := range record.Position {
			e.f32(value)
		}
		e.f32(record.Yaw)
		e.u8(record.Health)
	}
}

func encodeHostileState(e *byteEncoder, state HostileState) {
	e.u64(state.ServerTick)
	e.u8(uint8(len(state.States)))
	for _, record := range state.States {
		e.u64(record.ID)
		for _, value := range record.Position {
			e.f32(value)
		}
		for _, value := range record.Velocity {
			e.f32(value)
		}
		e.f32(record.Yaw)
		e.u8(record.Health)
	}
}

func encodeHostileDespawn(e *byteEncoder, despawn HostileDespawn) {
	e.u64(despawn.ServerTick)
	e.u8(uint8(len(despawn.IDs)))
	for _, id := range despawn.IDs {
		e.u64(id)
	}
}

func decodeHostileSpawn(d *byteDecoder) (ServerPacket, error) {
	var spawn HostileSpawn
	var err error
	if spawn.ServerTick, err = d.u64(); err != nil {
		return nil, err
	}
	count, err := d.u8()
	if err != nil {
		return nil, err
	}
	if count < 1 || int(count) > maxHostileRecords {
		return nil, fmt.Errorf("network: hostile spawn count is outside 1..%d", maxHostileRecords)
	}
	if len(d.data)-d.offset != int(count)*hostileSpawnWireBytes {
		return nil, errors.New("network: hostile spawn length does not match count")
	}
	spawn.Spawns = make([]HostileSpawnRecord, int(count))
	for index := range spawn.Spawns {
		record := &spawn.Spawns[index]
		if record.ID, err = d.u64(); err != nil {
			return nil, err
		}
		var dimension int32
		if dimension, err = d.i32(); err != nil {
			return nil, err
		}
		record.Dimension = core.DimensionID(dimension)
		for component := range record.Position {
			if record.Position[component], err = d.f32(); err != nil {
				return nil, err
			}
		}
		if record.Yaw, err = d.f32(); err != nil {
			return nil, err
		}
		if record.Health, err = d.u8(); err != nil {
			return nil, err
		}
	}
	return spawn, nil
}

func decodeHostileState(d *byteDecoder) (ServerPacket, error) {
	var state HostileState
	var err error
	if state.ServerTick, err = d.u64(); err != nil {
		return nil, err
	}
	count, err := d.u8()
	if err != nil {
		return nil, err
	}
	if count < 1 || int(count) > maxHostileRecords {
		return nil, fmt.Errorf("network: hostile state count is outside 1..%d", maxHostileRecords)
	}
	if len(d.data)-d.offset != int(count)*hostileStateWireBytes {
		return nil, errors.New("network: hostile state length does not match count")
	}
	state.States = make([]HostileStateRecord, int(count))
	for index := range state.States {
		record := &state.States[index]
		if record.ID, err = d.u64(); err != nil {
			return nil, err
		}
		for component := range record.Position {
			if record.Position[component], err = d.f32(); err != nil {
				return nil, err
			}
		}
		for component := range record.Velocity {
			if record.Velocity[component], err = d.f32(); err != nil {
				return nil, err
			}
		}
		if record.Yaw, err = d.f32(); err != nil {
			return nil, err
		}
		if record.Health, err = d.u8(); err != nil {
			return nil, err
		}
	}
	return state, nil
}

func decodeHostileDespawn(d *byteDecoder) (ServerPacket, error) {
	var despawn HostileDespawn
	var err error
	if despawn.ServerTick, err = d.u64(); err != nil {
		return nil, err
	}
	count, err := d.u8()
	if err != nil {
		return nil, err
	}
	if count < 1 || int(count) > maxHostileRecords {
		return nil, fmt.Errorf("network: hostile despawn count is outside 1..%d", maxHostileRecords)
	}
	if len(d.data)-d.offset != int(count)*hostileDespawnWireBytes {
		return nil, errors.New("network: hostile despawn length does not match count")
	}
	despawn.IDs = make([]uint64, int(count))
	for index := range despawn.IDs {
		if despawn.IDs[index], err = d.u64(); err != nil {
			return nil, err
		}
	}
	return despawn, nil
}
