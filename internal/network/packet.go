package network

import (
	"errors"
	"fmt"

	"github.com/channing771/mornlea/internal/core"
)

// ProtocolVersion 是当前唯一支持的协议版本；v30 新增 Play S→C ID 22/23/24
// 三类夜行者消息 `HostileSpawn`/`HostileState`/`HostileDespawn`（每类
// `ServerTick` u64 + count u8 + ≤64 条按 ID 严格升序的 record；spawn 携带
// ID/dimension/position/yaw/health，state 携带 ID/position/velocity/yaw/health，
// despawn 只携带 ID），并维护旧客户端握手拒绝语义；v29 在 `PlayerState` 尾部追加
// `SaturationZero` 饱和度归零提示位（紧跟 `Hunger` 之后、`WorldTimeTicks` 之前）；v28 在 `PlayerInput` 尾部追加 `Sprinting` 疾跑位（紧跟 `Eating` 之后）；v27 新增 Play C→S ID 14 `BoneMeal`，v26 新增 Play S→C ID 20 `PlaceBlockSucceeded`，v25 只扩展既有 `Mining` 位语义不新增字段，v24 上线权威饥饿 Eating/Hunger 并拒绝 v23 及更早登录。
//
// v30 是纯追加：不新增 C→S 消息、不改动既有 packet 的 wire 形状与全部长度
// 上限、不新增 `RejectReason`。v29/v28/v24 同为既有 packet 尾部追加：
//
//   - `PlayerInput`（Play/C→S ID 0）末尾追加 1 字节 `Sprinting`，紧跟 `Eating` 之后。
//     三者同形：客户端只声明按键意图，权威结算全在服务端。
//   - `PlayerState`（Play/S→C ID 3）在 v24 已追加 1 字节 `Hunger`，在 v29 再追加 1 字节 `SaturationZero`，落在 `Hunger` 之后、`WorldTimeTicks` 之前。三层饥饿状态里只有饥饿值与零提示位上线，饱和度与疲劳值是纯服务端量、不占 wire 字段（design.md D6，B-12 仅追加 1 位抖动提示）。
//
// 历史：v23 在 `LoginSuccess` 追加 `WorldSeed`（u64，wire 上紧跟 `PlayerID` 之后），
// 供客户端确定性生成远环壳——该段在旧基线上原编号 v18，main 合并 fluid 系列
// 已占用至 v21、authoritative-farming 合并后占用 v22，故整体重编为 v23；
// v22 追加客户端翻地命令（Play/C→S 多出 ID 13 的 `TillSoil`，u64 序号 +
// 两个 f32 朝向，与 `OpenContainer` 同形，拒绝路径全部复用既有 `RejectReason`）；
// v21 在 `PlayerState` 末尾追加 2 字节权威氧气（只发给玩家本人的权威
// 值）；v20 追加 8 个流体方块编号（只扩方块 ID 集合，wire 形状不变），流体
// 变更走既有区块变更通道（design.md D8）。
const ProtocolVersion uint32 = 30

// State 标识连接当前允许交换的 packet 集合。
type State uint8

const (
	StateHandshake State = iota + 1
	StateLogin
	StatePlay
)

// ClientPacket 是客户端可在线上发送的封闭 packet 集合。
type ClientPacket interface {
	clientPacket()
}

// ServerPacket 是服务端可在线上发送的封闭 packet 集合。
type ServerPacket interface {
	serverPacket()
}

type ClientHello struct {
	ProtocolVersion uint32
}

func (ClientHello) clientPacket() {}

type ServerHello struct {
	ProtocolVersion uint32
}

func (ServerHello) serverPacket() {}

type HandshakeRejectCode uint8

const (
	HandshakeVersionMismatch HandshakeRejectCode = 1
)

type HandshakeReject struct {
	ServerProtocolVersion uint32
	Code                  HandshakeRejectCode
	Message               string
}

func (HandshakeReject) serverPacket() {}

