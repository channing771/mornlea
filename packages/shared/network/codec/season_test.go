package codec

import (
	"encoding/hex"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// TestProtocolV37PlayerStateCarriesSeasonAndTemperature 覆盖 v37 追加的季节
// 三字节：`Season`（u8）、`SeasonProgress`（u8）按此序紧跟在 `WeatherKind`
// 之后，`Temperature`（i8）是载荷最末一字节，全合法季节往返保值。
//
// 样本取 2/128/−8 三个中间值：0 与「编码器漏写该字段、解码器读出零值」不可
// 分辨，进度取 128（0x80）与温度取 −8（0xf8）还能分辨「u8 当 i8 解」之类的
// 符号位错误。
func TestProtocolV37PlayerStateCarriesSeasonAndTemperature(t *testing.T) {
	for _, season := range []core.Season{core.SeasonSpring, core.SeasonSummer, core.SeasonAutumn, core.SeasonWinter} {
		state := protocol.PlayerState{
			Dimension:      core.Overworld,
			WorldTimeTicks: 0x0102030405060708,
			WeatherKind:    core.WeatherRain,
			Season:         season,
			SeasonProgress: 128,
			Temperature:    -8,
		}
		if err := state.Validate(); err != nil {
			t.Fatalf("季节 %d 被 Validate 拒绝：%v", season, err)
		}
		id, payload, err := encodeServerControlPayload(protocol.StatePlay, state)
		if err != nil {
			t.Fatal(err)
		}
		if id != 3 {
			t.Fatalf("protocol.PlayerState packet ID = %d，想要 3", id)
		}

		// 三新字节恰好接在天气之后：载荷末尾依次是 8 字节小端绝对世界时间、
		// 1 字节天气，再接季节、年内进度各 1 字节，最后 1 字节温度。
		got := hex.EncodeToString(payload)
		wantSuffix := "0807060504030201" + "01" +
			hex.EncodeToString([]byte{byte(season), 128, 0xF8})
		if len(got) < len(wantSuffix) || got[len(got)-len(wantSuffix):] != wantSuffix {
			t.Fatalf("季节 %d 的 payload = %s，想要以 %s 结尾", season, got, wantSuffix)
		}

		round, err := decodeServerControlPayload(protocol.StatePlay, id, payload)
		if err != nil {
			t.Fatal(err)
		}
		rounded := round.(protocol.PlayerState)
		if rounded.Season != season || rounded.SeasonProgress != 128 || rounded.Temperature != -8 {
			t.Fatalf("往返季节三字段 = (%d,%d,%d)，想要 (%d,128,-8)",
				rounded.Season, rounded.SeasonProgress, rounded.Temperature, season)
		}

		// payload 是固定长度：任何截断都必须被拒绝。
		for length := 0; length < len(payload); length++ {
			if _, err := decodeServerControlPayload(protocol.StatePlay, id, payload[:length]); err == nil {
				t.Fatalf("季节 %d 截断到 %d 字节被接受", season, length)
			}
		}
		if _, err := decodeServerControlPayload(protocol.StatePlay, id, append(payload, 0)); err == nil {
			t.Fatalf("季节 %d 多出尾随字节被接受", season)
		}
	}
}

// TestProtocolV37PlayerStateRejectsOutOfRangeSeason 与天气/相位偏移拒绝用例
// 同形：越界季节（wire 上 4..255 非法）必须在 Validate、编码与解码三处各自
// 被拒，且解码不得输出部分 protocol.PlayerState。取值 4 与 255 分别覆盖刚好
// 越界与全 1 两种形态：只测 4 的话，把高位置零的掩码实现照样绿。进度与温度
// 是全域合法的 u8/i8，不设越界拒绝。
func TestProtocolV37PlayerStateRejectsOutOfRangeSeason(t *testing.T) {
	for _, season := range []core.Season{core.SeasonWinter + 1, 255} {
		invalid := protocol.PlayerState{Dimension: core.Overworld, Season: season}
		if err := invalid.Validate(); err == nil {
			t.Fatalf("越界季节 %d 通过了 Validate", season)
		}
		if _, _, err := encodeServerControlPayload(protocol.StatePlay, invalid); err == nil {
			t.Fatalf("越界季节 %d 被编码接受", season)
		}
	}

	// 构造一份合法 wire 载荷，再把季节字节改写为越界值，验证解码器单独拒绝它。
	valid := protocol.PlayerState{
		Dimension:      core.Overworld,
		WorldTimeTicks: 24000,
		WeatherKind:    core.WeatherRain,
		Season:         core.SeasonAutumn,
		SeasonProgress: 128,
		Temperature:    -8,
	}
	id, payload, err := encodeServerControlPayload(protocol.StatePlay, valid)
	if err != nil {
		t.Fatal(err)
	}
	offset := playerStateSeasonOffset(len(payload))
	// 守卫排在真实断言之后：改写的必须确实是季节那一字节，否则下面的拒绝
	// 可能是被相邻字段（天气或年内进度）越界顶掉的。
	if payload[offset] != byte(core.SeasonAutumn) {
		t.Fatalf("夹具无效：seasonOffset 处是 %d，不是季节 %d",
			payload[offset], core.SeasonAutumn)
	}
	for _, season := range []byte{4, 255} {
		corrupted := append([]byte(nil), payload...)
		corrupted[offset] = season
		if packet, err := decodeServerControlPayload(protocol.StatePlay, id, corrupted); err == nil {
			t.Fatalf("越界季节 %d 的 wire 载荷被解码接受: %#v", season, packet)
		}
	}
}

// TestProtocolV37PlayerStateAcceptsFullRangeProgressAndTemperature 锁死进度与
// 温度的全域合法性：任何 u8 进度（含 0 与 255）与任何 i8 温度（含两端的
// −128 与 127）都必须通过三处校验并往返保值——这两个字段没有子域拒绝，
// 若误加边界会在此红。
func TestProtocolV37PlayerStateAcceptsFullRangeProgressAndTemperature(t *testing.T) {
	for _, sample := range []struct {
		progress    uint8
		temperature int8
	}{
		{0, -128}, {1, -40}, {128, -8}, {254, 45}, {255, 127},
	} {
		state := protocol.PlayerState{
			Dimension:      core.Overworld,
			WorldTimeTicks: 24000,
			Season:         core.SeasonWinter,
			SeasonProgress: sample.progress,
			Temperature:    sample.temperature,
		}
		if err := state.Validate(); err != nil {
			t.Fatalf("进度 %d/温度 %d 被 Validate 拒绝：%v", sample.progress, sample.temperature, err)
		}
		id, payload, err := encodeServerControlPayload(protocol.StatePlay, state)
		if err != nil {
			t.Fatalf("进度 %d/温度 %d 被编码拒绝：%v", sample.progress, sample.temperature, err)
		}
		round, err := decodeServerControlPayload(protocol.StatePlay, id, payload)
		if err != nil {
			t.Fatal(err)
		}
		rounded := round.(protocol.PlayerState)
		if rounded.SeasonProgress != sample.progress || rounded.Temperature != sample.temperature {
			t.Fatalf("往返进度/温度 = (%d,%d)，想要 (%d,%d)",
				rounded.SeasonProgress, rounded.Temperature, sample.progress, sample.temperature)
		}
	}
}
