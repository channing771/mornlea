package codec

import (
	"errors"
	"fmt"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// 投射物域消息的 wire 编解码函数：与 hostile_wire.go 同理归编解码簇（本
// 包），由 `codec_server.go` 包内直呼分发；消息 DTO 与 wire 上限常量定义在
// `packages/shared/network/protocol`，编解码原语是本包 unexported 类型，因此
// 这组函数保持包内 unexported。spawn/state record 不携带 yaw/health：投射物
// 是无姿态的点状瞬态实体，朝向由客户端沿速度插值呈现。
func encodeProjectileSpawn(e *byteEncoder, spawn protocol.ProjectileSpawn) {
	e.u64(spawn.ServerTick)
	e.u8(uint8(len(spawn.Spawns)))
	for _, record := range spawn.Spawns {
		e.u64(record.ID)
		e.u8(record.Kind)
		e.i32(int32(record.Dimension))
		for _, value := range record.Position {
			e.f32(value)
		}
		for _, value := range record.Velocity {
			e.f32(value)
		}
	}
}

func encodeProjectileState(e *byteEncoder, state protocol.ProjectileState) {
	e.u64(state.ServerTick)
	e.u8(uint8(len(state.States)))
	for _, record := range state.States {
		e.u64(record.ID)
		for _, value := range record.Position {
			e.f32(value)
		}
	}
}

func encodeProjectileDespawn(e *byteEncoder, despawn protocol.ProjectileDespawn) {
	e.u64(despawn.ServerTick)
	e.u8(uint8(len(despawn.IDs)))
	for _, id := range despawn.IDs {
		e.u64(id)
	}
}

func decodeProjectileSpawn(d *byteDecoder) (protocol.ServerPacket, error) {
	var spawn protocol.ProjectileSpawn
	var err error
	if spawn.ServerTick, err = d.u64(); err != nil {
		return nil, err
	}
	count, err := d.u8()
	if err != nil {
		return nil, err
	}
	if count < 1 || int(count) > protocol.MaxProjectileRecords {
		return nil, fmt.Errorf("network: projectile spawn count is outside 1..%d", protocol.MaxProjectileRecords)
	}
	if len(d.data)-d.offset != int(count)*protocol.ProjectileSpawnWireBytes {
		return nil, errors.New("network: projectile spawn length does not match count")
	}
	spawn.Spawns = make([]protocol.ProjectileSpawnRecord, int(count))
	for index := range spawn.Spawns {
		record := &spawn.Spawns[index]
		if record.ID, err = d.u64(); err != nil {
			return nil, err
		}
		if record.Kind, err = d.u8(); err != nil {
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
		for component := range record.Velocity {
			if record.Velocity[component], err = d.f32(); err != nil {
				return nil, err
			}
		}
	}
	return spawn, nil
}

func decodeProjectileState(d *byteDecoder) (protocol.ServerPacket, error) {
	var state protocol.ProjectileState
	var err error
	if state.ServerTick, err = d.u64(); err != nil {
		return nil, err
	}
	count, err := d.u8()
	if err != nil {
		return nil, err
	}
	if count < 1 || int(count) > protocol.MaxProjectileRecords {
		return nil, fmt.Errorf("network: projectile state count is outside 1..%d", protocol.MaxProjectileRecords)
	}
	if len(d.data)-d.offset != int(count)*protocol.ProjectileStateWireBytes {
		return nil, errors.New("network: projectile state length does not match count")
	}
	state.States = make([]protocol.ProjectileStateRecord, int(count))
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
	}
	return state, nil
}

func decodeProjectileDespawn(d *byteDecoder) (protocol.ServerPacket, error) {
	var despawn protocol.ProjectileDespawn
	var err error
	if despawn.ServerTick, err = d.u64(); err != nil {
		return nil, err
	}
	count, err := d.u8()
	if err != nil {
		return nil, err
	}
	if count < 1 || int(count) > protocol.MaxProjectileRecords {
		return nil, fmt.Errorf("network: projectile despawn count is outside 1..%d", protocol.MaxProjectileRecords)
	}
	if len(d.data)-d.offset != int(count)*protocol.ProjectileDespawnWireBytes {
		return nil, errors.New("network: projectile despawn length does not match count")
	}
	despawn.IDs = make([]uint64, int(count))
	for index := range despawn.IDs {
		if despawn.IDs[index], err = d.u64(); err != nil {
			return nil, err
		}
	}
	return despawn, nil
}
