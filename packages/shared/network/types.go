// Package network 定义端无关消息协议与传输接口。
//
// 拆分后根包保留会话与传输编排：登录状态机（login.go）、共享 stream 接口、
// Memory transport 与 Play endpoint 门面，外加对协议消息子包 `protocol` 与
// 编解码子包 `codec` 的别名再导出——既有 `network.X` 消费方（server/client/
// sim/cmd 与 tcp 子包）的引用逐符号保持可寻址、同名称、同类型/错误/常量身份，
// 源码零改动。类型别名（`type X = protocol.X`、`type Codec = codec.Codec`）
// 保方法集，常量与函数经 var/const 绑定同一底层值，不产生运行时转发。依赖
// 方向单向：network → {protocol, codec}，codec → protocol，任何子包不反向
// 依赖本包。
package network

import (
	"github.com/channing771/mornlea/packages/shared/network/codec"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// ClientPacket 是客户端可在线上发送的封闭 packet 集合，定义在 protocol
// 包（密封 marker 同包钉死）；类型别名保方法集与 switch 断言身份。
type ClientPacket = protocol.ClientPacket

// ServerPacket 是服务端可在线上发送的封闭 packet 集合，与 `ClientPacket`
// 同理别名再导出。
type ServerPacket = protocol.ServerPacket

// ClientMessage/ServerMessage 是 Play endpoint 收发的客户端/服务端消息封闭
// 集合，定义在 protocol 包；endpoint 门面（transport.go）经类型断言在二者与
// packet 之间转换，别名保证断言行为不变。
type ClientMessage = protocol.ClientMessage

// ServerMessage 是服务端可发送消息的封闭集合，见 `ClientMessage`。
type ServerMessage = protocol.ServerMessage

// State 标识连接当前允许交换的 packet 集合，定义在 protocol 包。
type State = protocol.State

// 连接状态常量随 `State` 定义在 protocol 包；再导出保持既有
// network.StateHandshake 等引用的身份与值不变。
const (
	// StateHandshake 是握手阶段：仅允许交换 Hello packet。
	StateHandshake = protocol.StateHandshake
	// StateLogin 是登录阶段：仅允许交换 Login 应答/发起 packet。
	StateLogin = protocol.StateLogin
	// StatePlay 是游戏阶段：允许交换全部业务 packet。
	StatePlay = protocol.StatePlay
)

// ProtocolVersion 是当前唯一支持的协议版本，定义在 protocol 包；完整版本
// 历史见彼处 doc comment。
const ProtocolVersion = protocol.ProtocolVersion

// 握手与登录 packet 及其拒绝码枚举定义在 protocol 包；再导出保持既有
// network.X 引用不变。
type (
	// ClientHello 是客户端握手请求。
	ClientHello = protocol.ClientHello
	// ServerHello 是服务端握手应答。
	ServerHello = protocol.ServerHello
	// HandshakeRejectCode 标识握手拒绝原因。
	HandshakeRejectCode = protocol.HandshakeRejectCode
	// HandshakeReject 是握手拒绝应答。
	HandshakeReject = protocol.HandshakeReject
	// LoginStart 是客户端登录发起。
	LoginStart = protocol.LoginStart
	// LoginSuccess 是登录成功应答。
	LoginSuccess = protocol.LoginSuccess
	// LoginRejectCode 标识登录拒绝原因。
	LoginRejectCode = protocol.LoginRejectCode
	// LoginReject 是登录拒绝应答。
	LoginReject = protocol.LoginReject
	// KeepAlive 是服务端心跳。
	KeepAlive = protocol.KeepAlive
	// KeepAliveReply 是客户端心跳应答。
	KeepAliveReply = protocol.KeepAliveReply
	// DisconnectCode 标识服务端主动断连原因。
	DisconnectCode = protocol.DisconnectCode
	// Disconnect 是服务端主动断连通知。
	Disconnect = protocol.Disconnect
)

// HandshakeVersionMismatch 是唯一的握手拒绝码，定义在 protocol 包。
const HandshakeVersionMismatch = protocol.HandshakeVersionMismatch

// 登录拒绝码枚举定义在 protocol 包，再导出保持既有引用与 wire 值不变。
const (
	// LoginServerFull 表示服务端并发会话已满。
	LoginServerFull = protocol.LoginServerFull
	// LoginInvalidIdentity 表示玩家 ID 或昵称未通过身份校验。
	LoginInvalidIdentity = protocol.LoginInvalidIdentity
	// LoginPlayerDataCorrupt 表示玩家存档数据损坏。
	LoginPlayerDataCorrupt = protocol.LoginPlayerDataCorrupt
	// LoginStoreUnavailable 表示玩家存储不可用。
	LoginStoreUnavailable = protocol.LoginStoreUnavailable
	// LoginProtocolViolation 表示登录过程违反协议。
	LoginProtocolViolation = protocol.LoginProtocolViolation
	// LoginInternalError 表示服务端内部错误。
	LoginInternalError = protocol.LoginInternalError
	// LoginAlreadyOnline 表示同一玩家已在会话中。
	LoginAlreadyOnline = protocol.LoginAlreadyOnline
)

// 断连原因枚举定义在 protocol 包，再导出保持既有引用与 wire 值不变。
const (
	// DisconnectProtocolViolation 表示协议违规断连。
	DisconnectProtocolViolation = protocol.DisconnectProtocolViolation
	// DisconnectTimeout 表示心跳超时断连。
	DisconnectTimeout = protocol.DisconnectTimeout
	// DisconnectServerShutdown 表示服务端停机断连。
	DisconnectServerShutdown = protocol.DisconnectServerShutdown
	// DisconnectSlowClient 表示消费过慢断连。
	DisconnectSlowClient = protocol.DisconnectSlowClient
	// DisconnectInternalError 表示服务端内部错误断连。
	DisconnectInternalError = protocol.DisconnectInternalError
)

// 主校验器定义在 protocol 包（全部消息的 Validate 同包扇出）；var 别名绑定
// 同一函数值，消费方经 `network.ValidateClientPacket` 与经
// `protocol.ValidateClientPacket` 调用完全等价。
var (
	// ValidateClientPacket 验证 state、client packet 类型及其线上字段。
	ValidateClientPacket = protocol.ValidateClientPacket
	// ValidateServerPacket 验证 state、server packet 类型及其线上字段。
	ValidateServerPacket = protocol.ValidateServerPacket
)

// 命令与玩家状态消息 DTO 定义在 protocol 包；再导出保持既有 network.X 引用
// 的类型身份（含 `Validate` 方法集）。
type (
	// PlayerInput 是客户端按键意图输入。
	PlayerInput = protocol.PlayerInput
	// PlaceBlock 是客户端放置方块命令。
	PlaceBlock = protocol.PlaceBlock
	// PlaceBlockSucceeded 是服务端对 PlaceBlock 的原子完成确认。
	PlaceBlockSucceeded = protocol.PlaceBlockSucceeded
	// SelectHotbar 是客户端快捷栏选择命令。
	SelectHotbar = protocol.SelectHotbar
	// RequestChunkResync 是客户端区块重同步请求。
	RequestChunkResync = protocol.RequestChunkResync
	// ForgetChunks 是服务端要求客户端放弃区块镜像的通知。
	ForgetChunks = protocol.ForgetChunks
	// DropSelectedItem 是客户端丢弃手持物品命令。
	DropSelectedItem = protocol.DropSelectedItem
	// TillSoil 是客户端翻地命令。
	TillSoil = protocol.TillSoil
	// BoneMeal 是客户端骨粉催熟命令。
	BoneMeal = protocol.BoneMeal
	// PlayerState 是服务端发给玩家本人的完整权威状态。
	PlayerState = protocol.PlayerState
	// RemotePlayerSpawn 是其他玩家进入同步范围的出生通知。
	RemotePlayerSpawn = protocol.RemotePlayerSpawn
	// RemotePlayerDespawn 是其他玩家离开同步范围的移除通知。
	RemotePlayerDespawn = protocol.RemotePlayerDespawn
	// RemotePlayerStates 是其他玩家的批量身体状态。
	RemotePlayerStates = protocol.RemotePlayerStates
	// RemotePlayerState 是单个远端玩家的身体状态。
	RemotePlayerState = protocol.RemotePlayerState
	// RejectReason 标识权威命令被拒绝的稳定原因值。
	RejectReason = protocol.RejectReason
	// CommandRejected 是对单个命令序号的拒绝应答。
	CommandRejected = protocol.CommandRejected
)

// 命令拒绝原因值定义在 protocol 包，再导出保持既有引用与 wire 编号不变。
const (
	// RejectInvalidRay 表示射线无效。
	RejectInvalidRay = protocol.RejectInvalidRay
	// RejectNoTarget 表示没有可作用目标。
	RejectNoTarget = protocol.RejectNoTarget
	// RejectChunkNotReady 表示区块尚未就绪。
	RejectChunkNotReady = protocol.RejectChunkNotReady
	// RejectProtectedBlock 表示目标方块受保护。
	RejectProtectedBlock = protocol.RejectProtectedBlock
	// RejectInvalidBlock 表示目标方块类型不合法。
	RejectInvalidBlock = protocol.RejectInvalidBlock
	// RejectOccupied 表示目标位置被占用。
	RejectOccupied = protocol.RejectOccupied
	// RejectInvalidInput 表示命令输入值域非法。
	RejectInvalidInput = protocol.RejectInvalidInput
	// RejectPlayerNotReady 表示玩家尚未就绪。
	RejectPlayerNotReady = protocol.RejectPlayerNotReady
	// RejectInvalidSlot 表示槽位引用越界。
	RejectInvalidSlot = protocol.RejectInvalidSlot
	// RejectHotbarFull 表示快捷栏已满。
	RejectHotbarFull = protocol.RejectHotbarFull
	// RejectDropCapacity 表示掉落物容量已满。
	RejectDropCapacity = protocol.RejectDropCapacity
	// RejectContainerCapacity 表示容器容量不足。
	RejectContainerCapacity = protocol.RejectContainerCapacity
)

// 物品栏与容器消息 DTO 定义在 protocol 包；再导出保持既有 network.X 引用。
type (
	// InventoryState 是服务端发给所属玩家的完整权威物品状态。
	InventoryState = protocol.InventoryState
	// MoveInventoryStack 是背包内整堆移动命令。
	MoveInventoryStack = protocol.MoveInventoryStack
	// MoveCraftingStack 是合成网格与背包之间的整堆移动命令。
	MoveCraftingStack = protocol.MoveCraftingStack
	// TakeCraftingOutput 是合成产物取出命令。
	TakeCraftingOutput = protocol.TakeCraftingOutput
	// CraftingState 是服务端发给网格所属玩家的完整合成网格状态。
	CraftingState = protocol.CraftingState
	// OpenContainer 是打开容器命令。
	OpenContainer = protocol.OpenContainer
	// MoveContainerStack 是容器统一栏位间的整堆移动命令。
	MoveContainerStack = protocol.MoveContainerStack
	// CloseContainer 是关闭容器命令。
	CloseContainer = protocol.CloseContainer
	// FurnaceState 是服务端发给当前查看者的完整熔炉状态。
	FurnaceState = protocol.FurnaceState
	// ChestState 是服务端发给当前查看者的完整箱子状态。
	ChestState = protocol.ChestState
	// ContainerClosed 是容器失效通知。
	ContainerClosed = protocol.ContainerClosed
)

// 掉落物消息 DTO 与批量上限定义在 protocol 包；再导出保持既有 network.X
// 引用与预分配上限值不变。
type (
	// ItemDrop 是一个权威掉落物堆的完整线上值。
	ItemDrop = protocol.ItemDrop
	// ItemDropUpserts 按稳定 ID 顺序新增或整体替换掉落物。
	ItemDropUpserts = protocol.ItemDropUpserts
	// ItemDropRemoves 按稳定 ID 顺序移除掉落物。
	ItemDropRemoves = protocol.ItemDropRemoves
)

// MaxItemDropBatch 是单条掉落物消息可携带的固定上限，定义在 protocol 包。
const MaxItemDropBatch = protocol.MaxItemDropBatch

// 聊天与伙伴消息 DTO、枚举定义在 protocol 包；再导出保持既有 network.X
// 引用（含组合校验 `Validate` 方法集）。
type (
	// ChatCommand 是客户端发送的有界聊天文本。
	ChatCommand = protocol.ChatCommand
	// ChatEventKind 标识聊天寻址与伙伴任务生命周期事件种类。
	ChatEventKind = protocol.ChatEventKind
	// ChatRejectReason 标识聊天寻址被拒绝的原因。
	ChatRejectReason = protocol.ChatRejectReason
	// TaskFailReason 是 TaskFailed 事件携带的稳定失败原因枚举。
	TaskFailReason = protocol.TaskFailReason
	// ChatEvent 是服务端在 tick 边界确认的聊天寻址事实。
	ChatEvent = protocol.ChatEvent
	// CompanionSpawn 是伙伴出生通知。
	CompanionSpawn = protocol.CompanionSpawn
	// CompanionState 是伙伴在一个 tick 的权威身体状态。
	CompanionState = protocol.CompanionState
	// CompanionStates 是按 ID 严格升序的有界伙伴状态批次。
	CompanionStates = protocol.CompanionStates
	// CompanionDespawn 是伙伴移除通知。
	CompanionDespawn = protocol.CompanionDespawn
)

// 聊天事件种类枚举定义在 protocol 包，再导出保持既有引用与 wire 值不变。
const (
	// ChatEventAccepted 表示聊天指令被伙伴接受。
	ChatEventAccepted = protocol.ChatEventAccepted
	// ChatEventRejected 表示聊天指令被拒绝。
	ChatEventRejected = protocol.ChatEventRejected
	// ChatEventTaskStarted 表示伙伴任务开始。
	ChatEventTaskStarted = protocol.ChatEventTaskStarted
	// ChatEventTaskProgress 表示伙伴任务推进。
	ChatEventTaskProgress = protocol.ChatEventTaskProgress
	// ChatEventTaskCompleted 表示伙伴任务完成。
	ChatEventTaskCompleted = protocol.ChatEventTaskCompleted
	// ChatEventTaskFailed 表示伙伴任务失败。
	ChatEventTaskFailed = protocol.ChatEventTaskFailed
	// ChatEventTaskTimedOut 表示伙伴任务超时。
	ChatEventTaskTimedOut = protocol.ChatEventTaskTimedOut
	// ChatEventTaskStopped 表示伙伴任务被停止旁路终结。
	ChatEventTaskStopped = protocol.ChatEventTaskStopped
	// ChatEventCompanionSpeech 表示伙伴台词事件。
	ChatEventCompanionSpeech = protocol.ChatEventCompanionSpeech
)

// 聊天拒绝原因枚举定义在 protocol 包，再导出保持既有引用与 wire 值不变。
const (
	// ChatRejectNone 表示未发生拒绝。
	ChatRejectNone = protocol.ChatRejectNone
	// ChatRejectInvalidFormat 表示指令格式非法。
	ChatRejectInvalidFormat = protocol.ChatRejectInvalidFormat
	// ChatRejectUnknownCompanion 表示寻址的伙伴不存在。
	ChatRejectUnknownCompanion = protocol.ChatRejectUnknownCompanion
	// ChatRejectQueueFull 表示伙伴任务队列已满。
	ChatRejectQueueFull = protocol.ChatRejectQueueFull
	// ChatRejectNotFollowing 表示目标伙伴没有可停止的持续任务。
	ChatRejectNotFollowing = protocol.ChatRejectNotFollowing
)

// 任务失败原因枚举定义在 protocol 包（16 起编号），再导出保持既有引用。
const (
	// TaskFailPlannerUnavailable 表示规划器不可用。
	TaskFailPlannerUnavailable = protocol.TaskFailPlannerUnavailable
	// TaskFailInvalidPlan 表示计划非法。
	TaskFailInvalidPlan = protocol.TaskFailInvalidPlan
	// TaskFailPathUnreachable 表示路径不可达。
	TaskFailPathUnreachable = protocol.TaskFailPathUnreachable
	// TaskFailWorldChanged 表示世界状态变化使任务失效。
	TaskFailWorldChanged = protocol.TaskFailWorldChanged
	// TaskFailInventoryFull 表示伙伴背包无容量。
	TaskFailInventoryFull = protocol.TaskFailInventoryFull
)

// 夜行者消息 DTO 定义在 protocol 包；再导出保持既有 network.X 引用。
type (
	// HostileSpawnRecord 是一只夜行者的出生事实。
	HostileSpawnRecord = protocol.HostileSpawnRecord
	// HostileSpawn 是夜行者出生批次通知。
	HostileSpawn = protocol.HostileSpawn
	// HostileStateRecord 是一只夜行者在一个权威 tick 的身体状态。
	HostileStateRecord = protocol.HostileStateRecord
	// HostileState 是夜行者状态批次。
	HostileState = protocol.HostileState
	// HostileDespawn 是夜行者移除批次通知。
	HostileDespawn = protocol.HostileDespawn
	// CombatHit 是私有战斗命中确认。
	CombatHit = protocol.CombatHit
)

// 被动牛消息 DTO 定义在 protocol 包；再导出保持既有 network.X 引用。
type (
	// PassiveSpawnRecord 是一头被动牛的出生事实。
	PassiveSpawnRecord = protocol.PassiveSpawnRecord
	// PassiveSpawn 是被动牛出生批次通知。
	PassiveSpawn = protocol.PassiveSpawn
	// PassiveStateRecord 是一头被动牛在一个权威 tick 的身体状态。
	PassiveStateRecord = protocol.PassiveStateRecord
	// PassiveState 是被动牛状态批次。
	PassiveState = protocol.PassiveState
	// PassiveDespawn 是被动牛移除批次通知。
	PassiveDespawn = protocol.PassiveDespawn
)

// 区块快照值类型定义在 protocol 包（密封接口与 Validate 同包）；再导出保持
// 既有 network.X 引用与编解码往返的类型身份。
type (
	// SectionStorage 标识区段编码存储方式。
	SectionStorage = protocol.SectionStorage
	// SectionData 是单个区段的压缩编码值。
	SectionData = protocol.SectionData
	// ChunkSnapshot 是一个区块的完整权威快照。
	ChunkSnapshot = protocol.ChunkSnapshot
	// BlockChange 是单个方块变更。
	BlockChange = protocol.BlockChange
	// BlockChanges 是同一区块内的一批方块变更。
	BlockChanges = protocol.BlockChanges
)

// 区段存储方式枚举定义在 protocol 包，再导出保持既有引用与 wire 值不变。
const (
	// SectionSingle 表示整段单一方块值存储。
	SectionSingle = protocol.SectionSingle
	// SectionIndexed 表示调色板索引存储。
	SectionIndexed = protocol.SectionIndexed
	// SectionDirect 表示全量直存。
	SectionDirect = protocol.SectionDirect
)

// Codec 是 Play 阶段 packet 与区块快照的编解码器，定义在 codec 包（zstd 快照
// 信封 + 全部控制 packet 的 wire 编解码门面）；类型别名保方法集，tcp 传输与
// 根包会话路径经既有 `network.Codec` 引用继续可用。
type Codec = codec.Codec

// 快照信封与帧边界常量定义在 codec 包，是 wire 契约的一部分；再导出保持既有
// network.X 引用与值不变。
const (
	// MaxCompressedSnapshot 是单条快照信封压缩载荷的字节上限。
	MaxCompressedSnapshot = codec.MaxCompressedSnapshot
	// MaxDecodedSnapshot 是解压后逻辑快照的字节上限。
	MaxDecodedSnapshot = codec.MaxDecodedSnapshot
	// MaxSmallPayload 是单个控制 packet 载荷的字节上限。
	MaxSmallPayload = codec.MaxSmallPayload
	// MaxFrameBytes 是单帧（packet ID + 载荷）的字节上限。
	MaxFrameBytes = codec.MaxFrameBytes
)

// 帧读写函数定义在 codec 包（帧封装复用 codec 的 canonical uvarint）；var
// 别名绑定同一函数值，tcp 传输经 `network.WriteFrame`/`network.ReadFrame`
// 的既有引用与直接调用 `codec.WriteFrame` 完全等价。
var (
	// WriteFrame 写入一条长度前缀封帧的 packet ID 与载荷。
	WriteFrame = codec.WriteFrame
	// ReadFrame 读取一条有界长度前缀封帧的 packet。
	ReadFrame = codec.ReadFrame
)

// NewCodec 创建 `Codec`；var 别名绑定同一函数值，消费方经
// `network.NewCodec` 与经 `codec.NewCodec` 调用完全等价。
var NewCodec = codec.NewCodec
