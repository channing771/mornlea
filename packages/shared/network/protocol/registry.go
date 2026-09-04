package protocol

// ClientPacketID 返回 packet 在 state 下的冻结 V1 包 ID；该 state 未注册
// 此 packet 类型时第二返回值为 false。导出供编解码层在编码与按 ID 解码时
// 查表，是协议冻结契约面的一部分。
func ClientPacketID(state State, packet ClientPacket) (uint32, bool) {
	switch state {
	case StateHandshake:
		_, ok := packet.(ClientHello)
		return 0, ok
	case StateLogin:
		_, ok := packet.(LoginStart)
		return 0, ok
	case StatePlay:
		switch packet.(type) {
		case PlayerInput:
			return 0, true
		case PlaceBlock:
			return 2, true
		case RequestChunkResync:
			return 3, true
		case KeepAliveReply:
			return 4, true
		case SelectHotbar:
			return 5, true
		case MoveInventoryStack:
			return 6, true
		case MoveCraftingStack:
			return 7, true
		case OpenContainer:
			return 8, true
		case MoveContainerStack:
			return 9, true
		case CloseContainer:
			return 10, true
		case DropSelectedItem:
			return 11, true
		case ChatCommand:
			return 12, true
		case TillSoil:
			return 13, true
		case BoneMeal:
			return 14, true
		case TakeCraftingOutput:
			return 15, true
		}
	}
	return 0, false
}

// ClientPacketForID 返回 state 下冻结 V1 包 ID 对应的空 packet 值；
// ID 未注册时第二返回值为 false。与 `ClientPacketID` 构成双向查表。
func ClientPacketForID(state State, id uint32) (ClientPacket, bool) {
	switch state {
	case StateHandshake:
		if id == 0 {
			return ClientHello{}, true
		}
	case StateLogin:
		if id == 0 {
			return LoginStart{}, true
		}
	case StatePlay:
		switch id {
		case 0:
			return PlayerInput{}, true
		case 2:
			return PlaceBlock{}, true
		case 3:
			return RequestChunkResync{}, true
		case 4:
			return KeepAliveReply{}, true
		case 5:
			return SelectHotbar{}, true
		case 6:
			return MoveInventoryStack{}, true
		case 7:
			return MoveCraftingStack{}, true
		case 8:
			return OpenContainer{}, true
		case 9:
			return MoveContainerStack{}, true
		case 10:
			return CloseContainer{}, true
		case 11:
			return DropSelectedItem{}, true
		case 12:
			return ChatCommand{}, true
		case 13:
			return TillSoil{}, true
		case 14:
			return BoneMeal{}, true
		case 15:
			return TakeCraftingOutput{}, true
		}
	}
	return nil, false
}

// ServerPacketID 返回 packet 在 state 下的冻结 V1 包 ID；该 state 未注册
// 此 packet 类型时第二返回值为 false。导出供编解码层在编码与按 ID 解码时
// 查表，是协议冻结契约面的一部分。
func ServerPacketID(state State, packet ServerPacket) (uint32, bool) {
	switch state {
	case StateHandshake:
		switch packet.(type) {
		case ServerHello:
			return 0, true
		case HandshakeReject:
			return 1, true
		}
	case StateLogin:
		switch packet.(type) {
		case LoginSuccess:
			return 0, true
		case LoginReject:
			return 1, true
		}
	case StatePlay:
		switch packet.(type) {
		case ChunkSnapshot:
			return 0, true
		case BlockChanges:
			return 1, true
		case ForgetChunks:
			return 2, true
		case PlayerState:
			return 3, true
		case CommandRejected:
			return 4, true
		case KeepAlive:
			return 5, true
		case Disconnect:
			return 6, true
		case RemotePlayerSpawn:
			return 7, true
		case RemotePlayerDespawn:
			return 8, true
		case RemotePlayerStates:
			return 9, true
		case InventoryState:
			return 10, true
		case ItemDropUpserts:
			return 11, true
		case ItemDropRemoves:
			return 12, true
		case FurnaceState:
			return 13, true
		case ContainerClosed:
			return 14, true
		case ChestState:
			return 15, true
		case ChatEvent:
			return 16, true
		case CompanionSpawn:
			return 17, true
		case CompanionStates:
			return 18, true
		case CompanionDespawn:
			return 19, true
		case PlaceBlockSucceeded:
			return 20, true
		// 格子工作台的网格状态：始终完整 9 格 + 产物格，只发所属玩家。
		case CraftingState:
			return 21, true
		// 夜行者三类 S→C 消息：spawn/state/despawn 依次占用 22/23/24。
		case HostileSpawn:
			return 22, true
		case HostileState:
			return 23, true
		case HostileDespawn:
			return 24, true
		// 私有战斗命中确认：固定 10-byte `CombatHit` 占用 25。
		case CombatHit:
			return 25, true
		// 被动牛三类 S→C 消息：spawn/state/despawn 依次占用 26/27/28，下一 ID 29 仍未分配。
		case PassiveSpawn:
			return 26, true
		case PassiveState:
			return 27, true
		case PassiveDespawn:
			return 28, true
		}
	}
	return 0, false
}