type LoginStart struct {
	PlayerID    core.PlayerID
	DisplayName string
}

func (LoginStart) clientPacket() {}

// LoginSuccess 是服务端对成功登录的应答。WorldSeed 是权威世界种子的
// 无损位视图（int64 按 two's complement 转 uint64 下发），编码为
// PlayerID 之后的 little-endian uint64；客户端用它本地生成同步半径外的
// 确定性远环壳。wire 层不对该值做任何隐式校验：0 是合法种子。
type LoginSuccess struct {
	PlayerID  core.PlayerID
	WorldSeed uint64
}

func (LoginSuccess) serverPacket() {}

type LoginRejectCode uint8

const (
	LoginServerFull LoginRejectCode = iota + 1
	LoginInvalidIdentity
	LoginPlayerDataCorrupt
	LoginStoreUnavailable
	LoginProtocolViolation
	LoginInternalError
	LoginAlreadyOnline
)

type LoginReject struct {
	Code    LoginRejectCode
	Message string
}

func (LoginReject) serverPacket() {}

type KeepAlive struct {
	Token uint64
}

func (KeepAlive) serverPacket()  {}
func (KeepAlive) serverMessage() {}

type KeepAliveReply struct {
	Token uint64
}

func (KeepAliveReply) clientPacket()  {}
func (KeepAliveReply) clientMessage() {}

type DisconnectCode uint8

const (
	DisconnectProtocolViolation DisconnectCode = iota + 1
	DisconnectTimeout
	DisconnectServerShutdown
	DisconnectSlowClient
	DisconnectInternalError
)

type Disconnect struct {
	Code    DisconnectCode
	Message string
}

func (Disconnect) serverPacket()  {}
func (Disconnect) serverMessage() {}

// ValidateClientPacket 验证 state、packet 类型及其线上字段。
func ValidateClientPacket(state State, packet ClientPacket) error {
	switch state {
	case StateHandshake:
		clientHello, ok := packet.(ClientHello)
		if !ok {
			return invalidClientPacket(state, packet)
		}
		if clientHello.ProtocolVersion != ProtocolVersion {
			return fmt.Errorf("network: unsupported client protocol version %d", clientHello.ProtocolVersion)
		}
		return nil
	case StateLogin:
		loginStart, ok := packet.(LoginStart)
		if !ok {
			return invalidClientPacket(state, packet)
		}
		if !loginStart.PlayerID.Valid() {
			return errors.New("network: login player ID is not UUIDv4")
		}
		if _, err := core.NormalizeDisplayName(loginStart.DisplayName); err != nil {
			return fmt.Errorf("network: invalid login display name: %w", err)
		}
		return nil
	case StatePlay:
		switch clientPacket := packet.(type) {
		case PlayerInput:
			return clientPacket.Validate()
		case PlaceBlock:
			return clientPacket.Validate()
		case SelectHotbar:
			return clientPacket.Validate()
		case MoveInventoryStack:
			return clientPacket.Validate()
		case OpenContainer:
			return clientPacket.Validate()
		case MoveContainerStack:
			return clientPacket.Validate()
		case CloseContainer:
			return clientPacket.Validate()
		case DropSelectedItem:
			return clientPacket.Validate()
		case ChatCommand:
			return clientPacket.Validate()
		case TillSoil:
			return clientPacket.Validate()
		case BoneMeal:
			return clientPacket.Validate()
		case MoveCraftingStack:
			return clientPacket.Validate()
		case TakeCraftingOutput:
			return clientPacket.Validate()
		case RequestChunkResync:
			return clientPacket.Validate()
		case KeepAliveReply:
			if clientPacket.Token == 0 {
				return errors.New("network: keep alive reply token is zero")
			}
			return nil
		default:
			return invalidClientPacket(state, packet)
		}
	default:
		return invalidClientPacket(state, packet)
	}
}

