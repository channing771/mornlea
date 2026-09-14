package codec

import (
	"encoding/hex"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// TestProtocolV42PlayerStateCarriesArmorPoints 覆盖 v42 追加的护甲点数：
// `ArmorPoints`（u8）是载荷最末一字节，紧跟 `Temperature` 之后，合法值域
// 0..`core.MaxArmorPoints` 全域往返保值。
//
// 样本覆盖 0、铁质满套 15 与上界 20：0 与「编码器漏写该字段、解码器读出
// 零值」不可分辨，故必须带上非零样本；上界 20 同时锁死「合法上界本身不被
// 越界拒绝」。相邻温度取 −8（0xf8）：纯零值样本无法分辨「点数与温度两字节
// 写反」的实现。
func TestProtocolV42PlayerStateCarriesArmorPoints(t *testing.T) {
	for _, points := range []uint8{0, 1, 15, core.MaxArmorPoints} {
		state := protocol.PlayerState{
			Dimension:      core.Overworld,
			WorldTimeTicks: 0x0102030405060708,
			WeatherKind:    core.WeatherRain,
			Season:         core.SeasonAutumn,
			Temperature:    -8,
			ArmorPoints:    points,
		}
		if err := state.Validate(); err != nil {
			t.Fatalf("护甲点数 %d 被 Validate 拒绝：%v", points, err)
		}
		id, payload, err := encodeServerControlPayload(protocol.StatePlay, state)
		if err != nil {
			t.Fatal(err)
		}
		if id != 3 {
			t.Fatalf("protocol.PlayerState packet ID = %d，想要 3", id)
		}

		// 点数恰好接在温度之后成为载荷最末一字节：末尾依次是 8 字节小端绝对
		// 世界时间、1 字节天气、1 字节季节、1 字节季内进度、1 字节温度（0xf8），
		// 最后 1 字节护甲点数。
		got := hex.EncodeToString(payload)
		wantSuffix := "0807060504030201" + "01" + "02" + "00" + "f8" +
			hex.EncodeToString([]byte{points})
		if len(got) < len(wantSuffix) || got[len(got)-len(wantSuffix):] != wantSuffix {
			t.Fatalf("护甲点数 %d 的 payload = %s，想要以 %s 结尾", points, got, wantSuffix)
		}

		round, err := decodeServerControlPayload(protocol.StatePlay, id, payload)
		if err != nil {
			t.Fatal(err)
		}
		if got := round.(protocol.PlayerState).ArmorPoints; got != points {
			t.Fatalf("往返护甲点数 = %d，想要 %d", got, points)
		}

		// payload 是固定长度：任何截断都必须被拒绝。
		for length := 0; length < len(payload); length++ {
			if _, err := decodeServerControlPayload(protocol.StatePlay, id, payload[:length]); err == nil {
				t.Fatalf("护甲点数 %d 截断到 %d 字节被接受", points, length)
			}
		}
		if _, err := decodeServerControlPayload(protocol.StatePlay, id, append(payload, 0)); err == nil {
			t.Fatalf("护甲点数 %d 多出尾随字节被接受", points)
		}
	}
}

// TestProtocolV42PlayerStateRejectsOutOfRangeArmorPoints 与天气/季节拒绝用例
// 同形：越界点数（wire 上 21..255 非法）必须在 Validate、编码与解码三处各自
// 被拒，且解码不得输出部分 protocol.PlayerState。取值 21 与 255 分别覆盖刚好
// 越界与全 1 两种形态：只测 21 的话，把高位置零的掩码实现照样绿。
func TestProtocolV42PlayerStateRejectsOutOfRangeArmorPoints(t *testing.T) {
	for _, points := range []uint8{core.MaxArmorPoints + 1, 255} {
		invalid := protocol.PlayerState{Dimension: core.Overworld, ArmorPoints: points}
		if err := invalid.Validate(); err == nil {
			t.Fatalf("越界护甲点数 %d 通过了 Validate", points)
		}
		if _, _, err := encodeServerControlPayload(protocol.StatePlay, invalid); err == nil {
			t.Fatalf("越界护甲点数 %d 被编码接受", points)
		}
	}

	// 构造一份合法 wire 载荷，再把最末一字节改写为越界值，验证解码器单独拒绝它。
	valid := protocol.PlayerState{
		Dimension:      core.Overworld,
		WorldTimeTicks: 24000,
		WeatherKind:    core.WeatherRain,
		ArmorPoints:    15,
	}
	id, payload, err := encodeServerControlPayload(protocol.StatePlay, valid)
	if err != nil {
		t.Fatal(err)
	}
	offset := playerStateArmorPointsOffset(len(payload))
	// 守卫排在真实断言之后：改写的必须确实是点数那一字节，否则下面的拒绝
	// 可能是被相邻字段（温度）越界顶掉的。
	if payload[offset] != 15 {
		t.Fatalf("夹具无效：armorPointsOffset 处是 %d，不是护甲点数 15", payload[offset])
	}
	for _, points := range []byte{core.MaxArmorPoints + 1, 255} {
		corrupted := append([]byte(nil), payload...)
		corrupted[offset] = points
		if packet, err := decodeServerControlPayload(protocol.StatePlay, id, corrupted); err == nil {
			t.Fatalf("越界护甲点数 %d 的 wire 载荷被解码接受: %#v", points, packet)
		}
	}
}

// TestEquipArmorCodecRoundTrip 覆盖 v42 新增的装备互换命令：Play C→S ID 18，
// 载荷只含 u64 序号（与 `DropSelectedItem` 同形），编解码必须对称往返；
// 零序号是合法值（序号语义由会话层解释，与既有命令惯例一致）；
// Handshake/Login 状态不接受它。
func TestEquipArmorCodecRoundTrip(t *testing.T) {
	if id, ok := protocol.ClientPacketID(protocol.StatePlay, protocol.EquipArmor{}); !ok || id != 18 {
		t.Fatalf("protocol.EquipArmor packet ID = (%d,%v)，想要 (18,true)", id, ok)
	}
	registered, ok := protocol.ClientPacketForID(protocol.StatePlay, 18)
	if !ok {
		t.Fatal("Play client packet ID 18 未注册")
	}
	if _, isEquip := registered.(protocol.EquipArmor); !isEquip {
		t.Fatalf("Play client packet ID 18 = %T，想要 protocol.EquipArmor", registered)
	}
	for _, sequence := range []uint64{0, 0x0102030405060708} {
		pkt := protocol.EquipArmor{Sequence: sequence}
		if err := pkt.Validate(); err != nil {
			t.Fatalf("序号 %d 被 Validate 拒绝：%v", sequence, err)
		}
		packetID, enc, err := encodeClientPacketPayload(protocol.StatePlay, pkt)
		if err != nil {
			t.Fatal(err)
		}
		if packetID != 18 {
			t.Fatalf("protocol.EquipArmor packet ID = %d，想要 18", packetID)
		}
		dec, err := decodeClientPacketPayload(protocol.StatePlay, packetID, enc)
		if err != nil {
			t.Fatal(err)
		}
		if dec != pkt {
			t.Fatalf("往返 = %+v，想要 %+v", dec, pkt)
		}
		// 载荷是固定长度：截断与尾随都必须被拒绝。
		for length := 0; length < len(enc); length++ {
			if _, err := decodeClientPacketPayload(protocol.StatePlay, packetID, enc[:length]); err == nil {
				t.Fatalf("序号 %d 截断到 %d 字节被接受", sequence, length)
			}
		}
		if _, err := decodeClientPacketPayload(protocol.StatePlay, packetID, append(enc, 0)); err == nil {
			t.Fatalf("序号 %d 多出尾随字节被接受", sequence)
		}
	}
	// 该命令在 Handshake 与 Login 阶段无效，只属于 Play。
	for _, state := range []protocol.State{protocol.StateHandshake, protocol.StateLogin} {
		if _, _, err := encodeClientPacketPayload(state, protocol.EquipArmor{}); err == nil {
			t.Fatalf("状态 %d 接受了 protocol.EquipArmor", state)
		}
	}
}
