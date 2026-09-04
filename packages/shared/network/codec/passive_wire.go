package codec

import (
	"errors"
	"fmt"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// 被动牛域消息的 wire 编解码函数：与 hostile_wire.go 同理归编解码簇（本
// 包），由 `codec_server.go` 包内直呼分发；消息 DTO 与 wire 上限常量定义在
// `packages/shared/network/protocol`，编解码原语是本包 unexported 类型，因此这组
// 函数保持包内 unexported。字段次序与夜行者侧同形，唯一区别是包 ID 与类型名。
func encodePassiveSpawn(e *byteEncoder, spawn protocol.PassiveSpawn) {
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

func encodePassiveState(e *byteEncoder, state protocol.PassiveState) {
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

func encodePassiveDespawn(e *byteEncoder, despawn protocol.PassiveDespawn) {
	e.u64(despawn.ServerTick)
	e.u8(uint8(len(despawn.IDs)))
	for _, id := range despawn.IDs {
		e.u64(id)
	}
}

func decodePassiveSpawn(d *byteDecoder) (protocol.ServerPacket, error) {
	var spawn protocol.PassiveSpawn
	var err error
	if spawn.ServerTick, err = d.u64(); err != nil {
		return nil, err
	}
	count, err := d.u8()
	if err != nil {
		return nil, err
	}
	if count < 1 || int(count) > protocol.MaxPassiveRecords {
		return nil, fmt.Errorf("network: passive spawn count is outside 1..%d", protocol.MaxPassiveRecords)
	}
	if len(d.data)-d.offset != int(count)*protocol.PassiveSpawnWireBytes {
		return nil, errors.New("network: passive spawn length does not match count")
	}
	spawn.Spawns = make([]protocol.PassiveSpawnRecord, int(count))
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

func decodePassiveState(d *byteDecoder) (protocol.ServerPacket, error) {
	var state protocol.PassiveState
	var err error
	if state.ServerTick, err = d.u64(); err != nil {
		return nil, err
	}
	count, err := d.u8()
	if err != nil {
		return nil, err
	}
	if count < 1 || int(count) > protocol.MaxPassiveRecords {
		return nil, fmt.Errorf("network: passive state count is outside 1..%d", protocol.MaxPassiveRecords)
	}
	if len(d.data)-d.offset != int(count)*protocol.PassiveStateWireBytes {
		return nil, errors.New("network: passive state length does not match count")
	}
	state.States = make([]protocol.PassiveStateRecord, int(count))
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

func decodePassiveDespawn(d *byteDecoder) (protocol.ServerPacket, error) {
	var despawn protocol.PassiveDespawn
	var err error
	if despawn.ServerTick, err = d.u64(); err != nil {
		return nil, err
	}
	count, err := d.u8()
	if err != nil {
		return nil, err
	}
	if count < 1 || int(count) > protocol.MaxPassiveRecords {
		return nil, fmt.Errorf("network: passive despawn count is outside 1..%d", protocol.MaxPassiveRecords)
	}
	if len(d.data)-d.offset != int(count)*protocol.PassiveDespawnWireBytes {
		return nil, errors.New("network: passive despawn length does not match count")
	}
	despawn.IDs = make([]uint64, int(count))
	for index := range despawn.IDs {
		if despawn.IDs[index], err = d.u64(); err != nil {
			return nil, err
		}
	}
	return despawn, nil
}