// ValidateServerPacket 验证 state、packet 类型及其线上字段。
func ValidateServerPacket(state State, packet ServerPacket) error {
	switch state {
	case StateHandshake:
		switch serverPacket := packet.(type) {
		case ServerHello:
			if serverPacket.ProtocolVersion != ProtocolVersion {
				return fmt.Errorf("network: unsupported server protocol version %d", serverPacket.ProtocolVersion)
			}
			return nil
		case HandshakeReject:
			if !validHandshakeRejectCode(serverPacket.Code) {
				return fmt.Errorf("network: invalid handshake reject code %d", serverPacket.Code)
			}
			return nil
		default:
			return invalidServerPacket(state, packet)
		}
	case StateLogin:
		switch serverPacket := packet.(type) {
		case LoginSuccess:
			if !serverPacket.PlayerID.Valid() {
				return errors.New("network: login success player ID is not UUIDv4")
			}
			return nil
		case LoginReject:
			if !validLoginRejectCode(serverPacket.Code) {
				return fmt.Errorf("network: invalid login reject code %d", serverPacket.Code)
			}
			return nil
		default:
			return invalidServerPacket(state, packet)
		}
	case StatePlay:
		switch serverPacket := packet.(type) {
		case ChunkSnapshot:
			return serverPacket.Validate()
		case BlockChanges:
			return serverPacket.Validate()
		case ForgetChunks:
			return serverPacket.Validate()
		case PlayerState:
			return serverPacket.Validate()
		case CommandRejected:
			return serverPacket.Validate()
		case PlaceBlockSucceeded:
			return nil
		case CraftingState:
			return serverPacket.Validate()
		case KeepAlive:
			if serverPacket.Token == 0 {
				return errors.New("network: keep alive token is zero")
			}
			return nil
		case Disconnect:
			if !validDisconnectCode(serverPacket.Code) {
				return fmt.Errorf("network: invalid disconnect code %d", serverPacket.Code)
			}
			return nil
		case RemotePlayerSpawn:
			return serverPacket.Validate()
		case RemotePlayerDespawn:
			return serverPacket.Validate()
		case RemotePlayerStates:
			return serverPacket.Validate()
		case InventoryState:
			return serverPacket.Validate()
		case ItemDropUpserts:
			return serverPacket.Validate()
		case ItemDropRemoves:
			return serverPacket.Validate()
		case FurnaceState:
			return serverPacket.Validate()
		case ChestState:
			return serverPacket.Validate()
		case ContainerClosed:
			return serverPacket.Validate()
		case ChatEvent:
			return serverPacket.Validate()
		case CompanionSpawn:
			return serverPacket.Validate()
		case CompanionStates:
			return serverPacket.Validate()
		case CompanionDespawn:
			return serverPacket.Validate()
		case HostileSpawn:
			return serverPacket.Validate()
		case HostileState:
			return serverPacket.Validate()
		case HostileDespawn:
			return serverPacket.Validate()
		default:
			return invalidServerPacket(state, packet)
		}
	default:
		return invalidServerPacket(state, packet)
	}
}

func validHandshakeRejectCode(code HandshakeRejectCode) bool {
	return code == HandshakeVersionMismatch
}

func validLoginRejectCode(code LoginRejectCode) bool {
	switch code {
	case LoginServerFull, LoginInvalidIdentity, LoginPlayerDataCorrupt,
		LoginStoreUnavailable, LoginProtocolViolation, LoginInternalError, LoginAlreadyOnline:
		return true
	default:
		return false
	}
}

func validDisconnectCode(code DisconnectCode) bool {
	switch code {
	case DisconnectProtocolViolation, DisconnectTimeout, DisconnectServerShutdown,
		DisconnectSlowClient, DisconnectInternalError:
		return true
	default:
		return false
	}
}

func invalidClientPacket(state State, packet ClientPacket) error {
	return fmt.Errorf("network: client packet %T is not valid in state %d", packet, state)
}

func invalidServerPacket(state State, packet ServerPacket) error {
	return fmt.Errorf("network: server packet %T is not valid in state %d", packet, state)
}
