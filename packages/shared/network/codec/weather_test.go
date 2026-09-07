package codec

import (
	"encoding/hex"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// TestProtocolV36PlayerStateCarriesWeather 覆盖 v36 追加的权威天气：它按 u8
// 紧跟在 `WorldTimeTicks` 之后（载荷最末一字节），全合法值域往返保值。
//
// 样本覆盖晴/雨/雷暴三态：0 与「编码器漏写该字段、解码器读出零值」不可分辨，
// 故必须带上 1 与 2 才能锁住字段确实在搬运。
func TestProtocolV36PlayerStateCarriesWeather(t *testing.T) {
	for _, kind := range []core.WeatherKind{core.WeatherClear, core.WeatherRain, core.WeatherThunder} {
		state := protocol.PlayerState{Dimension: core.Overworld, WorldTimeTicks: 0x0102030405060708, WeatherKind: kind}
		if err := state.Validate(); err != nil {
			t.Fatalf("天气 %d 被 Validate 拒绝：%v", kind, err)
		}
		id, payload, err := encodeServerControlPayload(protocol.StatePlay, state)
		if err != nil {
			t.Fatal(err)
		}
		if id != 3 {
			t.Fatalf("protocol.PlayerState packet ID = %d，想要 3", id)
		}

		// 天气恰好是载荷最末一字节：末尾依次是 8 字节小端绝对世界时间，
		// 再接 1 字节天气值。
		got := hex.EncodeToString(payload)
		wantSuffix := "0807060504030201" + hex.EncodeToString([]byte{byte(kind)})
		if len(got) < len(wantSuffix) || got[len(got)-len(wantSuffix):] != wantSuffix {
			t.Fatalf("天气 %d 的 payload = %s，想要以 %s 结尾", kind, got, wantSuffix)
		}

		round, err := decodeServerControlPayload(protocol.StatePlay, id, payload)
		if err != nil {
			t.Fatal(err)
		}
		if round.(protocol.PlayerState).WeatherKind != kind {
			t.Fatalf("往返天气 = %d，想要 %d", round.(protocol.PlayerState).WeatherKind, kind)
		}

		// payload 是固定长度：任何截断都必须被拒绝。
		for length := 0; length < len(payload); length++ {
			if _, err := decodeServerControlPayload(protocol.StatePlay, id, payload[:length]); err == nil {
				t.Fatalf("天气 %d 截断到 %d 字节被接受", kind, length)
			}
		}
		if _, err := decodeServerControlPayload(protocol.StatePlay, id, append(payload, 0)); err == nil {
			t.Fatalf("天气 %d 多出尾随字节被接受", kind)
		}
	}
}

// TestProtocolV36PlayerStateRejectsOutOfRangeWeather 与生命值/氧气/相位偏移
// 拒绝用例同形：越界天气（wire 上 3..255 非法）必须在 Validate、编码与解码
// 三处各自被拒，且解码不得输出部分 protocol.PlayerState。取值 3 与 255 分别
// 覆盖刚好越界与全 1 两种形态：只测 3 的话，把高位置零的掩码实现照样绿。
func TestProtocolV36PlayerStateRejectsOutOfRangeWeather(t *testing.T) {
	for _, kind := range []core.WeatherKind{3, 255} {
		invalid := protocol.PlayerState{Dimension: core.Overworld, WeatherKind: kind}
		if err := invalid.Validate(); err == nil {
			t.Fatalf("越界天气 %d 通过了 Validate", kind)
		}
		if _, _, err := encodeServerControlPayload(protocol.StatePlay, invalid); err == nil {
			t.Fatalf("越界天气 %d 被编码接受", kind)
		}
	}

	// 构造一份合法 wire 载荷，再把最末一字节改写为越界值，验证解码器单独拒绝它。
	valid := protocol.PlayerState{Dimension: core.Overworld, WorldTimeTicks: 24000, WeatherKind: core.WeatherRain}
	id, payload, err := encodeServerControlPayload(protocol.StatePlay, valid)
	if err != nil {
		t.Fatal(err)
	}
	offset := playerStateWeatherOffset(len(payload))
	// 守卫排在真实断言之后：改写的必须确实是天气那一字节，否则下面的拒绝
	// 可能是被相邻字段（世界时间高字节）越界顶掉的。
	if payload[offset] != byte(core.WeatherRain) {
		t.Fatalf("夹具无效：weatherOffset 处是 %d，不是天气 %d",
			payload[offset], core.WeatherRain)
	}
	for _, kind := range []byte{3, 255} {
		corrupted := append([]byte(nil), payload...)
		corrupted[offset] = kind
		if packet, err := decodeServerControlPayload(protocol.StatePlay, id, corrupted); err == nil {
			t.Fatalf("越界天气 %d 的 wire 载荷被解码接受: %#v", kind, packet)
		}
	}
}
