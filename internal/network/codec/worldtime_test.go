package codec

import (
	"encoding/hex"
	"fmt"
	"github.com/channing771/mornlea/internal/network/protocol"
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

func TestProtocolV9PlayerStateCarriesWorldTime(t *testing.T) {
	state := protocol.PlayerState{
		Dimension:      core.Overworld,
		ServerTick:     1,
		WorldTimeTicks: 0x0102030405060708,
	}
	id, payload, err := encodeServerControlPayload(protocol.StatePlay, state)
	if err != nil {
		t.Fatal(err)
	}
	if id != 3 {
		t.Fatalf("protocol.PlayerState packet ID = %d，想要 3", id)
	}

	// 绝对世界时间恰好追加在既有固定 payload 末尾。
	got := hex.EncodeToString(payload)
	const wantSuffix = "0807060504030201"
	if len(got) < len(wantSuffix) || got[len(got)-len(wantSuffix):] != wantSuffix {
		t.Fatalf("payload = %s，想要以 %s 结尾", got, wantSuffix)
	}

	round, err := decodeServerControlPayload(protocol.StatePlay, id, payload)
	if err != nil {
		t.Fatal(err)
	}
	if round != protocol.ServerPacket(state) {
		t.Fatalf("往返 = %#v，想要 %#v", round, state)
	}

	// payload 是固定长度：任何截断都必须被拒绝。
	for length := 0; length < len(payload); length++ {
		if _, err := decodeServerControlPayload(protocol.StatePlay, id, payload[:length]); err == nil {
			t.Fatalf("截断到 %d 字节被接受", length)
		}
	}
	if _, err := decodeServerControlPayload(protocol.StatePlay, id, append(payload, 0)); err == nil {
		t.Fatal("多出尾随字节被接受")
	}
}

func TestProtocolV9PlayerStateWorldTimeAcceptsFullRange(t *testing.T) {
	for _, ticks := range []uint64{0, 23999, 24000, 1 << 40, ^uint64(0)} {
		state := protocol.PlayerState{Dimension: core.Overworld, WorldTimeTicks: ticks}
		if err := state.Validate(); err != nil {
			t.Fatalf("时间 %d 被 Validate 拒绝：%v", ticks, err)
		}
		id, payload, err := encodeServerControlPayload(protocol.StatePlay, state)
		if err != nil {
			t.Fatal(err)
		}
		round, err := decodeServerControlPayload(protocol.StatePlay, id, payload)
		if err != nil {
			t.Fatal(err)
		}
		if round.(protocol.PlayerState).WorldTimeTicks != ticks {
			t.Fatalf("往返时间 = %d，想要 %d", round.(protocol.PlayerState).WorldTimeTicks, ticks)
		}
	}
}

func TestProtocolV14PlayerStateCarriesHealth(t *testing.T) {
	// 生命值恰好追加在既有采掘字段之后、世界时间之前的固定偏移。
	for _, health := range []uint8{0, 1, core.MaxHealth} {
		state := protocol.PlayerState{Dimension: core.Overworld, Health: health, WorldTimeTicks: 24000}
		if err := state.Validate(); err != nil {
			t.Fatalf("生命值 %d 被 Validate 拒绝：%v", health, err)
		}
		id, payload, err := encodeServerControlPayload(protocol.StatePlay, state)
		if err != nil {
			t.Fatal(err)
		}
		round, err := decodeServerControlPayload(protocol.StatePlay, id, payload)
		if err != nil {
			t.Fatal(err)
		}
		if round.(protocol.PlayerState).Health != health {
			t.Fatalf("往返生命值 = %d，想要 %d", round.(protocol.PlayerState).Health, health)
		}
	}
}

func TestProtocolV14PlayerStateRejectsOutOfRangeHealth(t *testing.T) {
	invalid := protocol.PlayerState{Dimension: core.Overworld, Health: core.MaxHealth + 1}
	if err := invalid.Validate(); err == nil {
		t.Fatal("越界生命值通过了 Validate")
	}
	if _, _, err := encodeServerControlPayload(protocol.StatePlay, invalid); err == nil {
		t.Fatal("越界生命值被编码接受")
	}

	// 构造一份合法 wire 载荷，再把生命值字节改写为越界值，验证解码器单独拒绝它，
	// 且不得输出部分 protocol.PlayerState。
	valid := protocol.PlayerState{Dimension: core.Overworld, Health: core.MaxHealth, WorldTimeTicks: 24000}
	id, payload, err := encodeServerControlPayload(protocol.StatePlay, valid)
	if err != nil {
		t.Fatal(err)
	}
	healthOffset := playerStateHealthOffset(len(payload))
	corrupted := append([]byte(nil), payload...)
	corrupted[healthOffset] = core.MaxHealth + 1
	if packet, err := decodeServerControlPayload(protocol.StatePlay, id, corrupted); err == nil {
		t.Fatalf("越界生命值 wire 载荷被解码接受: %#v", packet)
	}
	// 守卫排在真实断言之后：改写的必须确实是生命值字节，否则上面那条断言可能
	// 是被别的字段越界顶掉的（v21 就真实发生过一次）。
	//
	// 判据是"未改写的原始载荷在该偏移处正好是这份夹具编码进去的生命值"。这条
	// 断言的第一版写成了对比 corrupted 与 payload 的**相邻**字节、再解一次合法
	// 载荷的生命值：前者恒为 false（改写的是 healthOffset 那一字节，相邻字节
	// 当然没动），后者与偏移无关（只是重复了夹具编码了满血这件事）。两个子句
	// 都不看 healthOffset 指向哪里，因而恒真——把偏移改回错值照样 PASS，实测过。
	if payload[healthOffset] != core.MaxHealth {
		t.Fatalf("夹具无效：healthOffset 处是 %d，不是生命值 %d",
			payload[healthOffset], core.MaxHealth)
	}
}

// TestProtocolV21PlayerStateCarriesOxygen 覆盖 v21 追加的权威氧气：
// 它按 u16 小端紧跟在 Health 之后（v24 起其后还有 Hunger，再往后才是
// WorldTimeTicks），且往返保值。
func TestProtocolV21PlayerStateCarriesOxygen(t *testing.T) {
	for _, oxygen := range []uint16{0, 1, 0x0101, core.MaxOxygenTicks} {
		state := protocol.PlayerState{Dimension: core.Overworld, Oxygen: oxygen, WorldTimeTicks: 24000}
		if err := state.Validate(); err != nil {
			t.Fatalf("氧气 %d 被 Validate 拒绝：%v", oxygen, err)
		}
		id, payload, err := encodeServerControlPayload(protocol.StatePlay, state)
		if err != nil {
			t.Fatal(err)
		}
		round, err := decodeServerControlPayload(protocol.StatePlay, id, payload)
		if err != nil {
			t.Fatal(err)
		}
		if round.(protocol.PlayerState).Oxygen != oxygen {
			t.Fatalf("往返氧气 = %d，想要 %d", round.(protocol.PlayerState).Oxygen, oxygen)
		}
	}
}

// TestProtocolV21PlayerStateRejectsOutOfRangeOxygen 与生命值那条同形：
// 越界氧气必须在 Validate、编码与解码三处各自被拒。
func TestProtocolV21PlayerStateRejectsOutOfRangeOxygen(t *testing.T) {
	invalid := protocol.PlayerState{Dimension: core.Overworld, Oxygen: core.MaxOxygenTicks + 1}
	if err := invalid.Validate(); err == nil {
		t.Fatal("越界氧气通过了 Validate")
	}
	if _, _, err := encodeServerControlPayload(protocol.StatePlay, invalid); err == nil {
		t.Fatal("越界氧气被编码接受")
	}

	valid := protocol.PlayerState{Dimension: core.Overworld, Oxygen: core.MaxOxygenTicks, WorldTimeTicks: 24000}
	id, payload, err := encodeServerControlPayload(protocol.StatePlay, valid)
	if err != nil {
		t.Fatal(err)
	}
	oxygenOffset := playerStateOxygenOffset(len(payload))
	corrupted := append([]byte(nil), payload...)
	// 只动低字节即可越界：满值 300 = 0x012C，把低字节抬到 0xFF 得 0x01FF = 511。
	corrupted[oxygenOffset] = 0xFF
	if packet, err := decodeServerControlPayload(protocol.StatePlay, id, corrupted); err == nil {
		t.Fatalf("越界氧气 wire 载荷被解码接受: %#v", packet)
	}
	// 守卫排在真实断言之后：偏移必须真的指向氧气低字节。
	if payload[oxygenOffset] != byte(core.MaxOxygenTicks&0xFF) ||
		payload[oxygenOffset+1] != byte(core.MaxOxygenTicks>>8) {
		t.Fatal("夹具无效：oxygenOffset 没有指向氧气的两个字节")
	}
}

// TestProtocolV31PlayerStateCarriesDayPhaseOffset 覆盖 v31 追加的显示相位偏移：
// 它按 u16 小端紧跟在 `SaturationZero` 之后、`WorldTimeTicks` 之前，往返保值。
// 偏移取 12399（0x3063）这个既非 0 也非上界的中间值：取 0 与「编码器根本没写
// 这个字段」不可分辨，取上界又容易与截断/越界路径巧合重合。
func TestProtocolV31PlayerStateCarriesDayPhaseOffset(t *testing.T) {
	for _, offset := range []uint16{0, 1, 12399, 23999} {
		state := protocol.PlayerState{Dimension: core.Overworld, DayPhaseOffset: offset, WorldTimeTicks: 0x0102030405060708}
		if err := state.Validate(); err != nil {
			t.Fatalf("偏移 %d 被 Validate 拒绝：%v", offset, err)
		}
		id, payload, err := encodeServerControlPayload(protocol.StatePlay, state)
		if err != nil {
			t.Fatal(err)
		}
		if id != 3 {
			t.Fatalf("protocol.PlayerState packet ID = %d，想要 3", id)
		}

		// 偏移恰好落在 `SaturationZero` 与 `WorldTimeTicks` 之间：载荷末尾依次
		// 是偏移的低/高字节，再接 8 字节小端绝对世界时间。
		got := hex.EncodeToString(payload)
		wantSuffix := fmt.Sprintf("%02x%02x", offset&0xFF, offset>>8) + "0807060504030201"
		if len(got) < len(wantSuffix) || got[len(got)-len(wantSuffix):] != wantSuffix {
			t.Fatalf("偏移 %d 的 payload = %s，想要以 %s 结尾", offset, got, wantSuffix)
		}

		round, err := decodeServerControlPayload(protocol.StatePlay, id, payload)
		if err != nil {
			t.Fatal(err)
		}
		if round.(protocol.PlayerState).DayPhaseOffset != offset {
			t.Fatalf("往返偏移 = %d，想要 %d", round.(protocol.PlayerState).DayPhaseOffset, offset)
		}

		// payload 是固定长度：任何截断都必须被拒绝。
		for length := 0; length < len(payload); length++ {
			if _, err := decodeServerControlPayload(protocol.StatePlay, id, payload[:length]); err == nil {
				t.Fatalf("截断到 %d 字节被接受", length)
			}
		}
		if _, err := decodeServerControlPayload(protocol.StatePlay, id, append(payload, 0)); err == nil {
			t.Fatal("多出尾随字节被接受")
		}
	}
}

// TestProtocolV31PlayerStateRejectsOutOfRangeDayPhaseOffset 与氧气/生命值拒绝
// 用例同形：越界偏移（>23999）必须在 Validate、编码与解码三处各自被拒，且解码
// 不得输出部分 protocol.PlayerState。wire 侧从严拒绝是刻意的语义分界——存储侧（世界
// metadata 装配）对历史存档里的越界旧值宽容归一，wire 侧只传播权威单值、没有
// 兼容包袱。
func TestProtocolV31PlayerStateRejectsOutOfRangeDayPhaseOffset(t *testing.T) {
	invalid := protocol.PlayerState{Dimension: core.Overworld, DayPhaseOffset: core.DayLengthTicks}
	if err := invalid.Validate(); err == nil {
		t.Fatal("越界偏移通过了 Validate")
	}
	if _, _, err := encodeServerControlPayload(protocol.StatePlay, invalid); err == nil {
		t.Fatal("越界偏移被编码接受")
	}

	// 构造一份合法 wire 载荷，再把偏移高字节抬到 0xFF 使组合值越界
	// （0x0101 → 0xFF01），验证解码器单独拒绝它。
	valid := protocol.PlayerState{Dimension: core.Overworld, DayPhaseOffset: 0x0101, WorldTimeTicks: 24000}
	id, payload, err := encodeServerControlPayload(protocol.StatePlay, valid)
	if err != nil {
		t.Fatal(err)
	}
	offsetOffset := playerStateDayPhaseOffsetOffset(len(payload))
	// 守卫排在真实断言之后：改写的必须确实是偏移的两个字节，否则下面的拒绝
	// 可能是被别的字段越界顶掉的。
	if payload[offsetOffset] != 0x01 || payload[offsetOffset+1] != 0x01 {
		t.Fatalf("夹具无效：offsetOffset 处是 %02x %02x，不是偏移 0x0101",
			payload[offsetOffset], payload[offsetOffset+1])
	}
	corrupted := append([]byte(nil), payload...)
	corrupted[offsetOffset+1] = 0xFF
	if packet, err := decodeServerControlPayload(protocol.StatePlay, id, corrupted); err == nil {
		t.Fatalf("越界偏移 wire 载荷被解码接受: %#v", packet)
	}
}

func TestProtocolV10DropSelectedItemRegistryIsFrozen(t *testing.T) {
	// 新 packet 只占用此前未分配的 ID 11；ID 1 继续保持未分配且不复用。
	if id, ok := protocol.ClientPacketID(protocol.StatePlay, protocol.DropSelectedItem{}); !ok || id != 11 {
		t.Fatalf("protocol.DropSelectedItem packet ID = (%d,%v)，想要 (11,true)", id, ok)
	}
	packet, ok := protocol.ClientPacketForID(protocol.StatePlay, 11)
	if !ok {
		t.Fatal("Play client packet ID 11 未注册")
	}
	if _, isDrop := packet.(protocol.DropSelectedItem); !isDrop {
		t.Fatalf("ID 11 解析为 %T，想要 protocol.DropSelectedItem", packet)
	}
	if _, ok := protocol.ClientPacketForID(protocol.StatePlay, 1); ok {
		t.Fatal("Play client packet ID 1 必须保持未分配")
	}

	// 该消息在 Handshake 与 Login 阶段无效，只属于 Play。
	for _, state := range []protocol.State{protocol.StateHandshake, protocol.StateLogin} {
		if _, _, err := encodeClientPacketPayload(state, protocol.DropSelectedItem{}); err == nil {
			t.Fatalf("状态 %d 接受了 protocol.DropSelectedItem", state)
		}
	}
}

// protocol.PlayerState wire 载荷尾部各字段的字节宽度（v31 起）：
//
//	… | Health u8 | Oxygen u16 | Hunger u8 | SaturationZero u8 | DayPhaseOffset u16 | WorldTimeTicks u64
//
// 下面这些 helper 由末尾向前**链式**求偏移，而不是各写一串 `len(payload)-8-2-1`
// 这样的裸算式。理由是血的教训：v21 追加 `Oxygen` 时，`playerStateHealthOffset`
// 的前身（一句写死的 len(payload)-8-2-1）静默改指到了氧气高字节，用例照样绿（把高字节写成 21 让氧气变成 5376，越界拒绝碰巧
// 仍然发生）；v24 追加 `Hunger` 时同一个坑会再来一次。链式表达让「尾部又多了
// 一个字段」只需要改最外层一处，其余偏移自动跟上。
const (
	playerStateWorldTimeBytes      = 8
	playerStateDayPhaseOffsetBytes = 2
	playerStateSaturationZeroBytes = 1
	playerStateHungerBytes         = 1
	playerStateOxygenBytes         = 2
	playerStateHealthBytes         = 1
)

// playerStateDayPhaseOffsetOffset 返回显示相位偏移低字节在 `protocol.PlayerState` 载荷中的下标。
func playerStateDayPhaseOffsetOffset(payloadLen int) int {
	return payloadLen - playerStateWorldTimeBytes - playerStateDayPhaseOffsetBytes
}

// playerStateHungerOffset 返回饥饿值字节在 `protocol.PlayerState` 载荷中的下标。
func playerStateHungerOffset(payloadLen int) int {
	return playerStateDayPhaseOffsetOffset(payloadLen) - playerStateSaturationZeroBytes - playerStateHungerBytes
}

// playerStateOxygenOffset 返回氧气低字节在 `protocol.PlayerState` 载荷中的下标。
func playerStateOxygenOffset(payloadLen int) int {
	return playerStateHungerOffset(payloadLen) - playerStateOxygenBytes
}

// playerStateHealthOffset 返回生命值字节在 `protocol.PlayerState` 载荷中的下标。
func playerStateHealthOffset(payloadLen int) int {
	return playerStateOxygenOffset(payloadLen) - playerStateHealthBytes
}
