package codec

import (
	"errors"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

func encodeServerControlPayload(state protocol.State, packet protocol.ServerPacket) (packetID uint32, payload []byte, err error) {
	if state == protocol.StatePlay {
		if _, ok := packet.(protocol.ChunkSnapshot); ok {
			return 0, nil, codecError("encode server", state, 0, errSnapshotDelegated)
		}
	}
	if err := validateServerWirePacket(state, packet); err != nil {
		return 0, nil, codecError("encode server", state, 0, err)
	}
	packetID, ok := protocol.ServerPacketID(state, packet)
	if !ok {
		return 0, nil, codecError("encode server", state, 0, protocol.InvalidServerPacket(state, packet))
	}
	var e byteEncoder
	switch state {
	case protocol.StateHandshake:
		switch message := packet.(type) {
		case protocol.ServerHello:
			e.uvarint(message.ProtocolVersion)
		case protocol.HandshakeReject:
			e.uvarint(message.ServerProtocolVersion)
			e.u8(uint8(message.Code))
			e.string(message.Message, 256)
		}
	case protocol.StateLogin:
		switch message := packet.(type) {
		case protocol.LoginSuccess:
			e.data = append(e.data, message.PlayerID[:]...)
			// v23：种子恰好追加在 PlayerID 之后（little-endian uint64），
			// 既有 M5B 及更早字段的位置与字节序保持不变。
			e.u64(message.WorldSeed)
		case protocol.LoginReject:
			e.u8(uint8(message.Code))
			e.string(message.Message, 256)
		}
	case protocol.StatePlay:
		switch message := packet.(type) {
		case protocol.BlockChanges:
			e.i32(int32(message.Dimension))
			e.i32(message.Chunk.X)
			e.i32(message.Chunk.Z)
			e.u64(message.BaseRevision)
			e.u64(message.NewRevision)
			e.uvarint(uint32(len(message.Changes)))
			for _, change := range message.Changes {
				e.i32(change.Position.X)
				e.i32(change.Position.Y)
				e.i32(change.Position.Z)
				e.u16(uint16(change.Block))
			}
		case protocol.ForgetChunks:
			e.i32(int32(message.Dimension))
			e.uvarint(uint32(len(message.Chunks)))
			for _, chunk := range message.Chunks {
				e.i32(chunk.X)
				e.i32(chunk.Z)
			}
		case protocol.PlayerState:
			e.u64(message.ServerTick)
			e.u64(message.LastInputSequence)
			e.i32(int32(message.Dimension))
			for _, value := range message.Position {
				e.f32(value)
			}
			for _, value := range message.Velocity {
				e.f32(value)
			}
			e.f32(message.Yaw)
			e.f32(message.Pitch)
			e.bool(message.OnGround)
			e.bool(message.Ready)
			e.bool(message.Reset)
			e.bool(message.MiningActive)
			e.i32(message.MiningTarget.X)
			e.i32(message.MiningTarget.Y)
			e.i32(message.MiningTarget.Z)
			e.u16(message.MiningProgressTicks)
			e.u16(message.MiningRequiredTicks)
			e.bool(message.MiningHarvestable)
			e.u8(message.Health)
			e.u16(message.Oxygen)
			e.u8(message.Hunger)
			e.bool(message.SaturationZero)
			// v31：显示相位偏移追加在 `SaturationZero` 之后、绝对世界时间之前，
			// 既有字段的位置与字节序保持不变。
			e.u16(message.DayPhaseOffset)
			e.u64(message.WorldTimeTicks)
		case protocol.CommandRejected:
			reason, _ := protocol.CommandRejectReasonID(message.Reason)
			e.u64(message.Sequence)
			e.u8(reason)
		case protocol.PlaceBlockSucceeded:
			e.u64(message.Sequence)
		// 格子工作台：u8 尺寸 + 固定 9 格 5 字节栈 + 5 字节产物格（共 51 字节）。
		// 始终编码全部 9 格（尺寸 2 时格 4..8 恒空），不做变长分支。
		case protocol.CraftingState:
			e.u8(message.Size)
			for _, stack := range message.Slots {
				encodeItemStack(&e, stack)
			}
			encodeItemStack(&e, message.Output)
		case protocol.KeepAlive:
			e.u64(message.Token)
		case protocol.Disconnect:
			e.u8(uint8(message.Code))
			e.string(message.Message, 256)
		case protocol.RemotePlayerSpawn:
			e.data = append(e.data, message.PlayerID[:]...)
			e.string(message.DisplayName, 128)
			e.u64(message.ServerTick)
			e.i32(int32(message.Dimension))
			for _, value := range message.Position {
				e.f32(value)
			}
			e.f32(message.Yaw)
			e.f32(message.Pitch)
		case protocol.RemotePlayerDespawn:
			e.data = append(e.data, message.PlayerID[:]...)
		case protocol.RemotePlayerStates:
			e.u64(message.ServerTick)
			e.uvarint(uint32(len(message.Players)))
			for _, player := range message.Players {
				e.data = append(e.data, player.PlayerID[:]...)
				e.i32(int32(player.Dimension))
				for _, value := range player.Position {
					e.f32(value)
				}
				e.f32(player.Yaw)
				e.f32(player.Pitch)
				e.bool(player.Reset)
			}
		case protocol.InventoryState:
			e.u8(message.Inventory.Hotbar.Selected)
			for _, stack := range message.Inventory.Hotbar.Slots {
				encodeItemStack(&e, stack)
			}
			for _, stack := range message.Inventory.Backpack {
				encodeItemStack(&e, stack)
			}
		case protocol.FurnaceState:
			encodeContainerRef(&e, message.Furnace)
			for _, stack := range [3]core.ItemStack{message.Input, message.Fuel, message.Output} {
				encodeItemStack(&e, stack)
			}
			e.u8(message.ProgressTicks)
			e.u16(message.BurnTicks)
		case protocol.ChestState:
			encodeContainerRef(&e, message.Chest)
			for _, stack := range message.Items {
				encodeItemStack(&e, stack)
			}
		case protocol.ContainerClosed:
			encodeContainerRef(&e, message.Container)
		case protocol.ItemDropUpserts:
			e.u64(message.ServerTick)
			e.uvarint(uint32(len(message.Drops)))
			for _, drop := range message.Drops {
				encodeDropID(&e, drop.ID)
				e.u32(drop.BlockIndex)
				encodeItemStack(&e, core.ItemStack{
					Item: drop.Item, Count: drop.Count, Durability: drop.Durability,
				})
			}
		case protocol.ItemDropRemoves:
			e.u64(message.ServerTick)
			e.uvarint(uint32(len(message.IDs)))
			for _, id := range message.IDs {
				encodeDropID(&e, id)
			}
		case protocol.ChatEvent:
			encodeChatEvent(&e, message)
		case protocol.CompanionSpawn:
			encodeCompanionSpawn(&e, message)
		case protocol.CompanionStates:
			encodeCompanionStates(&e, message)
		case protocol.CompanionDespawn:
			e.data = append(e.data, message.ID[:]...)
		case protocol.HostileSpawn:
			encodeHostileSpawn(&e, message)
		case protocol.HostileState:
			encodeHostileState(&e, message)
		case protocol.HostileDespawn:
			encodeHostileDespawn(&e, message)
		case protocol.CombatHit:
			e.u64(message.ServerTick)
			e.u8(message.Damage)
			e.u8(uint8(message.TargetKind))
		default:
			return 0, nil, codecError("encode server", state, packetID, protocol.InvalidServerPacket(state, packet))
		}
	default:
		return 0, nil, codecError("encode server", state, packetID, protocol.InvalidServerPacket(state, packet))
	}
	return finishEncode("encode server", state, packetID, e)
}

func decodeServerControlPayload(state protocol.State, packetID uint32, payload []byte) (protocol.ServerPacket, error) {
	if err := checkSmallPayload(payload); err != nil {
		return nil, codecError("decode server", state, packetID, err)
	}
	if state == protocol.StatePlay {
		var max int
		switch packetID {
		case 16:
			max = protocol.ChatEventMaxWireBytes
		case 17:
			max = protocol.CompanionSpawnMaxWireBytes
		case 18:
			max = protocol.CompanionStatesMaxWireBytes
		case 19:
			max = len(protocol.CompanionDespawn{}.ID)
		case 22:
			max = protocol.HostileSpawnMaxWireBytes
		case 23:
			max = protocol.HostileStateMaxWireBytes
		case 24:
			max = protocol.HostileDespawnMaxWireBytes
		case 25:
			max = 10
		}
		if max > 0 && len(payload) > max {
			return nil, codecError("decode server", state, packetID, errors.New("network: payload exceeds fixed maximum"))
		}
	}
	if state == protocol.StatePlay && packetID == 9 && len(payload) > remotePlayerStatesMaxPayload {
		return nil, codecError("decode server", state, packetID, errors.New("network: remote player states payload exceeds 296 bytes"))
	}
	if state == protocol.StatePlay && packetID == 0 {
		return nil, codecError("decode server", state, packetID, errSnapshotDelegated)
	}
	d := byteDecoder{data: payload}
	var packet protocol.ServerPacket
	var err error
	switch state {
	case protocol.StateHandshake:
		switch packetID {
		case 0:
			var version uint32
			version, err = d.uvarint()
			packet = protocol.ServerHello{ProtocolVersion: version}
		case 1:
			var version uint32
			var code uint8
			var message string
			version, err = d.uvarint()
			if err == nil {
				code, err = d.u8()
			}
			if err == nil {
				message, err = d.string(256, 256)
			}
			packet = protocol.HandshakeReject{ServerProtocolVersion: version, Code: protocol.HandshakeRejectCode(code), Message: message}
		default:
			return nil, codecError("decode server", state, packetID, errUnknownPacketID)
		}
	case protocol.StateLogin:
		switch packetID {
		case 0:
			var id core.PlayerID
			if data, readErr := d.take(len(id)); readErr != nil {
				err = readErr
			} else {
				copy(id[:], data)
			}
			var worldSeed uint64
			if err == nil {
				worldSeed, err = d.u64()
			}
			packet = protocol.LoginSuccess{PlayerID: id, WorldSeed: worldSeed}
		case 1:
			var code uint8
			var message string
			code, err = d.u8()
			if err == nil {
				message, err = d.string(256, 256)
			}
			packet = protocol.LoginReject{Code: protocol.LoginRejectCode(code), Message: message}
		default:
			return nil, codecError("decode server", state, packetID, errUnknownPacketID)
		}
	case protocol.StatePlay:
		switch packetID {
		case 1:
			packet, err = decodeBlockChanges(&d)
		case 2:
			packet, err = decodeForgetChunks(&d)
		case 3:
			var statePacket protocol.PlayerState
			statePacket.ServerTick, err = d.u64()
			if err == nil {
				statePacket.LastInputSequence, err = d.u64()
			}
			var dimension int32
			if err == nil {
				dimension, err = d.i32()
				statePacket.Dimension = core.DimensionID(dimension)
			}
			for index := range statePacket.Position {
				if err == nil {
					statePacket.Position[index], err = d.f32()
				}
			}
			for index := range statePacket.Velocity {
				if err == nil {
					statePacket.Velocity[index], err = d.f32()
				}
			}
			if err == nil {
				statePacket.Yaw, err = d.f32()
			}
			if err == nil {
				statePacket.Pitch, err = d.f32()
			}
			if err == nil {
				statePacket.OnGround, err = d.bool()
			}
			if err == nil {
				statePacket.Ready, err = d.bool()
			}
			if err == nil {
				statePacket.Reset, err = d.bool()
			}
			if err == nil {
				statePacket.MiningActive, err = d.bool()
			}
			if err == nil {
				statePacket.MiningTarget.X, err = d.i32()
			}
			if err == nil {
				statePacket.MiningTarget.Y, err = d.i32()
			}
			if err == nil {
				statePacket.MiningTarget.Z, err = d.i32()
			}
			if err == nil {
				statePacket.MiningProgressTicks, err = d.u16()
			}
			if err == nil {
				statePacket.MiningRequiredTicks, err = d.u16()
			}
			if err == nil {
				statePacket.MiningHarvestable, err = d.bool()
			}
			if err == nil {
				statePacket.Health, err = d.u8()
			}
			if err == nil {
				statePacket.Oxygen, err = d.u16()
			}
			if err == nil {
				statePacket.Hunger, err = d.u8()
			}
			if err == nil {
				statePacket.SaturationZero, err = d.bool()
			}
			if err == nil {
				statePacket.DayPhaseOffset, err = d.u16()
			}
			if err == nil {
				statePacket.WorldTimeTicks, err = d.u64()
			}
			packet = statePacket
		case 4:
			var sequence uint64
			var reasonID uint8
			sequence, err = d.u64()
			if err == nil {
				reasonID, err = d.u8()
			}
			reason, ok := protocol.CommandRejectReasonForID(reasonID)
			if err == nil && !ok {
				err = errors.New("network: unknown command rejection reason ID")
			}
			packet = protocol.CommandRejected{Sequence: sequence, Reason: reason}
		case 5:
			var token uint64
			token, err = d.u64()
			packet = protocol.KeepAlive{Token: token}
		case 6:
			var code uint8
			var message string
			code, err = d.u8()
			if err == nil {
				message, err = d.string(256, 256)
			}
			packet = protocol.Disconnect{Code: protocol.DisconnectCode(code), Message: message}
		case 7:
			var spawn protocol.RemotePlayerSpawn
			if data, readErr := d.take(len(spawn.PlayerID)); readErr != nil {
				err = readErr
			} else {
				copy(spawn.PlayerID[:], data)
				spawn.DisplayName, err = d.string(128, 32)
			}
			if err == nil {
				spawn.ServerTick, err = d.u64()
			}
			var dimension int32
			if err == nil {
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
			packet = spawn
		case 8:
			var despawn protocol.RemotePlayerDespawn
			if data, readErr := d.take(len(despawn.PlayerID)); readErr != nil {
				err = readErr
			} else {
				copy(despawn.PlayerID[:], data)
			}
			packet = despawn
		case 9:
			packet, err = decodeRemotePlayerStates(&d)
		case 11:
			packet, err = decodeItemDropUpserts(&d)
		case 12:
			packet, err = decodeItemDropRemoves(&d)
		case 10:
			var inventory core.Inventory
			inventory.Hotbar.Selected, err = d.u8()
			for index := range inventory.Hotbar.Slots {
				inventory.Hotbar.Slots[index], err = decodeItemStack(&d, err)
			}
			for index := range inventory.Backpack {
				inventory.Backpack[index], err = decodeItemStack(&d, err)
			}
			packet = protocol.InventoryState{Inventory: inventory}
		case 13:
			var state protocol.FurnaceState
			state.Furnace, err = decodeContainerRef(&d)
			state.Input, err = decodeItemStack(&d, err)
			state.Fuel, err = decodeItemStack(&d, err)
			state.Output, err = decodeItemStack(&d, err)
			if err == nil {
				state.ProgressTicks, err = d.u8()
			}
			if err == nil {
				state.BurnTicks, err = d.u16()
			}
			packet = state
		case 14:
			var closed protocol.ContainerClosed
			closed.Container, err = decodeContainerRef(&d)
			packet = closed
		case 15:
			var chest protocol.ChestState
			chest.Chest, err = decodeContainerRef(&d)
			for index := range chest.Items {
				chest.Items[index], err = decodeItemStack(&d, err)
			}
			packet = chest
		case 16:
			packet, err = decodeChatEvent(&d)
		case 17:
			packet, err = decodeCompanionSpawn(&d)
		case 18:
			packet, err = decodeCompanionStates(&d)
		case 19:
			var despawn protocol.CompanionDespawn
			err = decodeFixedID(&d, despawn.ID[:])
			packet = despawn
		case 20:
			var sequence uint64
			sequence, err = d.u64()
			packet = protocol.PlaceBlockSucceeded{Sequence: sequence}
		case 21:
			var state protocol.CraftingState
			state.Size, err = d.u8()
			for index := range state.Slots {
				state.Slots[index], err = decodeItemStack(&d, err)
			}
			state.Output, err = decodeItemStack(&d, err)
			packet = state
		case 22:
			packet, err = decodeHostileSpawn(&d)
		case 23:
			packet, err = decodeHostileState(&d)
		case 24:
			packet, err = decodeHostileDespawn(&d)
		case 25:
			if len(payload) != 10 {
				err = errors.New("network: combat hit payload must be exactly 10 bytes")
			} else {
				var hit protocol.CombatHit
				hit.ServerTick, err = d.u64()
				if err == nil {
					hit.Damage, err = d.u8()
				}
				if err == nil {
					var kind uint8
					kind, err = d.u8()
					hit.TargetKind = core.CombatTargetKind(kind)
				}
				packet = hit
			}
		default:
			return nil, codecError("decode server", state, packetID, errUnknownPacketID)
		}
	default:
		return nil, codecError("decode server", state, packetID, errUnknownPacketID)
	}
	if err == nil {
		err = d.done()
	}
	if err == nil {
		err = validateServerWirePacket(state, packet)
	}
	if err != nil {
		return nil, codecError("decode server", state, packetID, err)
	}
	return packet, nil
}

func validateServerWirePacket(state protocol.State, packet protocol.ServerPacket) error {
	if err := protocol.ValidateServerPacket(state, packet); err != nil {
		return err
	}
	switch message := packet.(type) {
	case protocol.BlockChanges:
		if message.Dimension != core.Overworld {
			return errInvalidDimension
		}
	case protocol.ForgetChunks:
		if message.Dimension != core.Overworld {
			return errInvalidDimension
		}
	case protocol.PlayerState:
		if message.Dimension != core.Overworld {
			return errInvalidDimension
		}
	}
	return nil
}
