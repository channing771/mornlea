package codec

import (
	"fmt"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

func encodeClientPacketPayload(state protocol.State, packet protocol.ClientPacket) (packetID uint32, payload []byte, err error) {
	if err := validateClientWirePacket(state, packet); err != nil {
		return 0, nil, codecError("encode client", state, 0, err)
	}
	packetID, ok := protocol.ClientPacketID(state, packet)
	if !ok {
		return 0, nil, codecError("encode client", state, 0, protocol.InvalidClientPacket(state, packet))
	}
	var e byteEncoder
	switch state {
	case protocol.StateHandshake:
		message := packet.(protocol.ClientHello)
		e.uvarint(message.ProtocolVersion)
	case protocol.StateLogin:
		message := packet.(protocol.LoginStart)
		e.data = append(e.data, message.PlayerID[:]...)
		e.string(message.DisplayName, 128)
		// v40 起视距是载荷最末一字节（u8），紧跟长度前缀昵称之后；域外值
		// 已被前置的 `ValidateClientPacket` 拒绝，此处只搬运。
		e.u8(message.ViewDistance)
	case protocol.StatePlay:
		switch message := packet.(type) {
		case protocol.PlayerInput:
			e.u64(message.Sequence)
			e.i8(message.MoveX)
			e.i8(message.MoveZ)
			e.bool(message.Jump)
			e.f32(message.Yaw)
			e.f32(message.Pitch)
			e.bool(message.Mining)
			e.bool(message.Eating)
			e.bool(message.Sprinting)
			e.bool(message.Sneaking)
		case protocol.PlaceBlock:
			e.u64(message.Sequence)
			e.f32(message.Yaw)
			e.f32(message.Pitch)
			e.u8(message.Slot)
		case protocol.SelectHotbar:
			e.u64(message.Sequence)
			e.u8(message.Slot)
		case protocol.MoveInventoryStack:
			e.u64(message.Sequence)
			e.u8(message.From)
			e.u8(message.To)
		case protocol.DropSelectedItem:
			e.u64(message.Sequence)
		case protocol.OpenContainer:
			e.u64(message.Sequence)
			e.f32(message.Yaw)
			e.f32(message.Pitch)
		case protocol.MoveContainerStack:
			e.u64(message.Sequence)
			encodeContainerRef(&e, message.Container)
			e.u8(message.From)
			e.u8(message.To)
		case protocol.CloseContainer:
			e.u64(message.Sequence)
		case protocol.RequestChunkResync:
			e.u64(message.Sequence)
			e.i32(int32(message.Dimension))
			e.i32(message.Chunk.X)
			e.i32(message.Chunk.Z)
			e.u64(message.HaveRevision)
		case protocol.KeepAliveReply:
			e.u64(message.Token)
		case protocol.ChatCommand:
			e.string(message.Text, protocol.ChatCommandTextMaxBytes)
		case protocol.TillSoil:
			e.u64(message.Sequence)
			e.f32(message.Yaw)
			e.f32(message.Pitch)
		case protocol.BoneMeal:
			e.u64(message.Sequence)
			e.f32(message.Yaw)
			e.f32(message.Pitch)
		case protocol.CollectWater:
			e.u64(message.Sequence)
			e.f32(message.Yaw)
			e.f32(message.Pitch)
		case protocol.PlaceWater:
			e.u64(message.Sequence)
			e.f32(message.Yaw)
			e.f32(message.Pitch)
		case protocol.MoveCraftingStack:
			e.u64(message.Sequence)
			e.u8(message.From)
			e.u8(message.To)
		case protocol.TakeCraftingOutput:
			e.u64(message.Sequence)
		case protocol.EquipArmor:
			// v42：装备互换命令与 `DropSelectedItem` 同形，只携带 u64 序号；
			// 目标槽位由服务端按护甲件类映射决定，wire 上不携带。
			e.u64(message.Sequence)
		case protocol.MoveStackPartial:
			// v44：u64 序号 + 18 字节容器引用 + u8 视图 + u8 来源 + u8 目标 +
			// u8 单件标志，固定 30 字节；移动数量由服务端推导，wire 上无数量字段。
			e.u64(message.Sequence)
			encodeContainerRef(&e, message.Container)
			e.u8(message.View)
			e.u8(message.From)
			e.u8(message.To)
			e.bool(message.Single)
		case protocol.QuickMoveStack:
			// v44：u64 序号 + 18 字节容器引用 + u8 视图 + u8 来源，固定 28 字节；
			// 目标序由服务端按固定确定性契约推导，wire 上不携带目标格。
			e.u64(message.Sequence)
			encodeContainerRef(&e, message.Container)
			e.u8(message.View)
			e.u8(message.From)
		case protocol.DropStack:
			// v45 encodes a u64 sequence, an 18-byte container reference, a u8 view,
			// and a u8 unified slot in 28 bytes. The server derives position and count;
			// the wire carries neither.
			e.u64(message.Sequence)
			encodeContainerRef(&e, message.Container)
			e.u8(message.View)
			e.u8(message.Slot)
		default:
			return 0, nil, codecError("encode client", state, packetID, protocol.InvalidClientPacket(state, packet))
		}
	default:
		return 0, nil, codecError("encode client", state, packetID, protocol.InvalidClientPacket(state, packet))
	}
	return finishEncode("encode client", state, packetID, e)
}

func decodeClientPacketPayload(state protocol.State, packetID uint32, payload []byte) (protocol.ClientPacket, error) {
	if err := checkSmallPayload(payload); err != nil {
		return nil, codecError("decode client", state, packetID, err)
	}
	if state == protocol.StatePlay && packetID == 12 && len(payload) > protocol.ChatCommandMaxWireBytes {
		return nil, codecError("decode client", state, packetID,
			fmt.Errorf("network: chat command payload exceeds %d bytes", protocol.ChatCommandMaxWireBytes))
	}
	d := byteDecoder{data: payload}
	var packet protocol.ClientPacket
	var err error
	switch state {
	case protocol.StateHandshake:
		if packetID != 0 {
			return nil, codecError("decode client", state, packetID, errUnknownPacketID)
		}
		var version uint32
		version, err = d.uvarint()
		packet = protocol.ClientHello{ProtocolVersion: version}
	case protocol.StateLogin:
		if packetID != 0 {
			return nil, codecError("decode client", state, packetID, errUnknownPacketID)
		}
		var id core.PlayerID
		var name string
		var viewDistance uint8
		if data, readErr := d.take(len(id)); readErr != nil {
			err = readErr
		} else {
			copy(id[:], data)
			name, err = d.string(MaxSmallPayload, MaxSmallPayload)
			if err == nil {
				// v40 尾部 1 字节视距：缺失即截断、多余即尾随，均由原语
				// 读错误与 `done()` 拒绝；域外值经解码放行口交给登录驱动
				// 以冻结拒绝码处理。
				viewDistance, err = d.u8()
			}
		}
		packet = protocol.LoginStart{PlayerID: id, DisplayName: name, ViewDistance: viewDistance}
	case protocol.StatePlay:
		switch packetID {
		case 0:
			var sequence uint64
			var moveX, moveZ int8
			var jump bool
			var yaw, pitch float32
			var mining, eating, sprinting, sneaking bool
			sequence, err = d.u64()
			if err == nil {
				moveX, err = d.i8()
			}
			if err == nil {
				moveZ, err = d.i8()
			}
			if err == nil {
				jump, err = d.bool()
			}
			if err == nil {
				yaw, err = d.f32()
			}
			if err == nil {
				pitch, err = d.f32()
			}
			if err == nil {
				mining, err = d.bool()
			}
			if err == nil {
				eating, err = d.bool()
			}
			if err == nil {
				sprinting, err = d.bool()
			}
			if err == nil {
				sneaking, err = d.bool()
			}
			packet = protocol.PlayerInput{Sequence: sequence, MoveX: moveX, MoveZ: moveZ, Jump: jump, Yaw: yaw, Pitch: pitch, Mining: mining, Eating: eating, Sprinting: sprinting, Sneaking: sneaking}
		case 2:
			var sequence uint64
			var yaw, pitch float32
			var slot uint8
			sequence, err = d.u64()
			if err == nil {
				yaw, err = d.f32()
			}
			if err == nil {
				pitch, err = d.f32()
			}
			if err == nil {
				slot, err = d.u8()
			}
			packet = protocol.PlaceBlock{Sequence: sequence, Yaw: yaw, Pitch: pitch, Slot: slot}
		case 3:
			var sequence, revision uint64
			var dimension, chunkX, chunkZ int32
			sequence, err = d.u64()
			if err == nil {
				dimension, err = d.i32()
			}
			if err == nil {
				chunkX, err = d.i32()
			}
			if err == nil {
				chunkZ, err = d.i32()
			}
			if err == nil {
				revision, err = d.u64()
			}
			packet = protocol.RequestChunkResync{Sequence: sequence, Dimension: core.DimensionID(dimension), Chunk: core.ChunkPos{X: chunkX, Z: chunkZ}, HaveRevision: revision}
		case 4:
			var token uint64
			token, err = d.u64()
			packet = protocol.KeepAliveReply{Token: token}
		case 5:
			var sequence uint64
			var slot uint8
			sequence, err = d.u64()
			if err == nil {
				slot, err = d.u8()
			}
			packet = protocol.SelectHotbar{Sequence: sequence, Slot: slot}
		case 6:
			var sequence uint64
			var from, to uint8
			sequence, err = d.u64()
			if err == nil {
				from, err = d.u8()
			}
			if err == nil {
				to, err = d.u8()
			}
			packet = protocol.MoveInventoryStack{Sequence: sequence, From: from, To: to}
		case 7:
			var move protocol.MoveCraftingStack
			move.Sequence, err = d.u64()
			if err == nil {
				move.From, err = d.u8()
			}
			if err == nil {
				move.To, err = d.u8()
			}
			packet = move
		case 8:
			var open protocol.OpenContainer
			open.Sequence, err = d.u64()
			if err == nil {
				open.Yaw, err = d.f32()
			}
			if err == nil {
				open.Pitch, err = d.f32()
			}
			packet = open
		case 9:
			var move protocol.MoveContainerStack
			move.Sequence, err = d.u64()
			if err == nil {
				move.Container, err = decodeContainerRef(&d)
			}
			if err == nil {
				move.From, err = d.u8()
			}
			if err == nil {
				move.To, err = d.u8()
			}
			packet = move
		case 10:
			var closeContainer protocol.CloseContainer
			closeContainer.Sequence, err = d.u64()
			packet = closeContainer
		case 11:
			var drop protocol.DropSelectedItem
			drop.Sequence, err = d.u64()
			packet = drop
		case 12:
			var command protocol.ChatCommand
			// `d.string` 的两参分别是字节上限与 rune 上限；此处同值系现状保持，
			// 并非两个独立上限恰好相等的巧合约束。
			command.Text, err = d.string(protocol.ChatCommandTextMaxBytes, protocol.ChatCommandTextMaxBytes)
			packet = command
		case 13:
			var till protocol.TillSoil
			till.Sequence, err = d.u64()
			if err == nil {
				till.Yaw, err = d.f32()
			}
			if err == nil {
				till.Pitch, err = d.f32()
			}
			packet = till
		case 14:
			var meal protocol.BoneMeal
			meal.Sequence, err = d.u64()
			if err == nil {
				meal.Yaw, err = d.f32()
			}
			if err == nil {
				meal.Pitch, err = d.f32()
			}
			packet = meal
		case 15:
			var take protocol.TakeCraftingOutput
			take.Sequence, err = d.u64()
			packet = take
		case 16:
			var collect protocol.CollectWater
			collect.Sequence, err = d.u64()
			if err == nil {
				collect.Yaw, err = d.f32()
			}
			if err == nil {
				collect.Pitch, err = d.f32()
			}
			packet = collect
		case 17:
			var place protocol.PlaceWater
			place.Sequence, err = d.u64()
			if err == nil {
				place.Yaw, err = d.f32()
			}
			if err == nil {
				place.Pitch, err = d.f32()
			}
			packet = place
		case 18:
			// v42：装备互换命令与 `DropSelectedItem` 同形，只读 u64 序号。
			var equip protocol.EquipArmor
			equip.Sequence, err = d.u64()
			packet = equip
		case 19:
			// v44：分堆部分移动，固定 30 字节；域校验由本函数尾部的
			// `ValidateDecodedClientWirePacket` 统一入口执行，此处只搬运字节。
			var partial protocol.MoveStackPartial
			partial.Sequence, err = d.u64()
			if err == nil {
				partial.Container, err = decodeContainerRef(&d)
			}
			if err == nil {
				partial.View, err = d.u8()
			}
			if err == nil {
				partial.From, err = d.u8()
			}
			if err == nil {
				partial.To, err = d.u8()
			}
			if err == nil {
				partial.Single, err = d.bool()
			}
			packet = partial
		case 20:
			// v44：快捷搬运，固定 28 字节；与部分移动同前缀、去掉目标与单件标志。
			var quick protocol.QuickMoveStack
			quick.Sequence, err = d.u64()
			if err == nil {
				quick.Container, err = decodeContainerRef(&d)
			}
			if err == nil {
				quick.View, err = d.u8()
			}
			if err == nil {
				quick.From, err = d.u8()
			}
			packet = quick
		case 21:
			// v45 uses 28 bytes for a full-stack drop. The shared
			// `ValidateDecodedClientWirePacket` boundary validates the decoded fields.
			var drop protocol.DropStack
			drop.Sequence, err = d.u64()
			if err == nil {
				drop.Container, err = decodeContainerRef(&d)
			}
			if err == nil {
				drop.View, err = d.u8()
			}
			if err == nil {
				drop.Slot, err = d.u8()
			}
			packet = drop
		default:
			return nil, codecError("decode client", state, packetID, errUnknownPacketID)
		}
	default:
		return nil, codecError("decode client", state, packetID, errUnknownPacketID)
	}
	if err == nil {
		err = d.done()
	}
	if err == nil {
		err = protocol.ValidateDecodedClientWirePacket(state, packet)
	}
	if err != nil {
		return nil, codecError("decode client", state, packetID, err)
	}
	return packet, nil
}

func validateClientWirePacket(state protocol.State, packet protocol.ClientPacket) error {
	return protocol.ValidateClientPacket(state, packet)
}
