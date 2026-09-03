package codec

import (
	"errors"
	"fmt"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// 伙伴/聊天域消息的 wire 编解码函数：与其余消息的 wire 代码一致归编解码簇
// （本包），由 `codec_server.go` 包内直呼分发。消息 DTO 与 wire 上限常量定义
// 在 `packages/shared/network/protocol`（其 `Validate` 与本包的预分配拒绝双方消费该
// 常量面），编解码原语 `byteEncoder`/`byteDecoder` 是本包 unexported 类型，
// 因此这组函数保持包内 unexported。

func encodeChatEvent(e *byteEncoder, event protocol.ChatEvent) {
	e.u64(event.EventID)
	e.data = append(e.data, event.PlayerID[:]...)
	e.string(event.PlayerName, 128)
	e.data = append(e.data, event.CompanionID[:]...)
	e.string(event.CompanionName, 128)
	e.u8(uint8(event.Kind))
	e.u8(uint8(event.RejectReason))
	// 文本槽位按 kind 复用：CompanionSpeech 写入台词（编码层即收紧为
	// protocol.ChatSpeechTextMaxBytes 上界），其余 kind 保持
	// protocol.ChatCommandTextMaxBytes 指令编码，既有 kind 的 wire 字节不受影响。
	if event.Kind == protocol.ChatEventCompanionSpeech {
		e.string(event.Speech, protocol.ChatSpeechTextMaxBytes)
	} else {
		e.string(event.Command, protocol.ChatCommandTextMaxBytes)
	}
}

func encodeCompanionSpawn(e *byteEncoder, spawn protocol.CompanionSpawn) {
	e.data = append(e.data, spawn.ID[:]...)
	e.string(spawn.Name, 128)
	e.u64(spawn.Tick)
	e.i32(int32(spawn.Dimension))
	for _, value := range spawn.Position {
		e.f32(value)
	}
	e.f32(spawn.Yaw)
	e.f32(spawn.Pitch)
}

func encodeCompanionStates(e *byteEncoder, states protocol.CompanionStates) {
	e.u64(states.Tick)
	e.uvarint(uint32(len(states.States)))
	for _, state := range states.States {
		e.data = append(e.data, state.ID[:]...)
		e.i32(int32(state.Dimension))
		for _, value := range state.Position {
			e.f32(value)
		}
		e.f32(state.Yaw)
		e.f32(state.Pitch)
		e.bool(state.Reset)
	}
}

func decodeChatEvent(d *byteDecoder) (protocol.ServerPacket, error) {
	var event protocol.ChatEvent
	var err error
	event.EventID, err = d.u64()
	if err == nil {
		err = decodeFixedID(d, event.PlayerID[:])
	}
	if err == nil {
		event.PlayerName, err = d.string(128, 32)
	}
	if err == nil {
		err = decodeFixedID(d, event.CompanionID[:])
	}
	if err == nil {
		event.CompanionName, err = d.string(128, 32)
	}
	if err == nil {
		var kind uint8
		kind, err = d.u8()
		event.Kind = protocol.ChatEventKind(kind)
	}
	if err == nil {
		var reason uint8
		reason, err = d.u8()
		event.RejectReason = protocol.ChatRejectReason(reason)
	}
	if err == nil {
		// 文本槽位按 kind 复用：解码时先读出 kind，再把槽位读入台词（超过
		// protocol.ChatSpeechTextMaxBytes 直接拒绝）或玩家指令，随后 Validate
		// 按 kind 施加完整文本纪律。
		if event.Kind == protocol.ChatEventCompanionSpeech {
			event.Speech, err = d.string(protocol.ChatSpeechTextMaxBytes, protocol.ChatSpeechTextMaxBytes)
		} else {
			event.Command, err = d.string(protocol.ChatCommandTextMaxBytes, protocol.ChatCommandTextMaxBytes)
		}
	}
	if err != nil {
		return nil, err
	}
	return event, nil
}

func decodeCompanionSpawn(d *byteDecoder) (protocol.ServerPacket, error) {
	var spawn protocol.CompanionSpawn
	var err error
	err = decodeFixedID(d, spawn.ID[:])
	if err == nil {
		spawn.Name, err = d.string(128, 32)
	}
	if err == nil {
		spawn.Tick, err = d.u64()
	}
	if err == nil {
		var dimension int32
		dimension, err = d.i32()
		spawn.Dimension = core.DimensionID(dimension)
	}
	for index := range spawn.Position {
		if err == nil {
			spawn.Position[index], err = d.f32()
		}
	}
	if err == nil {
		spawn.Yaw, err = d.f32()
	}
	if err == nil {
		spawn.Pitch, err = d.f32()
	}
	if err != nil {
		return nil, err
	}
	return spawn, nil
}

func decodeCompanionStates(d *byteDecoder) (protocol.ServerPacket, error) {
	var states protocol.CompanionStates
	var err error
	states.Tick, err = d.u64()
	var count uint32
	if err == nil {
		count, err = d.uvarint()
	}
	if err == nil && (count < 1 || count > protocol.MaxCompanionStates) {
		err = fmt.Errorf("network: companion state count is outside 1..%d", protocol.MaxCompanionStates)
	}
	if err == nil && len(d.data)-d.offset != int(count)*protocol.CompanionStateWireBytes {
		err = errors.New("network: companion states length does not match count")
	}
	if err != nil {
		return nil, err
	}
	states.States = make([]protocol.CompanionState, int(count))
	for index := range states.States {
		state := &states.States[index]
		if err = decodeFixedID(d, state.ID[:]); err != nil {
			return nil, err
		}
		dimension, readErr := d.i32()
		if readErr != nil {
			return nil, readErr
		}
		state.Dimension = core.DimensionID(dimension)
		for component := range state.Position {
			state.Position[component], err = d.f32()
			if err != nil {
				return nil, err
			}
		}
		if state.Yaw, err = d.f32(); err != nil {
			return nil, err
		}
		if state.Pitch, err = d.f32(); err != nil {
			return nil, err
		}
		if state.Reset, err = d.bool(); err != nil {
			return nil, err
		}
	}
	return states, nil
}

func decodeFixedID(d *byteDecoder, destination []byte) error {
	data, err := d.take(len(destination))
	if err != nil {
		return err
	}
	copy(destination, data)
	return nil
}
