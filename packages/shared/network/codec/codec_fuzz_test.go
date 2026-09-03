package codec

import (
	"github.com/channing771/mornlea/packages/shared/network/protocol"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

func FuzzSmallPacketCodec(f *testing.F) {
	f.Add(uint8(protocol.StateHandshake), uint32(0), []byte{2})
	f.Add(uint8(protocol.StateHandshake), uint32(0), []byte{1})
	f.Add(uint8(protocol.StateHandshake), uint32(0), []byte{3})
	// 当前 v32 与刚退役 v31 都必须进入握手解码语料，防止版本门禁误接回退。
	f.Add(uint8(protocol.StateHandshake), uint32(0), []byte{32})
	f.Add(uint8(protocol.StateHandshake), uint32(0), []byte{31})
	f.Add(uint8(protocol.StateLogin), uint32(0), []byte{0})
	// v23 protocol.LoginSuccess：16 字节 UUIDv4 + little-endian uint64 世界种子。
	f.Add(uint8(protocol.StateLogin), uint32(0), []byte{
		0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77,
		0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff,
		0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11,
	})
	f.Add(uint8(protocol.StatePlay), uint32(0), []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	f.Add(uint8(protocol.StatePlay), uint32(5), []byte{1, 0, 0, 0, 0, 0, 0, 0})
	// protocol.PlayerState 的种子由编码器现算，尾部字段（v21 起含 Oxygen、v24 起含 Hunger）
	// 一变就自动跟上，不会像手写字节那样悄悄退化成"截断的旧版载荷"。饥饿值取
	// 非零非满的中间值：满值样本进不了"越界饥饿必须被拒"的邻域。
	if id, payload, err := encodeServerControlPayload(protocol.StatePlay, protocol.PlayerState{
		Dimension: 0, Health: 15, Oxygen: 0x0101, Hunger: 12, WorldTimeTicks: 24000,
	}); err == nil {
		f.Add(uint8(protocol.StatePlay), id, payload)
	}
	// protocol.PlayerInput 的种子同理现算：v24 的进食位必须进入语料，且取 Mining=false、
	// Eating=true 这组能分辨"两个布尔字节写反"的组合。
	if id, payload, err := encodeClientPacketPayload(protocol.StatePlay, protocol.PlayerInput{
		Sequence: 0x0102030405060708, Yaw: 1.5, Pitch: -0.5, Eating: true,
	}); err == nil {
		f.Add(uint8(protocol.StatePlay), id, payload)
	}
	// protocol.TillSoil 的种子同样由编码器现算：v22 的唯一 wire 变化必须进入语料，
	// 且形状随字段变动自动跟上。
	if id, payload, err := encodeClientPacketPayload(protocol.StatePlay, protocol.TillSoil{
		Sequence: 0x0102030405060708, Yaw: 1.5, Pitch: -0.5,
	}); err == nil {
		f.Add(uint8(protocol.StatePlay), id, payload)
	}
	// v26 放置成功确认只携带原命令序号。
	if id, payload, err := encodeServerControlPayload(protocol.StatePlay, protocol.PlaceBlockSucceeded{
		Sequence: 0x0102030405060708,
	}); err == nil {
		f.Add(uint8(protocol.StatePlay), id, payload)
	}
	// 格子工作台三条消息的种子同样由编码器现算：网格移动取「背包→网格」与
	// 「网格→网格」两种形态，网格状态取尺寸 3 且格 0/4 非空的样本——尺寸字节、
	// 扩展格与产物格都进入语料。
	if id, payload, err := encodeClientPacketPayload(protocol.StatePlay, protocol.MoveCraftingStack{
		Sequence: 0x0102030405060708, From: 9, To: 0,
	}); err == nil {
		f.Add(uint8(protocol.StatePlay), id, payload)
	}
	if id, payload, err := encodeClientPacketPayload(protocol.StatePlay, protocol.TakeCraftingOutput{
		Sequence: 0x0102030405060708,
	}); err == nil {
		f.Add(uint8(protocol.StatePlay), id, payload)
	}
	if id, payload, err := encodeServerControlPayload(protocol.StatePlay, protocol.CraftingState{
		Size: 3,
		Slots: [core.CraftingGridSlots]core.ItemStack{
			0: {Item: core.ItemStone, Count: 2},
			4: {Item: core.ItemStick, Count: 1},
		},
		Output: core.ItemStack{Item: core.ItemStoneBrick, Count: 4},
	}); err == nil {
		f.Add(uint8(protocol.StatePlay), id, payload)
	}
	// 尺寸 2 的空扩展格样本：把尺寸字节从 3 改回 2 即得到合法变体，帮助 fuzzer
	// 在「尺寸边界 × 扩展格残留」的邻域里立刻撞上拒绝路径。
	if id, payload, err := encodeServerControlPayload(protocol.StatePlay, protocol.CraftingState{
		Size: 2,
		Slots: [core.CraftingGridSlots]core.ItemStack{
			0: {Item: core.ItemStone, Count: 1},
		},
	}); err == nil {
		f.Add(uint8(protocol.StatePlay), id, payload)
	}
	// v32 私有战斗命中确认的合法种子：tick 非零、伤害 6、hostile kind。
	if id, payload, err := encodeServerControlPayload(protocol.StatePlay, protocol.CombatHit{
		ServerTick: 0x0102030405060708, Damage: 6, TargetKind: core.CombatTargetHostile,
	}); err == nil {
		f.Add(uint8(protocol.StatePlay), id, payload)
	}
	f.Fuzz(func(t *testing.T, rawState uint8, packetID uint32, payload []byte) {
		state := protocol.State(rawState)
		if packet, err := decodeClientPacketPayload(state, packetID, payload); err == nil {
			isStateMachineRejection := false
			switch packet.(type) {
			case protocol.ClientHello:
				isStateMachineRejection = state == protocol.StateHandshake
			case protocol.LoginStart:
				isStateMachineRejection = state == protocol.StateLogin
			}
			if !isStateMachineRejection {
				gotID, gotPayload, encodeErr := encodeClientPacketPayload(state, packet)
				if encodeErr != nil || gotID != packetID || string(gotPayload) != string(payload) {
					t.Fatalf("client canonical round trip id=%d payload=%x; got id=%d payload=%x err=%v", packetID, payload, gotID, gotPayload, encodeErr)
				}
			}
		}
		if packet, err := decodeServerControlPayload(state, packetID, payload); err == nil {
			gotID, gotPayload, encodeErr := encodeServerControlPayload(state, packet)
			if encodeErr != nil || gotID != packetID || string(gotPayload) != string(payload) {
				t.Fatalf("server canonical round trip id=%d payload=%x; got id=%d payload=%x err=%v", packetID, payload, gotID, gotPayload, encodeErr)
			}
		}
	})
}