// ServerPacketForID 返回 state 下冻结 V1 包 ID 对应的空 packet 值；
// ID 未注册时第二返回值为 false。与 `ServerPacketID` 构成双向查表。
func ServerPacketForID(state State, id uint32) (ServerPacket, bool) {
	switch state {
	case StateHandshake:
		switch id {
		case 0:
			return ServerHello{}, true
		case 1:
			return HandshakeReject{}, true
		}
	case StateLogin:
		switch id {
		case 0:
			return LoginSuccess{}, true
		case 1:
			return LoginReject{}, true
		}
	case StatePlay:
		switch id {
		case 0:
			return ChunkSnapshot{}, true
		case 1:
			return BlockChanges{}, true
		case 2:
			return ForgetChunks{}, true
		case 3:
			return PlayerState{}, true
		case 4:
			return CommandRejected{}, true
		case 5:
			return KeepAlive{}, true
		case 6:
			return Disconnect{}, true
		case 7:
			return RemotePlayerSpawn{}, true
		case 8:
			return RemotePlayerDespawn{}, true
		case 9:
			return RemotePlayerStates{}, true
		case 10:
			return InventoryState{}, true
		case 11:
			return ItemDropUpserts{}, true
		case 12:
			return ItemDropRemoves{}, true
		case 13:
			return FurnaceState{}, true
		case 14:
			return ContainerClosed{}, true
		case 15:
			return ChestState{}, true
		case 16:
			return ChatEvent{}, true
		case 17:
			return CompanionSpawn{}, true
		case 18:
			return CompanionStates{}, true
		case 19:
			return CompanionDespawn{}, true
		case 20:
			return PlaceBlockSucceeded{}, true
		case 21:
			return CraftingState{}, true
		// 夜行者三类 S→C 消息：与 `ServerPacketID` 的 22/23/24 对称。
		case 22:
			return HostileSpawn{}, true
		case 23:
			return HostileState{}, true
		case 24:
			return HostileDespawn{}, true
		// 私有战斗命中确认：与 `ServerPacketID` 的 25 对称。
		case 25:
			return CombatHit{}, true
		// 被动牛三类 S→C 消息：与 `ServerPacketID` 的 26/27/28 对称。
		case 26:
			return PassiveSpawn{}, true
		case 27:
			return PassiveState{}, true
		case 28:
			return PassiveDespawn{}, true
		}
	}
	return nil, false
}

// CommandRejectReasonID 返回 `RejectReason` 在 `CommandRejected` wire
// 槽位上的冻结编号；未注册原因第二返回值为 false。
func CommandRejectReasonID(reason RejectReason) (uint8, bool) {
	switch reason {
	case RejectInvalidRay:
		return 1, true
	case RejectNoTarget:
		return 2, true
	case RejectChunkNotReady:
		return 3, true
	case RejectProtectedBlock:
		return 4, true
	case RejectInvalidBlock:
		return 5, true
	case RejectOccupied:
		return 6, true
	case RejectInvalidInput:
		return 7, true
	case RejectPlayerNotReady:
		return 8, true
	case RejectInvalidSlot:
		return 9, true
	case RejectHotbarFull:
		return 10, true
	case RejectDropCapacity:
		return 11, true
	case RejectContainerCapacity:
		return 12, true
	default:
		return 0, false
	}
}

// CommandRejectReasonForID 是 `CommandRejectReasonID` 的逆查表；未注册
// 编号第二返回值为 false。
func CommandRejectReasonForID(id uint8) (RejectReason, bool) {
	switch id {
	case 1:
		return RejectInvalidRay, true
	case 2:
		return RejectNoTarget, true
	case 3:
		return RejectChunkNotReady, true
	case 4:
		return RejectProtectedBlock, true
	case 5:
		return RejectInvalidBlock, true
	case 6:
		return RejectOccupied, true
	case 7:
		return RejectInvalidInput, true
	case 8:
		return RejectPlayerNotReady, true
	case 9:
		return RejectInvalidSlot, true
	case 10:
		return RejectHotbarFull, true
	case 11:
		return RejectDropCapacity, true
	case 12:
		return RejectContainerCapacity, true
	default:
		return "", false
	}
}
